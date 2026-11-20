package services

import (
	"context"
	"errors"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

// visibleFeedEvents filters resources before summaries/pagination, including
// every entry of grouped uploads. Stored activity paths/counts are not current
// authorization or recipient-safe navigation context.
func (s *FeedService) visibleFeedEvents(ctx context.Context, tenantID, userID, eventID string) ([]models.FeedEvent, error) {
	visible := []models.FeedEvent{}
	var user models.User
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return visible, nil
		}
		return nil, err
	}
	q := s.feedQuery(ctx, tenantID, user.ID, user.Role)
	if q == nil {
		return visible, nil
	}
	if eventID != "" {
		q = q.Where("fe.id = ?", eventID)
	}
	var events []models.FeedEvent
	if err := q.Order("fe.created_at DESC, fe.id DESC").Scan(&events).Error; err != nil {
		return nil, err
	}
	batchIDs := []string{}
	seenBatch := map[string]bool{}
	for _, event := range events {
		if event.SourceType == models.FeedEventSourceTypeFileUploadBatch && !seenBatch[event.SourceID] {
			batchIDs = append(batchIDs, event.SourceID)
			seenBatch[event.SourceID] = true
		}
	}
	batchFiles := map[string][]string{}
	for start := 0; start < len(batchIDs); start += maxAccessItems {
		var activities []models.FileActivity
		if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND upload_batch_id IN ? AND action = 'upload'", tenantID, batchIDs[start:min(start+maxAccessItems, len(batchIDs))]).Order("created_at ASC, id ASC").Find(&activities).Error; err != nil {
			return nil, err
		}
		for _, activity := range activities {
			if activity.UploadBatchID != nil && activity.FileID != nil {
				key := activity.ClientID + ":" + *activity.UploadBatchID
				batchFiles[key] = append(batchFiles[key], *activity.FileID)
			}
		}
	}
	eventFiles := map[string][]string{}
	clientFiles := map[string][]string{}
	seenFile := map[string]bool{}
	for _, event := range events {
		if event.EventType != models.FeedEventTypeFileUploaded || event.ClientID == nil {
			continue
		}
		ids := []string{}
		if event.SourceType == models.FeedEventSourceTypeFileUploadBatch {
			ids = batchFiles[*event.ClientID+":"+event.SourceID]
		} else if event.SourceType == models.FeedEventSourceTypeFileActivity && event.FileID != nil {
			ids = []string{*event.FileID}
		}
		eventFiles[event.ID] = ids
		for _, id := range ids {
			key := *event.ClientID + ":" + id
			if !seenFile[key] {
				clientFiles[*event.ClientID] = append(clientFiles[*event.ClientID], id)
				seenFile[key] = true
			}
		}
	}
	resolver := &AccessService{DB: s.DB}
	readable := map[string]models.File{}
	for clientID, ids := range clientFiles {
		for start := 0; start < len(ids); start += maxAccessItems {
			batch := ids[start:min(start+maxAccessItems, len(ids))]
			decisions, err := resolver.ResolveItems(ctx, AccessRequest{TenantID: tenantID, ClientID: clientID, Actor: AccessActor{UserID: user.ID}, Operation: AccessRead}, batch)
			if err != nil {
				return nil, err
			}
			var files []models.File
			if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND id IN ? AND type = 'file'", tenantID, clientID, batch).Find(&files).Error; err != nil {
				return nil, err
			}
			for _, file := range files {
				if decisions[file.ID].Allowed {
					readable[clientID+":"+file.ID] = file
				}
			}
		}
	}
	scope, err := resolver.ClientScope(ctx, tenantID, user.ID, AccessRead)
	if err != nil {
		return nil, err
	}
	var clientIDs []string
	if err := scope.Pluck("clients.id", &clientIDs).Error; err != nil {
		return nil, err
	}
	wholeClient := map[string]bool{}
	for _, id := range clientIDs {
		wholeClient[id] = true
	}
	for _, event := range events {
		switch event.EventType {
		case models.FeedEventTypeRequestCreated, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestAccepted, models.FeedEventTypeRequestReplacement:
			ok, err := hydrateRequestActivity(ctx, s.DB.WithContext(ctx), user, &event)
			if err != nil {
				return nil, err
			}
			if ok {
				visible = append(visible, event)
			}
			continue
		case models.FeedEventTypeFileComment:
			ok, err := hydrateCommentActivity(ctx, s.DB.WithContext(ctx), user, &event)
			if err != nil {
				return nil, err
			}
			if ok {
				if !wholeClient[*event.ClientID] {
					event.ClientName, event.ActorName, event.ActorEmail = "", "", ""
					event.ActorUserID = nil
				}
				visible = append(visible, event)
			}
			continue
		}
		if event.EventType == models.FeedEventTypeInviteAccepted {
			if user.Role == "admin" {
				visible = append(visible, event)
			}
			continue
		}
		if event.EventType != models.FeedEventTypeFileUploaded || event.ClientID == nil {
			continue
		}
		files := []models.FeedEventFile{}
		seen := map[string]bool{}
		for _, id := range eventFiles[event.ID] {
			if file, ok := readable[*event.ClientID+":"+id]; ok && !seen[id] {
				files = append(files, models.FeedEventFile{ID: file.ID, Path: file.Filename})
				seen[id] = true
			}
		}
		if len(files) == 0 {
			continue
		}
		event.FileID, event.FilePath = &files[0].ID, files[0].Path
		if event.SourceType == models.FeedEventSourceTypeFileUploadBatch {
			event.Files, event.FileCount = files, len(files)
		}
		if !wholeClient[*event.ClientID] {
			event.ClientName, event.ActorName, event.ActorEmail, event.SubjectName, event.SubjectEmail = "", "", "", "", ""
			event.ActorUserID, event.SubjectUserID = nil, nil
		}
		visible = append(visible, event)
	}
	return visible, nil
}
