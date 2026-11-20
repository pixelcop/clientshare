package services

import (
	"context"
	"time"

	"github.com/pixelcop/clientshare/internal/dbtime"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

// CollectionLink deliberately excludes bearer tokens and recipient addresses.
type CollectionLink struct {
	ID         string    `json:"id"`
	AccessType string    `json:"access_type"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Received   bool      `json:"received"`
}

func CollectionStatus(ctx context.Context, db *gorm.DB, tenantID, clientID string, since time.Time) ([]CollectionLink, error) {
	items := []CollectionLink{}
	linkCreated := dbtime.Instant(db, "secure_links.created_at")
	activityCreated := dbtime.Instant(db, "a.created_at")
	err := db.WithContext(ctx).Model(&models.SecureLink{}).
		Select(`secure_links.id, secure_links.access_type, secure_links.created_at, secure_links.expires_at,
		EXISTS (SELECT 1 FROM file_activity a WHERE a.tenant_id = secure_links.tenant_id
		AND a.client_id = secure_links.client_id AND a.secure_link_id = secure_links.id
		AND a.action = 'upload' AND ? AND `+activityCreated+` >= `+linkCreated+`) AS received`, dbtime.Compare(db, "a.created_at", ">", since)).
		Where("tenant_id = ? AND client_id = ? AND file_id IS NULL AND revoked_at IS NULL", tenantID, clientID).
		Order(linkCreated + " DESC").Scan(&items).Error
	return items, err
}
