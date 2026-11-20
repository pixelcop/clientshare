package services

import (
	"context"
	"errors"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/email"
	"gorm.io/gorm"
)

func recordCommentParticipation(ctx context.Context, tx *gorm.DB, comment models.FileComment, baseURL string) error {
	_, ids := prepareCommentMarkdown(comment.Body)
	var file models.File
	if err := tx.Where("tenant_id = ? AND id = ?", comment.TenantID, comment.FileID).First(&file).Error; err != nil {
		return err
	}
	var mentioned []models.User
	if len(ids) > 0 {
		if err := tx.Where("tenant_id = ? AND id IN ?", comment.TenantID, ids).Find(&mentioned).Error; err != nil {
			return err
		}
	}
	eligible := make(map[string]models.User, len(mentioned))
	for _, user := range mentioned {
		if user.ID == comment.AuthorID {
			continue
		}
		allowed, err := email.CommentReader(ctx, tx, file, comment.Visibility, user, comment.CreatedAt)
		if err != nil {
			return err
		}
		if allowed {
			eligible[user.ID] = user
			if err := tx.Create(&models.FileCommentParticipant{TenantID: comment.TenantID, CommentID: comment.ID, UserID: user.ID}).Error; err != nil {
				return err
			}
		}
	}
	// Select only people involved in this audience. Access alone is never a
	// subscription, regardless of whether it is inherited or granted directly.
	priorAuthors := email.CommentPriorParticipants(tx, comment).Select("author_id")
	priorIDs := email.CommentPriorParticipants(tx, comment).Select("id")
	priorMentions := tx.Model(&models.FileCommentParticipant{}).Select("user_id").
		Where("tenant_id = ? AND comment_id IN (?)", comment.TenantID, priorIDs)
	participants := tx.Where("(users.id IN (?) OR users.id IN (?))", priorAuthors, priorMentions)
	if comment.Visibility == CommentShared && file.UserID != nil {
		participants = participants.Or("users.id = ?", *file.UserID)
	}
	var readers []models.User
	if err := tx.Where("users.tenant_id = ? AND users.id <> ?", comment.TenantID, comment.AuthorID).
		Where(participants).Find(&readers).Error; err != nil {
		return err
	}
	deliveries := make([]models.QueuedEmail, 0, len(eligible)+len(readers))
	feedUsers := make([]string, 0, len(eligible)+len(readers))
	queue := func(user models.User, event string) {
		feedUsers = append(feedUsers, user.ID)
		if baseURL == "" {
			return
		}
		address := email.CommentNotificationAddress(user.Email)
		if address == "" {
			return
		}
		key := "comment:" + comment.TenantID + ":" + comment.ID + ":" + user.ID
		deliveries = append(deliveries, models.QueuedEmail{
			TenantID: comment.TenantID, ClientID: &file.ClientID, Recipient: address,
			RecipientUserID: &user.ID, FileCommentID: &comment.ID, EventType: event,
			ScheduledFor: comment.CreatedAt, DeduplicationKey: &key, NotificationBaseURL: baseURL,
			Subject: "New file comment",
		})
	}
	for _, reader := range readers {
		if _, ok := eligible[reader.ID]; ok {
			queue(reader, email.CommentMentionAndReplyNotification)
			delete(eligible, reader.ID)
			continue
		}
		allowed, err := email.CommentReader(ctx, tx, file, comment.Visibility, reader, comment.CreatedAt)
		if err != nil {
			return err
		}
		if allowed {
			queue(reader, email.CommentNotification)
		}
	}
	for _, user := range eligible {
		queue(user, email.CommentMentionNotification)
	}
	var actorID *string
	if !isRequestGuestAuthor(comment.AuthorID) {
		actorID = &comment.AuthorID
	}
	err := (&FeedService{DB: tx}).createEvents(ctx, feedUsers, func(userID string) models.FeedEvent {
		return models.FeedEvent{
			TenantID: comment.TenantID, ClientID: &file.ClientID, UserID: userID, ActorUserID: actorID,
			SourceType: models.FeedEventSourceTypeFileComment, SourceID: comment.ID, EventType: models.FeedEventTypeFileComment,
			FileID: &file.ID, CreatedAt: comment.CreatedAt,
		}
	})
	if err != nil {
		return err
	}
	if len(deliveries) == 0 {
		return nil
	}
	return tx.CreateInBatches(&deliveries, 25).Error
}

func hydrateCommentActivity(ctx context.Context, db *gorm.DB, user models.User, event *models.FeedEvent) (bool, error) {
	if event.ClientID == nil || event.FileID == nil || event.SourceType != models.FeedEventSourceTypeFileComment {
		return false, nil
	}
	var comment models.FileComment
	err := db.First(&comment, "tenant_id = ? AND file_id = ? AND id = ? AND deleted_at IS NULL",
		event.TenantID, *event.FileID, event.SourceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var file models.File
	err = db.First(&file, "tenant_id = ? AND client_id = ? AND id = ? AND type = 'file' AND deleted_at IS NULL",
		event.TenantID, *event.ClientID, *event.FileID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	allowed, err := email.CommentReader(ctx, db, file, comment.Visibility, user, time.Now())
	if allowed {
		event.FilePath = file.Filename
		if comment.Visibility == "team" {
			event.EventType = models.FeedEventTypeTeamNote
		}
	}
	return allowed, err
}

func pruneCommentParticipation(tx *gorm.DB, comment models.FileComment) error {
	_, ids := prepareCommentMarkdown(comment.Body)
	query := tx.Where("tenant_id = ? AND comment_id = ?", comment.TenantID, comment.ID)
	if len(ids) > 0 {
		query = query.Where("user_id NOT IN ?", ids)
	}
	return query.Delete(&models.FileCommentParticipant{}).Error
}
