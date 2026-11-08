package access

import (
	"context"
	"errors"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

// ClientScope is a composable query for whole-client authority. Item grants are
// deliberately absent. It reloads the actor, so supplied role strings cannot
// broaden lists, feeds, notifications or management operations.
func (s *AccessService) ClientScope(ctx context.Context, tenantID, userID string, op AccessOperation) (*gorm.DB, error) {
	q := s.DB.WithContext(ctx).Model(&models.Client{}).Where("clients.tenant_id = ?", tenantID)
	deny := func() (*gorm.DB, error) { return q.Where("1 = 0"), nil }
	if tenantID == "" || userID == "" {
		return deny()
	}
	var user models.User
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return deny()
		}
		return nil, err
	}
	if user.Role != "admin" && user.Role != "manager" && user.Role != "client" && user.Role != "customer" {
		return deny()
	}
	var explicit string
	switch op {
	case AccessContext:
		explicit = "(scope_uc.can_read OR scope_uc.can_upload)"
	case AccessRead:
		explicit = "scope_uc.can_read"
	case AccessShare, AccessInviteClient:
		explicit = "scope_uc.can_share"
	case AccessManageClient, AccessManageQueue:
		if user.Role != "manager" && user.Role != "admin" {
			return deny()
		}
		explicit = "scope_uc.can_share"
	default:
		return deny()
	}
	if user.Role == "admin" || (user.Role == "manager" && user.ManagerFullAccess) {
		return q, nil
	}
	return q.Where(`EXISTS (SELECT 1 FROM user_clients scope_uc
		WHERE scope_uc.tenant_id = clients.tenant_id AND scope_uc.client_id = clients.id
		AND scope_uc.user_id = ? AND (NOT scope_uc.capabilities_set OR `+explicit+`))`, userID), nil
}
