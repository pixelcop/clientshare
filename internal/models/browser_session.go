package models

import (
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
	"time"
)

// BrowserSession stores only a digest of the browser's random credential.
// Audiences separate portal credentials from server-side account credentials.
type BrowserSession struct {
	ID            string     `gorm:"primaryKey;size:26" json:"-"`
	TokenHash     string     `gorm:"not null;size:64;uniqueIndex" json:"-"`
	TenantID      string     `gorm:"not null;index" json:"-"`
	UserID        string     `gorm:"not null;index" json:"-"`
	LinkID        string     `gorm:"not null;index" json:"-"`
	Audience      string     `gorm:"not null;size:32;index" json:"-"`
	GroupID       string     `gorm:"not null;index" json:"-"`
	ExpiresAt     time.Time  `gorm:"not null;index" json:"-"`
	IdleExpiresAt time.Time  `gorm:"not null;index" json:"-"`
	RevokedAt     *time.Time `gorm:"index" json:"-"`
	CreatedAt     time.Time  `json:"-"`
}

func (s *BrowserSession) BeforeCreate(_ *gorm.DB) error {
	if s.ID == "" {
		s.ID = ids.NewULID()
	}
	if s.GroupID == "" {
		s.GroupID = s.ID
	}
	return nil
}
