package main

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
	"gorm.io/gorm"
)

// This additive migration prepares the new model without switching authority.
// Original memberships, grants, invitations, links and account roles are kept.
func upAddAccessRoleFoundation(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	timestamp := "TIMESTAMP"
	if db.Dialector.Name() == "postgres" {
		timestamp = "TIMESTAMPTZ"
	}
	queries := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_access_identity ON users (tenant_id, id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_access_identity ON clients (tenant_id, id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_files_access_identity ON files (tenant_id, client_id, id)`,
		`CREATE TABLE access_profiles (
			tenant_id TEXT NOT NULL, user_id TEXT NOT NULL,
			user_type TEXT NOT NULL CHECK(user_type IN ('internal','external')),
			organization_role TEXT NOT NULL CHECK(organization_role IN ('member','administrator','none')),
			can_manage_users BOOLEAN NOT NULL DEFAULT FALSE,
			all_workspaces BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY(tenant_id,user_id),
			CHECK(user_type = 'internal' OR (organization_role = 'none' AND NOT can_manage_users AND NOT all_workspaces)),
			FOREIGN KEY(tenant_id,user_id) REFERENCES users(tenant_id,id) ON DELETE CASCADE
		)`,
		fmt.Sprintf(`CREATE TABLE access_roles (
			id TEXT PRIMARY KEY NOT NULL, tenant_id TEXT NOT NULL,
			name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', template_key TEXT,
			audience TEXT NOT NULL CHECK(audience IN ('internal','external','both')),
			allow_workspace BOOLEAN NOT NULL DEFAULT FALSE,
			allow_folder BOOLEAN NOT NULL DEFAULT FALSE,
			allow_file BOOLEAN NOT NULL DEFAULT FALSE,
			can_read BOOLEAN NOT NULL DEFAULT FALSE, can_upload BOOLEAN NOT NULL DEFAULT FALSE,
			can_delete BOOLEAN NOT NULL DEFAULT FALSE, can_invite_external BOOLEAN NOT NULL DEFAULT FALSE,
			can_create_links BOOLEAN NOT NULL DEFAULT FALSE, can_manage_access BOOLEAN NOT NULL DEFAULT FALSE,
			version BIGINT NOT NULL DEFAULT 1 CHECK(version > 0),
			archived_at %[1]s, created_at %[1]s, updated_at %[1]s,
			UNIQUE(tenant_id,id), UNIQUE(tenant_id,name), UNIQUE(tenant_id,template_key),
			CHECK(allow_workspace OR allow_folder OR allow_file),
			FOREIGN KEY(tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
		)`, timestamp),
		fmt.Sprintf(`CREATE TABLE access_groups (
			id TEXT PRIMARY KEY NOT NULL, tenant_id TEXT NOT NULL, name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '', version BIGINT NOT NULL DEFAULT 1 CHECK(version > 0),
			created_at %[1]s, updated_at %[1]s, UNIQUE(tenant_id,id), UNIQUE(tenant_id,name),
			FOREIGN KEY(tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
		)`, timestamp),
		fmt.Sprintf(`CREATE TABLE access_group_members (
			tenant_id TEXT NOT NULL, group_id TEXT NOT NULL, user_id TEXT NOT NULL, created_at %[1]s,
			PRIMARY KEY(tenant_id,group_id,user_id),
			FOREIGN KEY(tenant_id,group_id) REFERENCES access_groups(tenant_id,id) ON DELETE CASCADE,
			FOREIGN KEY(tenant_id,user_id) REFERENCES users(tenant_id,id) ON DELETE CASCADE
		)`, timestamp),
		fmt.Sprintf(`CREATE TABLE role_assignments (
			id TEXT PRIMARY KEY NOT NULL, tenant_id TEXT NOT NULL, role_id TEXT NOT NULL,
			user_id TEXT, group_id TEXT, client_id TEXT NOT NULL, file_id TEXT,
			source_kind TEXT NOT NULL DEFAULT 'direct', source_id TEXT,
			version BIGINT NOT NULL DEFAULT 1 CHECK(version > 0),
			expires_at %[1]s, revoked_at %[1]s, created_at %[1]s, updated_at %[1]s,
			CHECK((user_id IS NOT NULL AND group_id IS NULL) OR (group_id IS NOT NULL AND user_id IS NULL)),
			UNIQUE(tenant_id,source_kind,source_id),
			FOREIGN KEY(tenant_id,role_id) REFERENCES access_roles(tenant_id,id),
			FOREIGN KEY(tenant_id,user_id) REFERENCES users(tenant_id,id) ON DELETE CASCADE,
			FOREIGN KEY(tenant_id,group_id) REFERENCES access_groups(tenant_id,id) ON DELETE CASCADE,
			FOREIGN KEY(tenant_id,client_id) REFERENCES clients(tenant_id,id) ON DELETE CASCADE,
			FOREIGN KEY(tenant_id,client_id,file_id) REFERENCES files(tenant_id,client_id,id) ON DELETE CASCADE
		)`, timestamp),
		`CREATE INDEX idx_role_assignment_target ON role_assignments(tenant_id,client_id,file_id)`,
		`CREATE INDEX idx_role_assignment_role ON role_assignments(tenant_id,role_id)`,
		`CREATE INDEX idx_role_assignment_user ON role_assignments(tenant_id,user_id)`,
		`CREATE INDEX idx_role_assignment_group ON role_assignments(tenant_id,group_id)`,
		`CREATE INDEX idx_access_group_member_user ON access_group_members(tenant_id,user_id)`,
	}
	for _, query := range queries {
		if err := db.Exec(query).Error; err != nil {
			return err
		}
	}
	return backfillAccessRoles(db)
}

func backfillAccessRoles(db *gorm.DB) error {
	var tenants []models.Tenant
	if err := db.Find(&tenants).Error; err != nil {
		return err
	}
	for _, tenant := range tenants {
		roles := models.DefaultAccessRoles(tenant.ID)
		for i := range roles {
			if err := db.Create(&roles[i]).Error; err != nil {
				return err
			}
		}
		var users []models.User
		if err := db.Where("tenant_id = ?", tenant.ID).Find(&users).Error; err != nil {
			return err
		}
		userByID := make(map[string]models.User, len(users))
		for _, user := range users {
			profile := models.AccessProfile{TenantID: tenant.ID, UserID: user.ID, UserType: "external", OrganizationRole: "none"}
			switch user.Role {
			case "admin":
				profile.UserType, profile.OrganizationRole = "internal", "administrator"
				profile.CanManageUsers, profile.AllWorkspaces = true, true
			case "manager":
				profile.UserType, profile.OrganizationRole = "internal", "member"
				profile.CanManageUsers, profile.AllWorkspaces = true, user.ManagerFullAccess
			case "client", "customer":
			default:
				return fmt.Errorf("cannot classify existing account role %q", user.Role)
			}
			if err := db.Create(&profile).Error; err != nil {
				return err
			}
			userByID[user.ID] = user
		}
		// Only exact matches can reuse a template. Preserve nonstandard combinations
		// as tenant-owned legacy roles; never round up to a stronger role.
		findRole := func(caps models.AccessCapabilities, user models.User, file bool) (string, error) {
			internal := user.Role == "admin" || user.Role == "manager"
			permissions := models.RolePermissions{CanRead: caps.CanRead, CanUpload: caps.CanUpload, CanDelete: caps.CanDelete,
				CanInviteExternal: caps.CanShare, CanCreateLinks: caps.CanShare, CanManageAccess: caps.CanShare && internal}
			audience, scope := "external", "workspace"
			if internal {
				audience = "internal"
			}
			if file {
				scope = "file"
			}
			for _, role := range roles {
				projected, _ := role.PermissionsAt(scope)
				if projected == permissions && role.AppliesTo(audience, scope) {
					return role.ID, nil
				}
			}
			bits := 0
			for i, enabled := range []bool{caps.CanRead, caps.CanUpload, caps.CanDelete, caps.CanShare} {
				if enabled {
					bits |= 1 << i
				}
			}
			key := "legacy-" + audience + "-" + scope + "-" + strconv.Itoa(bits)
			for _, role := range roles {
				if role.TemplateKey != nil && *role.TemplateKey == key {
					return role.ID, nil
				}
			}
			actions := []string{}
			for _, action := range []struct {
				enabled bool
				name    string
			}{
				{caps.CanRead, "view"}, {caps.CanUpload, "upload"}, {caps.CanDelete, "delete"}, {caps.CanShare, "share"},
			} {
				if action.enabled {
					actions = append(actions, action.name)
				}
			}
			if len(actions) == 0 {
				actions = append(actions, "no access")
			}
			name := "Legacy " + audience + ": " + strings.Join(actions, ", ")
			if bits == 11 && !internal {
				name = "Client contact (legacy)"
			}
			if file {
				name += " — file"
			}
			role := models.AccessRole{TenantID: tenant.ID, Name: name, Description: "Preserved existing permissions; review before replacing",
				TemplateKey: &key, Audience: audience, AllowWorkspace: !file, AllowFolder: !file, AllowFile: file,
				RolePermissions: permissions, Version: 1}
			if err := db.Create(&role).Error; err != nil {
				return "", err
			}
			roles = append(roles, role)
			return role.ID, nil
		}
		var memberships []models.UserClient
		if err := db.Where("tenant_id = ?", tenant.ID).Find(&memberships).Error; err != nil {
			return err
		}
		for _, membership := range memberships {
			user, ok := userByID[membership.UserID]
			if !ok {
				return fmt.Errorf("membership has missing tenant-local user")
			}
			caps := membership.AccessCapabilities
			if !membership.CapabilitiesSet {
				caps = models.AccessCapabilities{CanRead: true, CanUpload: true, CanDelete: user.Role == "manager", CanShare: true}
			}
			roleID, err := findRole(caps, user, false)
			if err != nil {
				return err
			}
			source := membership.UserID + ":" + membership.ClientID
			assignment := models.RoleAssignment{TenantID: tenant.ID, RoleID: roleID, UserID: &membership.UserID,
				ClientID: membership.ClientID, SourceKind: "membership", SourceID: &source, Version: 1, CreatedAt: membership.CreatedAt}
			if err := db.Create(&assignment).Error; err != nil {
				return err
			}
		}
		var grants []models.ItemGrant
		if err := db.Where("tenant_id = ?", tenant.ID).Find(&grants).Error; err != nil {
			return err
		}
		for _, grant := range grants {
			user, ok := userByID[grant.UserID]
			if !ok {
				return fmt.Errorf("grant has missing tenant-local user")
			}
			var target models.File
			if err := db.Where("tenant_id = ? AND client_id = ? AND id = ?", tenant.ID, grant.ClientID, grant.FileID).First(&target).Error; err != nil {
				return err
			}
			roleID, err := findRole(grant.AccessCapabilities, user, target.Type == "file")
			if err != nil {
				return err
			}
			assignment := models.RoleAssignment{TenantID: tenant.ID, RoleID: roleID, UserID: &grant.UserID,
				ClientID: grant.ClientID, FileID: &grant.FileID, SourceKind: "grant", SourceID: &grant.ID,
				Version: grant.Version, ExpiresAt: grant.ExpiresAt, RevokedAt: grant.RevokedAt, CreatedAt: grant.CreatedAt, UpdatedAt: grant.UpdatedAt}
			if err := db.Create(&assignment).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func downAddAccessRoleFoundation(ctx context.Context, tx *sql.Tx) error {
	// No original authorization data was changed, so rollback restores the old
	// schema without trying to reverse a potentially lossy permission conversion.
	for _, table := range []string{"role_assignments", "access_group_members", "access_groups", "access_roles", "access_profiles"} {
		if _, err := tx.ExecContext(ctx, "DROP TABLE "+table); err != nil {
			return err
		}
	}
	return nil
}
