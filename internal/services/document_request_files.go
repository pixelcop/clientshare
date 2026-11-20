package services

import (
	"context"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

const requestFileSubmittedAt = "COALESCE(drr.submitted_at, CASE WHEN dri.round = drf.round THEN dri.submitted_at END)"

func requestFilesWithSubmission(db *gorm.DB, row models.DocumentRequest) *gorm.DB {
	return liveRequestFiles(db, row).
		Joins("JOIN document_request_items dri ON dri.id = drf.item_id AND dri.request_id = ?", row.ID).
		Joins("LEFT JOIN document_request_reviews drr ON drr.item_id = drf.item_id AND drr.round = drf.round").
		Where("f.type = 'file'")
}

// RecipientFile revalidates the request and proves exact submitted-file
// membership. It never grants access through the destination folder.
func (s *DocumentRequestService) RecipientFile(ctx context.Context, tenantID, token, fileID string) (*models.File, error) {
	file, _, err := s.recipientFile(s.DB.WithContext(ctx), tenantID, token, fileID)
	return file, err
}

func (s *DocumentRequestService) recipientFile(db *gorm.DB, tenantID, token, fileID string) (*models.File, models.DocumentRequest, error) {
	row, err := s.publicRow(db, tenantID, token, false)
	if err != nil {
		return nil, row, err
	}
	var result struct {
		models.File
		RequestName string
	}
	err = requestFilesWithSubmission(db, row).Where("drf.file_id = ? AND "+requestFileSubmittedAt+" IS NOT NULL", fileID).
		Select("f.*, drf.name AS request_name").Take(&result).Error
	result.Filename = result.RequestName
	return &result.File, row, err
}

func (s *DocumentRequestService) RecipientComments(token, baseURL string) *FileCommentService {
	return &FileCommentService{DB: s.DB, NotificationBaseURL: baseURL, requestGuest: func(db *gorm.DB, req AccessRequest) (models.User, error) {
		_, row, err := s.recipientFile(db, req.TenantID, token, req.FileID)
		if err != nil {
			return models.User{}, err
		}
		// A reserved, non-user author ID scopes idempotency to this bearer
		// audience without creating an account or impersonating a portal user.
		return models.User{TenantID: row.TenantID, ID: "~" + row.ID[1:]}, nil
	}}
}
