package main

import (
	"context"
	"database/sql"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

// Reconcile the additive/activation checkpoints without replacing tenant edits
// or resetting source records. This also supports previews that installed an
// early activation checkpoint before its catalog marker was introduced.
func upReconcileAccessRoleActivation(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	if err := removePreviewFileUploadConstraint(db); err != nil {
		return err
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS access_role_catalogs (tenant_id TEXT PRIMARY KEY NOT NULL, FOREIGN KEY(tenant_id) REFERENCES tenants(id) ON DELETE CASCADE)`).Error; err != nil {
		return err
	}
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
		if err := db.Exec("INSERT INTO access_role_catalogs (tenant_id) VALUES (?) ON CONFLICT (tenant_id) DO NOTHING", tenant.ID).Error; err != nil {
			return err
		}
		// Only untouched seed templates get the corrected single-file eligibility.
		// Renamed/customized roles and their permission bundles are not overwritten.
		for _, template := range models.DefaultAccessRoles(tenant.ID) {
			if template.TemplateKey == nil || (*template.TemplateKey != "file-manager" && *template.TemplateKey != "workspace-manager") {
				continue
			}
			var role models.AccessRole
			err := db.Where("tenant_id = ? AND template_key = ? AND version = 1", tenant.ID, *template.TemplateKey).First(&role).Error
			if err == gorm.ErrRecordNotFound {
				continue
			}
			if err != nil {
				return err
			}
			if role.Name == template.Name && role.Description == template.Description && role.Audience == template.Audience && role.RolePermissions == template.RolePermissions && role.AllowWorkspace && role.AllowFolder && !role.AllowFile {
				if err := db.Model(&role).Updates(map[string]any{"allow_file": true, "version": 2}).Error; err != nil {
					return err
				}
			}
		}
	}
	if err := db.Exec("UPDATE access_roles SET can_manage_access=TRUE,version=version+1 WHERE legacy_sharing=TRUE AND NOT can_manage_access").Error; err != nil {
		return err
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
			var existing models.AccessProfile
			if err := db.Where("tenant_id = ? AND user_id = ?", user.TenantID, user.ID).First(&existing).Error; err == nil {
				if existing.UserType == "internal" {
					profile.CanManageUsers = existing.CanManageUsers
				}
			} else if err != gorm.ErrRecordNotFound {
				return err
			}
		}
		if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"user_type", "organization_role", "can_manage_users", "all_workspaces"})}).Create(&profile).Error; err != nil {
			return err
		}
	}
	var memberships []models.UserClient
	if err := db.Find(&memberships).Error; err != nil {
		return err
	}
	for _, member := range memberships {
		if member.RoleID != nil {
			var role models.AccessRole
			if err := db.Where("tenant_id = ? AND id = ?", member.TenantID, *member.RoleID).First(&role).Error; err != nil {
				return err
			}
			if member.CapabilitiesSet || role.LegacySharing || role.TemplateKey == nil || *role.TemplateKey != "workspace-manager" {
				continue
			}
		}
		var user models.User
		if err := db.Where("tenant_id = ? AND id = ?", member.TenantID, member.UserID).First(&user).Error; err != nil {
			return err
		}
		caps := member.AccessCapabilities
		if !member.CapabilitiesSet {
			caps = models.AccessCapabilities{CanRead: true, CanUpload: true, CanDelete: user.Role == "manager", CanShare: true}
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
		if err := db.Table(table).Select("id,tenant_id,file_id,can_read,can_upload,can_delete,can_share," + userColumn + " AS user_id").Where("role_id IS NULL").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			scope := "workspace"
			if row.FileID != nil {
				var file models.File
				err := db.Where("tenant_id = ? AND id = ?", row.TenantID, *row.FileID).First(&file).Error
				if err == gorm.ErrRecordNotFound {
					continue
				}
				if err != nil {
					return err
				}
				scope = file.Type
			}
			audience := "external"
			if row.UserID != "" {
				var user models.User
				err := db.Where("tenant_id = ? AND id = ?", row.TenantID, row.UserID).First(&user).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				}
				if user.Role == "admin" || user.Role == "manager" {
					audience = "internal"
				}
			}
			roleID, err := migrationAccessRole(db, row.TenantID, audience, scope, models.AccessCapabilities{CanRead: row.CanRead, CanUpload: row.CanUpload, CanDelete: row.CanDelete, CanShare: row.CanShare})
			if err != nil {
				return err
			}
			if err := db.Table(table).Where("tenant_id = ? AND id = ?", row.TenantID, row.ID).Update("role_id", roleID).Error; err != nil {
				return err
			}
		}
	}
	// These were migration-owned snapshots, never independently granted access.
	// Keeping them would resurrect permissions after a direct role change.
	return db.Exec("DELETE FROM role_assignments WHERE source_kind IN ('membership','grant')").Error
}

// Data reconciliation is intentionally retained on downgrade. Original source
// capabilities, invitation credentials and IDs have not been changed. The 056
// downgrade guard prevents collapsing independent rights into legacy sharing.
func downReconcileAccessRoleActivation(context.Context, *sql.Tx) error { return nil }

func removePreviewFileUploadConstraint(db *gorm.DB) error {
	if db.Dialector.Name() == "sqlite" {
		var schema string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='access_roles'").Scan(&schema).Error; err != nil {
			return err
		}
		old := "CHECK(NOT allow_file OR NOT can_upload),"
		if !strings.Contains(schema, old) {
			return nil
		}
		// SQLite cannot drop an unnamed CHECK in place. Preserve assignment rows
		// explicitly: dropping a parent can fire the old FK's cascade even while
		// constraints are deferred. Recreate the same identity, then restore rows
		// with their exact IDs, provenance, revisions and timestamps atomically.
		var assignments []models.RoleAssignment
		if err := db.Find(&assignments).Error; err != nil {
			return err
		}
		if err := db.Exec("PRAGMA defer_foreign_keys = ON").Error; err != nil {
			return err
		}
		create := strings.Replace(schema, "CREATE TABLE access_roles", "CREATE TABLE access_roles_reconciled", 1)
		create = strings.Replace(create, old, "", 1)
		if err := db.Exec(create).Error; err != nil {
			return err
		}
		if err := db.Exec("INSERT INTO access_roles_reconciled SELECT * FROM access_roles").Error; err != nil {
			return err
		}
		if err := db.Exec("DROP TABLE access_roles").Error; err != nil {
			return err
		}
		if err := db.Exec("ALTER TABLE access_roles_reconciled RENAME TO access_roles").Error; err != nil {
			return err
		}
		if len(assignments) > 0 {
			if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&assignments, 50).Error; err != nil {
				return err
			}
		}
		return nil
	}
	// PostgreSQL names unnamed CHECKs itself. Inspect definitions rather than
	// guessing its generated constraint name.
	var checks []struct{ Name, Definition string }
	if err := db.Raw("SELECT conname AS name,pg_get_constraintdef(oid) AS definition FROM pg_constraint WHERE conrelid='access_roles'::regclass AND contype='c'").Scan(&checks).Error; err != nil {
		return err
	}
	for _, check := range checks {
		if strings.Contains(check.Definition, "NOT allow_file") && strings.Contains(check.Definition, "NOT can_upload") {
			name := strings.ReplaceAll(check.Name, `"`, `""`)
			if err := db.Exec(`ALTER TABLE access_roles DROP CONSTRAINT "` + name + `"`).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
