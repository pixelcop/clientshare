package services

import (
	"context"
	"time"

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
	dateFilter := "a.created_at > ? AND a.created_at >= secure_links.created_at"
	if db.Dialector.Name() == "sqlite" {
		// SQLite stores offsets as text; compare instants rather than wall-clock strings.
		dateFilter = "julianday(a.created_at) > julianday(?) AND julianday(a.created_at) >= julianday(secure_links.created_at)"
	}
	err := db.WithContext(ctx).Model(&models.SecureLink{}).
		Select(`secure_links.id, secure_links.access_type, secure_links.created_at, secure_links.expires_at,
		EXISTS (SELECT 1 FROM file_activity a WHERE a.tenant_id = secure_links.tenant_id
		AND a.client_id = secure_links.client_id AND a.secure_link_id = secure_links.id
		AND a.action = 'upload' AND `+dateFilter+`) AS received`, since).
		Where("tenant_id = ? AND client_id = ?", tenantID, clientID).
		Order("created_at DESC").Scan(&items).Error
	return items, err
}
