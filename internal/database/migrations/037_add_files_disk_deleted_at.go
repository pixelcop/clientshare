package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddFilesDiskDeletedAt(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}

	m := db.WithContext(ctx).Migrator()
	if !m.HasColumn(&models.File{}, "disk_deleted_at") {
		if err := m.AddColumn(&models.File{}, "DiskDeletedAt"); err != nil {
			return fmt.Errorf("add files.disk_deleted_at: %w", err)
		}
	}
	if !m.HasIndex(&models.File{}, "DiskDeletedAt") {
		if err := m.CreateIndex(&models.File{}, "DiskDeletedAt"); err != nil {
			return fmt.Errorf("create files.disk_deleted_at index: %w", err)
		}
	}
	return nil
}

func downAddFilesDiskDeletedAt(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}

	m := db.WithContext(ctx).Migrator()
	if m.HasIndex(&models.File{}, "DiskDeletedAt") {
		if err := m.DropIndex(&models.File{}, "DiskDeletedAt"); err != nil {
			return fmt.Errorf("drop files.disk_deleted_at index: %w", err)
		}
	}
	if m.HasColumn(&models.File{}, "disk_deleted_at") {
		if err := m.DropColumn(&models.File{}, "disk_deleted_at"); err != nil {
			return fmt.Errorf("drop files.disk_deleted_at: %w", err)
		}
	}
	return nil
}
