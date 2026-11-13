package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/pixelcop/clientshare/internal/models"
)

type ReplayTargetOption struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	TenantID   string `json:"tenant_id,omitempty"`
	TenantName string `json:"tenant_name,omitempty"`
}

// SearchTargets is internal-only and returns a bounded directory projection,
// never credentials or the full user model.
func (s *ReplayRequestService) SearchTargets(ctx context.Context, kind, query, tenantID string) ([]ReplayTargetOption, error) {
	query = strings.TrimSpace(query)
	if len(query) > 254 || len(tenantID) > 26 {
		return nil, fmt.Errorf("%w: search is too long", ErrReplayRequest)
	}
	pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(query)) + "%"
	result := []ReplayTargetOption{}
	if kind == "tenants" {
		err := s.db.WithContext(ctx).Model(&models.Tenant{}).Select("id, name").Where("LOWER(name) LIKE ? ESCAPE '!' OR LOWER(slug) LIKE ? ESCAPE '!' OR LOWER(id) LIKE ? ESCAPE '!'", pattern, pattern, pattern).Order("name, id").Limit(20).Scan(&result).Error
		return result, err
	}
	if kind != "users" {
		return nil, fmt.Errorf("%w: unknown directory", ErrReplayRequest)
	}
	q := s.db.WithContext(ctx).Table("users AS u").Select("u.id, u.name, u.email, u.tenant_id, t.name AS tenant_name").Joins("JOIN tenants t ON t.id = u.tenant_id").Where("LOWER(u.name) LIKE ? ESCAPE '!' OR LOWER(u.email) LIKE ? ESCAPE '!' OR LOWER(u.id) LIKE ? ESCAPE '!'", pattern, pattern, pattern)
	if tenantID != "" {
		q = q.Where("u.tenant_id = ?", tenantID)
	}
	err := q.Order("u.email, u.id").Limit(20).Scan(&result).Error
	return result, err
}

func (s *ReplayRequestService) validateTargetIDs(ctx context.Context, targets []models.ReplayTarget) error {
	for _, target := range targets {
		if target.TenantID != "" {
			var count int64
			if err := s.db.WithContext(ctx).Model(&models.Tenant{}).Where("id = ?", target.TenantID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("%w: select an existing tenant", ErrReplayRequest)
			}
		}
		if target.UserID != "" {
			var count int64
			q := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", target.UserID)
			if target.TenantID != "" {
				q = q.Where("tenant_id = ?", target.TenantID)
			}
			if err := q.Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("%w: select an existing user in the selected tenant", ErrReplayRequest)
			}
		}
	}
	return nil
}
