package email

import (
	"context"
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/access"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

const (
	CommentNotification                = "file_comment"
	CommentMentionNotification         = "file_comment_mention"
	CommentMentionAndReplyNotification = "file_comment_mention_and_reply"
)

func CommentReader(ctx context.Context, db *gorm.DB, file models.File, visibility string, user models.User, now time.Time) (bool, error) {
	if user.TenantID != file.TenantID || user.ID == "" {
		return false, nil
	}
	switch user.Role {
	case "admin", "manager":
	case "client", "customer":
		if visibility != "shared" {
			return false, nil
		}
	default:
		return false, nil
	}
	if visibility != "shared" && visibility != "team" {
		return false, nil
	}
	decision, err := (&access.AccessService{DB: db}).Resolve(ctx, access.AccessRequest{
		TenantID: file.TenantID, ClientID: file.ClientID, FileID: file.ID,
		Actor: access.AccessActor{UserID: user.ID}, Operation: access.AccessRead, Now: now,
	})
	return decision.Allowed, err
}

func CommentNotificationAddress(value string) string {
	value = strings.TrimSpace(value)
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

// Only earlier, live comments in this audience count as participation.
func CommentPriorParticipants(db *gorm.DB, comment models.FileComment) *gorm.DB {
	return db.Model(&models.FileComment{}).
		Where("tenant_id = ? AND file_id = ? AND visibility = ? AND deleted_at IS NULL", comment.TenantID, comment.FileID, comment.Visibility).
		Where("(created_at < ? OR (created_at = ? AND id < ?))", comment.CreatedAt, comment.CreatedAt, comment.ID)
}

func (q *DBEmailQueue) revalidateCommentNotification(item *models.QueuedEmail) (bool, error) {
	if item.RecipientUserID == nil || item.ClientID == nil || item.FileCommentID == nil || item.RecipientLinkID != nil {
		return false, nil
	}
	ctx := context.Background()
	db := q.DB.WithContext(ctx)
	var comment models.FileComment
	err := db.Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", item.TenantID, *item.FileCommentID).First(&comment).Error
	if err != nil {
		return false, missingCommentNotification(err)
	}
	if comment.AuthorID == *item.RecipientUserID {
		return false, nil
	}
	var file models.File
	if err := db.Where("tenant_id = ? AND client_id = ? AND id = ? AND type = 'file' AND deleted_at IS NULL",
		item.TenantID, *item.ClientID, comment.FileID).First(&file).Error; err != nil {
		return false, missingCommentNotification(err)
	}
	var recipient models.User
	if err := db.Where("tenant_id = ? AND id = ?", item.TenantID, *item.RecipientUserID).First(&recipient).Error; err != nil {
		return false, missingCommentNotification(err)
	}
	if address := CommentNotificationAddress(recipient.Email); address == "" || !strings.EqualFold(address, item.Recipient) {
		return false, nil
	}
	allowed, err := CommentReader(ctx, db, file, comment.Visibility, recipient, q.now())
	if err != nil || !allowed {
		return false, err
	}
	mentioned := false
	switch item.EventType {
	case CommentMentionNotification, CommentMentionAndReplyNotification:
		var count int64
		if err := db.Model(&models.FileCommentParticipant{}).Where("tenant_id = ? AND comment_id = ? AND user_id = ?", item.TenantID, comment.ID, recipient.ID).Count(&count).Error; err != nil {
			return false, err
		}
		mentioned = count > 0
		if !mentioned && item.EventType == CommentMentionNotification {
			return false, nil
		}
	case CommentNotification:
	default:
		return false, nil
	}
	if !mentioned && !(comment.Visibility == "shared" && file.UserID != nil && *file.UserID == recipient.ID) {
		var count int64
		prior := CommentPriorParticipants(db, comment)
		eligibleMentions := db.Model(&models.FileCommentParticipant{}).Select("comment_id").
			Where("tenant_id = ? AND user_id = ?", item.TenantID, recipient.ID)
		if err := prior.Where("(author_id = ? OR id IN (?))", recipient.ID, eligibleMentions).Count(&count).Error; err != nil || count == 0 {
			return false, err
		}
	}
	base, err := url.Parse(item.NotificationBaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil {
		return false, nil
	}
	base = base.JoinPath("files", file.ID)
	base.RawQuery, base.Fragment = "", "comment-"+comment.ID
	var author models.User
	if err := db.Select("id", "name").Where("tenant_id = ? AND id = ?", item.TenantID, comment.AuthorID).First(&author).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	item.Subject, item.Body, item.HTMLBody, err = renderCommentNotification(author.Name, file.Filename, base.String(), comment.Visibility == "team", mentioned)
	return err == nil, err
}

func missingCommentNotification(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}
