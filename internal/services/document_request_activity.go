package services

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/email"
	"gorm.io/gorm"
)

func recordDocumentRequestActivity(ctx context.Context, tx *gorm.DB, request models.DocumentRequest, eventType, sourceType, sourceID string, actor *string, users []string, at time.Time) error {
	feed := &FeedService{DB: tx}
	return feed.createEvents(ctx, users, func(userID string) models.FeedEvent {
		return models.FeedEvent{TenantID: request.TenantID, ClientID: &request.ClientID, UserID: userID,
			EventType: eventType, SourceType: sourceType, SourceID: sourceID, ActorUserID: actor, CreatedAt: at}
	})
}

func requestItemActivity(ctx context.Context, tx *gorm.DB, request models.DocumentRequest, itemID string, round int, eventType string, actor *string, at time.Time) error {
	resolver := &AccessService{DB: tx}
	staff, err := resolver.WorkspaceReaderIDs(ctx, request.TenantID, request.ClientID, []string{"admin", "manager"})
	if err != nil {
		return err
	}
	users := make([]string, 0, len(staff))
	for _, id := range staff {
		err := (&DocumentRequestService{}).staff(ctx, tx, AccessRequest{
			TenantID: request.TenantID, ClientID: request.ClientID, Actor: AccessActor{UserID: id},
		}, false)
		if err == nil {
			users = append(users, id)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	if eventType != models.FeedEventTypeRequestSubmitted {
		var recipients []string
		if err := requestAudience(tx, request).Pluck("user_id", &recipients).Error; err != nil {
			return err
		}
		seen := make(map[string]bool, len(users))
		for _, id := range users {
			seen[id] = true
		}
		for _, id := range recipients {
			if !seen[id] {
				users = append(users, id)
			}
		}
	}
	return recordDocumentRequestActivity(ctx, tx, request, eventType, models.FeedEventSourceTypeDocumentRequestItem,
		itemID+":"+strconv.Itoa(round), actor, users, at)
}

func requestAudience(db *gorm.DB, request models.DocumentRequest) *gorm.DB {
	return db.Model(&models.FeedEvent{}).Where(
		"tenant_id = ? AND client_id = ? AND event_type = ? AND source_type = ? AND source_id = ?",
		request.TenantID, request.ClientID, models.FeedEventTypeRequestCreated, models.FeedEventSourceTypeDocumentRequest, request.ID,
	)
}

// AccountLink only restores the link for a named recipient. Workspace membership
// by itself does not grant access to privately addressed, or copy-only, requests.
func (s *DocumentRequestService) AccountLink(ctx context.Context, tenantID, userID, requestID string) (string, error) {
	if userID == "" || s.SigningKey == "" {
		return "", gorm.ErrRecordNotFound
	}
	db := s.DB.WithContext(ctx)
	token := s.token(tenantID, requestID)
	request, err := s.publicRow(db, tenantID, token, false)
	if err != nil {
		return "", err
	}
	var user models.User
	if err := db.First(&user, "tenant_id = ? AND id = ?", tenantID, userID).Error; err != nil {
		return "", err
	}
	allowed, err := requestActivityRecipient(ctx, db, request, user)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", gorm.ErrRecordNotFound
	}
	return token, nil
}

func requestActivityRecipient(ctx context.Context, db *gorm.DB, request models.DocumentRequest, user models.User) (bool, error) {
	allowed, err := email.DocumentRequestRecipient(ctx, db, request.ClientID, user, time.Now())
	if err != nil || !allowed {
		return false, err
	}
	var count int64
	err = requestAudience(db, request).Where("user_id = ?", user.ID).Count(&count).Error
	return count > 0, err
}

func hydrateRequestActivity(ctx context.Context, db *gorm.DB, user models.User, event *models.FeedEvent) (bool, error) {
	if event.ClientID == nil {
		return false, nil
	}
	requestID := event.SourceID
	switch event.SourceType {
	case models.FeedEventSourceTypeDocumentRequest:
		if event.EventType != models.FeedEventTypeRequestCreated {
			return false, nil
		}
	case models.FeedEventSourceTypeDocumentRequestItem:
		itemID, _, ok := strings.Cut(event.SourceID, ":")
		if !ok {
			return false, nil
		}
		var item models.DocumentRequestItem
		err := db.Joins("JOIN document_requests dr ON dr.id = document_request_items.request_id").
			Where("dr.tenant_id = ? AND dr.client_id = ? AND document_request_items.id = ?", event.TenantID, *event.ClientID, itemID).First(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		requestID, event.RequestItem = item.RequestID, item.Name
	default:
		return false, nil
	}
	var request models.DocumentRequest
	err := db.First(&request, "tenant_id = ? AND client_id = ? AND id = ?", event.TenantID, *event.ClientID, requestID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if user.Role == "admin" || user.Role == "manager" {
		err = (&DocumentRequestService{}).staff(ctx, db, AccessRequest{TenantID: event.TenantID, ClientID: request.ClientID, Actor: AccessActor{UserID: user.ID}}, false)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		event.RequestOpen = true
	} else {
		allowed, err := requestActivityRecipient(ctx, db, request, user)
		if err != nil || !allowed || event.EventType == models.FeedEventTypeRequestSubmitted {
			return false, err
		}
		if request.RevokedAt != nil || !request.ExpiresAt.After(time.Now()) {
			return false, nil
		}
		_, err = requestFolder(db, request.TenantID, request.ClientID, request.FolderID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		event.RequestOpen = true
	}
	event.RequestID, event.RequestTitle = request.ID, request.Title
	return true, nil
}
