package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// PasskeyCredential stores the credential record required to validate a WebAuthn assertion.
// CredentialData is intentionally never exposed through the API.
type PasskeyCredential struct {
	ID               string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID         string     `gorm:"not null;index;uniqueIndex:idx_passkeys_tenant_credential,priority:1" json:"tenant_id"`
	UserID           string     `gorm:"not null;index" json:"user_id"`
	CredentialID     string     `gorm:"not null;size:1400" json:"-"`
	CredentialIDHash string     `gorm:"not null;size:64;uniqueIndex:idx_passkeys_tenant_credential,priority:2" json:"-"`
	CredentialData   []byte     `gorm:"not null" json:"-"`
	Name             string     `gorm:"not null;size:100" json:"name"`
	CreatedAt        time.Time  `json:"created_at"`
	LastUsedAt       *time.Time `gorm:"index" json:"last_used_at,omitempty"`
}

func (p *PasskeyCredential) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&p.TenantID)
	if p.ID == "" {
		p.ID = ids.NewULID()
	}
	return nil
}

// WebAuthnChallenge persists server-side ceremony state. Challenge data is never sent back to clients.
type WebAuthnChallenge struct {
	ID          string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID    string     `gorm:"not null;index" json:"tenant_id"`
	UserID      string     `gorm:"not null;index" json:"user_id"`
	Purpose     string     `gorm:"not null;size:20;index" json:"purpose"`
	SessionData []byte     `gorm:"not null" json:"-"`
	ExpiresAt   time.Time  `gorm:"not null;index" json:"expires_at"`
	UsedAt      *time.Time `gorm:"index" json:"used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (p *WebAuthnChallenge) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&p.TenantID)
	if p.ID == "" {
		p.ID = ids.NewULID()
	}
	return nil
}
