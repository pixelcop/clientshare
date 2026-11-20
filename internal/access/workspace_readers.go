package access

import (
	"context"
	"time"

	"github.com/pixelcop/clientshare/internal/dbtime"
	"github.com/pixelcop/clientshare/internal/models"
)

// DocumentRequestRecipient reports whether a client account can use a request
// in this workspace, independently of whether it can receive email.
func (s *AccessService) DocumentRequestRecipient(ctx context.Context, clientID string, user models.User, now time.Time) (bool, error) {
	if user.Role != "client" && user.Role != "customer" {
		return false, nil
	}
	decision, err := s.Resolve(ctx, AccessRequest{
		TenantID: user.TenantID, ClientID: clientID, Actor: AccessActor{UserID: user.ID},
		Operation: AccessContext, Now: now,
	})
	return decision.Allowed && (decision.Capabilities.CanRead || decision.Capabilities.CanUpload), err
}

// WorkspaceReaderIDs selects notification candidates from whole-workspace
// sources only. Item-only recipients do not become client subscribers. Dispatch
// still rechecks each item's current authorization before rendering its name.
func (s *AccessService) WorkspaceReaderIDs(ctx context.Context, tenantID, clientID string, roles []string) ([]string, error) {
	ids := []string{}
	err := s.DB.WithContext(ctx).Model(&models.User{}).Where("users.tenant_id = ? AND users.role IN ?", tenantID, roles).
		Where(`users.role='admin' OR (users.role='manager' AND users.manager_full_access) OR EXISTS (
		SELECT 1 FROM user_clients uc WHERE uc.tenant_id=users.tenant_id AND uc.user_id=users.id AND uc.client_id=? AND (NOT uc.capabilities_set OR uc.can_read)
		AND (uc.role_id IS NULL OR EXISTS (SELECT 1 FROM access_roles ar WHERE ar.tenant_id=uc.tenant_id AND ar.id=uc.role_id AND ar.allow_workspace=TRUE AND ar.can_read=TRUE AND (ar.audience='both' OR ar.audience=CASE WHEN users.role IN ('admin','manager') THEN 'internal' ELSE 'external' END)))
		) OR EXISTS (
		SELECT 1 FROM role_assignments ra JOIN access_roles ar ON ar.tenant_id=ra.tenant_id AND ar.id=ra.role_id
		WHERE ra.tenant_id=users.tenant_id AND ra.client_id=? AND ra.file_id IS NULL AND ra.revoked_at IS NULL AND ra.source_kind NOT IN ('membership','grant')
		AND (ra.expires_at IS NULL OR ?) AND ar.allow_workspace=TRUE AND ar.can_read=TRUE
		AND (ar.audience='both' OR ar.audience=CASE WHEN users.role IN ('admin','manager') THEN 'internal' ELSE 'external' END)
		AND (ra.user_id=users.id OR EXISTS (SELECT 1 FROM access_group_members gm WHERE gm.tenant_id=ra.tenant_id AND gm.group_id=ra.group_id AND gm.user_id=users.id)))`, clientID, clientID, dbtime.Compare(s.DB, "ra.expires_at", ">", time.Now())).Order("users.id ASC").Pluck("users.id", &ids).Error
	return ids, err
}
