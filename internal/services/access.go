package services

import (
	"context"
	"errors"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

type AccessOperation string

const (
	AccessContext      AccessOperation = "context"
	AccessRead         AccessOperation = "read"
	AccessList         AccessOperation = "list"
	AccessDownload     AccessOperation = "download"
	AccessZIP          AccessOperation = "zip"
	AccessUpload       AccessOperation = "upload"
	AccessDelete       AccessOperation = "delete"
	AccessShare        AccessOperation = "share"
	AccessInviteClient AccessOperation = "invite_client"
	AccessManageClient AccessOperation = "manage_client"
	AccessManageQueue  AccessOperation = "manage_queue"

	// Limits bound both SQL parameters and adversarial/corrupt ancestry. Lists
	// must paginate first and use ResolveItems, not call Resolve per row.
	accessBatchSize = 200
	maxAccessItems  = 500
	maxAccessDepth  = 128
)

// AccessActor contains IDs from an already authenticated tenant-local session,
// never caller-supplied roles/capabilities. A link session is recipient-only:
// LinkID takes precedence even when UserID also identifies a signed-in admin.
// Authentication/session expiry remains the responsibility of auth middleware.
type AccessActor struct {
	UserID string
	LinkID string
}

type AccessRequest struct {
	TenantID  string
	ClientID  string
	FileID    string // empty requests the whole client, not a shared root folder
	Actor     AccessActor
	Operation AccessOperation
	Now       time.Time // zero uses the current server time
}

// AccessSource is internal management evidence. Never expose it in recipient
// DTOs; those receive effective capabilities and a separately redacted context.
type AccessSource struct {
	Kind   string
	ID     string
	FileID string
}

type AccessDecision struct {
	Allowed      bool                      `json:"allowed"`
	Capabilities models.AccessCapabilities `json:"capabilities"`
	Sources      []AccessSource            `json:"-"`
}

// AccessService is deliberately uncached: revocation, expiry, role changes and
// reparenting take effect at the next request. It is not wired into scoped
// endpoints until every protected surface has migrated to this resolver.
type AccessService struct{ DB *gorm.DB }

func (s *AccessService) Resolve(ctx context.Context, req AccessRequest) (AccessDecision, error) {
	if req.FileID == "" {
		base, err := s.resolveBase(ctx, req)
		if err != nil || !base.valid {
			return AccessDecision{}, err
		}
		// An item link is never whole-client authority.
		if base.link != nil && base.link.FileID != nil {
			return AccessDecision{}, nil
		}
		return accessDecision(base.caps, base.sources, req.Operation, "client", base.staff), nil
	}
	decisions, err := s.ResolveItems(ctx, req, []string{req.FileID})
	return decisions[req.FileID], err
}

// ResolveItems evaluates exact items from one tenant/client with batched parent
// and grant queries. Missing/inaccessible/corrupt items have identical zero
// decisions. Callers must filter before computing authorized totals/pagination;
// this method does not itself implement a listing or disclose sibling names.
func (s *AccessService) ResolveItems(ctx context.Context, req AccessRequest, fileIDs []string) (map[string]AccessDecision, error) {
	decisions := make(map[string]AccessDecision, len(fileIDs))
	if len(fileIDs) > maxAccessItems {
		return nil, errors.New("access batch exceeds 500 items")
	}
	for _, id := range fileIDs {
		decisions[id] = AccessDecision{}
	}
	if len(fileIDs) == 0 {
		return decisions, nil
	}
	base, err := s.resolveBase(ctx, req)
	if err != nil || !base.valid {
		return decisions, err
	}
	nodes, err := s.loadAccessAncestry(ctx, req, fileIDs)
	if err != nil {
		return nil, err
	}
	grants := make(map[string]models.ItemGrant)
	if base.link == nil && base.user != nil {
		ids := make([]string, 0, len(nodes))
		for id := range nodes {
			ids = append(ids, id)
		}
		for start := 0; start < len(ids); start += accessBatchSize {
			var rows []models.ItemGrant
			if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND user_id = ? AND file_id IN ? AND revoked_at IS NULL", req.TenantID, req.ClientID, base.user.ID, ids[start:min(start+accessBatchSize, len(ids))]).Find(&rows).Error; err != nil {
				return nil, err
			}
			for _, grant := range rows {
				// SQLite timestamp ordering is textual across stored offsets.
				if grant.ExpiresAt == nil || grant.ExpiresAt.After(base.now) {
					grants[grant.FileID] = grant
				}
			}
		}
	}
	for _, id := range fileIDs {
		chain, valid := accessChain(nodes, id)
		if !valid {
			continue
		}
		caps := base.caps
		sources := append([]AccessSource(nil), base.sources...)
		if base.link != nil {
			if base.link.FileID != nil {
				target, inScope := nodes[*base.link.FileID]
				if !inScope || !chainContains(chain, target.ID) || (target.Type != "folder" && target.ID != id) {
					continue
				}
				// A malformed write/readwrite link to a file is not a valid file
				// link, even if its read bit could otherwise be salvaged.
				if target.Type == "file" && base.link.AccessType != "read" && base.link.AccessType != "view" {
					continue
				}
			}
		} else {
			for _, ancestor := range chain {
				if grant, ok := grants[ancestor.ID]; ok {
					caps = unionAccess(caps, grant.AccessCapabilities)
					sources = append(sources, AccessSource{Kind: "grant", ID: grant.ID, FileID: grant.FileID})
				}
			}
		}
		if nodes[id].Type != "folder" {
			caps.CanUpload = false
		}
		decisions[id] = accessDecision(caps, sources, req.Operation, nodes[id].Type, base.staff)
	}
	return decisions, nil
}

type accessBase struct {
	valid   bool
	staff   bool // authority for staff-only client/queue operations
	user    *models.User
	link    *models.SecureLink
	now     time.Time
	caps    models.AccessCapabilities
	sources []AccessSource
}

func (s *AccessService) resolveBase(ctx context.Context, req AccessRequest) (accessBase, error) {
	var base accessBase
	if req.TenantID == "" || req.ClientID == "" {
		return base, nil
	}
	base.now = req.Now
	if base.now.IsZero() {
		base.now = time.Now()
	}
	db := s.DB.WithContext(ctx)
	var client models.Client
	if err := db.Where("tenant_id = ? AND id = ?", req.TenantID, req.ClientID).First(&client).Error; err != nil {
		return base, accessQueryError(err)
	}
	if req.Actor.LinkID != "" {
		var link models.SecureLink
		if err := db.Where("tenant_id = ? AND client_id = ? AND id = ? AND revoked_at IS NULL", req.TenantID, req.ClientID, req.Actor.LinkID).First(&link).Error; err != nil {
			return base, accessQueryError(err)
		}
		if !link.ExpiresAt.After(base.now) {
			return base, nil
		}
		switch link.AccessType {
		case "view", "read":
			base.caps.CanRead = true
		case "write":
			base.caps.CanUpload = true
		case "readwrite":
			base.caps.CanRead, base.caps.CanUpload = true, true
		default:
			return base, nil
		}
		base.link, base.valid = &link, true
		source := AccessSource{Kind: "link", ID: link.ID}
		if link.FileID != nil {
			source.FileID = *link.FileID
		}
		base.sources = []AccessSource{source}
		return base, nil
	}
	if req.Actor.UserID == "" {
		return base, nil
	}
	var user models.User
	if err := db.Where("tenant_id = ? AND id = ?", req.TenantID, req.Actor.UserID).First(&user).Error; err != nil {
		return base, accessQueryError(err)
	}
	switch user.Role {
	case "admin", "manager", "client", "customer":
	default:
		return base, nil
	}
	base.user, base.valid = &user, true
	if user.Role == "admin" || (user.Role == "manager" && user.ManagerFullAccess) {
		base.caps = models.AccessCapabilities{CanRead: true, CanUpload: true, CanDelete: true, CanShare: true}
		base.staff = true
		base.sources = []AccessSource{{Kind: "staff", ID: user.ID}}
		return base, nil
	}
	var membership models.UserClient
	if err := db.Where("tenant_id = ? AND client_id = ? AND user_id = ?", req.TenantID, req.ClientID, user.ID).First(&membership).Error; err != nil {
		return base, accessQueryError(err)
	}
	if membership.CapabilitiesSet {
		base.caps = membership.AccessCapabilities
	} else {
		base.caps = models.AccessCapabilities{CanRead: true, CanUpload: true, CanShare: true, CanDelete: user.Role == "manager"}
	}
	base.staff = user.Role == "manager"
	base.sources = []AccessSource{{Kind: "membership", ID: user.ID}}
	return base, nil
}

func accessQueryError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (s *AccessService) loadAccessAncestry(ctx context.Context, req AccessRequest, ids []string) (map[string]models.File, error) {
	nodes := make(map[string]models.File)
	seen := make(map[string]bool)
	frontier := append([]string(nil), ids...)
	for depth := 0; depth < maxAccessDepth && len(frontier) > 0; depth++ {
		pending := make([]string, 0, len(frontier))
		for _, id := range frontier {
			if id != "" && !seen[id] {
				seen[id] = true
				pending = append(pending, id)
			}
		}
		frontier = nil
		for start := 0; start < len(pending); start += accessBatchSize {
			var rows []models.File
			if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND id IN ? AND deleted_at IS NULL AND disk_deleted_at IS NULL", req.TenantID, req.ClientID, pending[start:min(start+accessBatchSize, len(pending))]).Find(&rows).Error; err != nil {
				return nil, err
			}
			for _, file := range rows {
				nodes[file.ID] = file
				if file.FolderID != nil {
					frontier = append(frontier, *file.FolderID)
				}
			}
		}
	}
	return nodes, nil
}

// accessChain validates every ancestor even for admins/direct grantees: deleted
// or foreign parents, cycles, non-folder parents and excessive depth fail shut.
func accessChain(nodes map[string]models.File, id string) ([]models.File, bool) {
	chain := make([]models.File, 0)
	seen := make(map[string]bool)
	for depth := 0; depth < maxAccessDepth; depth++ {
		file, ok := nodes[id]
		if !ok || seen[id] || (file.Type != "file" && file.Type != "folder") || (depth > 0 && file.Type != "folder") {
			return nil, false
		}
		seen[id] = true
		chain = append(chain, file)
		if file.FolderID == nil {
			return chain, true
		}
		id = *file.FolderID
	}
	return nil, false
}

func chainContains(chain []models.File, id string) bool {
	for _, file := range chain {
		if file.ID == id {
			return true
		}
	}
	return false
}

func unionAccess(a, b models.AccessCapabilities) models.AccessCapabilities {
	return models.AccessCapabilities{
		CanRead: a.CanRead || b.CanRead, CanUpload: a.CanUpload || b.CanUpload,
		CanDelete: a.CanDelete || b.CanDelete, CanShare: a.CanShare || b.CanShare,
	}
}

func accessDecision(caps models.AccessCapabilities, sources []AccessSource, op AccessOperation, targetType string, staff bool) AccessDecision {
	var allowed bool
	switch op {
	case AccessContext:
		allowed = caps.CanRead || (caps.CanUpload && targetType != "file")
	case AccessRead, AccessDownload:
		allowed = caps.CanRead
	case AccessList, AccessZIP:
		// ZIP authorization covers the requested folder, not a license to bypass
		// checks on archive entries. Callers must resolve included items too.
		allowed = caps.CanRead && targetType == "folder"
	case AccessUpload:
		allowed = caps.CanUpload && targetType == "folder"
	case AccessDelete:
		allowed = caps.CanDelete
	case AccessShare:
		allowed = caps.CanShare
	case AccessInviteClient:
		allowed = targetType == "client" && caps.CanShare
	case AccessManageClient, AccessManageQueue:
		allowed = targetType == "client" && staff && caps.CanShare
	}
	return AccessDecision{Allowed: allowed, Capabilities: caps, Sources: sources}
}

// CanGrant applies a ceiling to a previously resolved exact-scope decision.
// Membership invitation additionally requires AccessInviteClient, not an item
// decision. Role assignment and invitee ownership remain separate checks.
func (d AccessDecision) CanGrant(requested models.AccessCapabilities) bool {
	return d.Allowed && d.Capabilities.CanShare &&
		(!requested.CanRead || d.Capabilities.CanRead) &&
		(!requested.CanUpload || d.Capabilities.CanUpload) &&
		(!requested.CanDelete || d.Capabilities.CanDelete) &&
		(!requested.CanShare || d.Capabilities.CanShare)
}
