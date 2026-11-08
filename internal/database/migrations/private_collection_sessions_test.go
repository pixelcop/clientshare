package main

import (
	"context"
	"database/sql"
	"testing"

	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/stretchr/testify/require"
)

func TestPrivateCollectionSessionMigrationPreservesExistingFiles(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var db *sql.DB
			if engine == "postgres" {
				var cfg dbpkg.DatabaseConfig
				db, cfg = openPostgresMigrationTestDB(t)
				migration.SetMigrationDriver(cfg.Driver)
			} else {
				db = openMigrationTestDB(t)
				migration.SetMigrationDriver(dbpkg.DatabaseDriverSQLite)
			}
			_, err := db.Exec(`CREATE TABLE files (id TEXT PRIMARY KEY, filename TEXT NOT NULL)`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO files VALUES ('legacy', 'unchanged.txt')`)
			require.NoError(t, err)
			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, upAddPrivateCollectionSessions(context.Background(), tx))
			require.NoError(t, tx.Commit())
			var name string
			var session sql.NullString
			require.NoError(t, db.QueryRow(`SELECT filename,upload_session_id FROM files WHERE id='legacy'`).Scan(&name, &session))
			require.Equal(t, "unchanged.txt", name)
			require.False(t, session.Valid)
			tx, err = db.Begin()
			require.NoError(t, err)
			require.NoError(t, downAddPrivateCollectionSessions(context.Background(), tx))
			require.NoError(t, tx.Commit())
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM files WHERE filename='unchanged.txt'`))
		})
	}
}
