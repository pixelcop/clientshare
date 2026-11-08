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
		if op == AccessManageQueue {
			explicit = "(scope_uc.can_share AND scope_uc.can_read)"
		}
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

// ResolveClients decorates authorized client pages without per-row queries or
// treating an item grant as membership. It mirrors the single-client resolver.
func (s *AccessService) ResolveClients(ctx context.Context, tenantID, userID string, ids []string, op AccessOperation) (map[string]AccessDecision, error) {
	result := make(map[string]AccessDecision, len(ids))
	if len(ids) > maxAccessItems {
		return nil, errors.New("access batch exceeds 500 clients")
	}
	for _, id := range ids {
		result[id] = AccessDecision{}
	}
	if len(ids) == 0 || tenantID == "" || userID == "" {
		return result, nil
	}
	var user models.User
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, nil
		}
		return nil, err
	}
	if user.Role != "admin" && user.Role != "manager" && user.Role != "client" && user.Role != "customer" {
		return result, nil
	}
	var clients []models.Client
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, ids).Find(&clients).Error; err != nil {
		return nil, err
	}
	full := user.Role == "admin" || (user.Role == "manager" && user.ManagerFullAccess)
	memberships := map[string]models.UserClient{}
	if !full {
		var rows []models.UserClient
		if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND user_id = ? AND client_id IN ?", tenantID, userID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			memberships[row.ClientID] = row
		}
	}
	for _, client := range clients {
		caps := models.AccessCapabilities{}
		staff := full
		var sources []AccessSource
		if full {
			caps = models.AccessCapabilities{CanRead: true, CanUpload: true, CanDelete: true, CanShare: true}
			sources = append(sources, AccessSource{Kind: "staff", ID: user.ID})
		} else if membership, ok := memberships[client.ID]; ok {
			caps = membershipCapabilities(user.Role, membership)
			staff = user.Role == "manager"
			sources = append(sources, AccessSource{Kind: "membership", ID: user.ID})
		}
		result[client.ID] = accessDecision(caps, sources, op, "client", staff)
	}
	return result, nil
}
