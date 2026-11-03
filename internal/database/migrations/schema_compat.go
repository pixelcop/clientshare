package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

// MySQL does not accept CREATE INDEX IF NOT EXISTS. Keep the already-applied
// SQLite/PostgreSQL migration unchanged and adapt its new MySQL execution path.
func upImmutableObjectsCompatible(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	if db.Dialector.Name() != "mysql" {
		return upImmutableObjects(ctx, tx)
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
	if db.Migrator().HasIndex(&models.File{}, "idx_files_tenant_object") {
		return nil
	}
	return db.Exec("CREATE UNIQUE INDEX idx_files_tenant_object ON files (tenant_id, storage_key)").Error
}
