package main

import (
	"database/sql"
	"testing"

	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/stretchr/testify/require"
)

func TestFileCommentsMigrationRoundTrip(t *testing.T) {
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
			for range 2 {
				tx, err := db.Begin()
				require.NoError(t, err)
				require.NoError(t, upAddFileComments(t.Context(), tx))
				require.NoError(t, tx.Commit())
			}
			require.EqualValues(t, 0, scalarCount(t, db, "SELECT COUNT(*) FROM file_comments"))
			require.EqualValues(t, 0, scalarCount(t, db, "SELECT COUNT(*) FROM file_comment_revisions"))
			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, downAddFileComments(t.Context(), tx))
			require.NoError(t, tx.Commit())
		})
	}
}
