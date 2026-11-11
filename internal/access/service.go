package access

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
	AccessInvite       AccessOperation = "invite"
	AccessCreateLink   AccessOperation = "create_link"
	AccessManageAccess AccessOperation = "manage_access"
	AccessManageOwn    AccessOperation = "manage_own_access"
	AccessInviteClient AccessOperation = "invite_client"
	AccessManageClient AccessOperation = "manage_client"
	AccessDeleteClient AccessOperation = "delete_client"
	AccessManageQueue  AccessOperation = "manage_queue"

	// Limits bound both SQL parameters and adversarial/corrupt ancestry. Lists
	// must paginate first and use ResolveItems, not call Resolve per row.
	accessBatchSize = 200
	maxAccessItems  = 500
	maxAccessDepth  = 128
)

const (
	MaxItems = maxAccessItems
	MaxDepth = maxAccessDepth
)

// AccessActor contains IDs from an already authenticated tenant-local session,
// never caller-supplied roles/capabilities. A link session is recipient-only:
// LinkID takes precedence even when UserID also identifies a signed-in admin.
// Authentication/session expiry remains the responsibility of auth middleware.
type AccessActor struct {
	UserID    string
	LinkID    string
	SessionID string
}

type AccessRequest struct {
	TenantID  string
	ClientID  string
	FileID    string // empty requests the whole client, not a shared root folder
	Actor     AccessActor
	Operation AccessOperation
	// Internal service input, not accepted directly from HTTP callers.
	GrantRoleID          string
	PreserveArchivedRole bool
	Now                  time.Time // zero uses the current server time
}

// AccessSource is internal management evidence. Never expose it in recipient
// DTOs; those receive effective capabilities and a separately redacted context.
type AccessSource struct {
	Kind   string
	ID     string
	FileID string
}

type AccessDecision struct {
	Allowed         bool                      `json:"allowed"`
	Capabilities    models.AccessCapabilities `json:"capabilities"`
	CanManageClient bool                      `json:"can_manage_client,omitempty"`
	CanManageQueue  bool                      `json:"can_manage_queue,omitempty"`
	Sources         []AccessSource            `json:"-"`
	ReadOwnOnly     bool                      `json:"-"` // read authority comes only from creating this item
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
			roleIDs := []string{}
			for _, row := range rows {
				if row.RoleID != nil {
					roleIDs = append(roleIDs, *row.RoleID)
				}
			}
			roleByID := map[string]models.AccessRole{}
			if len(roleIDs) > 0 {
				var roles []models.AccessRole
				if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id IN ?", req.TenantID, roleIDs).Find(&roles).Error; err != nil {
					return nil, err
				}
				for _, r := range roles {
					roleByID[r.ID] = r
				}
			}
			for _, grant := range rows {
				if grant.RoleID != nil {
					role, ok := roleByID[*grant.RoleID]
					if !ok {
						return nil, gorm.ErrRecordNotFound
					}
					grant.AccessCapabilities = models.AccessCapabilities{}
					if role.Supports(userTypeForRole(base.user.Role), nodes[grant.FileID].Type) {
						grant.AccessCapabilities = projectRoleCapabilities(role, nodes[grant.FileID].Type)
					}
				}
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
			for _, assignment := range base.assignments {
				if assignment.assignment.FileID != nil && chainContains(chain, *assignment.assignment.FileID) {
					caps = unionAccess(caps, assignment.caps)
					sources = append(sources, assignmentSource(assignment.assignment))
				}
			}
		}
		// Creation is not a grant to the entire subtree. Only a signed-in
		// uploader with current upload authority may read their exact items.
		// Bearer collection links retain their private, upload-only behavior.
		file := nodes[id]
		readOwnOnly := !caps.CanRead && caps.CanUpload && base.link == nil && base.user != nil && file.UserID != nil && *file.UserID == base.user.ID
		if readOwnOnly {
			caps.CanRead = true
		}
		if nodes[id].Type != "folder" {
			caps.CanUpload = false
		}
		if !caps.CanRead && caps.CanUpload && requiresKnownUploadDestination(req.Operation) {
			// Private collection may address its explicit destination and new
			// directory trees from this session, never probe/reuse old folders.
			knownDestination := base.rootFolderID == id && base.caps.CanUpload
			if base.link != nil && base.link.FileID != nil {
				knownDestination = *base.link.FileID == id
			}
			if grant, ok := grants[id]; ok && grant.CanUpload {
				knownDestination = true
			}
			for _, a := range base.assignments {
				if a.assignment.FileID != nil && *a.assignment.FileID == id && a.caps.CanUpload {
					knownDestination = true
				}
			}
			file := nodes[id]
			if req.Actor.SessionID != "" && file.UploadSessionID != nil && *file.UploadSessionID == req.Actor.SessionID {
				knownDestination = true
			}
			if !knownDestination {
				continue
			}
		}
		d := accessDecision(caps, sources, req.Operation, nodes[id].Type, base.staff)
		d.ReadOwnOnly = readOwnOnly
		decisions[id] = d
	}
	return decisions, nil
}

type accessBase struct {
	assignments  []resolvedAssignment
	valid        bool
	staff        bool // authority for staff-only client/queue operations
	user         *models.User
	link         *models.SecureLink
	now          time.Time
	caps         models.AccessCapabilities
	sources      []AccessSource
	rootFolderID string
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
	if client.RootFolderID != nil {
		base.rootFolderID = *client.RootFolderID
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
	err := db.Where("tenant_id = ? AND client_id = ? AND user_id = ?", req.TenantID, req.ClientID, user.ID).First(&membership).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return base, err
	}
	if err == nil {
		base.caps = MembershipCapabilities(user.Role, membership)
		resolved, err := s.RoleCapabilities(ctx, req.TenantID, membership.RoleID, base.caps, "workspace", userTypeForRole(user.Role))
		if err != nil {
			return base, err
		}
		base.caps = resolved
		base.sources = []AccessSource{{Kind: "membership", ID: user.ID}}
	}
	base.staff = user.Role == "manager"
	assignments, err := s.assignments(ctx, req.TenantID, user.ID, userTypeForRole(user.Role), []string{req.ClientID}, base.now)
	if err != nil {
		return base, err
	}
	base.assignments = assignments[req.ClientID]
	for _, a := range base.assignments {
		if a.assignment.FileID == nil {
			base.caps = unionAccess(base.caps, a.caps)
			base.sources = append(base.sources, assignmentSource(a.assignment))
		}
	}
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
	c := models.AccessCapabilities{
		CanRead: a.CanRead || b.CanRead, CanUpload: a.CanUpload || b.CanUpload,
		CanDelete: a.CanDelete || b.CanDelete, CanShare: a.CanShare || b.CanShare,
	}
	if a.Sharing != nil || b.Sharing != nil {
		ap, bp := a.Permissions(), b.Permissions()
		c.Sharing = &models.SharingPermissions{InviteExternal: ap.CanInviteExternal || bp.CanInviteExternal, CreateLinks: ap.CanCreateLinks || bp.CanCreateLinks, ManageAccess: ap.CanManageAccess || bp.CanManageAccess}
	}
	return c
}

// MembershipCapabilities decorates an already-authorized membership DTO. It
// does not establish identity, staff authority, or permission to inspect it.
func MembershipCapabilities(role string, membership models.UserClient) models.AccessCapabilities {
	if membership.CapabilitiesSet {
		return membership.AccessCapabilities
	}
	return models.AccessCapabilities{CanRead: true, CanUpload: true, CanShare: true, CanDelete: role == "manager"}
}

func accessDecision(caps models.AccessCapabilities, sources []AccessSource, op AccessOperation, targetType string, staff bool) AccessDecision {
	var allowed bool
	p := caps.Permissions()
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
	case AccessInvite:
		allowed = p.CanInviteExternal
	case AccessCreateLink:
		allowed = p.CanCreateLinks
	case AccessManageAccess:
		allowed = p.CanManageAccess
	case AccessManageOwn:
		allowed = p.CanInviteExternal || p.CanManageAccess
	case AccessInviteClient:
		allowed = targetType == "client" && p.CanInviteExternal
	case AccessManageClient:
		allowed = targetType == "client" && staff && p.CanManageAccess
	case AccessDeleteClient:
		allowed = targetType == "client" && staff && p.CanManageAccess && caps.CanDelete
	case AccessManageQueue:
		allowed = targetType == "client" && staff && p.CanManageAccess && caps.CanRead
	}
	return AccessDecision{Allowed: allowed, Capabilities: caps, Sources: sources, CanManageClient: targetType == "client" && staff && p.CanManageAccess, CanManageQueue: targetType == "client" && staff && p.CanManageAccess && caps.CanRead}
}

func requiresKnownUploadDestination(op AccessOperation) bool {
	switch op {
	case AccessContext, AccessUpload, AccessShare, AccessInvite, AccessInviteClient, AccessCreateLink, AccessManageAccess, AccessManageOwn:
		return true
	}
	return false
}

// CanGrant applies a ceiling to a previously resolved exact-scope decision.
// Membership invitation additionally requires AccessInviteClient, not an item
// decision. Role assignment and invitee ownership remain separate checks.
func (d AccessDecision) CanGrant(requested models.AccessCapabilities) bool {
	return d.Allowed && d.Capabilities.CanShare && requested.Permissions().Within(d.Capabilities.Permissions())
}

func (s *AccessService) RoleCapabilities(ctx context.Context, tenantID string, roleID *string, fallback models.AccessCapabilities, scope string, userType ...string) (models.AccessCapabilities, error) {
	if roleID == nil {
		return fallback, nil
	}
	var role models.AccessRole
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, *roleID).First(&role).Error; err != nil {
		return models.AccessCapabilities{}, err
	}
	if len(userType) > 0 && !role.Supports(userType[0], scope) {
		return models.AccessCapabilities{}, nil
	}
	return projectRoleCapabilities(role, scope), nil
}

func userTypeForRole(role string) string {
	if role == "admin" || role == "manager" {
		return "internal"
	}
	return "external"
}

func projectRoleCapabilities(role models.AccessRole, scope string) models.AccessCapabilities {
	p, valid := role.PermissionsAt(scope)
	if !valid {
		return models.AccessCapabilities{}
	}
	// Archival blocks new assignments, not the access of existing recipients.
	c := p.Capabilities()
	if role.LegacySharing {
		c.Sharing = nil
	}
	return c
}
