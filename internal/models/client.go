package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type Client struct {
	ID           string    `gorm:"primaryKey;size:26" json:"id"`
	TenantID     string    `gorm:"not null;index;uniqueIndex:idx_clients_tenant_folder_path,priority:1" json:"tenant_id"`
	Name         string    `gorm:"not null" json:"name"`
	FolderPath   string    `gorm:"not null;uniqueIndex:idx_clients_tenant_folder_path,priority:2" json:"folder_path"`
	RootFolderID *string   `gorm:"index" json:"root_folder_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (c *Client) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&c.TenantID)
	if c.ID == "" {
		c.ID = ids.NewULID()
	}
	return nil
}
