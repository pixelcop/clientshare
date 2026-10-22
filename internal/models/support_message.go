package models

import (
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
	"time"
)

// SupportMessage is a private receipt/outbox, separate from tenant-editable email queues.
type SupportMessage struct {
	ID            string `gorm:"primaryKey;size:26"`
	TenantID      string `gorm:"not null;uniqueIndex:idx_support_retry,priority:1"`
	RequesterID   string `gorm:"not null;uniqueIndex:idx_support_retry,priority:2"`
	RetryKey      string `gorm:"size:36;not null;uniqueIndex:idx_support_retry,priority:3"`
	Role          string
	Name          string
	Email         string
	Provider      string
	Destination   string
	Message       string `gorm:"type:text"`
	Screen        string
	Version       string
	CreatedAt     time.Time
	NotifiedAt    *time.Time
	NextAttemptAt time.Time `gorm:"index"`
	Attempts      int
}

func (m *SupportMessage) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = ids.NewULID()
	}
	return nil
}
