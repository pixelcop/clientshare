package models

import (
	"time"

	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

// AccessRoleCatalog records one-time seeding so deleting a default role never
// recreates it on the next visit to the sharing dropdown.
type AccessRoleCatalog struct {
	TenantID string `gorm:"primaryKey" json:"tenant_id"`
}

// RolePermissions intentionally does not embed AccessCapabilities. Legacy
// CanShare combines three different authorities and cannot represent this model.
type RolePermissions struct {
	CanRead           bool `gorm:"not null;default:false" json:"can_read"`
	CanUpload         bool `gorm:"not null;default:false" json:"can_upload"`
	CanDelete         bool `gorm:"not null;default:false" json:"can_delete"`
	CanInviteExternal bool `gorm:"not null;default:false" json:"can_invite_external"`
	CanCreateLinks    bool `gorm:"not null;default:false" json:"can_create_links"`
	CanManageAccess   bool `gorm:"not null;default:false" json:"can_manage_access"`
}

func (p RolePermissions) Within(ceiling RolePermissions) bool {
	return (!p.CanRead || ceiling.CanRead) && (!p.CanUpload || ceiling.CanUpload) &&
		(!p.CanDelete || ceiling.CanDelete) && (!p.CanInviteExternal || ceiling.CanInviteExternal) &&
		(!p.CanCreateLinks || ceiling.CanCreateLinks) && (!p.CanManageAccess || ceiling.CanManageAccess)
}

// AccessRole is tenant-owned, including seeded roles. TemplateKey is provenance,
// never an authorization shortcut. Archived roles retain existing assignments.
type AccessRole struct {
	ID              string  `gorm:"primaryKey;size:26" json:"id"`
	TenantID        string  `gorm:"not null;uniqueIndex:idx_access_role_name,priority:1;uniqueIndex:idx_access_role_template,priority:1" json:"tenant_id"`
	Name            string  `gorm:"not null;uniqueIndex:idx_access_role_name,priority:2" json:"name"`
	Description     string  `gorm:"not null;default:''" json:"description"`
	TemplateKey     *string `gorm:"uniqueIndex:idx_access_role_template,priority:2" json:"template_key,omitempty"`
	LegacySharing   bool    `gorm:"->;not null;default:false" json:"legacy_sharing"`
	Audience        string  `gorm:"not null" json:"audience"`
	AllowWorkspace  bool    `gorm:"not null;default:false" json:"allow_workspace"`
	AllowFolder     bool    `gorm:"not null;default:false" json:"allow_folder"`
	AllowFile       bool    `gorm:"not null;default:false" json:"allow_file"`
	RolePermissions `gorm:"embedded"`
	Version         uint64     `gorm:"not null;default:1" json:"version"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (r *AccessRole) BeforeCreate(_ *gorm.DB) error {
	ensureTenantID(&r.TenantID)
	if r.ID == "" {
		r.ID = ids.NewULID()
	}
	return nil
}

func (r AccessRole) AppliesTo(userType, scope string) bool {
	return r.ArchivedAt == nil && r.Supports(userType, scope)
}

// Supports describes existing assignments; archival only changes eligibility
// for new assignments, not the scope/audience of current access.
func (r AccessRole) Supports(userType, scope string) bool {
	if (userType != "internal" && userType != "external") ||
		(r.Audience != "both" && r.Audience != userType) {
		return false
	}
	switch scope {
	case "workspace":
		return r.AllowWorkspace
	case "folder":
		return r.AllowFolder
	case "file":
		return r.AllowFile
	default:
		return false
	}
}

// PermissionsAt projects a role onto the target's supported operations. Upload
// means adding children to a folder/workspace, never replacing a single file.
func (r AccessRole) PermissionsAt(scope string) (RolePermissions, bool) {
	p := r.RolePermissions
	switch scope {
	case "workspace":
		return p, r.AllowWorkspace
	case "folder":
		return p, r.AllowFolder
	case "file":
		p.CanUpload = false
		return p, r.AllowFile
	default:
		return RolePermissions{}, false
	}
}

// DefaultAccessRoles returns fresh templates. The caller supplies tenant-local
// identity. No code should infer capabilities from these names or keys.
func DefaultAccessRoles(tenantID string) []AccessRole {
	role := func(key, name, description string, permissions RolePermissions, file bool) AccessRole {
		return AccessRole{TenantID: tenantID, Name: name, Description: description,
			TemplateKey: &key, Audience: "both", AllowWorkspace: true, AllowFolder: true,
			AllowFile: file, RolePermissions: permissions, Version: 1}
	}
	roles := []AccessRole{
		role("viewer", "Viewer", "View and download files", RolePermissions{CanRead: true}, true),
		role("uploader", "Uploader", "Upload only; existing files stay private", RolePermissions{CanUpload: true}, false),
		role("contributor", "Contributor", "View, download and upload files", RolePermissions{CanRead: true, CanUpload: true}, false),
		role("client-contact", "Client contact", "View, upload and invite external people", RolePermissions{CanRead: true, CanUpload: true, CanInviteExternal: true}, false),
		role("file-manager", "File manager", "View, upload and delete files", RolePermissions{CanRead: true, CanUpload: true, CanDelete: true}, true),
		role("workspace-manager", "Workspace manager", "Manage documents and access in this workspace", RolePermissions{CanRead: true, CanUpload: true, CanDelete: true, CanInviteExternal: true, CanCreateLinks: true, CanManageAccess: true}, true),
	}
	roles[len(roles)-1].Audience = "internal"
	return roles
}
