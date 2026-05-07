package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type AuditEvent struct {
	ID        string    `gorm:"primaryKey;size:26" json:"id"`
	TenantID  *string   `gorm:"index" json:"tenant_id"`
	Actor     string    `gorm:"not null" json:"actor"`
	Action    string    `gorm:"not null;index" json:"action"`
	TargetID  *string   `json:"target_id"`
	Payload   string    `gorm:"not null;default:'{}'" json:"payload"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (e *AuditEvent) BeforeCreate(_ *gorm.DB) error {
	if e.ID == "" {
		e.ID = ids.NewULID()
	}
	return nil
}

func (AuditEvent) TableName() string {
	return "audit_events"
}
