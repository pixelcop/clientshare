package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type PasswordResetToken struct {
	ID        string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID  string     `gorm:"not null;index" json:"tenant_id"`
	UserID    string     `gorm:"not null;index" json:"user_id"`
	TokenHash string     `gorm:"not null" json:"token_hash"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `gorm:"not null" json:"created_at"`
}

func (t *PasswordResetToken) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&t.TenantID)
	if t.ID == "" {
		t.ID = ids.NewULID()
	}
	return nil
}

func (PasswordResetToken) TableName() string {
	return "password_reset_tokens"
}

func (t *PasswordResetToken) Used() bool {
	return t.UsedAt != nil
}
