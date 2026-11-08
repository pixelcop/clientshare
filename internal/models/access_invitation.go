package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// AccessInvitation is target-aware access intent, not an identity credential.
// Acceptance binds a verified tenant-local identity without changing its role.
type AccessInvitation struct {
	ID                 string  `gorm:"primaryKey;size:26" json:"id"`
	TenantID           string  `gorm:"not null;index" json:"tenant_id"`
	ClientID           string  `gorm:"not null;index" json:"client_id"`
	FileID             *string `gorm:"index" json:"file_id,omitempty"`
	Email              string  `gorm:"not null;index" json:"email"`
	RecipientUserID    *string `gorm:"index" json:"recipient_user_id,omitempty"`
	TokenHash          string  `gorm:"not null;uniqueIndex" json:"-"`
	CreatedBy          string  `gorm:"not null;index" json:"created_by"`
	AccessCapabilities `gorm:"embedded"`
	GrantExpiresAt     *time.Time `json:"grant_expires_at,omitempty"`
	ExpiresAt          time.Time  `gorm:"not null" json:"expires_at"`
	RevokedAt          *time.Time `gorm:"index" json:"revoked_at,omitempty"`
	AcceptedAt         *time.Time `json:"accepted_at,omitempty"`
	AcceptedBy         *string    `json:"accepted_by,omitempty"`
	CreationKey        *string    `gorm:"uniqueIndex" json:"-"`
	CreationHash       string     `gorm:"not null;default:''" json:"-"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (i *AccessInvitation) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&i.TenantID)
	if i.ID == "" {
		i.ID = ids.NewULID()
	}
	return nil
}
