package models

import (
	"time"

	"gorm.io/gorm"
)

// UserClient maps users to the clients they are allowed to access.
type UserClient struct {
	TenantID  string    `gorm:"not null;index" json:"tenant_id"`
	UserID    string    `gorm:"primaryKey;size:26" json:"user_id"`
	ClientID  string    `gorm:"primaryKey;size:26" json:"client_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (u *UserClient) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&u.TenantID)
	return nil
}

func (UserClient) TableName() string {
	return "user_clients"
}
