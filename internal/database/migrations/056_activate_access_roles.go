package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func upActivateAccessRoles(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	var tenants []models.Tenant
	if err := db.Find(&tenants).Error; err != nil {
		return err
	}
	for _, tenant := range tenants {
		var count int64
		if err := db.Model(&models.AccessRole{}).Where("tenant_id = ?", tenant.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			for _, role := range models.DefaultAccessRoles(tenant.ID) {
				if err := db.Create(&role).Error; err != nil {
					return err
				}
			}
		}
	}
	var users []models.User
	if err := db.Find(&users).Error; err != nil {
		return err
	}
	for _, user := range users {
		profile := models.AccessProfile{TenantID: user.TenantID, UserID: user.ID, UserType: "external", OrganizationRole: "none"}
		if user.Role == "admin" {
			profile.UserType, profile.OrganizationRole, profile.CanManageUsers, profile.AllWorkspaces = "internal", "administrator", true, true
		}
		if user.Role == "manager" {
			profile.UserType, profile.OrganizationRole, profile.CanManageUsers, profile.AllWorkspaces = "internal", "member", true, user.ManagerFullAccess
		}
		if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"user_type", "organization_role", "can_manage_users", "all_workspaces"})}).Create(&profile).Error; err != nil {
			return err
		}
	}
	if err := db.Exec(`CREATE TABLE access_role_catalogs (tenant_id TEXT PRIMARY KEY NOT NULL, FOREIGN KEY(tenant_id) REFERENCES tenants(id) ON DELETE CASCADE)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`INSERT INTO access_role_catalogs (tenant_id) SELECT id FROM tenants`).Error; err != nil {
		return err
	}
	for _, change := range []struct {
		model any
		field string
	}{
		{&models.AccessRole{}, "LegacySharing"}, {&models.UserClient{}, "RoleID"},
		{&models.ItemGrant{}, "RoleID"}, {&models.AccessInvitation{}, "RoleID"},
	} {
		if !db.Migrator().HasColumn(change.model, change.field) {
			if err := db.Migrator().AddColumn(change.model, change.field); err != nil {
				return err
			}
		}
	}
	// A legacy sharing flag also carried provenance/ownership behavior. Retain
	// that contract until an administrator explicitly edits the permissions.
	if err := db.Exec(`UPDATE access_roles SET legacy_sharing = TRUE WHERE template_key LIKE 'legacy-%' AND (can_invite_external OR can_create_links OR can_manage_access)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`UPDATE access_roles SET can_manage_access=TRUE WHERE legacy_sharing=TRUE`).Error; err != nil {
		return err
	}
	// Resnapshot current source records, not the potentially stale 055 shadow
	// assignments. Match exact permissions/audience or create a legacy role.
	var memberships []models.UserClient
	if err := db.Find(&memberships).Error; err != nil {
		return err
	}
	for _, member := range memberships {
		var user models.User
		if err := db.Where("tenant_id = ? AND id = ?", member.TenantID, member.UserID).First(&user).Error; err != nil {
			return err
		}
		caps := member.AccessCapabilities
		if !member.CapabilitiesSet {
			caps = models.AccessCapabilities{CanRead: true, CanUpload: true, CanShare: true, CanDelete: user.Role == "manager"}
		}
		audience := "external"
		if user.Role == "admin" || user.Role == "manager" {
			audience = "internal"
		}
		roleID, err := migrationAccessRole(db, member.TenantID, audience, "workspace", caps)
		if err != nil {
			return err
		}
		if err := db.Model(&models.UserClient{}).Where("tenant_id = ? AND user_id = ? AND client_id = ?", member.TenantID, member.UserID, member.ClientID).Update("role_id", roleID).Error; err != nil {
			return err
		}
	}
	// Item grants and pending invitations retain their original lifecycle. Attach
	// an exact role where possible without changing their capabilities or dates.
	for _, table := range []string{"item_grants", "access_invitations"} {
		var rows []struct {
			ID, TenantID, UserID                    string
			FileID                                  *string
			CanRead, CanUpload, CanDelete, CanShare bool
		}
		userColumn := "user_id"
		if table == "access_invitations" {
			userColumn = "recipient_user_id"
		}
		if err := db.Table(table).Select("id,tenant_id,file_id,can_read,can_upload,can_delete,can_share," + userColumn + " AS user_id").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			scope := "workspace"
			if row.FileID != nil {
				var file models.File
				if err := db.Where("tenant_id = ? AND id = ?", row.TenantID, *row.FileID).First(&file).Error; err != nil {
					return err
				}
				scope = file.Type
			}
			audience := "external"
			if row.UserID != "" {
				var user models.User
				if err := db.Where("tenant_id = ? AND id = ?", row.TenantID, row.UserID).First(&user).Error; err != nil {
					return err
				}
				if user.Role == "admin" || user.Role == "manager" {
					audience = "internal"
				}
			}
			caps := models.AccessCapabilities{CanRead: row.CanRead, CanUpload: row.CanUpload, CanDelete: row.CanDelete, CanShare: row.CanShare}
			roleID, err := migrationAccessRole(db, row.TenantID, audience, scope, caps)
			if err != nil {
				return err
			}
			if err := db.Table(table).Where("tenant_id = ? AND id = ?", row.TenantID, row.ID).Update("role_id", roleID).Error; err != nil {
				return err
			}
		}
	}
	// Direct authority now lives on its original source row. Only group grants
	// use role_assignments at runtime; discard the obsolete migration snapshots.
	return db.Exec("DELETE FROM role_assignments WHERE source_kind IN ('membership','grant')").Error
}

func migrationAccessRole(db *gorm.DB, tenantID, audience, scope string, caps models.AccessCapabilities) (string, error) {
	if scope == "file" {
		caps.CanUpload = false
	}
	var roles []models.AccessRole
	if err := db.Where("tenant_id = ? AND can_read = ? AND can_delete = ? AND (audience = ? OR audience = 'both')", tenantID, caps.CanRead, caps.CanDelete, audience).Order("created_at ASC,id ASC").Find(&roles).Error; err != nil {
		return "", err
	}
	for _, role := range roles {
		p, valid := role.PermissionsAt(scope)
		if !valid || p.CanUpload != caps.CanUpload || (p.CanInviteExternal || p.CanCreateLinks || p.CanManageAccess) != caps.CanShare {
			continue
		}
		if caps.CanShare && !role.LegacySharing {
			continue
		}
		return role.ID, nil
	}
	bits := 0
	for i, enabled := range []bool{caps.CanRead, caps.CanUpload, caps.CanDelete, caps.CanShare} {
		if enabled {
			bits |= 1 << i
		}
	}
	name := fmt.Sprintf("Preserved %s %s access (%d)", audience, scope, bits)
	if audience == "internal" && bits == 15 {
		name = "Workspace manager (legacy)"
		if scope != "workspace" {
			name += " — " + scope
		}
	}
	role := models.AccessRole{TenantID: tenantID, Name: name, Description: "Preserved existing permissions; review before replacing", Audience: audience, AllowWorkspace: scope == "workspace", AllowFolder: scope == "folder", AllowFile: scope == "file", Version: 1,
		RolePermissions: models.RolePermissions{CanRead: caps.CanRead, CanUpload: caps.CanUpload, CanDelete: caps.CanDelete, CanInviteExternal: caps.CanShare, CanCreateLinks: caps.CanShare, CanManageAccess: caps.CanShare}}
	if err := db.Create(&role).Error; err != nil {
		return "", err
	}
	if caps.CanShare {
		if err := db.Exec("UPDATE access_roles SET legacy_sharing=TRUE WHERE tenant_id=? AND id=?", tenantID, role.ID).Error; err != nil {
			return "", err
		}
	}
	return role.ID, nil
}

func downActivateAccessRoles(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	var groupAssignments int64
	if err := db.Model(&models.RoleAssignment{}).Count(&groupAssignments).Error; err != nil {
		return err
	}
	if groupAssignments > 0 {
		return fmt.Errorf("remove group and additive role assignments before downgrading access roles")
	}
	for _, table := range []string{"user_clients", "item_grants", "access_invitations"} {
		var split int64
		if err := db.Table(table + " AS source").Joins("JOIN access_roles ar ON ar.tenant_id=source.tenant_id AND ar.id=source.role_id").Where("NOT ar.legacy_sharing AND (ar.can_invite_external OR ar.can_create_links OR ar.can_manage_access) AND NOT (ar.can_invite_external AND ar.can_create_links AND ar.can_manage_access)").Count(&split).Error; err != nil {
			return err
		}
		if split > 0 {
			return fmt.Errorf("replace assignments with independent sharing rights before downgrading access roles")
		}
	}
	if err := db.Exec("DROP TABLE access_role_catalogs").Error; err != nil {
		return err
	}
	for _, change := range []struct {
		model any
		field string
	}{
		{&models.AccessInvitation{}, "RoleID"}, {&models.ItemGrant{}, "RoleID"},
		{&models.UserClient{}, "RoleID"}, {&models.AccessRole{}, "LegacySharing"},
	} {
		if err := db.WithContext(ctx).Migrator().DropColumn(change.model, change.field); err != nil {
			return err
		}
	}
	return nil
}
