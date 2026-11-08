package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddPrivateCollectionSessions(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	if !m.HasColumn(&models.File{}, "UploadSessionID") {
		if err := m.AddColumn(&models.File{}, "UploadSessionID"); err != nil {
			return err
		}
	}
	if !m.HasIndex(&models.File{}, "UploadSessionID") {
		return m.CreateIndex(&models.File{}, "UploadSessionID")
	}
	return nil
}

func downAddPrivateCollectionSessions(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	if err := m.DropIndex(&models.File{}, "UploadSessionID"); err != nil {
		return err
	}
	return m.DropColumn(&models.File{}, "UploadSessionID")
}
