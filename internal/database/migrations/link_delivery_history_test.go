package main

import (
	"context"
	"database/sql"
	"testing"

	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/stretchr/testify/require"
)

func TestLinkDeliveryMigrationPreservesLegacyRecords(t *testing.T) {
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
			// An actual pre-049 schema, not AutoMigrate with today's structs.
			_, err := db.Exec(`CREATE TABLE secure_links (id TEXT PRIMARY KEY, token TEXT NOT NULL, access_type TEXT NOT NULL, expires_at TEXT NOT NULL)`)
			require.NoError(t, err)
			_, err = db.Exec(`CREATE TABLE email_queue (id TEXT PRIMARY KEY)`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO secure_links (id,token,access_type,expires_at) VALUES ('legacy','unchanged-token','view','2027-01-01T00:00:00Z')`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO email_queue (id) VALUES ('legacy-email')`)
			require.NoError(t, err)
			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, upAddLinkDeliveryHistory(context.Background(), tx))
			require.NoError(t, tx.Commit())
			var token, mode, expiry, hash string
			var revoked, key sql.NullString
			require.NoError(t, db.QueryRow(`SELECT token,access_type,expires_at,revoked_at,creation_key,creation_hash FROM secure_links WHERE id='legacy'`).Scan(&token, &mode, &expiry, &revoked, &key, &hash))
			require.Equal(t, "unchanged-token", token)
			require.Equal(t, "view", mode)
			require.Equal(t, "2027-01-01T00:00:00Z", expiry)
			require.False(t, revoked.Valid)
			require.False(t, key.Valid)
			require.Empty(t, hash)
			var dedup, target, canceled sql.NullString
			require.NoError(t, db.QueryRow(`SELECT deduplication_key,secure_link_id,canceled_at FROM email_queue WHERE id='legacy-email'`).Scan(&dedup, &target, &canceled))
			require.False(t, dedup.Valid)
			require.False(t, target.Valid)
			require.False(t, canceled.Valid)
		})
	}
}
