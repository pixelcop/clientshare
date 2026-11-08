package main

import (
	"context"
	"database/sql"
	"testing"

	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/stretchr/testify/require"
)

func TestNotificationContextMigrationCancelsOnlyUnboundUnsentUploadBatches(t *testing.T) {
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
			for _, query := range []string{
				`CREATE TABLE email_queue (id TEXT PRIMARY KEY, file_ids_csv TEXT NOT NULL DEFAULT '', sent_at TIMESTAMP, canceled_at TIMESTAMP)`,
				`INSERT INTO email_queue (id,file_ids_csv) VALUES ('pending-upload','file-one,file-two'), ('identity',''), ('link','')`,
				`INSERT INTO email_queue (id,file_ids_csv,sent_at) VALUES ('sent-upload','file-one',CURRENT_TIMESTAMP)`,
			} {
				_, err := db.Exec(query)
				require.NoError(t, err)
			}
			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, upAddNotificationAccessContext(context.Background(), tx))
			require.NoError(t, tx.Commit())
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM email_queue WHERE canceled_at IS NOT NULL`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM email_queue WHERE id='pending-upload' AND canceled_at IS NOT NULL`))
			require.Equal(t, int64(3), scalarCount(t, db, `SELECT COUNT(*) FROM email_queue WHERE canceled_at IS NULL`))
			tx, err = db.Begin()
			require.NoError(t, err)
			require.NoError(t, downAddNotificationAccessContext(context.Background(), tx))
			require.NoError(t, tx.Commit())
			require.Equal(t, int64(4), scalarCount(t, db, `SELECT COUNT(*) FROM email_queue`))
		})
	}
}
