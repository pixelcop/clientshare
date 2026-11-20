package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type DocumentRequest struct {
	ID          string `gorm:"size:26;primaryKey"`
	TenantID    string `gorm:"size:26;not null;index:idx_document_requests_client"`
	ClientID    string `gorm:"size:26;not null;index:idx_document_requests_client"`
	FolderID    string `gorm:"size:26;not null"`
	Title       string `gorm:"size:160;not null"`
	Description string `gorm:"type:text"`
	CreatedBy   string `gorm:"size:26;not null"`
	CreatedAt   time.Time
	ExpiresAt   time.Time `gorm:"not null"`
	RevokedAt   *time.Time
}

func (r *DocumentRequest) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = ids.NewULID()
	}
	return nil
}

type DocumentRequestUpdate struct {
	ID          string `gorm:"size:26;primaryKey"`
	RequestID   string `gorm:"size:26;not null;index"`
	Description string `gorm:"type:text;not null"`
	CreatedAt   time.Time
}

type DocumentRequestItem struct {
	ID           string `gorm:"size:26;primaryKey"`
	RequestID    string `gorm:"size:26;not null;index"`
	Position     int    `gorm:"not null"`
	Name         string `gorm:"size:160;not null"`
	Instructions string `gorm:"type:text;not null"`
	Status       string `gorm:"size:20;not null"`
	Round        int    `gorm:"not null"`
	Reason       string `gorm:"type:text;not null"`
	SubmittedAt  *time.Time
}

func (r *DocumentRequestItem) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = ids.NewULID()
	}
	return nil
}

type DocumentRequestFile struct {
	FileID    string `gorm:"size:26;primaryKey"`
	ItemID    string `gorm:"size:26;not null;index:idx_document_request_files_round"`
	Round     int    `gorm:"not null;index:idx_document_request_files_round"`
	Name      string `gorm:"type:text;not null"`
	CreatedAt time.Time
}

type DocumentRequestReview struct {
	ItemID      string `gorm:"size:26;primaryKey"`
	Round       int    `gorm:"primaryKey;autoIncrement:false"`
	Status      string `gorm:"size:20;not null"`
	Reason      string `gorm:"type:text;not null"`
	ReviewerID  string `gorm:"size:26;not null"`
	SubmittedAt time.Time
	CreatedAt   time.Time
}
