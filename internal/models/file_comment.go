package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// FileComment visibility is immutable. Editing a team note must never silently
// publish its content (or revision history) to a wider audience.
type FileComment struct {
	ID             string     `gorm:"primaryKey;size:26" json:"id"`
	TenantID       string     `gorm:"not null;index;uniqueIndex:idx_comment_creation,priority:1;index:idx_comment_file,priority:1" json:"-"`
	FileID         string     `gorm:"not null;size:26;index:idx_comment_file,priority:2" json:"file_id"`
	AuthorID       string     `gorm:"not null;size:26;uniqueIndex:idx_comment_creation,priority:2" json:"author_id"`
	Visibility     string     `gorm:"not null;size:16" json:"visibility"`
	Body           string     `gorm:"not null;type:text" json:"body"`
	Version        int        `gorm:"not null" json:"version"`
	CreationKey    string     `gorm:"not null;size:128;uniqueIndex:idx_comment_creation,priority:3" json:"-"`
	CreationHash   string     `gorm:"not null;size:64" json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
	AuthorName     string     `gorm:"-" json:"author_name"`
	CanEdit        bool       `gorm:"-" json:"can_edit"`
	CanViewHistory bool       `gorm:"-" json:"can_view_history"`
}

func (c *FileComment) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&c.TenantID)
	if c.ID == "" {
		c.ID = ids.NewULID()
	}
	return nil
}

// Revisions are append-only. They are accessible only to the author while the
// author still has file and visibility access; comments are not an access grant.
type FileCommentRevision struct {
	ID        string    `gorm:"primaryKey;size:26" json:"id"`
	TenantID  string    `gorm:"not null;uniqueIndex:idx_comment_revision,priority:1" json:"-"`
	CommentID string    `gorm:"not null;size:26;uniqueIndex:idx_comment_revision,priority:2" json:"comment_id"`
	Version   int       `gorm:"not null;uniqueIndex:idx_comment_revision,priority:3" json:"version"`
	Body      string    `gorm:"not null;type:text" json:"body"`
	Action    string    `gorm:"not null;size:16" json:"action"`
	CreatedAt time.Time `json:"created_at"`
}

func (r *FileCommentRevision) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&r.TenantID)
	if r.ID == "" {
		r.ID = ids.NewULID()
	}
	return nil
}
