package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// File stores metadata for uploaded files
type File struct {
	ID            string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID      string     `gorm:"not null;index" json:"tenant_id"`
	ClientID      string     `gorm:"not null;index" json:"client_id"`
	UserID        *string    `json:"user_id"`
	FolderID      *string    `gorm:"index" json:"folder_id"`
	Filename      string     `gorm:"not null" json:"filename"`
	Path          string     `gorm:"not null" json:"path"`
	Type          string     `gorm:"not null;default:file;size:16" json:"type"`
	Size          int64      `gorm:"not null" json:"size"`
	UploadedAt    time.Time  `gorm:"autoCreateTime" json:"uploaded_at"`
	DeletedAt     *time.Time `gorm:"index" json:"deleted_at,omitempty"`
	DiskDeletedAt *time.Time `gorm:"index" json:"disk_deleted_at,omitempty"`
}

func (f *File) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&f.TenantID)
	if f.ID == "" {
		f.ID = ids.NewULID()
	}
	return nil
}

func (File) TableName() string {
	return "files"
}

// FileItem is used for TypeScript type generation
// (hack to emit a type with the proper name that doesn't conflict with the native JS File type)
type FileItem struct {
	File `tstype:",extends,required"`
}
