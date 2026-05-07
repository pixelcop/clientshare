package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type QueuedEmail struct {
	ID           string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID     string     `gorm:"not null;index" json:"tenant_id"`
	Recipient    string     `gorm:"not null" json:"recipient"`
	Subject      string     `gorm:"not null" json:"subject"`
	Body         string     `gorm:"not null" json:"body"`
	HTMLBody     string     `gorm:"column:html_body;not null;default:''" json:"html_body"`
	FileIDsCSV   string     `gorm:"column:file_ids_csv;not null;default:''" json:"file_ids_csv"`
	ClientID     *string    `json:"client_id"`
	EventType    string     `gorm:"not null" json:"event_type"`
	ScheduledFor time.Time  `gorm:"not null" json:"scheduled_for"`
	SentAt       *time.Time `json:"sent_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (e *QueuedEmail) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&e.TenantID)
	if e.ID == "" {
		e.ID = ids.NewULID()
	}
	return nil
}

func (QueuedEmail) TableName() string {
	return "email_queue"
}

func (e *QueuedEmail) Sent() bool {
	return e.SentAt != nil
}
