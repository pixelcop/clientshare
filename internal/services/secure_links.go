package services

import (
	"context"
	"errors"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrLinkRequestConflict = errors.New("request key was already used with different link settings")
var ErrScopedSharingUnavailable = errors.New("item sharing is not available until scoped enforcement is complete")

// PopulateLinkOpens enriches management responses from recorded opens, including
// older activity that predates usage display. A nil timestamp means no recorded
// open, not proof that the link has never been used. No bearer response uses this.
func PopulateLinkOpens(ctx context.Context, db *gorm.DB, tenantID string, links []models.SecureLink) error {
	if len(links) == 0 {
		return nil
	}
	byID := make(map[string]*models.SecureLink, len(links))
	ids := make([]string, 0, len(links))
	for i := range links {
		byID[links[i].ID] = &links[i]
		ids = append(ids, links[i].ID)
	}
	latest := db.WithContext(ctx).Model(&models.FileActivity{}).
		Select("secure_link_id, MAX(created_at)").
		Where("tenant_id = ? AND secure_link_id IN ? AND action = ?", tenantID, ids, "view_link").
		Group("secure_link_id")
	// Select the original timestamp column so database drivers retain its time type.
	var activity []models.FileActivity
	if err := db.WithContext(ctx).Select("secure_link_id, created_at").
		Where("tenant_id = ? AND action = ? AND (secure_link_id, created_at) IN (?)", tenantID, "view_link", latest).
		Find(&activity).Error; err != nil {
		return err
	}
	for _, event := range activity {
		if event.SecureLinkID == nil {
			continue
		}
		link := byID[*event.SecureLinkID]
		if link != nil && (link.LastAccessed == nil || event.CreatedAt.After(*link.LastAccessed)) {
			opened := event.CreatedAt
			link.LastAccessed = &opened
		}
	}
	return nil
}

// PersistClientLinkForActor preserves legacy request keys/fingerprints while
// applying the same transactional permission ceiling as scoped creation.
func (s *SharingService) PersistClientLinkForActor(ctx context.Context, req AccessRequest, link *models.SecureLink) error {
	req.Operation = AccessCreateLink
	if req.FileID != "" || link.FileID != nil {
		return ErrScopedSharingUnavailable
	}
	caps, valid := linkCapabilities(link.AccessType)
	if !valid {
		return ErrSharingInput
	}
	return s.withAuthority(ctx, req, caps, func(tx *gorm.DB, _ *models.File) error {
		candidateID := ids.NewULID()
		link.ID, link.TenantID, link.ClientID, link.CreatedBy = candidateID, req.TenantID, req.ClientID, req.Actor.UserID
		if err := PersistSecureLink(ctx, tx, link); err != nil {
			return err
		}
		if link.ID != candidateID {
			return nil
		}
		return sharingAudit(tx, req, "link.create", link.Email, nil, linkAudit(*link))
	})
}

// PersistSecureLink deduplicates a retried creation without ever reviving an
// expired/revoked link. Tokens and existing expiry timestamps are preserved.
func PersistSecureLink(ctx context.Context, db *gorm.DB, link *models.SecureLink) error {
	if link.FileID != nil {
		return ErrScopedSharingUnavailable
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var client models.Client
		if err := tx.Where("tenant_id = ? AND id = ?", link.TenantID, link.ClientID).First(&client).Error; err != nil {
			return err
		}
		if link.CreationKey == nil {
			return tx.Create(link).Error
		}
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "creation_key"}}, DoNothing: true}).Create(link)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			return nil
		}
		var existing models.SecureLink
		if err := tx.Where("tenant_id = ? AND creation_key = ?", link.TenantID, *link.CreationKey).First(&existing).Error; err != nil {
			return err
		}
		if existing.CreationHash != link.CreationHash || existing.ClientID != link.ClientID || existing.CreatedBy != link.CreatedBy {
			return ErrLinkRequestConflict
		}
		*link = existing
		return nil
	})
}

// RevokeSecureLink retains actual history rather than inventing revoked rows in
// the UI. Already-open sessions check revoked_at on their next request.
func RevokeSecureLink(ctx context.Context, db *gorm.DB, tenantID, linkID string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var link models.SecureLink
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, linkID).First(&link).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&models.SecureLink{}).Where("tenant_id = ? AND id = ? AND revoked_at IS NULL", tenantID, linkID).Update("revoked_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&models.QueuedEmail{}).Where("tenant_id = ? AND secure_link_id = ? AND sent_at IS NULL AND canceled_at IS NULL", tenantID, linkID).Update("canceled_at", now).Error
	})
}
