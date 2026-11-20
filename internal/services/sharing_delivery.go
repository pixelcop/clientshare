package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// InvitationDelivery is internal mail composition. Never return credentials or
// account-setup links to the inviter, including when outbound mail is disabled.
type InvitationDelivery struct {
	Invitation       models.AccessInvitation
	User             models.User
	NeedsIdentity    bool
	IdentityInviteID *string
	RawToken         string `json:"-"`
	ExpiresAt        time.Time
}

func (s *SharingService) identityInvitationToken(id string) string {
	mac := hmac.New(sha256.New, []byte(s.TokenSecret))
	_, _ = mac.Write([]byte("access-identity:" + id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *SharingService) identityInvitationHash(token string) string {
	mac := hmac.New(sha256.New, []byte(s.TokenSecret))
	_, _ = mac.Write([]byte(token)) // existing identity-token verifier's format
	return hex.EncodeToString(mac.Sum(nil))
}

// PrepareInvitationDelivery reuses the tenant-local identity without changing
// its role/memberships/credentials. A new identity is client-role with no client
// assignments. Only email possession can redeem its account-setup credential.
func (s *SharingService) PrepareInvitationDelivery(ctx context.Context, req AccessRequest, invitationID string, identityTTL time.Duration) (*InvitationDelivery, error) {
	invitation, _, err := s.InvitationForDelivery(ctx, req, invitationID)
	if err != nil {
		return nil, err
	}
	req.ClientID, req.FileID = invitation.ClientID, ""
	if invitation.FileID != nil {
		req.FileID = *invitation.FileID
	}
	var delivery InvitationDelivery
	err = s.withAuthority(ctx, req, invitation.AccessCapabilities, func(tx *gorm.DB, _ *models.File) error {
		var intent models.AccessInvitation
		if err := tx.First(&intent, "tenant_id = ? AND id = ?", req.TenantID, invitationID).Error; err != nil {
			return err
		}
		if intent.RevokedAt != nil || intent.AcceptedAt != nil || !intent.ExpiresAt.After(s.now()) {
			return gorm.ErrRecordNotFound
		}
		var users []models.User
		if err := tx.Where("tenant_id = ? AND LOWER(email) = ?", req.TenantID, intent.Email).Limit(2).Find(&users).Error; err != nil {
			return err
		}
		if len(users) > 1 {
			return ErrSharingConflict
		}
		var user models.User
		if len(users) == 0 {
			if intent.RecipientUserID != nil {
				return gorm.ErrRecordNotFound
			}
			var err error
			user, err = s.createExternalInvitationRecipient(tx, req, intent.Email)
			if err != nil {
				return err
			}
		} else {
			user = users[0]
		}
		if intent.RecipientUserID != nil && *intent.RecipientUserID != user.ID {
			return gorm.ErrRecordNotFound
		}
		if intent.RoleID == nil && intent.FileID == nil && user.Role != "client" && user.Role != "customer" {
			return ErrSharingInput
		}
		if intent.RecipientUserID == nil {
			before := intent
			if err := tx.Model(&intent).Update("recipient_user_id", user.ID).Error; err != nil {
				return err
			}
			if err := sharingAudit(tx, req, "invitation.bind_identity", user.ID, before, intent); err != nil {
				return err
			}
		}
		delivery.Invitation, delivery.User = intent, user
		var credentials int64
		if err := tx.Model(&models.PasskeyCredential{}).Where("tenant_id = ? AND user_id = ?", req.TenantID, user.ID).Count(&credentials).Error; err != nil {
			return err
		}
		if user.PasswordHash != "" || credentials != 0 {
			delivery.RawToken, delivery.ExpiresAt = s.invitationToken(intent.ID), intent.ExpiresAt
			return nil
		}
		delivery.NeedsIdentity = true
		var proof models.InviteToken
		err := tx.Where("tenant_id = ? AND user_id = ? AND access_invitation_id = ? AND used_at IS NULL", req.TenantID, user.ID, intent.ID).Order("created_at DESC, id DESC").First(&proof).Error
		if err == nil && proof.ExpiresAt.After(s.now()) && proof.TokenHash == s.identityInvitationHash(s.identityInvitationToken(proof.ID)) {
			delivery.IdentityInviteID, delivery.RawToken, delivery.ExpiresAt = &proof.ID, s.identityInvitationToken(proof.ID), proof.ExpiresAt
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if err := tx.Model(&proof).Update("used_at", s.now()).Error; err != nil {
				return err
			}
		}
		if identityTTL <= 0 {
			identityTTL = 72 * time.Hour
		}
		expiry := s.now().Add(identityTTL)
		if intent.ExpiresAt.Before(expiry) {
			expiry = intent.ExpiresAt.UTC()
		}
		proof = models.InviteToken{ID: ids.NewULID(), TenantID: req.TenantID, UserID: user.ID, Email: user.Email, AccessInvitationID: &intent.ID, ExpiresAt: expiry, InvitedBy: req.Actor.UserID, CreatedAt: s.now()}
		proof.TokenHash = s.identityInvitationHash(s.identityInvitationToken(proof.ID))
		if err := tx.Create(&proof).Error; err != nil {
			return err
		}
		delivery.IdentityInviteID, delivery.RawToken, delivery.ExpiresAt = &proof.ID, s.identityInvitationToken(proof.ID), proof.ExpiresAt
		return sharingAudit(tx, req, "identity.invitation.create", user.ID, nil, map[string]any{"identity_invite_id": proof.ID, "access_invitation_id": intent.ID, "expires_at": expiry})
	})
	return &delivery, err
}

func (s *SharingService) createExternalInvitationRecipient(tx *gorm.DB, req AccessRequest, email string) (models.User, error) {
	user := models.User{TenantID: req.TenantID, Email: email, Role: "client", CreatedAt: s.now(), UpdatedAt: s.now()}
	if err := tx.Create(&user).Error; err != nil {
		return user, err
	}
	profile := models.AccessProfile{TenantID: req.TenantID, UserID: user.ID, UserType: "external", OrganizationRole: "none"}
	if err := tx.Create(&profile).Error; err != nil {
		return user, err
	}
	return user, sharingAudit(tx, req, "identity.invite", user.ID, nil, map[string]any{"user_id": user.ID, "email": user.Email, "role": user.Role})
}
