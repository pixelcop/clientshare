package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// ItemGrant is the current grant for an item/recipient, including revoked state.
// Updates retain the row's ID; before/after history belongs in AuditEvent.
// CreatedBy is provenance, not a dependency on the creator's continued access.
type ItemGrant struct {
	ID                 string  `gorm:"primaryKey;size:26" json:"id"`
	RoleID             *string `gorm:"index" json:"role_id,omitempty"`
	TenantID           string  `gorm:"not null;uniqueIndex:idx_item_grants_recipient,priority:1;index:idx_item_grants_target,priority:1" json:"tenant_id"`
	ClientID           string  `gorm:"not null;index:idx_item_grants_target,priority:2" json:"client_id"`
	UserID             string  `gorm:"not null;uniqueIndex:idx_item_grants_recipient,priority:2" json:"user_id"`
	FileID             string  `gorm:"not null;uniqueIndex:idx_item_grants_recipient,priority:3;index:idx_item_grants_target,priority:3" json:"file_id"`
	AccessCapabilities `gorm:"embedded"`
	CreatedBy          string     `gorm:"not null" json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	Version            uint64     `gorm:"not null;default:1" json:"version"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	RevokedAt          *time.Time `gorm:"index" json:"revoked_at,omitempty"`
}

func (g *ItemGrant) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&g.TenantID)
	if g.ID == "" {
		g.ID = ids.NewULID()
	}
	return nil
}
