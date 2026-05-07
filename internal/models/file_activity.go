package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type FileActivity struct {
	ID            string    `gorm:"primaryKey;size:26" json:"id"`
	TenantID      string    `gorm:"not null;index" json:"tenant_id"`
	ClientID      string    `gorm:"not null;index" json:"client_id"`
	UserID        *string   `json:"user_id"`
	SecureLinkID  *string   `json:"secure_link_id"`
	FileID        *string   `gorm:"index" json:"file_id,omitempty"`
	UploadBatchID *string   `gorm:"index" json:"upload_batch_id,omitempty"`
	Action        string    `gorm:"not null" json:"action"`
	FilePath      string    `gorm:"not null" json:"file_path"`
	CreatedAt     time.Time `json:"created_at"`
}

func (f *FileActivity) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&f.TenantID)
	if f.ID == "" {
		f.ID = ids.NewULID()
	}
	return nil
}

func (FileActivity) TableName() string {
	return "file_activity"
}
