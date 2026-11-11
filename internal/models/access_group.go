package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// AccessGroup contains people, never nested groups or implicit capabilities.
type AccessGroup struct {
	ID          string    `gorm:"primaryKey;size:26" json:"id"`
	TenantID    string    `gorm:"not null;uniqueIndex:idx_access_group_name,priority:1" json:"tenant_id"`
	Name        string    `gorm:"not null;uniqueIndex:idx_access_group_name,priority:2" json:"name"`
	Description string    `gorm:"not null;default:''" json:"description"`
	Version     uint64    `gorm:"not null;default:1" json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (g *AccessGroup) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&g.TenantID)
	if g.ID == "" {
		g.ID = ids.NewULID()
	}
	return nil
}

type AccessGroupMember struct {
	TenantID  string    `gorm:"primaryKey" json:"tenant_id"`
	GroupID   string    `gorm:"primaryKey;size:26" json:"group_id"`
	UserID    string    `gorm:"primaryKey;size:26" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// RoleAssignment references exactly one tenant-local person/group and one scope.
// Source fields identify migrated rows without replacing their original records.
type RoleAssignment struct {
	ScopeName string `gorm:"-" json:"scope_name,omitempty"`
	ID         string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID   string     `gorm:"not null;index:idx_role_assignment_target,priority:1;uniqueIndex:idx_role_assignment_source,priority:1" json:"tenant_id"`
	RoleID     string     `gorm:"not null;index" json:"role_id"`
	UserID     *string    `gorm:"index" json:"user_id,omitempty"`
	GroupID    *string    `gorm:"index" json:"group_id,omitempty"`
	ClientID   string     `gorm:"not null;index:idx_role_assignment_target,priority:2" json:"client_id"`
	FileID     *string    `gorm:"index:idx_role_assignment_target,priority:3" json:"file_id,omitempty"`
	SourceKind string     `gorm:"not null;default:direct;uniqueIndex:idx_role_assignment_source,priority:2" json:"source_kind"`
	SourceID   *string    `gorm:"uniqueIndex:idx_role_assignment_source,priority:3" json:"source_id,omitempty"`
	Version    uint64     `gorm:"not null;default:1" json:"version"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (a *RoleAssignment) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&a.TenantID)
	if a.ID == "" {
		a.ID = ids.NewULID()
	}
	return nil
}
