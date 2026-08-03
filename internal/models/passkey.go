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
	RPID             string     `gorm:"not null;default:'';size:255;index" json:"-"`
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
	RPID        string     `gorm:"not null;default:'';size:255" json:"-"`
	SessionData []byte     `gorm:"not null" json:"-"`
	ExpiresAt   time.Time  `gorm:"not null;index" json:"expires_at"`
	UsedAt      *time.Time `gorm:"index" json:"used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// HostedLoginHandoff is a short-lived, single-use code used to transfer a
// hosted SaaS sign-in to its tenant portal without exposing a session JWT.
type HostedLoginHandoff struct {
	ID        string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID  string     `gorm:"not null;index" json:"tenant_id"`
	UserID    string     `gorm:"not null;index" json:"user_id"`
	Role      string     `gorm:"not null;size:32" json:"role"`
	CodeHash  string     `gorm:"not null;size:64;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `gorm:"not null;index" json:"expires_at"`
	UsedAt    *time.Time `gorm:"index" json:"used_at,omitempty"`
	CreatedAt time.Time  `gorm:"not null" json:"created_at"`
}

func (h *HostedLoginHandoff) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&h.TenantID)
	if h.ID == "" {
		h.ID = ids.NewULID()
	}
	return nil
}

func (p *WebAuthnChallenge) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&p.TenantID)
	if p.ID == "" {
		p.ID = ids.NewULID()
	}
	return nil
}
