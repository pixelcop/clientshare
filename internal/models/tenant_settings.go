package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type TenantSettings struct {
	ID                          string    `gorm:"primaryKey;size:26" json:"id"`
	TenantID                    string    `gorm:"not null;uniqueIndex" json:"tenant_id"`
	SiteTitle                   string    `gorm:"not null;default:''" json:"site_title"`
	LogoPath                    string    `gorm:"not null;default:''" json:"logo_path"`
	PrimaryColor                string    `gorm:"not null;default:''" json:"primary_color"`
	PublicBaseURL               string    `gorm:"column:public_base_url;not null;default:''" json:"public_base_url"`
	EffectivePublicBaseURL      string    `gorm:"-" json:"effective_public_base_url,omitempty"`
	InviteWelcomeText           string    `gorm:"column:invite_welcome_text;not null;default:''" json:"invite_welcome_text"`
	SecureLinkDefaultExpiryDays int       `gorm:"column:secure_link_default_expiry_days;not null;default:90" json:"secure_link_default_expiry_days"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

func (s *TenantSettings) BeforeCreate(_ *gorm.DB) error {
	if s.ID == "" {
		s.ID = ids.NewULID()
	}
	return nil
}

func (TenantSettings) TableName() string {
	return "tenant_settings"
}
