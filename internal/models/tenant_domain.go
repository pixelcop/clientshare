package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type TenantDomain struct {
	ID        string    `gorm:"primaryKey;size:26" json:"id"`
	TenantID  string    `gorm:"not null;index" json:"tenant_id"`
	Domain    string    `gorm:"not null;uniqueIndex" json:"domain"`
	Kind      string    `gorm:"not null;default:base_url" json:"kind"`
	IsPrimary bool      `gorm:"not null;default:false" json:"is_primary"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (d *TenantDomain) BeforeCreate(_ *gorm.DB) error {
	if d.ID == "" {
		d.ID = ids.NewULID()
	}
	return nil
}
