package main

import (
	"context"
	"database/sql"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upImmutableObjects(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	if err := db.AutoMigrate(&models.ReadableView{}); err != nil {
		return err
	}
	for _, field := range []string{"StorageKey", "Checksum"} {
		if !db.Migrator().HasColumn(&models.File{}, field) {
			if err := db.Migrator().AddColumn(&models.File{}, field); err != nil {
				return err
			}
		}
	}
	return db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_files_tenant_object ON files (tenant_id, storage_key)").Error
}

func downImmutableObjects(ctx context.Context, tx *sql.Tx) error {
	// Deliberately preserve additive identity columns: rolling back application
	// code must never discard the only reference to newly uploaded objects.
	return nil
}
