package services

import (
	"context"
	"errors"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrLinkRequestConflict = errors.New("request key was already used with different link settings")
var ErrScopedSharingUnavailable = errors.New("item sharing is not available until scoped enforcement is complete")

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
