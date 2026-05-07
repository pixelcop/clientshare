package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upBackfillPublicIdNanoid, downBackfillPublicIdNanoid)
}

func upBackfillPublicIdNanoid(ctx context.Context, tx *sql.Tx) error {
	tables := []string{"clients", "users", "email_queue", "file_activity"}
	for _, table := range tables {
		if err := backfillTablePublicID(ctx, tx, table); err != nil {
			return err
		}
	}
	return nil
}

func downBackfillPublicIdNanoid(ctx context.Context, tx *sql.Tx) error {
	tables := []string{"clients", "users", "email_queue", "file_activity"}
	for _, table := range tables {
		query := fmt.Sprintf("UPDATE %s SET public_id = NULL", table)
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func backfillTablePublicID(ctx context.Context, tx *sql.Tx, table string) error {
	selectQuery := fmt.Sprintf("SELECT id FROM %s WHERE public_id IS NULL", table)
	rows, err := tx.QueryContext(ctx, selectQuery)
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

	updateQuery := fmt.Sprintf("UPDATE %s SET public_id = ? WHERE id = ?", table)
	for _, id := range ids {
		if err := updateRowPublicID(ctx, tx, updateQuery, id); err != nil {
			return err
		}
	}
	return nil
}

func updateRowPublicID(ctx context.Context, tx *sql.Tx, updateQuery string, id int64) error {
	for attempts := 0; attempts < 5; attempts++ {
		publicID := gonanoid.Must()
		if _, err := tx.ExecContext(ctx, updateQuery, publicID, id); err != nil {
			if isSQLiteUniqueConstraintErr(err) {
				continue
			}
			return err
		}
		return nil
	}

	return fmt.Errorf("failed to assign unique public_id for row id=%d after retries", id)
}

func isSQLiteUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
