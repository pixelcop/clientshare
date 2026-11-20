package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upDocumentRequestGuidance(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	m := db.Migrator()
	if !m.HasColumn(&models.DocumentRequest{}, "Description") {
		if err := m.AddColumn(&models.DocumentRequest{}, "Description"); err != nil {
			return err
		}
	}
	if !m.HasColumn(&models.QueuedEmail{}, "DocumentRequestUpdateID") {
		if err := m.AddColumn(&models.QueuedEmail{}, "DocumentRequestUpdateID"); err != nil {
			return err
		}
	}
	return db.AutoMigrate(&models.DocumentRequestUpdate{})
}

func downDocumentRequestGuidance(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range []string{
		"DELETE FROM email_queue WHERE document_request_update_id IS NOT NULL",
		"ALTER TABLE email_queue DROP COLUMN document_request_update_id",
		"ALTER TABLE document_requests DROP COLUMN description",
		"DROP TABLE document_request_updates",
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
