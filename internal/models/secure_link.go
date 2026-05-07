package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type SecureLink struct {
	ID           string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID     string     `gorm:"not null;index" json:"tenant_id"`
	ClientID     string     `gorm:"not null;index" json:"client_id"`
	Token        string     `gorm:"unique;not null" json:"token"`
	LinkURL      string     `gorm:"-" json:"link_url,omitempty"`
	Email        string     `gorm:"not null;default:''" json:"email"`
	ExpiresAt    time.Time  `gorm:"not null" json:"expires_at"`
	AccessType   string     `gorm:"not null" json:"access_type"`
	CreatedBy    string     `gorm:"not null;index" json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	LastAccessed *time.Time `json:"last_accessed"`
}

func (s *SecureLink) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&s.TenantID)
	if s.ID == "" {
		s.ID = ids.NewULID()
	}
	return nil
}
