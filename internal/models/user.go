package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type User struct {
	ID                     string    `gorm:"primaryKey;size:26" json:"id"`
	TenantID               string    `gorm:"not null;index;uniqueIndex:idx_users_tenant_email,priority:1" json:"tenant_id"`
	Email                  string    `gorm:"not null;uniqueIndex:idx_users_tenant_email,priority:2" json:"email"`
	PasswordHash           string    `gorm:"not null" json:"-"`
	Role                   string    `gorm:"not null" json:"role"`
	Name                   string    `json:"name"`
	ClientIDs              []string  `gorm:"-" json:"client_ids,omitempty"`
	InviteAccepted         bool      `gorm:"-" json:"invite_accepted"`
	PasskeyPromptDismissed bool      `gorm:"not null;default:false" json:"-"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

func (u *User) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&u.TenantID)
	if u.ID == "" {
		u.ID = ids.NewULID()
	}
	return nil
}
