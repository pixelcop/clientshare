package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRoleInput = errors.New("invalid role or group settings")
var ErrRoleConflict = errors.New("access settings changed; refresh and review the impact again")
var ErrRoleForbidden = errors.New("only organization administrators can manage roles and groups")

type AccessRoleService struct{ DB *gorm.DB }

// AccessProfileFor keeps compatibility with existing accounts; new creation
// paths persist an explicit profile in the same transaction as the account.
func LegacyAccessProfile(user models.User) models.AccessProfile {
	p := models.AccessProfile{TenantID: user.TenantID, UserID: user.ID, UserType: "external", OrganizationRole: "none"}
	if user.Role == "admin" {
		p.UserType, p.OrganizationRole, p.CanManageUsers, p.AllWorkspaces = "internal", "administrator", true, true
	}
	if user.Role == "manager" {
		p.UserType, p.OrganizationRole, p.CanManageUsers, p.AllWorkspaces = "internal", "member", true, user.ManagerFullAccess
	}
	return p
}

func AccessProfileFor(db *gorm.DB, user models.User) (models.AccessProfile, error) {
	p := LegacyAccessProfile(user)
	var stored models.AccessProfile
	err := db.Where("tenant_id = ? AND user_id = ?", user.TenantID, user.ID).First(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p, nil
	}
	return stored, err
}

func (s *AccessRoleService) administrator(tx *gorm.DB, tenantID, actorID string) error {
	return RequireOrganizationAdministrator(tx, tenantID, actorID)
}

func RequireOrganizationAdministrator(tx *gorm.DB, tenantID, actorID string) error {
	if err := LockTenantAccess(tx, tenantID); err != nil {
		return err
	}
	var actor models.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, actorID).First(&actor).Error; err != nil {
		return err
	}
	if actor.Role != "admin" {
		return ErrRoleForbidden
	}
	return nil
}

func (s *AccessRoleService) EnsureDefaults(ctx context.Context, tenantID string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant models.Tenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			return err
		}
		created := tx.Exec("INSERT INTO access_role_catalogs (tenant_id) VALUES (?) ON CONFLICT (tenant_id) DO NOTHING", tenantID)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			return nil
		}
		// Archiving/renaming defaults must never resurrect or overwrite them.
		for _, role := range models.DefaultAccessRoles(tenantID) {
			var count int64
			if err := tx.Model(&models.AccessRole{}).Where("tenant_id = ? AND template_key = ?", tenantID, *role.TemplateKey).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				// A tenant may have reused a template name for a custom role.
				if err := tx.Model(&models.AccessRole{}).Where("tenant_id = ? AND name = ?", tenantID, role.Name).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					role.Name += " (default)"
				}
				if err := tx.Create(&role).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *AccessRoleService) Roles(ctx context.Context, tenantID, actorID string, includeArchived bool) ([]models.AccessRole, error) {
	var user models.User
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, actorID).First(&user).Error; err != nil {
		return nil, err
	}
	if includeArchived && user.Role != "admin" {
		return nil, ErrRoleForbidden
	}
	if err := s.EnsureDefaults(ctx, tenantID); err != nil {
		return nil, err
	}
	roles := []models.AccessRole{}
	q := s.DB.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if !includeArchived {
		q = q.Where("archived_at IS NULL")
	}
	err := q.Order("created_at ASC, id ASC").Find(&roles).Error
	return roles, err
}

func validateRole(role *models.AccessRole) error {
	role.Name, role.Description = strings.TrimSpace(role.Name), strings.TrimSpace(role.Description)
	if role.Name == "" || len(role.Name) > 100 || len(role.Description) > 500 ||
		(role.Audience != "internal" && role.Audience != "external" && role.Audience != "both") ||
		(!role.AllowWorkspace && !role.AllowFolder && !role.AllowFile) {
		return ErrRoleInput
	}
	if role.CanUpload && !role.AllowWorkspace && !role.AllowFolder {
		return ErrRoleInput
	}
	return nil
}

type RoleImpact struct {
	GroupMembers     int64  `json:"group_members"`
	Memberships      int64  `json:"memberships"`
	Grants           int64  `json:"grants"`
	Invitations      int64  `json:"invitations"`
	GroupAssignments int64  `json:"group_assignments"`
	Version          uint64 `json:"version"`
	Fingerprint      string `json:"fingerprint"`
}

func roleImpact(tx *gorm.DB, tenantID, roleID string) (RoleImpact, error) {
	var role models.AccessRole
	if err := tx.Where("tenant_id = ? AND id = ?", tenantID, roleID).First(&role).Error; err != nil {
		return RoleImpact{}, err
	}
	i := RoleImpact{Version: role.Version}
	for _, item := range []struct {
		model any
		count *int64
	}{
		{&models.UserClient{}, &i.Memberships}, {&models.ItemGrant{}, &i.Grants}, {&models.AccessInvitation{}, &i.Invitations}, {&models.RoleAssignment{}, &i.GroupAssignments},
	} {
		if err := tx.Model(item.model).Where("tenant_id = ? AND role_id = ?", tenantID, roleID).Count(item.count).Error; err != nil {
			return i, err
		}
	}
	if err := tx.Table("access_group_members AS gm").Joins("JOIN role_assignments ra ON ra.tenant_id=gm.tenant_id AND ra.group_id=gm.group_id").Where("ra.tenant_id = ? AND ra.role_id = ? AND ra.revoked_at IS NULL", tenantID, roleID).Distinct("gm.user_id").Count(&i.GroupMembers).Error; err != nil {
		return i, err
	}
	// Counts alone miss a same-size recipient swap. Bind the confirmation to
	// actual source identities and group membership as well as the role version.
	var memberships []models.UserClient
	var grants []models.ItemGrant
	var invitations []models.AccessInvitation
	var assignments []models.RoleAssignment
	var members []models.AccessGroupMember
	for _, query := range []struct {
		model any
		order string
	}{
		{&memberships, "user_id,client_id"}, {&grants, "id"}, {&invitations, "id"}, {&assignments, "id"},
	} {
		if err := tx.Where("tenant_id = ? AND role_id = ?", tenantID, roleID).Order(query.order).Find(query.model).Error; err != nil {
			return i, err
		}
	}
	if err := tx.Where("tenant_id = ? AND group_id IN (SELECT group_id FROM role_assignments WHERE tenant_id = ? AND role_id = ?)", tenantID, tenantID, roleID).Order("group_id,user_id").Find(&members).Error; err != nil {
		return i, err
	}
	i.Fingerprint = sharingDigest(struct {
		Impact      RoleImpact
		Memberships []models.UserClient
		Grants      []models.ItemGrant
		Invitations []models.AccessInvitation
		Assignments []models.RoleAssignment
		Members     []models.AccessGroupMember
	}{i, memberships, grants, invitations, assignments, members})
	return i, nil
}

func (s *AccessRoleService) Impact(ctx context.Context, tenantID, actorID, roleID string) (RoleImpact, error) {
	if err := s.administrator(s.DB.WithContext(ctx), tenantID, actorID); err != nil {
		return RoleImpact{}, err
	}
	return roleImpact(s.DB.WithContext(ctx), tenantID, roleID)
}

func (s *AccessRoleService) SaveRole(ctx context.Context, tenantID, actorID, roleID string, input models.AccessRole, version uint64, impact string) (*models.AccessRole, error) {
	if err := validateRole(&input); err != nil {
		return nil, err
	}
	var result models.AccessRole
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.administrator(tx, tenantID, actorID); err != nil {
			return err
		}
		if roleID == "" {
			result = models.AccessRole{TenantID: tenantID, Name: input.Name, Description: input.Description, Audience: input.Audience, AllowWorkspace: input.AllowWorkspace, AllowFolder: input.AllowFolder, AllowFile: input.AllowFile, RolePermissions: input.RolePermissions, Version: 1}
			if err := tx.Create(&result).Error; err != nil {
				return err
			}
			return sharingAudit(tx, AccessRequest{TenantID: tenantID, Actor: AccessActor{UserID: actorID}}, "role.create", result.ID, nil, result)
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, roleID).First(&result).Error; err != nil {
			return err
		}
		current, err := roleImpact(tx, tenantID, roleID)
		if err != nil {
			return err
		}
		if version == 0 || version != result.Version || impact != current.Fingerprint {
			return ErrRoleConflict
		}
		before := result
		// Audience/scope changes cannot invalidate existing assignments silently.
		if (input.Audience != result.Audience || input.AllowWorkspace != result.AllowWorkspace || input.AllowFolder != result.AllowFolder || input.AllowFile != result.AllowFile) && (current.Memberships+current.Grants+current.Invitations+current.GroupAssignments) > 0 {
			return ErrRoleInput
		}
		updates := map[string]any{"name": input.Name, "description": input.Description, "audience": input.Audience, "allow_workspace": input.AllowWorkspace, "allow_folder": input.AllowFolder, "allow_file": input.AllowFile, "can_read": input.CanRead, "can_upload": input.CanUpload, "can_delete": input.CanDelete, "can_invite_external": input.CanInviteExternal, "can_create_links": input.CanCreateLinks, "can_manage_access": input.CanManageAccess, "version": version + 1, "updated_at": time.Now().UTC()}
		updated := tx.Model(&models.AccessRole{}).Where("tenant_id = ? AND id = ? AND version = ?", tenantID, roleID, version).Updates(updates)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrRoleConflict
		}
		if before.CanInviteExternal != input.CanInviteExternal || before.CanCreateLinks != input.CanCreateLinks || before.CanManageAccess != input.CanManageAccess {
			if err := tx.Exec("UPDATE access_roles SET legacy_sharing = FALSE WHERE tenant_id = ? AND id = ?", tenantID, roleID).Error; err != nil {
				return err
			}
		}
		if err := refreshRoleSnapshots(tx, tenantID, roleID, input.RolePermissions); err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, roleID).First(&result).Error; err != nil {
			return err
		}
		return sharingAudit(tx, AccessRequest{TenantID: tenantID, Actor: AccessActor{UserID: actorID}}, "role.update", roleID, before, result)
	})
	return &result, err
}

func refreshRoleSnapshots(tx *gorm.DB, tenantID, roleID string, p models.RolePermissions) error {
	caps := p.Capabilities()
	for _, model := range []any{&models.UserClient{}, &models.ItemGrant{}, &models.AccessInvitation{}} {
		updates := map[string]any{"can_read": caps.CanRead, "can_upload": caps.CanUpload, "can_delete": caps.CanDelete, "can_share": caps.CanShare}
		if _, ok := model.(*models.UserClient); ok {
			updates["capabilities_set"] = true
		}
		if err := tx.Model(model).Where("tenant_id = ? AND role_id = ?", tenantID, roleID).Updates(updates).Error; err != nil {
			return err
		}
		if _, ok := model.(*models.UserClient); !ok {
			// Single-file projections never gain upload semantics.
			if err := tx.Model(model).Where("tenant_id = ? AND role_id = ? AND file_id IN (SELECT id FROM files WHERE tenant_id = ? AND type = 'file')", tenantID, roleID, tenantID).Update("can_upload", false).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *AccessRoleService) ArchiveRole(ctx context.Context, tenantID, actorID, roleID string, version uint64, impact string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.administrator(tx, tenantID, actorID); err != nil {
			return err
		}
		var role models.AccessRole
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, roleID).First(&role).Error; err != nil {
			return err
		}
		current, err := roleImpact(tx, tenantID, roleID)
		if err != nil {
			return err
		}
		if version != role.Version || impact != current.Fingerprint {
			return ErrRoleConflict
		}
		before := role
		if err := tx.Model(&role).Updates(map[string]any{"archived_at": time.Now().UTC(), "version": version + 1}).Error; err != nil {
			return err
		}
		return sharingAudit(tx, AccessRequest{TenantID: tenantID, Actor: AccessActor{UserID: actorID}}, "role.archive", roleID, before, role)
	})
}

// AssignmentRole revalidates recipient type, scope, archival and actor ceiling
// from tenant-local records. Browser-supplied permission bits are not authority.
func AssignmentRole(tx *gorm.DB, tenantID, roleID, userType, scope string, existing ...bool) (models.AccessRole, models.AccessCapabilities, error) {
	var role models.AccessRole
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, roleID).First(&role).Error; err != nil {
		return role, models.AccessCapabilities{}, err
	}
	allowExisting := len(existing) > 0 && existing[0]
	if !role.Supports(userType, scope) || (role.ArchivedAt != nil && !allowExisting) {
		return role, models.AccessCapabilities{}, ErrRoleInput
	}
	p, _ := role.PermissionsAt(scope)
	caps := p.Capabilities()
	if role.LegacySharing {
		caps.Sharing = nil
	}
	return role, caps, nil
}
