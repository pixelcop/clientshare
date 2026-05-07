package models

import (
	"time"

	"gorm.io/cli/gorm/genconfig"
)

var _ = genconfig.Config{
	OutPath:        "./internal/db",
	ExcludeStructs: []any{"Pagination*", "Paginated*"},
}

// PaginationResponse represents pagination metadata
type PaginationResponse struct {
	TotalItems   int64 `json:"total_items"`
	TotalPages   int   `json:"total_pages"`
	Page         int   `json:"page"`
	PageSize     int   `json:"page_size"`
	HasMore      bool  `json:"has_more"`
	ItemsPerPage int   `json:"items_per_page"`
}

// PaginatedResult represents a paginated list of items
type PaginatedResult[T any] struct {
	Pagination PaginationResponse `json:"pagination"`
	Items      []T                `json:"items"`
}

func Now() *time.Time {
	t := time.Now()
	return &t
}
