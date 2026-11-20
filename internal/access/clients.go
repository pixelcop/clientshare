package access

import (
	"context"
	"errors"
	"time"

	"github.com/pixelcop/clientshare/internal/dbtime"
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
	var groupExplicit string
	switch op {
	case AccessContext:
		explicit = "(scope_uc.can_read OR scope_uc.can_upload)"
		groupExplicit = "(ar.can_read OR ar.can_upload)"
	case AccessRead:
		explicit = "scope_uc.can_read"
		groupExplicit = "ar.can_read"
	case AccessShare:
		explicit = "scope_uc.can_share"
		groupExplicit = "(ar.can_invite_external OR ar.can_create_links OR ar.can_manage_access)"
	case AccessInviteClient:
		groupExplicit = "(ar.can_invite_external OR ar.legacy_sharing)"
		explicit = "(scope_uc.can_share AND (scope_uc.role_id IS NULL OR EXISTS (SELECT 1 FROM access_roles ar WHERE ar.tenant_id = scope_uc.tenant_id AND ar.id = scope_uc.role_id AND (ar.legacy_sharing OR ar.can_invite_external))))"
	case AccessCreateLink:
		groupExplicit = "(ar.can_create_links OR ar.legacy_sharing)"
		explicit = "(scope_uc.can_share AND (scope_uc.role_id IS NULL OR EXISTS (SELECT 1 FROM access_roles ar WHERE ar.tenant_id = scope_uc.tenant_id AND ar.id = scope_uc.role_id AND (ar.legacy_sharing OR ar.can_create_links))))"
	case AccessManageClient, AccessManageQueue:
		if user.Role != "manager" && user.Role != "admin" {
			return deny()
		}
		explicit = "(scope_uc.can_share AND (scope_uc.role_id IS NULL OR EXISTS (SELECT 1 FROM access_roles ar WHERE ar.tenant_id = scope_uc.tenant_id AND ar.id = scope_uc.role_id AND (ar.legacy_sharing OR ar.can_manage_access))))"
		groupExplicit = "(ar.can_manage_access OR ar.legacy_sharing)"
		if op == AccessManageQueue {
			explicit = "(" + explicit + " AND scope_uc.can_read)"
			groupExplicit = "(" + groupExplicit + " AND ar.can_read)"
		}
	default:
		return deny()
	}
	if user.Role == "admin" || (user.Role == "manager" && user.ManagerFullAccess) {
		return q, nil
	}
	return q.Where(`(EXISTS (SELECT 1 FROM user_clients scope_uc
		WHERE scope_uc.tenant_id = clients.tenant_id AND scope_uc.client_id = clients.id
		AND scope_uc.user_id = ? AND (scope_uc.role_id IS NULL OR EXISTS (
		SELECT 1 FROM access_roles ar WHERE ar.tenant_id=scope_uc.tenant_id AND ar.id=scope_uc.role_id AND ar.allow_workspace=TRUE
		AND (ar.audience='both' OR ar.audience=?) AND `+groupExplicit+`))
		AND (NOT scope_uc.capabilities_set OR `+explicit+`)) OR EXISTS (
		SELECT 1 FROM role_assignments ra JOIN access_roles ar ON ar.tenant_id=ra.tenant_id AND ar.id=ra.role_id
		WHERE ra.tenant_id=clients.tenant_id AND ra.client_id=clients.id AND ra.file_id IS NULL AND ar.allow_workspace=TRUE
		AND ra.revoked_at IS NULL AND ra.source_kind NOT IN ('membership','grant') AND (ra.expires_at IS NULL OR ?)
		AND (ra.user_id = ? OR EXISTS (SELECT 1 FROM access_group_members gm WHERE gm.tenant_id=ra.tenant_id AND gm.group_id=ra.group_id AND gm.user_id=?))
		AND (ar.audience='both' OR ar.audience=?) AND `+groupExplicit+`))`, userID, userTypeForRole(user.Role), dbtime.Compare(q, "ra.expires_at", ">", time.Now()), userID, userID, userTypeForRole(user.Role)), nil
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
	assignments := map[string][]resolvedAssignment{}
	if !full {
		var err error
		assignments, err = s.assignments(ctx, tenantID, userID, userTypeForRole(user.Role), ids, time.Now())
		if err != nil {
			return nil, err
		}
	}
	memberships := map[string]models.UserClient{}
	roleByID := map[string]models.AccessRole{}
	if !full {
		var rows []models.UserClient
		if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND user_id = ? AND client_id IN ?", tenantID, userID, ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		roleIDs := []string{}
		for _, row := range rows {
			memberships[row.ClientID] = row
			if row.RoleID != nil {
				roleIDs = append(roleIDs, *row.RoleID)
			}
		}
		for start := 0; start < len(roleIDs); start += accessBatchSize {
			var roles []models.AccessRole
			if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, roleIDs[start:min(start+accessBatchSize, len(roleIDs))]).Find(&roles).Error; err != nil {
				return nil, err
			}
			for _, role := range roles {
				roleByID[role.ID] = role
			}
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
			caps = MembershipCapabilities(user.Role, membership)
			if membership.RoleID != nil {
				role, ok := roleByID[*membership.RoleID]
				if !ok {
					return nil, gorm.ErrRecordNotFound
				}
				caps = models.AccessCapabilities{}
				if role.Supports(userTypeForRole(user.Role), "workspace") {
					caps = projectRoleCapabilities(role, "workspace")
				}
			}
			staff = user.Role == "manager"
			sources = append(sources, AccessSource{Kind: "membership", ID: user.ID})
		}
		if !full {
			for _, a := range assignments[client.ID] {
				if a.assignment.FileID == nil {
					caps = unionAccess(caps, a.caps)
					sources = append(sources, assignmentSource(a.assignment))
					staff = user.Role == "manager"
				}
			}
		}
		result[client.ID] = accessDecision(caps, sources, op, "client", staff)
	}
	return result, nil
}
