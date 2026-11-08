package services

import (
	"context"

	"github.com/pixelcop/clientshare/internal/models"
)

func (s *SharingService) SharedWithMe(ctx context.Context, req AccessRequest, page, pageSize int) (*models.PaginatedResult[models.File], error) {
	req.Now = s.now()
	files, err := (&AccessService{DB: s.DB}).SharedRoots(ctx, req)
	if err != nil {
		return nil, err
	}
	page, pageSize = normalizePage(page), normalizePageSize(pageSize)
	pageSize = min(pageSize, 100)
	total := len(files)
	pages := (total + pageSize - 1) / pageSize
	// Clamp before multiplying so hostile page values cannot overflow offsets.
	start := total
	if page <= pages {
		start = (page - 1) * pageSize
	}
	return &models.PaginatedResult[models.File]{
		Pagination: models.PaginationResponse{Page: page, PageSize: pageSize, TotalItems: int64(total), TotalPages: pages, HasMore: page < pages, ItemsPerPage: min(pageSize, total-start)},
		Items:      files[start:min(start+pageSize, total)],
	}, nil
}
