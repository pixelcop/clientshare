package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddScopedAccess(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	m := db.Migrator()
	for _, change := range []struct {
		model           any
		fields, indexes []string
	}{
		{&models.User{}, []string{"ManagerFullAccess"}, nil},
		{&models.UserClient{}, []string{"CapabilitiesSet", "CanRead", "CanUpload", "CanDelete", "CanShare", "AccessSource", "InvitedBy"}, []string{"InvitedBy"}},
		{&models.SecureLink{}, []string{"FileID"}, []string{"FileID"}},
	} {
		for _, field := range change.fields {
			if !m.HasColumn(change.model, field) {
				if err := m.AddColumn(change.model, field); err != nil {
					return err
				}
			}
		}
		for _, index := range change.indexes {
			if !m.HasIndex(change.model, index) {
				if err := m.CreateIndex(change.model, index); err != nil {
					return err
				}
			}
		}
	}
	// This is a one-time compatibility backfill, not a default for new managers.
	if err := db.Exec("UPDATE users SET manager_full_access = TRUE WHERE role = 'manager'").Error; err != nil {
		return err
	}
	// Composite references prevent mixed-tenant/client grant targets. Existing
	// tables retain their current keys; these additive indexes support the FKs.
	timestampType := "TIMESTAMP"
	if db.Dialector.Name() == "postgres" {
		timestampType = "TIMESTAMPTZ"
	}
	for _, query := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_users_access_identity ON users (tenant_id, id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_access_identity ON clients (tenant_id, id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_files_access_identity ON files (tenant_id, client_id, id)",
		fmt.Sprintf(`CREATE TABLE item_grants (
			id TEXT PRIMARY KEY NOT NULL,
			tenant_id TEXT NOT NULL, client_id TEXT NOT NULL,
			user_id TEXT NOT NULL, file_id TEXT NOT NULL,
			can_read BOOLEAN NOT NULL DEFAULT FALSE,
			can_upload BOOLEAN NOT NULL DEFAULT FALSE,
			can_delete BOOLEAN NOT NULL DEFAULT FALSE,
			can_share BOOLEAN NOT NULL DEFAULT FALSE,
			created_by TEXT NOT NULL, created_at %[1]s, updated_at %[1]s,
			expires_at %[1]s, revoked_at %[1]s,
			FOREIGN KEY (tenant_id, client_id) REFERENCES clients (tenant_id, id),
			FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id),
			FOREIGN KEY (tenant_id, client_id, file_id) REFERENCES files (tenant_id, client_id, id)
		)`, timestampType),
		"CREATE UNIQUE INDEX idx_item_grants_recipient ON item_grants (tenant_id, user_id, file_id)",
		"CREATE INDEX idx_item_grants_target ON item_grants (tenant_id, client_id, file_id)",
		"CREATE INDEX idx_item_grants_revoked_at ON item_grants (revoked_at)",
	} {
		if err := db.Exec(query).Error; err != nil {
			return err
		}
	}
	return nil
}

func downAddScopedAccess(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	if err := m.DropTable(&models.ItemGrant{}); err != nil {
		return err
	}
	for _, index := range []string{"idx_users_access_identity", "idx_clients_access_identity", "idx_files_access_identity"} {
		if err := db.WithContext(ctx).Exec("DROP INDEX " + index).Error; err != nil {
			return err
		}
	}
	for _, change := range []struct {
		model           any
		fields, indexes []string
	}{
		{&models.SecureLink{}, []string{"FileID"}, []string{"FileID"}},
		{&models.UserClient{}, []string{"CapabilitiesSet", "CanRead", "CanUpload", "CanDelete", "CanShare", "AccessSource", "InvitedBy"}, []string{"InvitedBy"}},
		{&models.User{}, []string{"ManagerFullAccess"}, nil},
	} {
		for _, index := range change.indexes {
			if err := m.DropIndex(change.model, index); err != nil {
				return err
			}
		}
		for _, field := range change.fields {
			if err := m.DropColumn(change.model, field); err != nil {
				return err
			}
		}
	}
	return nil
}
