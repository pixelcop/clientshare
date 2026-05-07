package main

import (
	"context"
	"database/sql"

	"github.com/oklog/ulid/v2"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upBackfillFeedUploadEvents, downBackfillFeedUploadEvents)
}

func upBackfillFeedUploadEvents(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, client_id, user_id, file_path, created_at
		FROM file_activity
		WHERE action = 'upload'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	insertStmt := `
		INSERT OR IGNORE INTO feed_events (
			id,
			event_type,
			client_id,
			actor_user_id,
			subject_user_id,
			source_type,
			source_id,
			file_path,
			created_at
		) VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?)
	`

	for rows.Next() {
		var sourceID string
		var clientID string
		var actorUserID sql.NullString
		var filePath string
		var createdAt string
		if err := rows.Scan(&sourceID, &clientID, &actorUserID, &filePath, &createdAt); err != nil {
			return err
		}

		if _, err := tx.ExecContext(
			ctx,
			insertStmt,
			ulid.Make().String(),
			"file_uploaded",
			clientID,
			actorUserID,
			"file_activity",
			sourceID,
			filePath,
			createdAt,
		); err != nil {
			return err
		}
	}

	return rows.Err()
}

func downBackfillFeedUploadEvents(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM feed_events
		WHERE event_type = 'file_uploaded' AND source_type = 'file_activity'
	`)
	return err
}
