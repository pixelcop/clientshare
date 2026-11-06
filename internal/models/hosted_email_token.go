package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// HostedEmailToken is separate from portal reset tokens and login handoffs.
// Only a keyed hash is stored; one current token exists per user and purpose.
type HostedEmailToken struct {
	ID             string `gorm:"primaryKey;size:26"`
	TenantID       string `gorm:"not null;uniqueIndex:idx_hosted_email_user_purpose"`
	UserID         string `gorm:"not null;uniqueIndex:idx_hosted_email_user_purpose"`
	Purpose        string `gorm:"not null;uniqueIndex:idx_hosted_email_user_purpose"`
	TokenHash      string `gorm:"not null;uniqueIndex" json:"-"`
	CredentialHash string `gorm:"not null" json:"-"`
	Destination    string `gorm:"not null"`
	ExpiresAt      time.Time
	UsedAt         *time.Time
	CreatedAt      time.Time
}

func (t *HostedEmailToken) BeforeCreate(_ *gorm.DB) error {
	if t.ID == "" {
		t.ID = ids.NewULID()
	}
	return nil
}
