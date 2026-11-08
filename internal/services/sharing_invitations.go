package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InvitationInput struct {
	Email          string                    `json:"email"`
	Capabilities   models.AccessCapabilities `json:"capabilities"`
	GrantExpiresAt *time.Time                `json:"expires_at"`
}

func sharingDigest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (s *SharingService) invitationToken(id string) string {
	mac := hmac.New(sha256.New, []byte(s.TokenSecret))
	_, _ = mac.Write([]byte("access-invitation:" + id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *SharingService) invitationHash(token string) string {
	mac := hmac.New(sha256.New, []byte(s.TokenSecret))
	_, _ = mac.Write([]byte("access-redemption:" + token))
	return hex.EncodeToString(mac.Sum(nil))
}

func invitationTargetQuery(db *gorm.DB, req AccessRequest) *gorm.DB {
	db = db.Where("tenant_id = ? AND client_id = ?", req.TenantID, req.ClientID)
	if req.FileID == "" {
		return db.Where("file_id IS NULL")
	}
	return db.Where("file_id = ?", req.FileID)
}

func (s *SharingService) CreateInvitation(ctx context.Context, req AccessRequest, input InvitationInput, requestKey string) (*models.AccessInvitation, error) {
	if s.TokenSecret == "" || requestKey == "" || len(requestKey) > 128 || !validGrantInput(GrantInput{Capabilities: input.Capabilities, ExpiresAt: input.GrantExpiresAt}, s.now()) || (req.FileID == "" && input.GrantExpiresAt != nil) {
		return nil, ErrSharingInput
	}
	address, err := mail.ParseAddress(strings.TrimSpace(input.Email))
	if err != nil || strings.ContainsAny(input.Email, "\r\n") {
		return nil, ErrSharingInput
	}
	input.Email = strings.ToLower(strings.TrimSpace(address.Address))
	if input.GrantExpiresAt != nil {
		utc := input.GrantExpiresAt.UTC()
		input.GrantExpiresAt = &utc
	}
	key := sharingDigest([]string{req.TenantID, req.Actor.UserID, req.ClientID, req.FileID, requestKey})
	fingerprint := sharingDigest(input)
	var invitation models.AccessInvitation
	err = s.withAuthority(ctx, req, input.Capabilities, func(tx *gorm.DB, _ *models.File) error {
		err := tx.Where("tenant_id = ? AND creation_key = ?", req.TenantID, key).First(&invitation).Error
		if err == nil {
			if invitation.CreationHash != fingerprint {
				return ErrSharingConflict
			}
			return nil // a replay never revives accepted/revoked/expired intent
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var recipient []models.User
		if err := tx.Where("tenant_id = ? AND LOWER(email) = ?", req.TenantID, input.Email).Limit(2).Find(&recipient).Error; err != nil {
			return err
		}
		if len(recipient) > 1 {
			return ErrSharingConflict
		}
		var recipientID *string
		if len(recipient) == 1 {
			role := recipient[0].Role
			if role != "admin" && role != "manager" && role != "client" && role != "customer" {
				return ErrSharingInput
			}
			if req.FileID == "" && recipient[0].Role != "client" && recipient[0].Role != "customer" {
				return ErrSharingInput
			}
			recipientID = &recipient[0].ID
		}
		var pending models.AccessInvitation
		err = invitationTargetQuery(tx, req).Where("email = ? AND accepted_at IS NULL AND revoked_at IS NULL", input.Email).First(&pending).Error
		if err == nil {
			if pending.ExpiresAt.After(s.now()) {
				if pending.CreationHash != fingerprint {
					return ErrSharingConflict
				}
				invitation = pending
				return nil
			}
			before := pending
			if err := tx.Model(&pending).Update("revoked_at", s.now()).Error; err != nil {
				return err
			}
			if err := cancelInvitationMail(tx, req.TenantID, pending.ID, s.now()); err != nil {
				return err
			}
			if err := sharingAudit(tx, req, "invitation.expire", pending.Email, before, pending); err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		expiry := s.now().Add(7 * 24 * time.Hour)
		if input.GrantExpiresAt != nil && input.GrantExpiresAt.Before(expiry) {
			expiry = *input.GrantExpiresAt
		}
		invitation = models.AccessInvitation{ID: ids.NewULID(), TenantID: req.TenantID, ClientID: req.ClientID, Email: input.Email, RecipientUserID: recipientID, CreatedBy: req.Actor.UserID, AccessCapabilities: input.Capabilities, GrantExpiresAt: input.GrantExpiresAt, ExpiresAt: expiry, CreationKey: &key, CreationHash: fingerprint, CreatedAt: s.now(), UpdatedAt: s.now()}
		if req.FileID != "" {
			target := req.FileID
			invitation.FileID = &target
		}
		invitation.TokenHash = s.invitationHash(s.invitationToken(invitation.ID))
		if err := tx.Create(&invitation).Error; err != nil {
			return err
		}
		return sharingAudit(tx, req, "invitation.create", invitation.Email, nil, invitation)
	})
	return &invitation, err
}

// InvitationForDelivery is internal mail composition, not an API result. Never
// return its token to an inviter: recipient email possession plus authenticated
// identity is the acceptance boundary, distinct from a bearer sharing link.
func (s *SharingService) InvitationForDelivery(ctx context.Context, req AccessRequest, invitationID string) (*models.AccessInvitation, string, error) {
	var invitation models.AccessInvitation
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", req.TenantID, invitationID).First(&invitation).Error; err != nil {
		return nil, "", err
	}
	req.ClientID, req.FileID = invitation.ClientID, ""
	if invitation.FileID != nil {
		req.FileID = *invitation.FileID
	}
	err := s.withAuthority(ctx, req, invitation.AccessCapabilities, func(tx *gorm.DB, _ *models.File) error {
		if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, invitationID).First(&invitation).Error; err != nil {
			return err
		}
		if invitation.FileID == nil && invitation.CreatedBy != req.Actor.UserID {
			staffReq := req
			staffReq.Operation = AccessManageClient
			staff, err := (&AccessService{DB: tx}).Resolve(ctx, staffReq)
			if err != nil {
				return err
			}
			if !staff.Allowed {
				return gorm.ErrRecordNotFound
			}
		}
		if invitation.RevokedAt != nil || invitation.AcceptedAt != nil || !invitation.ExpiresAt.After(s.now()) {
			return gorm.ErrRecordNotFound
		}
		if invitation.TokenHash != s.invitationHash(s.invitationToken(invitation.ID)) {
			return ErrSharingConflict
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return &invitation, s.invitationToken(invitation.ID), nil
}

func (s *SharingService) AcceptInvitation(ctx context.Context, tenantID, userID, token string) (*models.AccessInvitation, error) {
	if s.TokenSecret == "" || len(token) != 43 || userID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var invitation models.AccessInvitation
	hash := s.invitationHash(token)
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND token_hash = ?", tenantID, hash).First(&invitation).Error; err != nil {
		return nil, err
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ? AND role IN ?", tenantID, userID, []string{"admin", "manager", "client", "customer"}).First(&user).Error; err != nil {
			return err
		}
		var client models.Client
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, invitation.ClientID).First(&client).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ? AND token_hash = ?", tenantID, invitation.ID, hash).First(&invitation).Error; err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(user.Email), invitation.Email) || (invitation.RecipientUserID != nil && *invitation.RecipientUserID != user.ID) {
			return gorm.ErrRecordNotFound
		}
		if invitation.RevokedAt != nil || !invitation.ExpiresAt.After(s.now()) || (invitation.GrantExpiresAt != nil && !invitation.GrantExpiresAt.After(s.now())) {
			return gorm.ErrRecordNotFound
		}
		if invitation.AcceptedAt != nil {
			if invitation.AcceptedBy == nil || *invitation.AcceptedBy != user.ID {
				return gorm.ErrRecordNotFound
			}
			return nil // never restore access on a duplicate acceptance
		}
		req := AccessRequest{TenantID: tenantID, ClientID: invitation.ClientID, Actor: AccessActor{UserID: user.ID}}
		if invitation.FileID == nil {
			if user.Role != "client" && user.Role != "customer" {
				return ErrSharingInput
			}
			var membership models.UserClient
			err := tx.Where("tenant_id = ? AND client_id = ? AND user_id = ?", tenantID, invitation.ClientID, user.ID).First(&membership).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				membership = models.UserClient{TenantID: tenantID, ClientID: invitation.ClientID, UserID: user.ID, CapabilitiesSet: true, AccessCapabilities: invitation.AccessCapabilities, AccessSource: "invitation", InvitedBy: &invitation.CreatedBy, CreatedAt: s.now()}
				if err := tx.Create(&membership).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			// An independently authorized member is never taken over, reduced
			// or upgraded by a later invitation from somebody else.
		} else {
			req.FileID = *invitation.FileID
			file, err := (&AccessService{DB: tx}).ValidateTarget(ctx, tenantID, invitation.ClientID, req.FileID)
			if err != nil {
				return err
			}
			if file.Type == "file" && invitation.CanUpload {
				return ErrSharingInput
			}
			var grant models.ItemGrant
			err = tx.Where("tenant_id = ? AND client_id = ? AND file_id = ? AND user_id = ?", tenantID, invitation.ClientID, req.FileID, user.ID).First(&grant).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				grant = models.ItemGrant{TenantID: tenantID, ClientID: invitation.ClientID, FileID: req.FileID, UserID: user.ID, CreatedBy: invitation.CreatedBy, AccessCapabilities: invitation.AccessCapabilities, ExpiresAt: invitation.GrantExpiresAt, Version: 1, CreatedAt: s.now(), UpdatedAt: s.now()}
				if err := tx.Create(&grant).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				if grant.RevokedAt != nil && !invitation.CreatedAt.After(*grant.RevokedAt) {
					return gorm.ErrRecordNotFound
				}
				caps, expiry := invitation.AccessCapabilities, invitation.GrantExpiresAt
				if grant.RevokedAt == nil && (grant.ExpiresAt == nil || grant.ExpiresAt.After(s.now())) {
					caps.CanRead = caps.CanRead || grant.CanRead
					caps.CanUpload = caps.CanUpload || grant.CanUpload
					caps.CanDelete = caps.CanDelete || grant.CanDelete
					caps.CanShare = caps.CanShare || grant.CanShare
					if grant.ExpiresAt == nil || (expiry != nil && grant.ExpiresAt.After(*expiry)) {
						expiry = grant.ExpiresAt
					}
				}
				if err := tx.Model(&grant).Updates(map[string]any{"can_read": caps.CanRead, "can_upload": caps.CanUpload, "can_delete": caps.CanDelete, "can_share": caps.CanShare, "expires_at": expiry, "revoked_at": nil, "version": gorm.Expr("version + 1"), "updated_at": s.now()}).Error; err != nil {
					return err
				}
			}
		}
		before := invitation
		result := tx.Model(&models.AccessInvitation{}).Where("tenant_id = ? AND id = ? AND accepted_at IS NULL AND revoked_at IS NULL", tenantID, invitation.ID).Updates(map[string]any{"accepted_at": s.now(), "accepted_by": user.ID, "recipient_user_id": user.ID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSharingConflict
		}
		if err := tx.First(&invitation, "tenant_id = ? AND id = ?", tenantID, invitation.ID).Error; err != nil {
			return err
		}
		return sharingAudit(tx, req, "invitation.accept", user.ID, before, invitation)
	})
	return &invitation, err
}

// AcceptIdentityInvitation is called only after the existing identity token has
// been verified and atomically claimed in the same transaction. The witness is
// user/email/tenant/target-bound; an item invitation never assigns the client.
func (s *SharingService) AcceptIdentityInvitation(ctx context.Context, tenantID, userID, identityInviteID string) (*models.AccessInvitation, error) {
	var proof models.InviteToken
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND user_id = ? AND id = ? AND used_at IS NOT NULL", tenantID, userID, identityInviteID).First(&proof).Error; err != nil {
		return nil, err
	}
	if proof.AccessInvitationID == nil || proof.Email == "" || !proof.ExpiresAt.After(s.now()) {
		return nil, gorm.ErrRecordNotFound
	}
	var intent models.AccessInvitation
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ? AND LOWER(email) = LOWER(?)", tenantID, *proof.AccessInvitationID, proof.Email).First(&intent).Error; err != nil {
		return nil, err
	}
	if intent.RecipientUserID != nil && *intent.RecipientUserID != userID {
		return nil, gorm.ErrRecordNotFound
	}
	return s.AcceptInvitation(ctx, tenantID, userID, s.invitationToken(intent.ID))
}

func (s *SharingService) RevokeInvitation(ctx context.Context, req AccessRequest, invitationID string) error {
	var invitation models.AccessInvitation
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", req.TenantID, invitationID).First(&invitation).Error; err != nil {
		return err
	}
	req.ClientID, req.FileID = invitation.ClientID, ""
	if invitation.FileID != nil {
		req.FileID = *invitation.FileID
	}
	return s.withAuthority(ctx, req, models.AccessCapabilities{}, func(tx *gorm.DB, _ *models.File) error {
		if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, invitationID).First(&invitation).Error; err != nil {
			return err
		}
		if invitation.FileID == nil && invitation.CreatedBy != req.Actor.UserID {
			staffReq := req
			staffReq.Operation = AccessManageClient
			staff, err := (&AccessService{DB: tx}).Resolve(ctx, staffReq)
			if err != nil {
				return err
			}
			if !staff.Allowed {
				return gorm.ErrRecordNotFound
			}
		}
		if invitation.RevokedAt != nil {
			return nil
		}
		if invitation.AcceptedAt != nil {
			return ErrSharingConflict
		}
		before := invitation
		if err := tx.Model(&invitation).Update("revoked_at", s.now()).Error; err != nil {
			return err
		}
		if err := cancelInvitationMail(tx, req.TenantID, invitation.ID, s.now()); err != nil {
			return err
		}
		return sharingAudit(tx, req, "invitation.revoke", invitation.Email, before, invitation)
	})
}
