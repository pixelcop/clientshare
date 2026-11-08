package main

import (
	"context"
	"database/sql"
	"testing"

	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/stretchr/testify/require"
)

func TestInvitationDeliveryContextPreservesExistingQueue(t *testing.T) {
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
			_, err := db.Exec(`CREATE TABLE email_queue (id TEXT PRIMARY KEY, body TEXT NOT NULL); INSERT INTO email_queue (id,body) VALUES ('existing','preserved')`)
			require.NoError(t, err)
			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, upAddInvitationDeliveryContext(context.Background(), tx))
			require.NoError(t, tx.Commit())
			requireColumnPresent(t, db, "email_queue", "access_invitation_id")
			requireColumnPresent(t, db, "email_queue", "identity_invite_id")
			require.EqualValues(t, 1, scalarCount(t, db, `SELECT COUNT(*) FROM email_queue WHERE body='preserved' AND access_invitation_id IS NULL AND identity_invite_id IS NULL`))
			tx, err = db.Begin()
			require.NoError(t, err)
			require.NoError(t, downAddInvitationDeliveryContext(context.Background(), tx))
			require.NoError(t, tx.Commit())
			require.EqualValues(t, 1, scalarCount(t, db, `SELECT COUNT(*) FROM email_queue WHERE body='preserved'`))
		})
	}
}
