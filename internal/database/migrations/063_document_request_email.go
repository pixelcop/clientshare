package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upDocumentRequestEmail(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	if m.HasColumn(&models.QueuedEmail{}, "DocumentRequestID") {
		return nil
	}
	return m.AddColumn(&models.QueuedEmail{}, "DocumentRequestID")
}

func downDocumentRequestEmail(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM email_queue WHERE document_request_id IS NOT NULL"); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "ALTER TABLE email_queue DROP COLUMN document_request_id")
	return err
}
