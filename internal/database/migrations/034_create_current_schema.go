package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
	"gorm.io/gorm"
)

func checkAndRenameColumn(m gorm.Migrator, model any, name string) error {
	if m.HasColumn(model, strings.ToUpper(name)) {
		err := m.RenameColumn(model, strings.ToUpper(name), strings.ToLower(name))
		if err != nil {
			return fmt.Errorf("rename %s column: %w", name, err)
		}
	}
	return nil
}

func fixExistingDB(ctx context.Context, db *gorm.DB) error {
	// if db already exists with uppercase column names, rename them before automigrating the new schema
	m := db.WithContext(ctx).Migrator()
	if !m.HasTable(&models.Tenant{}) {
		return nil // no existing tables, nothing to fix
	}

	err := checkAndRenameColumn(m, &models.TenantEntitlement{}, "status")
	if err != nil {
		return err
	}
	err = checkAndRenameColumn(m, &models.File{}, "type")
	if err != nil {
		return err
	}
	err = checkAndRenameColumn(m, &models.FileActivity{}, "action")
	if err != nil {
		return err
	}
	err = checkAndRenameColumn(m, &models.AuditEvent{}, "action")
	if err != nil {
		return err
	}
	return nil
}

func upCreateCurrentSchema(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}

	fixExistingDB(ctx, db)

	// get up to date or create for new db
	if err := db.WithContext(ctx).AutoMigrate(
		&models.Tenant{},
		&models.TenantDomain{},
		&models.TenantSettings{},
		&models.TenantEntitlement{},
		&models.Client{},
		&models.User{},
		&models.File{},
		&models.SecureLink{},
		&models.FileActivity{},
		&models.QueuedEmail{},
		&models.PasswordResetToken{},
		&models.InviteToken{},
		&models.UserClient{},
		&models.FeedEvent{},
		&models.AuditEvent{},
	); err != nil {
		return fmt.Errorf("automigrate current schema: %w", err)
	}
	return nil
}

func downCreateCurrentSchema(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	if err := db.WithContext(ctx).Migrator().DropTable(
		&models.FeedEvent{},
		&models.UserClient{},
		&models.InviteToken{},
		&models.PasswordResetToken{},
		&models.QueuedEmail{},
		&models.FileActivity{},
		&models.SecureLink{},
		&models.File{},
		&models.User{},
		&models.Client{},
		&models.AuditEvent{},
		&models.TenantEntitlement{},
		&models.TenantSettings{},
		&models.TenantDomain{},
		&models.Tenant{},
	); err != nil {
		return fmt.Errorf("drop current schema tables: %w", err)
	}
	return nil
}
