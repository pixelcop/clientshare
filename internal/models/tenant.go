package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type Tenant struct {
	ID        string    `gorm:"primaryKey;size:26" json:"id"`
	Slug      string    `gorm:"not null;uniqueIndex" json:"slug"`
	Name      string    `gorm:"not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (t *Tenant) BeforeCreate(_ *gorm.DB) error {
	if t.ID == "" {
		t.ID = ids.NewULID()
	}
	return nil
}
