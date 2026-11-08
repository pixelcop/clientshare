package services

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/links"
	"gorm.io/gorm"
)

type LinkInput struct {
	AccessType string     `json:"access_type"`
	Email      string     `json:"email"`
	ExpiryDays int        `json:"expiry_days"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// CreateLink is the authorized creation path for both client and item targets.
// Item route registration remains separately gated until enforcement is ready.
func (s *SharingService) CreateLink(ctx context.Context, req AccessRequest, input LinkInput, requestKey string) (*models.SecureLink, error) {
	caps, valid := linkCapabilities(input.AccessType)
	if !valid || s.LinkSigningKey == "" || len(requestKey) > 128 {
		return nil, ErrSharingInput
	}
	input.Email = strings.TrimSpace(input.Email)
	if input.Email != "" {
		address, err := mail.ParseAddress(input.Email)
		if err != nil || strings.ContainsAny(input.Email, "\r\n") {
			return nil, ErrSharingInput
		}
		input.Email = strings.ToLower(address.Address)
	}
	if input.ExpiryDays < 1 || input.ExpiryDays > MaxSecureLinkDefaultExpiryDays {
		return nil, ErrSharingInput
	}
	expiry := s.now().Add(time.Duration(input.ExpiryDays) * 24 * time.Hour)
	if input.ExpiresAt != nil {
		utc := input.ExpiresAt.UTC()
		input.ExpiresAt, expiry = &utc, utc
	}
	if !expiry.After(s.now()) || expiry.After(s.now().Add(time.Duration(MaxSecureLinkDefaultExpiryDays)*24*time.Hour)) {
		return nil, ErrSharingInput
	}
	fingerprint := sharingDigest(input)
	key := sharingDigest([]string{req.TenantID, req.Actor.UserID, req.ClientID, req.FileID, "link", requestKey})
	var link models.SecureLink
	err := s.withAuthority(ctx, req, caps, func(tx *gorm.DB, _ *models.File) error {
		if requestKey != "" {
			err := tx.Where("tenant_id = ? AND creation_key = ?", req.TenantID, key).First(&link).Error
			if err == nil {
				if link.CreationHash != fingerprint {
					return ErrLinkRequestConflict
				}
				return nil // never revive expired or revoked links
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		token, err := links.GenerateSecureToken(s.LinkSigningKey)
		if err != nil {
			return err
		}
		link = models.SecureLink{TenantID: req.TenantID, ClientID: req.ClientID, Token: token, Email: input.Email, AccessType: input.AccessType, ExpiresAt: expiry, CreatedBy: req.Actor.UserID, CreatedAt: s.now(), CreationHash: fingerprint}
		if req.FileID != "" {
			target := req.FileID
			link.FileID = &target
		}
		if requestKey != "" {
			link.CreationKey = &key
		}
		if err := tx.Create(&link).Error; err != nil {
			return err
		}
		return sharingAudit(tx, req, "link.create", link.Email, nil, linkAudit(link))
	})
	return &link, err
}

func (s *SharingService) LinkForDelivery(ctx context.Context, req AccessRequest, linkID string) (*models.SecureLink, error) {
	var link models.SecureLink
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", req.TenantID, linkID).First(&link).Error; err != nil {
		return nil, err
	}
	req.ClientID, req.FileID = link.ClientID, ""
	if link.FileID != nil {
		req.FileID = *link.FileID
	}
	caps, valid := linkCapabilities(link.AccessType)
	if !valid {
		return nil, gorm.ErrRecordNotFound
	}
	err := s.withAuthority(ctx, req, caps, func(tx *gorm.DB, _ *models.File) error {
		if err := tx.Where("tenant_id = ? AND id = ? AND revoked_at IS NULL", req.TenantID, linkID).First(&link).Error; err != nil {
			return err
		}
		if !link.ExpiresAt.After(s.now()) {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	return &link, err
}

func (s *SharingService) RevokeLink(ctx context.Context, req AccessRequest, linkID string) error {
	var link models.SecureLink
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", req.TenantID, linkID).First(&link).Error; err != nil {
		return err
	}
	req.ClientID, req.FileID = link.ClientID, ""
	if link.FileID != nil {
		req.FileID = *link.FileID
	}
	return s.withAuthority(ctx, req, models.AccessCapabilities{}, func(tx *gorm.DB, _ *models.File) error {
		if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, linkID).First(&link).Error; err != nil {
			return err
		}
		if link.RevokedAt != nil {
			return nil
		}
		before := linkAudit(link)
		now := s.now()
		if err := tx.Model(&link).Update("revoked_at", now).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.QueuedEmail{}).Where("tenant_id = ? AND sent_at IS NULL AND canceled_at IS NULL AND (secure_link_id = ? OR recipient_link_id = ?)", req.TenantID, link.ID, link.ID).Update("canceled_at", now).Error; err != nil {
			return err
		}
		link.RevokedAt = &now
		return sharingAudit(tx, req, "link.revoke", link.Email, before, linkAudit(link))
	})
}

func (s *SharingService) LinksAtScope(ctx context.Context, req AccessRequest) ([]models.SecureLink, error) {
	result := []models.SecureLink{}
	err := s.withAuthority(ctx, req, models.AccessCapabilities{}, func(tx *gorm.DB, _ *models.File) error {
		req.Operation, req.Now = AccessShare, s.now()
		d, err := (&AccessService{DB: tx}).Resolve(ctx, req)
		if err != nil {
			return err
		}
		if !d.Allowed {
			return gorm.ErrRecordNotFound
		}
		if err := invitationTargetQuery(tx, req).Order("created_at DESC, id DESC").Find(&result).Error; err != nil {
			return err
		}
		for i := range result {
			link := &result[i]
			caps, valid := linkCapabilities(link.AccessType)
			if !valid || !d.CanGrant(caps) || link.RevokedAt != nil || !link.ExpiresAt.After(s.now()) {
				link.Token = ""
			}
		}
		return nil
	})
	return result, err
}
