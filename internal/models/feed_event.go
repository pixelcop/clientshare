package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

const (
	FeedEventTypeFileUploaded   = "file_uploaded"
	FeedEventTypeInviteAccepted = "invite_accepted"

	FeedEventSourceTypeFileActivity    = "file_activity"
	FeedEventSourceTypeFileUploadBatch = "file_upload_batch"
	FeedEventSourceTypeInviteToken     = "invite_token"

	FeedStateUnread = "unread"
	FeedStateRead   = "read"
	FeedStateAll    = "all"
)

type FeedSummary struct {
	UnreadCount int64 `json:"unread_count"`
	ReadCount   int64 `json:"read_count"`
}

type FeedListResponse struct {
	Pagination PaginationResponse `json:"pagination"`
	Items      []FeedEvent        `json:"items"`
	Summary    FeedSummary        `json:"summary"`
}

type FeedEventFile struct {
	ID   string `gorm:"-" json:"id"`
	Path string `gorm:"-" json:"path"`
}

type FeedEvent struct {
	ID            string          `gorm:"primaryKey;size:26" json:"id"`
	TenantID      string          `gorm:"not null;index;index:idx_feed_events_user_created_at,priority:1;uniqueIndex:idx_feed_events_user_source,priority:1" json:"tenant_id"`
	UserID        string          `gorm:"not null;index:idx_feed_events_user_created_at,priority:2;uniqueIndex:idx_feed_events_user_source,priority:2" json:"user_id"`
	EventType     string          `gorm:"not null;index;uniqueIndex:idx_feed_events_user_source,priority:3" json:"event_type"`
	ClientID      *string         `gorm:"index" json:"client_id,omitempty"`
	ActorUserID   *string         `gorm:"index" json:"actor_user_id,omitempty"`
	SubjectUserID *string         `gorm:"index" json:"subject_user_id,omitempty"`
	SourceType    string          `gorm:"not null;uniqueIndex:idx_feed_events_user_source,priority:4" json:"source_type"`
	SourceID      string          `gorm:"not null;uniqueIndex:idx_feed_events_user_source,priority:5" json:"source_id"`
	FileID        *string         `gorm:"index" json:"file_id,omitempty"`
	FilePath      string          `gorm:"not null" json:"file_path"`
	CreatedAt     time.Time       `gorm:"index:idx_feed_events_user_created_at,priority:3" json:"created_at"`
	ClientName    string          `gorm:"->;column:client_name;-:migration" json:"client_name,omitempty"`
	ActorName     string          `gorm:"->;column:actor_name;-:migration" json:"actor_name,omitempty"`
	ActorEmail    string          `gorm:"->;column:actor_email;-:migration" json:"actor_email,omitempty"`
	SubjectName   string          `gorm:"->;column:subject_name;-:migration" json:"subject_name,omitempty"`
	SubjectEmail  string          `gorm:"->;column:subject_email;-:migration" json:"subject_email,omitempty"`
	Files         []FeedEventFile `gorm:"-" json:"files,omitempty"`
	FileCount     int             `gorm:"-" json:"file_count,omitempty"`
	IsRead        bool            `gorm:"not null;default:false" json:"is_read"`
	ReadAt        *time.Time      `json:"read_at,omitempty"`
}

func (f *FeedEvent) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&f.TenantID)
	if f.ID == "" {
		f.ID = ids.NewULID()
	}
	return nil
}

func (FeedEvent) TableName() string {
	return "feed_events"
}
