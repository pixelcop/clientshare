package models

import (
	"time"

	"gorm.io/gorm"
)

// UserClient maps users to the clients they are allowed to access.
type UserClient struct {
	TenantID string  `gorm:"not null;index" json:"tenant_id"`
	UserID   string  `gorm:"primaryKey;size:26" json:"user_id"`
	ClientID string  `gorm:"primaryKey;size:26" json:"client_id"`
	RoleID   *string `gorm:"index" json:"role_id,omitempty"`
	// Unset capabilities preserve legacy role-specific membership behavior.
	// An explicit all-false membership grants nothing, rather than legacy rights.
	CapabilitiesSet    bool `gorm:"not null;default:false" json:"capabilities_set"`
	AccessCapabilities `gorm:"embedded"`
	// Independent membership is never taken over by a subsequent invitation.
	// Only invitation-owned rows may be managed through InvitedBy.
	AccessSource string    `gorm:"not null;default:independent" json:"access_source"`
	InvitedBy    *string   `gorm:"index" json:"invited_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (u *UserClient) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&u.TenantID)
	return nil
}

func (UserClient) TableName() string {
	return "user_clients"
}
