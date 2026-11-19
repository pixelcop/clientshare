package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upCommentNotifications(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	if err := db.WithContext(ctx).AutoMigrate(&models.FileCommentParticipant{}); err != nil {
		return err
	}
	if !db.WithContext(ctx).Migrator().HasColumn(&models.QueuedEmail{}, "FileCommentID") {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE email_queue ADD COLUMN file_comment_id VARCHAR(26)"); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_email_queue_file_comment_id ON email_queue (file_comment_id)")
	return err
}

func downCommentNotifications(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM email_queue WHERE file_comment_id IS NOT NULL"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP INDEX idx_email_queue_file_comment_id"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE email_queue DROP COLUMN file_comment_id"); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DROP TABLE file_comment_participants")
	return err
}
