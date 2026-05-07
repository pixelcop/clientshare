package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type TenantEntitlement struct {
	ID              string    `gorm:"primaryKey;size:26" json:"id"`
	TenantID        string    `gorm:"not null;uniqueIndex" json:"tenant_id"`
	Status          string    `gorm:"not null;default:active" json:"status"`
	PlanCode        string    `gorm:"column:plan_code;not null;default:self_hosted" json:"plan_code"`
	MaxUsers        int64     `gorm:"column:max_users;not null;default:0" json:"max_users"`
	MaxStorageBytes int64     `gorm:"column:max_storage_bytes;not null;default:0" json:"max_storage_bytes"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (e *TenantEntitlement) BeforeCreate(_ *gorm.DB) error {
	if e.ID == "" {
		e.ID = ids.NewULID()
	}
	return nil
}

func (TenantEntitlement) TableName() string {
	return "tenant_entitlements"
}
