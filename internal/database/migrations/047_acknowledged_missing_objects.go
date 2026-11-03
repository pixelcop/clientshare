package main

import (
	"context"
	"database/sql"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAcknowledgedMissingObjects(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	if db.Migrator().HasColumn(&models.File{}, "StorageMissing") {
		return nil
	}
	return db.WithContext(ctx).Migrator().AddColumn(&models.File{}, "StorageMissing")
}
func downAcknowledgedMissingObjects(ctx context.Context, tx *sql.Tx) error { return nil }
