package main

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/oklog/ulid/v2"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upBackfillUlidValues, downBackfillUlidValues)
}

func upBackfillUlidValues(ctx context.Context, tx *sql.Tx) error {
	if err := backfillTableULID(ctx, tx, "clients"); err != nil {
		return err
	}
	if err := backfillTableULID(ctx, tx, "users"); err != nil {
		return err
	}
	if err := backfillTableULID(ctx, tx, "files"); err != nil {
		return err
	}
	if err := backfillTableULID(ctx, tx, "secure_links"); err != nil {
		return err
	}
	if err := backfillTableULID(ctx, tx, "file_activity"); err != nil {
		return err
	}
	if err := backfillTableULID(ctx, tx, "email_queue"); err != nil {
		return err
	}
	if err := backfillTableULID(ctx, tx, "password_reset_tokens"); err != nil {
		return err
	}

	updates := []string{
		"UPDATE clients SET root_folder_id_ulid = (SELECT f.id_ulid FROM files f WHERE f.id = clients.root_folder_id)",
		"UPDATE users SET client_id_ulid = (SELECT c.id_ulid FROM clients c WHERE c.id = users.client_id)",
		"UPDATE files SET client_id_ulid = (SELECT c.id_ulid FROM clients c WHERE c.id = files.client_id)",
		"UPDATE files SET user_id_ulid = (SELECT u.id_ulid FROM users u WHERE u.id = files.user_id)",
		"UPDATE files SET folder_id_ulid = (SELECT f2.id_ulid FROM files f2 WHERE f2.id = files.folder_id)",
		"UPDATE secure_links SET client_id_ulid = (SELECT c.id_ulid FROM clients c WHERE c.id = secure_links.client_id)",
		"UPDATE secure_links SET created_by_ulid = (SELECT u.id_ulid FROM users u WHERE u.id = secure_links.created_by)",
		"UPDATE file_activity SET client_id_ulid = (SELECT c.id_ulid FROM clients c WHERE c.id = file_activity.client_id)",
		"UPDATE file_activity SET user_id_ulid = (SELECT u.id_ulid FROM users u WHERE u.id = file_activity.user_id)",
		"UPDATE file_activity SET secure_link_id_ulid = (SELECT sl.id_ulid FROM secure_links sl WHERE sl.id = file_activity.secure_link_id)",
		"UPDATE email_queue SET client_id_ulid = (SELECT c.id_ulid FROM clients c WHERE c.id = email_queue.client_id)",
		"UPDATE password_reset_tokens SET user_id_ulid = (SELECT u.id_ulid FROM users u WHERE u.id = password_reset_tokens.user_id)",
	}
	for _, stmt := range updates {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	if err := convertEmailQueueFileCSVToULID(ctx, tx); err != nil {
		return err
	}

	return nil
}

func downBackfillUlidValues(ctx context.Context, tx *sql.Tx) error {
	for _, table := range []string{"clients", "users", "files", "secure_links", "file_activity", "email_queue", "password_reset_tokens"} {
		query := fmt.Sprintf("UPDATE %s SET id_ulid = NULL", table)
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}

	updates := []string{
		"UPDATE clients SET root_folder_id_ulid = NULL",
		"UPDATE users SET client_id_ulid = NULL",
		"UPDATE files SET client_id_ulid = NULL, user_id_ulid = NULL, folder_id_ulid = NULL",
		"UPDATE secure_links SET client_id_ulid = NULL, created_by_ulid = NULL",
		"UPDATE file_activity SET client_id_ulid = NULL, user_id_ulid = NULL, secure_link_id_ulid = NULL",
		"UPDATE email_queue SET client_id_ulid = NULL",
		"UPDATE password_reset_tokens SET user_id_ulid = NULL",
	}
	for _, stmt := range updates {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func backfillTableULID(ctx context.Context, tx *sql.Tx, table string) error {
	query := fmt.Sprintf("SELECT id FROM %s WHERE id_ulid IS NULL", table)
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	updateQuery := fmt.Sprintf("UPDATE %s SET id_ulid = ? WHERE id = ?", table)
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, updateQuery, ulid.Make().String(), id); err != nil {
			return err
		}
	}
	return nil
}

func convertEmailQueueFileCSVToULID(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "SELECT id, file_ids_csv FROM email_queue")
	if err != nil {
		return err
	}
	defer rows.Close()

	type rowData struct {
		id    int64
		files string
	}

	var queueRows []rowData
	for rows.Next() {
		var row rowData
		if err := rows.Scan(&row.id, &row.files); err != nil {
			return err
		}
		queueRows = append(queueRows, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, row := range queueRows {
		if strings.TrimSpace(row.files) == "" {
			continue
		}

		numericIDs := strings.Split(row.files, ",")
		ulids := make([]string, 0, len(numericIDs))
		for _, numericID := range numericIDs {
			numericID = strings.TrimSpace(numericID)
			if numericID == "" {
				continue
			}
			idNum, err := strconv.ParseInt(numericID, 10, 64)
			if err != nil {
				continue
			}
			var ulidValue string
			if err := tx.QueryRowContext(ctx, "SELECT id_ulid FROM files WHERE id = ?", idNum).Scan(&ulidValue); err != nil {
				continue
			}
			if ulidValue != "" {
				ulids = append(ulids, ulidValue)
			}
		}

		if _, err := tx.ExecContext(ctx, "UPDATE email_queue SET file_ids_csv = ? WHERE id = ?", strings.Join(ulids, ","), row.id); err != nil {
			return err
		}
	}

	return nil
}
