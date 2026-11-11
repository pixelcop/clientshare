package access

import (
	"context"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
)

type resolvedAssignment struct {
	assignment models.RoleAssignment
	caps       models.AccessCapabilities
}

func (s *AccessService) assignments(ctx context.Context, tenantID, userID, userType string, clientIDs []string, now time.Time) (map[string][]resolvedAssignment, error) {
	result := map[string][]resolvedAssignment{}
	var assignments []models.RoleAssignment
	err := s.DB.WithContext(ctx).Where("tenant_id = ? AND client_id IN ? AND revoked_at IS NULL AND source_kind NOT IN ('membership','grant')", tenantID, clientIDs).
		Where("user_id = ? OR EXISTS (SELECT 1 FROM access_group_members gm WHERE gm.tenant_id = role_assignments.tenant_id AND gm.group_id = role_assignments.group_id AND gm.user_id = ?)", userID, userID).Find(&assignments).Error
	if err != nil {
		return nil, err
	}
	roleIDs := []string{}
	seenRoles := map[string]bool{}
	fileIDs := []string{}
	for _, a := range assignments {
		if a.FileID != nil {
			fileIDs = append(fileIDs, *a.FileID)
		}
	}
	targetTypes := map[string]string{}
	for start := 0; start < len(fileIDs); start += accessBatchSize {
		var files []models.File
		if err := s.DB.WithContext(ctx).Select("id,client_id,type").Where("tenant_id = ? AND client_id IN ? AND id IN ?", tenantID, clientIDs, fileIDs[start:min(start+accessBatchSize, len(fileIDs))]).Find(&files).Error; err != nil {
			return nil, err
		}
		for _, f := range files {
			targetTypes[f.ClientID+"/"+f.ID] = f.Type
		}
	}
	for _, a := range assignments {
		if !seenRoles[a.RoleID] {
			seenRoles[a.RoleID] = true
			roleIDs = append(roleIDs, a.RoleID)
		}
	}
	if len(roleIDs) == 0 {
		return result, nil
	}
	byID := map[string]models.AccessRole{}
	for start := 0; start < len(roleIDs); start += accessBatchSize {
		var roles []models.AccessRole
		if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, roleIDs[start:min(start+accessBatchSize, len(roleIDs))]).Find(&roles).Error; err != nil {
			return nil, err
		}
		for _, r := range roles {
			byID[r.ID] = r
		}
	}
	for _, a := range assignments {
		if a.ExpiresAt != nil && !a.ExpiresAt.After(now) {
			continue
		}
		r, ok := byID[a.RoleID]
		if !ok {
			continue
		}
		// Target validity/ancestry is checked by ResolveItems. Workspace sources
		// never originate from an item grant or an item-scoped link.
		scope := "workspace"
		if a.FileID != nil {
			scope = targetTypes[a.ClientID+"/"+*a.FileID]
		}
		if !r.Supports(userType, scope) {
			continue
		}
		p, ok := r.PermissionsAt(scope)
		if !ok {
			continue
		}
		caps := p.Capabilities()
		if r.LegacySharing {
			caps.Sharing = nil
		}
		result[a.ClientID] = append(result[a.ClientID], resolvedAssignment{a, caps})
	}
	return result, nil
}

func assignmentSource(a models.RoleAssignment) AccessSource {
	kind := "role_assignment"
	if a.GroupID != nil {
		kind = "group"
	}
	source := AccessSource{Kind: kind, ID: a.ID}
	if a.FileID != nil {
		source.FileID = *a.FileID
	}
	return source
}
