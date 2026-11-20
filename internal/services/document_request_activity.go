package services

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
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
	users, err := requestActivityAudience(ctx, tx, request, true)
	if err != nil {
		return err
	}
	return recordDocumentRequestActivity(ctx, tx, request, eventType, models.FeedEventSourceTypeDocumentRequestItem,
		itemID+":"+strconv.Itoa(round), actor, users, at)
}

func requestActivityAudience(ctx context.Context, tx *gorm.DB, request models.DocumentRequest, includeClients bool) ([]string, error) {
	users, err := (&AccessService{DB: tx}).WorkspaceReaderIDs(ctx, request.TenantID, request.ClientID, []string{"admin", "manager"})
	if err != nil || !includeClients {
		return users, err
	}
	var recipients []string
	currentClients := tx.Model(&models.User{}).Select("id").Where("tenant_id = ? AND role IN ?", request.TenantID, []string{"client", "customer"})
	if err := requestAudience(tx, request).Where("user_id IN (?)", currentClients).Pluck("user_id", &recipients).Error; err != nil {
		return nil, err
	}
	return append(users, recipients...), nil
}

// Creation events for client recipients also retain the named audience.
// Keep staff events separate so staff access never acts as a recipient grant.
func requestAudience(db *gorm.DB, request models.DocumentRequest) *gorm.DB {
	return db.Model(&models.FeedEvent{}).Where(
		"tenant_id = ? AND client_id = ? AND event_type = ? AND source_type = ? AND source_id = ?",
		request.TenantID, request.ClientID, models.FeedEventTypeRequestCreated, models.FeedEventSourceTypeDocumentRequest, request.ID,
	)
}

type AccountDocumentRequest struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	ClientName  string `json:"client_name"`
	NeedsAction int    `json:"needs_action"`
	UnderReview int    `json:"under_review"`
}

func (s *DocumentRequestService) AccountRequests(ctx context.Context, tenantID, userID string) ([]AccountDocumentRequest, error) {
	db := s.DB.WithContext(ctx)
	var user models.User
	if err := db.First(&user, "tenant_id = ? AND id = ? AND role IN ?", tenantID, userID, []string{"client", "customer"}).Error; err != nil {
		return nil, err
	}
	var candidates []struct {
		AccountDocumentRequest
		ClientID  string
		ExpiresAt time.Time
	}
	err := db.Table("document_requests AS dr").
		Select(`dr.id, dr.title, dr.client_id, dr.expires_at, c.name AS client_name,
			(SELECT COUNT(*) FROM document_request_items i WHERE i.request_id = dr.id AND i.status IN ?) AS needs_action,
			(SELECT COUNT(*) FROM document_request_items i WHERE i.request_id = dr.id AND i.status = ?) AS under_review`,
			[]string{requestNeeded, requestReplacement}, requestSubmitted).
		Joins("JOIN clients c ON c.id = dr.client_id AND c.tenant_id = dr.tenant_id").
		Joins("JOIN files f ON f.id = dr.folder_id AND f.tenant_id = dr.tenant_id AND f.client_id = dr.client_id AND f.type = 'folder' AND f.deleted_at IS NULL AND f.disk_deleted_at IS NULL").
		Where("dr.tenant_id = ? AND dr.revoked_at IS NULL", tenantID).
		Where(`EXISTS (SELECT 1 FROM feed_events fe WHERE fe.tenant_id = dr.tenant_id AND fe.client_id = dr.client_id AND fe.source_id = dr.id
			AND fe.user_id = ? AND fe.event_type = ? AND fe.source_type = ?)`, user.ID, models.FeedEventTypeRequestCreated, models.FeedEventSourceTypeDocumentRequest).
		Order("dr.created_at DESC, dr.id DESC").Scan(&candidates).Error
	if err != nil {
		return nil, err
	}
	results := make([]AccountDocumentRequest, 0, len(candidates))
	now := time.Now()
	allowedByClient := make(map[string]bool)
	for _, candidate := range candidates {
		if !candidate.ExpiresAt.After(now) {
			continue
		}
		allowed, checked := allowedByClient[candidate.ClientID]
		if !checked {
			allowed, err = (&AccessService{DB: db}).DocumentRequestRecipient(ctx, candidate.ClientID, user, now)
			if err != nil {
				return nil, err
			}
			allowedByClient[candidate.ClientID] = allowed
		}
		if allowed {
			results = append(results, candidate.AccountDocumentRequest)
		}
	}
	return results, nil
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
	allowed, err := (&AccessService{DB: db}).DocumentRequestRecipient(ctx, request.ClientID, user, time.Now())
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
	case models.FeedEventSourceTypeDocumentRequest, models.FeedEventSourceTypeDocumentRequestStaff:
		if event.EventType != models.FeedEventTypeRequestCreated {
			return false, nil
		}
	case models.FeedEventSourceTypeDocumentRequestUpdate:
		if event.EventType != models.FeedEventTypeRequestUpdated {
			return false, nil
		}
		var update models.DocumentRequestUpdate
		err := db.Select("document_request_updates.request_id").Joins("JOIN document_requests dr ON dr.id = document_request_updates.request_id").
			Where("dr.tenant_id = ? AND dr.client_id = ? AND document_request_updates.id = ?", event.TenantID, *event.ClientID, event.SourceID).First(&update).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		requestID = update.RequestID
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
		decision, err := (&AccessService{DB: db}).Resolve(ctx, AccessRequest{TenantID: event.TenantID, ClientID: request.ClientID, Actor: AccessActor{UserID: user.ID}, Operation: AccessRead})
		if err != nil {
			return false, err
		}
		if !decision.Allowed {
			return false, nil
		}
		event.RequestOpen = decision.CanManageClient
	} else {
		allowed, err := requestActivityRecipient(ctx, db, request, user)
		if err != nil || !allowed {
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
