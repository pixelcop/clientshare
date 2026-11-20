package services

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

var ErrDocumentRequestItemLimit = errors.New("a request can contain up to 100 documents")

type DocumentRequestAddition struct {
	Description string                     `json:"description"`
	Items       []DocumentRequestItemInput `json:"items"`
}

func newRequestItems(requestID string, input []DocumentRequestItemInput) ([]models.DocumentRequestItem, error) {
	if len(input) == 0 || len(input) > 30 {
		return nil, ErrDocumentRequestInput
	}
	items := make([]models.DocumentRequestItem, 0, len(input))
	for position, item := range input {
		item.Name, item.Instructions = strings.TrimSpace(item.Name), strings.TrimSpace(item.Instructions)
		if item.Name == "" || utf8.RuneCountInString(item.Name) > 160 || utf8.RuneCountInString(item.Instructions) > 1000 {
			return nil, ErrDocumentRequestInput
		}
		items = append(items, models.DocumentRequestItem{RequestID: requestID, Position: position, Name: item.Name, Instructions: item.Instructions, Status: requestNeeded, Round: 1})
	}
	return items, nil
}

func (s *DocumentRequestService) AddItems(ctx context.Context, req AccessRequest, requestID string, input DocumentRequestAddition) (*DocumentRequestView, error) {
	description := strings.TrimSpace(input.Description)
	items, err := newRequestItems(requestID, input.Items)
	if err != nil || utf8.RuneCountInString(description) > 4000 {
		return nil, ErrDocumentRequestInput
	}
	var emailStatus string
	var emailCount int
	err = s.mutateStaff(ctx, req, requestID, func(tx *gorm.DB, row models.DocumentRequest) error {
		if err := s.staff(ctx, tx, req, true); err != nil {
			return err
		}
		now := time.Now().UTC()
		if row.RevokedAt != nil || !row.ExpiresAt.After(now) {
			return gorm.ErrRecordNotFound
		}
		if _, err := requestFolder(tx, row.TenantID, row.ClientID, row.FolderID); err != nil {
			return err
		}
		var next int
		if err := tx.Model(&models.DocumentRequestItem{}).Where("request_id = ?", row.ID).Select("COALESCE(MAX(position) + 1, 0)").Scan(&next).Error; err != nil {
			return err
		}
		if next+len(items) > 100 {
			return ErrDocumentRequestItemLimit
		}
		for i := range items {
			items[i].Position += next
		}
		update := models.DocumentRequestUpdate{ID: ids.NewULID(), RequestID: row.ID, Description: description, CreatedAt: now}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		var named []models.User
		audience := requestAudience(tx, row).Select("user_id")
		if err := tx.Where("tenant_id = ? AND id IN (?)", row.TenantID, audience).Find(&named).Error; err != nil {
			return err
		}
		recipients := make([]models.User, 0, len(named))
		users := make([]string, 0, len(named))
		for _, user := range named {
			allowed, err := email.DocumentRequestRecipient(ctx, tx, row.ClientID, user, now)
			if err != nil {
				return err
			}
			if allowed {
				recipients = append(recipients, user)
				users = append(users, user.ID)
			}
		}
		if err := recordDocumentRequestActivity(ctx, tx, row, models.FeedEventTypeRequestUpdated, models.FeedEventSourceTypeDocumentRequestUpdate, update.ID, &req.Actor.UserID, users, now); err != nil {
			return err
		}
		emailStatus, emailCount, err = s.queueRequestEmails(tx, row, recipients, &update.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	view, err := s.StaffView(ctx, req, requestID)
	if err == nil {
		view.EmailStatus, view.EmailCount = emailStatus, emailCount
	}
	return view, err
}

func (s *DocumentRequestService) queueRequestEmails(tx *gorm.DB, row models.DocumentRequest, recipients []models.User, updateID *string) (string, int, error) {
	if len(recipients) == 0 {
		return "no_recipients", 0, nil
	}
	if s.NotificationBaseURL == "" {
		return "unavailable", 0, nil
	}
	event, subject, keyPrefix := email.DocumentRequestNotification, "New document request", "document-request:"+row.ID
	if updateID != nil {
		event, subject, keyPrefix = email.DocumentRequestUpdateNotification, "More documents requested", "document-request-update:"+*updateID
	}
	deliveries := make([]models.QueuedEmail, 0, len(recipients))
	now := time.Now().UTC()
	seen := make(map[string]bool)
	for _, recipient := range recipients {
		address := strings.ToLower(recipient.Email)
		if seen[address] {
			continue
		}
		seen[address] = true
		key := keyPrefix + ":" + recipient.ID
		deliveries = append(deliveries, models.QueuedEmail{
			TenantID: row.TenantID, ClientID: &row.ClientID, DocumentRequestID: &row.ID, DocumentRequestUpdateID: updateID,
			Recipient: recipient.Email, RecipientUserID: &recipient.ID, Subject: subject,
			ScheduledFor: now, DeduplicationKey: &key, EventType: event, NotificationBaseURL: s.NotificationBaseURL,
		})
	}
	return "queued", len(deliveries), tx.CreateInBatches(&deliveries, 25).Error
}
