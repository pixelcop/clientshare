package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrSharingConflict     = errors.New("access changed; refresh before updating")
	ErrSharingCapabilities = errors.New("requested capabilities exceed current sharing authority")
	ErrSharingInput        = errors.New("invalid sharing settings")
)

type SharingService struct {
	DB          *gorm.DB
	TokenSecret string
	Now         func() time.Time
}

type GrantInput struct {
	UserID       string                    `json:"user_id"`
	Capabilities models.AccessCapabilities `json:"capabilities"`
	ExpiresAt    *time.Time                `json:"expires_at"`
}

func (s *SharingService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *SharingService) withAuthority(ctx context.Context, req AccessRequest, caps models.AccessCapabilities, fn func(*gorm.DB, *models.File) error) error {
	if req.Actor.UserID == "" || req.Actor.LinkID != "" {
		return gorm.ErrRecordNotFound
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var client models.Client
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", req.TenantID, req.ClientID).First(&client).Error; err != nil {
			return err
		}
		req.Operation, req.Now = AccessShare, s.now()
		if req.FileID == "" {
			req.Operation = AccessInviteClient
		}
		resolver := &AccessService{DB: tx}
		d, err := resolver.Resolve(ctx, req)
		if err != nil {
			return err
		}
		if !d.Allowed {
			return gorm.ErrRecordNotFound
		}
		if !d.CanGrant(caps) {
			return ErrSharingCapabilities
		}
		var file *models.File
		if req.FileID != "" {
			file, err = resolver.ValidateTarget(ctx, req.TenantID, req.ClientID, req.FileID)
			if err != nil {
				return err
			}
			if file.Type == "file" && caps.CanUpload {
				return ErrSharingInput
			}
		}
		return fn(tx, file)
	})
}

func sharingAudit(tx *gorm.DB, req AccessRequest, action, recipient string, before, after any) error {
	target := req.FileID
	if target == "" {
		target = req.ClientID
	}
	payload, err := json.Marshal(map[string]any{"client_id": req.ClientID, "file_id": req.FileID, "recipient": recipient, "before": before, "after": after})
	if err != nil {
		return err
	}
	return tx.Create(&models.AuditEvent{TenantID: &req.TenantID, Actor: req.Actor.UserID, Action: action, TargetID: &target, Payload: string(payload)}).Error
}

func validGrantInput(input GrantInput, now time.Time) bool {
	c := input.Capabilities
	return (c.CanRead || c.CanUpload || c.CanDelete || c.CanShare) && (input.ExpiresAt == nil || input.ExpiresAt.After(now))
}

func sameGrantExpiry(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (s *SharingService) CreateGrant(ctx context.Context, req AccessRequest, input GrantInput) (*models.ItemGrant, error) {
	if req.FileID == "" || input.UserID == "" || !validGrantInput(input, s.now()) {
		return nil, ErrSharingInput
	}
	var grant models.ItemGrant
	err := s.withAuthority(ctx, req, input.Capabilities, func(tx *gorm.DB, _ *models.File) error {
		var recipient models.User
		if err := tx.Where("tenant_id = ? AND id = ? AND role IN ?", req.TenantID, input.UserID, []string{"admin", "manager", "client", "customer"}).First(&recipient).Error; err != nil {
			return err
		}
		err := tx.Where("tenant_id = ? AND client_id = ? AND file_id = ? AND user_id = ?", req.TenantID, req.ClientID, req.FileID, input.UserID).First(&grant).Error
		if err == nil {
			if grant.RevokedAt == nil && (grant.ExpiresAt == nil || grant.ExpiresAt.After(s.now())) && grant.AccessCapabilities == input.Capabilities && sameGrantExpiry(grant.ExpiresAt, input.ExpiresAt) {
				return nil
			}
			return ErrSharingConflict // creation never silently updates/reactivates
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		grant = models.ItemGrant{TenantID: req.TenantID, ClientID: req.ClientID, FileID: req.FileID, UserID: input.UserID, CreatedBy: req.Actor.UserID, AccessCapabilities: input.Capabilities, ExpiresAt: input.ExpiresAt, Version: 1, CreatedAt: s.now(), UpdatedAt: s.now()}
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}, {Name: "file_id"}}, DoNothing: true}).Create(&grant)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSharingConflict
		}
		return sharingAudit(tx, req, "grant.create", grant.UserID, nil, grant)
	})
	return &grant, err
}

func (s *SharingService) UpdateGrant(ctx context.Context, req AccessRequest, grantID string, version uint64, input GrantInput, revoke bool) (*models.ItemGrant, error) {
	if req.FileID == "" || grantID == "" || version == 0 || (!revoke && !validGrantInput(input, s.now())) {
		return nil, ErrSharingInput
	}
	caps := input.Capabilities
	if revoke {
		caps = models.AccessCapabilities{}
	}
	var grant models.ItemGrant
	err := s.withAuthority(ctx, req, caps, func(tx *gorm.DB, _ *models.File) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND client_id = ? AND file_id = ? AND id = ?", req.TenantID, req.ClientID, req.FileID, grantID).First(&grant).Error; err != nil {
			return err
		}
		if grant.Version != version || (input.UserID != "" && input.UserID != grant.UserID) {
			return ErrSharingConflict
		}
		before := grant
		updates := map[string]any{"version": version + 1, "updated_at": s.now()}
		action := "grant.update"
		if revoke {
			updates["revoked_at"], action = s.now(), "grant.revoke"
			var user models.User
			if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, grant.UserID).First(&user).Error; err != nil {
				return err
			}
			if err := RevokePendingRecipientInvitations(tx, req, user.ID, user.Email, s.now()); err != nil {
				return err
			}
		} else {
			updates["can_read"], updates["can_upload"], updates["can_delete"], updates["can_share"] = caps.CanRead, caps.CanUpload, caps.CanDelete, caps.CanShare
			updates["expires_at"], updates["revoked_at"] = input.ExpiresAt, nil
		}
		result := tx.Model(&models.ItemGrant{}).Where("id = ? AND tenant_id = ? AND version = ?", grant.ID, req.TenantID, version).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSharingConflict
		}
		var current models.ItemGrant
		if err := tx.First(&current, "tenant_id = ? AND id = ?", req.TenantID, grant.ID).Error; err != nil {
			return err
		}
		grant = current
		return sharingAudit(tx, req, action, grant.UserID, before, grant)
	})
	return &grant, err
}
