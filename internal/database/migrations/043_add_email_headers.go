package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddEmailHeaders(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	if err := db.AutoMigrate(&models.QueuedEmail{}); err != nil {
		return err
	}
	// Preserve recipient privacy for mail queued before the explicit policy existed.
	return db.Model(&models.QueuedEmail{}).Where("sent_at IS NULL").Update("hide_recipients", true).Error
}

func downAddEmailHeaders(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	if err := db.Migrator().DropColumn(&models.QueuedEmail{}, "ReplyTo"); err != nil {
		return err
	}
	return db.Migrator().DropColumn(&models.QueuedEmail{}, "HideRecipients")
}
