package models

// FileCommentParticipant records an eligible mention, not a subscription.
// References to people who could not read the comment are never recorded.
type FileCommentParticipant struct {
	TenantID  string `gorm:"primaryKey;size:26"`
	CommentID string `gorm:"primaryKey;size:26"`
	UserID    string `gorm:"primaryKey;size:26"`
}
