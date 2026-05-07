package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FeedService struct {
	DB *gorm.DB
}

func NewFeedService(db *gorm.DB) *FeedService {
	return &FeedService{DB: db}
}

func (s *FeedService) RecordFileUploaded(ctx context.Context, activity *models.FileActivity) error {
	if activity == nil {
		return errors.New("file activity is required")
	}
	if strings.TrimSpace(activity.ID) == "" {
		return errors.New("file activity id is required")
	}
	tenantID, err := requireTenantID(activity.TenantID)
	if err != nil {
		return err
	}
	userIDs, err := s.usersForFileUpload(ctx, tenantID, activity.ClientID)
	if err != nil {
		return err
	}
	return s.createEvents(ctx, userIDs, func(userID string) models.FeedEvent {
		clientID := activity.ClientID
		return models.FeedEvent{
			TenantID:    tenantID,
			UserID:      userID,
			EventType:   models.FeedEventTypeFileUploaded,
			ClientID:    &clientID,
			ActorUserID: activity.UserID,
			SourceType:  models.FeedEventSourceTypeFileActivity,
			SourceID:    activity.ID,
			FileID:      activity.FileID,
			FilePath:    activity.FilePath,
			CreatedAt:   activity.CreatedAt,
		}
	})
}

func (s *FeedService) RecordFilesUploaded(ctx context.Context, activities []*models.FileActivity) error {
	if len(activities) == 0 {
		return errors.New("file activities are required")
	}

	firstActivity := activities[0]
	if firstActivity == nil {
		return errors.New("file activity is required")
	}
	batchID := ""
	if firstActivity.UploadBatchID != nil {
		batchID = strings.TrimSpace(*firstActivity.UploadBatchID)
	}
	if batchID == "" {
		return errors.New("upload batch id is required")
	}
	tenantID, err := requireTenantID(firstActivity.TenantID)
	if err != nil {
		return err
	}
	clientID := strings.TrimSpace(firstActivity.ClientID)
	if clientID == "" {
		return errors.New("client id is required")
	}

	createdAt := firstActivity.CreatedAt
	for _, activity := range activities {
		if activity == nil {
			return errors.New("file activity is required")
		}
		if strings.TrimSpace(activity.ID) == "" {
			return errors.New("file activity id is required")
		}
		if strings.TrimSpace(activity.ClientID) != clientID {
			return errors.New("file activities must share a client id")
		}
		activityBatchID := ""
		if activity.UploadBatchID != nil {
			activityBatchID = strings.TrimSpace(*activity.UploadBatchID)
		}
		if activityBatchID != batchID {
			return errors.New("file activities must share an upload batch id")
		}
		if strings.TrimSpace(activity.TenantID) != tenantID {
			return errors.New("file activities must share a tenant id")
		}
		if activity.CreatedAt.After(createdAt) {
			createdAt = activity.CreatedAt
		}
	}

	userIDs, err := s.usersForFileUpload(ctx, tenantID, clientID)
	if err != nil {
		return err
	}

	return s.createEvents(ctx, userIDs, func(userID string) models.FeedEvent {
		return models.FeedEvent{
			TenantID:    tenantID,
			UserID:      userID,
			EventType:   models.FeedEventTypeFileUploaded,
			ClientID:    &clientID,
			ActorUserID: firstActivity.UserID,
			SourceType:  models.FeedEventSourceTypeFileUploadBatch,
			SourceID:    batchID,
			FileID:      firstActivity.FileID,
			FilePath:    firstActivity.FilePath,
			CreatedAt:   createdAt,
		}
	})
}

func (s *FeedService) RecordInviteAccepted(ctx context.Context, tenantID, inviteID, userID string, createdAt time.Time) error {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return err
	}
	inviteID = strings.TrimSpace(inviteID)
	userID = strings.TrimSpace(userID)
	if inviteID == "" {
		return errors.New("invite id is required")
	}
	if userID == "" {
		return errors.New("user id is required")
	}
	userIDs, err := s.usersForInviteAccepted(ctx, tenantID)
	if err != nil {
		return err
	}
	return s.createEvents(ctx, userIDs, func(feedUserID string) models.FeedEvent {
		actorUserID := userID
		subjectUserID := userID
		return models.FeedEvent{
			TenantID:      tenantID,
			UserID:        feedUserID,
			EventType:     models.FeedEventTypeInviteAccepted,
			ActorUserID:   &actorUserID,
			SubjectUserID: &subjectUserID,
			SourceType:    models.FeedEventSourceTypeInviteToken,
			SourceID:      inviteID,
			FilePath:      "",
			CreatedAt:     createdAt,
		}
	})
}

func (s *FeedService) ListFeed(ctx context.Context, tenantID, userID, role string, page, pageSize int) (*models.FeedListResponse, error) {
	return s.ListFeedByState(ctx, tenantID, userID, role, models.FeedStateUnread, page, pageSize)
}

func (s *FeedService) ListFeedByState(ctx context.Context, tenantID, userID, role, state string, page, pageSize int) (*models.FeedListResponse, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	page = normalizePage(page)
	pageSize = normalizePageSize(pageSize)
	state = normalizeFeedState(state)
	summary, err := s.feedSummary(ctx, tenantID, userID, role)
	if err != nil {
		return nil, err
	}

	q := s.feedQuery(ctx, tenantID, userID, role)
	if q == nil {
		return &models.FeedListResponse{
			Pagination: models.PaginationResponse{
				TotalItems:   0,
				TotalPages:   0,
				Page:         page,
				PageSize:     pageSize,
				HasMore:      false,
				ItemsPerPage: 0,
			},
			Items:   []models.FeedEvent{},
			Summary: summary,
		}, nil
	}
	q = applyFeedState(q, state)

	var totalItems int64
	if err := q.Count(&totalItems).Error; err != nil {
		return nil, err
	}

	totalPages := 0
	if totalItems > 0 {
		totalPages = int(math.Ceil(float64(totalItems) / float64(pageSize)))
		if page > totalPages {
			page = totalPages
		}
	}

	items := []models.FeedEvent{}
	offset := (page - 1) * pageSize
	if err := q.Order("fe.created_at DESC, fe.id DESC").Limit(pageSize).Offset(offset).Scan(&items).Error; err != nil {
		return nil, err
	}
	if err := s.hydrateGroupedUploadFiles(ctx, tenantID, items); err != nil {
		return nil, err
	}

	return &models.FeedListResponse{
		Pagination: models.PaginationResponse{
			TotalItems:   totalItems,
			TotalPages:   totalPages,
			Page:         page,
			PageSize:     pageSize,
			HasMore:      page < totalPages,
			ItemsPerPage: len(items),
		},
		Items:   items,
		Summary: summary,
	}, nil
}

func (s *FeedService) MarkRead(ctx context.Context, tenantID, userID, role, eventID string) error {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return err
	}
	userID = strings.TrimSpace(userID)
	role = strings.TrimSpace(role)
	eventID = strings.TrimSpace(eventID)
	if userID == "" {
		return errors.New("user id is required")
	}
	if eventID == "" {
		return errors.New("event id is required")
	}
	if role != "admin" && role != "manager" && role != "client" && role != "customer" {
		return gorm.ErrRecordNotFound
	}

	q := s.feedQuery(ctx, tenantID, userID, role)
	if q == nil {
		return gorm.ErrRecordNotFound
	}

	readAt := time.Now().UTC()
	result := s.DB.WithContext(ctx).Model(&models.FeedEvent{}).Where("tenant_id = ? AND id = ? AND user_id = ?", tenantID, eventID, userID).Updates(map[string]any{
		"is_read": true,
		"read_at": readAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *FeedService) MarkAllRead(ctx context.Context, tenantID, userID, role string) error {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return err
	}
	userID = strings.TrimSpace(userID)
	role = strings.TrimSpace(role)
	if userID == "" {
		return errors.New("user id is required")
	}
	if role != "admin" && role != "manager" && role != "client" && role != "customer" {
		return gorm.ErrRecordNotFound
	}

	readAt := time.Now().UTC()
	return s.DB.WithContext(ctx).Model(&models.FeedEvent{}).
		Where("tenant_id = ? AND user_id = ? AND is_read = ?", tenantID, userID, false).
		Where("actor_user_id IS NULL OR actor_user_id <> user_id").
		Updates(map[string]any{
			"is_read": true,
			"read_at": readAt,
		}).Error
}

func (s *FeedService) createEventWithDB(ctx context.Context, db *gorm.DB, event *models.FeedEvent) error {
	if event == nil {
		return errors.New("feed event is required")
	}
	if event.ActorUserID != nil {
		actorUserID := strings.TrimSpace(*event.ActorUserID)
		feedUserID := strings.TrimSpace(event.UserID)
		if actorUserID != "" && feedUserID != "" && actorUserID == feedUserID {
			return nil
		}
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}, {Name: "event_type"}, {Name: "source_type"}, {Name: "source_id"}},
		DoNothing: true,
	}).Create(event).Error
}

func (s *FeedService) feedQuery(ctx context.Context, tenantID, userID, role string) *gorm.DB {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	role = strings.TrimSpace(role)
	if role != "admin" && role != "manager" && role != "client" && role != "customer" {
		return nil
	}

	q := s.DB.WithContext(ctx).Table("feed_events AS fe").
		Select(strings.Join([]string{
			"fe.tenant_id",
			"fe.id",
			"fe.user_id",
			"fe.event_type",
			"fe.client_id",
			"fe.actor_user_id",
			"fe.subject_user_id",
			"fe.source_type",
			"fe.source_id",
			"fe.file_id",
			"fe.file_path",
			"fe.created_at",
			"fe.is_read",
			"fe.read_at",
			"clients.name AS client_name",
			"actor.name AS actor_name",
			"actor.email AS actor_email",
			"subject.name AS subject_name",
			"subject.email AS subject_email",
		}, ", ")).
		Joins("LEFT JOIN clients ON clients.tenant_id = fe.tenant_id AND clients.id = fe.client_id").
		Joins("LEFT JOIN users actor ON actor.tenant_id = fe.tenant_id AND actor.id = fe.actor_user_id").
		Joins("LEFT JOIN users subject ON subject.tenant_id = fe.tenant_id AND subject.id = fe.subject_user_id").
		Where("fe.tenant_id = ?", tenantID).
		Where("fe.user_id = ?", userID).
		Where("fe.actor_user_id IS NULL OR fe.actor_user_id <> fe.user_id")

	return q
}

func (s *FeedService) feedSummary(ctx context.Context, tenantID, userID, role string) (models.FeedSummary, error) {
	unreadCount, err := s.countFeedItems(ctx, tenantID, userID, role, models.FeedStateUnread)
	if err != nil {
		return models.FeedSummary{}, err
	}
	readCount, err := s.countFeedItems(ctx, tenantID, userID, role, models.FeedStateRead)
	if err != nil {
		return models.FeedSummary{}, err
	}
	return models.FeedSummary{
		UnreadCount: unreadCount,
		ReadCount:   readCount,
	}, nil
}

func (s *FeedService) countFeedItems(ctx context.Context, tenantID, userID, role, state string) (int64, error) {
	q := s.feedQuery(ctx, tenantID, userID, role)
	if q == nil {
		return 0, nil
	}
	q = applyFeedState(q, state)
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func applyFeedState(q *gorm.DB, state string) *gorm.DB {
	switch normalizeFeedState(state) {
	case models.FeedStateRead:
		return q.Where("fe.is_read = ?", true)
	case models.FeedStateAll:
		return q
	default:
		return q.Where("fe.is_read = ?", false)
	}
}

func (s *FeedService) createEvents(ctx context.Context, userIDs []string, build func(userID string) models.FeedEvent) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, userID := range userIDs {
			event := build(userID)
			if err := s.createEventWithDB(ctx, tx, &event); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *FeedService) usersForFileUpload(ctx context.Context, tenantID, clientID string) ([]string, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, errors.New("client id is required")
	}

	ids := make([]string, 0)
	seen := make(map[string]struct{})
	appendUnique := func(values []string) {
		for _, id := range values {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	adminIDs, err := s.userIDsByRole(ctx, tenantID, "admin")
	if err != nil {
		return nil, err
	}
	appendUnique(adminIDs)

	managerIDs, err := s.userIDsByRole(ctx, tenantID, "manager")
	if err != nil {
		return nil, err
	}
	appendUnique(managerIDs)

	clientIDs, err := s.assignedUserIDs(ctx, tenantID, clientID)
	if err != nil {
		return nil, err
	}
	appendUnique(clientIDs)

	return ids, nil
}

func (s *FeedService) usersForInviteAccepted(ctx context.Context, tenantID string) ([]string, error) {
	return s.userIDsByRole(ctx, tenantID, "admin")
}

func (s *FeedService) userIDsByRole(ctx context.Context, tenantID, role string) ([]string, error) {
	var userIDs []string
	if err := s.DB.WithContext(ctx).Table("users").Where("tenant_id = ? AND role = ?", tenantID, role).Order("id ASC").Pluck("id", &userIDs).Error; err != nil {
		return nil, fmt.Errorf("list %s users: %w", role, err)
	}
	return userIDs, nil
}

func (s *FeedService) assignedUserIDs(ctx context.Context, tenantID, clientID string) ([]string, error) {
	var userIDs []string
	if err := s.DB.WithContext(ctx).Table("user_clients AS uc").
		Select("uc.user_id").
		Joins("JOIN users ON users.tenant_id = uc.tenant_id AND users.id = uc.user_id").
		Where("uc.tenant_id = ? AND uc.client_id = ?", tenantID, clientID).
		Where("users.role IN ?", []string{"client", "customer"}).
		Order("uc.user_id ASC").
		Pluck("uc.user_id", &userIDs).Error; err != nil {
		return nil, fmt.Errorf("list assigned users: %w", err)
	}
	return userIDs, nil
}

func normalizeFeedState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case models.FeedStateRead:
		return models.FeedStateRead
	case models.FeedStateAll:
		return models.FeedStateAll
	default:
		return models.FeedStateUnread
	}
}

func (s *FeedService) hydrateGroupedUploadFiles(ctx context.Context, tenantID string, items []models.FeedEvent) error {
	batchIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range items {
		if item.SourceType != models.FeedEventSourceTypeFileUploadBatch {
			continue
		}
		batchID := strings.TrimSpace(item.SourceID)
		if batchID == "" {
			continue
		}
		if _, ok := seen[batchID]; ok {
			continue
		}
		seen[batchID] = struct{}{}
		batchIDs = append(batchIDs, batchID)
	}
	if len(batchIDs) == 0 {
		return nil
	}

	activities := make([]models.FileActivity, 0)
	if err := s.DB.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Where("upload_batch_id IN ?", batchIDs).
		Where("action = ?", "upload").
		Order("created_at ASC, id ASC").
		Find(&activities).Error; err != nil {
		return err
	}

	filesByBatch := make(map[string][]models.FeedEventFile, len(batchIDs))
	for _, activity := range activities {
		if activity.UploadBatchID == nil {
			continue
		}
		batchID := strings.TrimSpace(*activity.UploadBatchID)
		if batchID == "" {
			continue
		}
		fileID := ""
		if activity.FileID != nil {
			fileID = strings.TrimSpace(*activity.FileID)
		}
		filesByBatch[batchID] = append(filesByBatch[batchID], models.FeedEventFile{
			ID:   fileID,
			Path: activity.FilePath,
		})
	}

	for i := range items {
		if items[i].SourceType != models.FeedEventSourceTypeFileUploadBatch {
			continue
		}

		files := filesByBatch[strings.TrimSpace(items[i].SourceID)]
		if len(files) == 0 {
			if strings.TrimSpace(items[i].FilePath) == "" {
				continue
			}
			fallbackID := ""
			if items[i].FileID != nil {
				fallbackID = strings.TrimSpace(*items[i].FileID)
			}
			items[i].Files = []models.FeedEventFile{{ID: fallbackID, Path: items[i].FilePath}}
			items[i].FileCount = 1
			continue
		}

		items[i].Files = files
		items[i].FileCount = len(files)
		if items[i].FileID == nil && files[0].ID != "" {
			firstFileID := files[0].ID
			items[i].FileID = &firstFileID
		}
		if strings.TrimSpace(items[i].FilePath) == "" {
			items[i].FilePath = files[0].Path
		}
	}

	return nil
}
