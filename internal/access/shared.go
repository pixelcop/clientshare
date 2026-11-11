package access

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
)

// SharedRoots returns explicit, currently usable named-share entry points, not
// client memberships or a client-wide directory. Resolve before deduplication
// and pagination: expired/deleted/corrupt targets must not affect visible totals.
// A stronger child grant remains discoverable even under another shared root.
func (s *AccessService) SharedRoots(ctx context.Context, req AccessRequest) ([]models.File, error) {
	visible := []models.File{}
	if req.TenantID == "" || req.Actor.UserID == "" || req.Actor.LinkID != "" {
		return visible, nil
	}
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}
	var grants []models.ItemGrant
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND user_id = ? AND revoked_at IS NULL", req.TenantID, req.Actor.UserID).Order("client_id ASC, file_id ASC").Find(&grants).Error; err != nil {
		return nil, err
	}
	clients := map[string][]string{}
	for _, grant := range grants {
		if grant.ExpiresAt != nil && !grant.ExpiresAt.After(req.Now) {
			continue
		}
		clients[grant.ClientID] = append(clients[grant.ClientID], grant.FileID)
	}
	var assignments []models.RoleAssignment
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND file_id IS NOT NULL AND revoked_at IS NULL AND source_kind NOT IN ('membership','grant')", req.TenantID).
		Where("user_id = ? OR EXISTS (SELECT 1 FROM access_group_members gm WHERE gm.tenant_id=role_assignments.tenant_id AND gm.group_id=role_assignments.group_id AND gm.user_id=?)", req.Actor.UserID, req.Actor.UserID).Find(&assignments).Error; err != nil {
		return nil, err
	}
	for _, a := range assignments {
		if a.ExpiresAt == nil || a.ExpiresAt.After(req.Now) {
			clients[a.ClientID] = append(clients[a.ClientID], *a.FileID)
		}
	}
	for clientID, ids := range clients {
		unique := []string{}
		seen := map[string]bool{}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				unique = append(unique, id)
			}
		}
		ids = unique
		req.ClientID, req.Operation = clientID, AccessContext
		nodes := map[string]models.File{}
		decisions := map[string]AccessDecision{}
		for start := 0; start < len(ids); start += maxAccessItems {
			batch := ids[start:min(start+maxAccessItems, len(ids))]
			resolved, err := s.ResolveItems(ctx, req, batch)
			if err != nil {
				return nil, err
			}
			ancestry, err := s.loadAccessAncestry(ctx, req, batch)
			if err != nil {
				return nil, err
			}
			for id, decision := range resolved {
				decisions[id] = decision
			}
			for id, file := range ancestry {
				nodes[id] = file
			}
		}
		for _, id := range ids {
			d := decisions[id]
			if !d.Allowed {
				continue
			}
			chain, valid := accessChain(nodes, id)
			if !valid {
				continue
			}
			covered := false
			for _, parent := range chain[1:] {
				ancestor := decisions[parent.ID]
				if ancestor.Allowed && capabilitiesCover(ancestor.Capabilities, d.Capabilities) {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
			file := nodes[id]
			// Discovery exposes only the target, never its private ancestry,
			// uploader, storage path, client roster or grant provenance.
			file.FolderID, file.UserID = nil, nil
			file.Path, file.NavigationRootID = file.Filename, file.ID
			file.Capabilities = &d.Capabilities
			if !d.Capabilities.CanRead {
				file.Filename, file.Path = "Upload destination", ""
				file.Size, file.UploadedAt = 0, time.Time{}
			}
			visible = append(visible, file)
		}
	}
	sort.Slice(visible, func(i, j int) bool {
		a, b := strings.ToLower(visible[i].Filename), strings.ToLower(visible[j].Filename)
		if a == b {
			return visible[i].ID < visible[j].ID
		}
		return a < b
	})
	return visible, nil
}

func capabilitiesCover(a, b models.AccessCapabilities) bool {
	return b.Permissions().Within(a.Permissions())
}
