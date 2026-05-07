package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type InviteToken struct {
	ID        string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID  string     `gorm:"not null;index" json:"tenant_id"`
	UserID    string     `gorm:"not null;index" json:"user_id"`
	Email     string     `gorm:"not null;default:''" json:"email"`
	TokenHash string     `gorm:"not null" json:"token_hash"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	InvitedBy string     `gorm:"not null;index" json:"invited_by"`
	CreatedAt time.Time  `gorm:"not null" json:"created_at"`
}

func (t *InviteToken) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&t.TenantID)
	if t.ID == "" {
		t.ID = ids.NewULID()
	}
	return nil
}

func (InviteToken) TableName() string {
	return "invite_tokens"
}

func (t *InviteToken) Used() bool {
	return t.UsedAt != nil
}
