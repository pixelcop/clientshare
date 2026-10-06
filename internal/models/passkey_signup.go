package models

import "time"

// PasskeySignup holds a registration before its tenant/user has been provisioned.
// The random bearer token is stored only as a hash. Reserved registrations survive
// checkout and email verification delays; provisioning consumes them atomically.
type PasskeySignup struct {
	ID             string     `gorm:"primaryKey;size:26" json:"-"`
	TokenHash      string     `gorm:"not null;size:64;uniqueIndex" json:"-"`
	Email          string     `gorm:"not null" json:"-"`
	TenantSlug     string     `gorm:"not null" json:"-"`
	UserID         string     `gorm:"not null;size:26" json:"-"`
	SessionData    []byte     `gorm:"not null" json:"-"`
	CredentialData []byte     `json:"-"`
	RPID           string     `gorm:"not null" json:"-"`
	ExpiresAt      time.Time  `gorm:"not null;index" json:"-"`
	VerifiedAt     *time.Time `json:"-"`
	Reserved       bool       `gorm:"not null;default:false" json:"-"`
	TenantID       string     `gorm:"not null;default:''" json:"-"`
}
