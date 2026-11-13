package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// ReplayTarget fields are ANDed; a request's targets are ORed. Empty fields
// impose no restriction. Identity fields only match verified portal sessions.
type ReplayTarget struct {
	IP        string `json:"ip,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
	TenantID  string `json:"tenant_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
}

type ReplayRequest struct {
	ID             string         `gorm:"primaryKey;size:26" json:"id"`
	Label          string         `gorm:"not null" json:"label"`
	Targets        []ReplayTarget `gorm:"serializer:json;type:text;not null" json:"targets"`
	StartOnFailure bool           `gorm:"not null" json:"start_on_failure"`
	ExpiresAt      time.Time      `gorm:"not null;index" json:"expires_at"`
	RevokedAt      *time.Time     `gorm:"index" json:"revoked_at"`
	CreatedAt      time.Time      `json:"created_at"`
}

func (r *ReplayRequest) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = ids.NewULID()
	}
	return nil
}
