package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/stretchr/testify/require"
)

func TestScopedAccessMigrationPreservesLegacyAndConstrainsGrants(t *testing.T) {
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
			// Frozen pre-050 shapes: old migrations AutoMigrate current models and
			// therefore alone cannot exercise column addition on a real upgrade.
			for _, query := range []string{
				`CREATE TABLE users (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, role TEXT NOT NULL)`,
				`CREATE TABLE clients (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL)`,
				`CREATE TABLE files (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, client_id TEXT NOT NULL)`,
				`CREATE TABLE user_clients (tenant_id TEXT NOT NULL, user_id TEXT NOT NULL, client_id TEXT NOT NULL, PRIMARY KEY (user_id, client_id))`,
				`CREATE TABLE secure_links (id TEXT PRIMARY KEY, token TEXT NOT NULL, access_type TEXT NOT NULL, expires_at TEXT NOT NULL)`,
				`INSERT INTO users VALUES ('manager','tenant','manager'), ('member','tenant','client'), ('admin','tenant','admin'), ('foreign','other','client')`,
				`INSERT INTO clients VALUES ('client','tenant'), ('other-client','tenant')`,
				`INSERT INTO files VALUES ('file','tenant','client'), ('other-file','tenant','other-client')`,
				`INSERT INTO user_clients VALUES ('tenant','member','client')`,
				`INSERT INTO secure_links VALUES ('legacy','unchanged-token','view','2027-01-01T00:00:00Z')`,
			} {
				_, err := db.Exec(query)
				require.NoError(t, err)
			}
			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, upAddScopedAccess(context.Background(), tx))
			require.NoError(t, tx.Commit())

			var token, mode, expiry string
			var target sql.NullString
			require.NoError(t, db.QueryRow(`SELECT token,access_type,expires_at,file_id FROM secure_links WHERE id='legacy'`).Scan(&token, &mode, &expiry, &target))
			require.Equal(t, "unchanged-token", token)
			require.Equal(t, "view", mode)
			require.Equal(t, "2027-01-01T00:00:00Z", expiry)
			require.False(t, target.Valid)

			var set, read, upload, del, share, full bool
			var source string
			var inviter sql.NullString
			require.NoError(t, db.QueryRow(`SELECT capabilities_set,can_read,can_upload,can_delete,can_share,access_source,invited_by FROM user_clients`).Scan(&set, &read, &upload, &del, &share, &source, &inviter))
			require.False(t, set || read || upload || del || share)
			require.Equal(t, "independent", source)
			require.False(t, inviter.Valid)
			require.NoError(t, db.QueryRow(`SELECT manager_full_access FROM users WHERE id='manager'`).Scan(&full))
			require.True(t, full)
			require.NoError(t, db.QueryRow(`SELECT manager_full_access FROM users WHERE id='admin'`).Scan(&full))
			require.False(t, full)
			_, err = db.Exec(`INSERT INTO users (id,tenant_id,role) VALUES ('new-manager','tenant','manager')`)
			require.NoError(t, err)
			require.NoError(t, db.QueryRow(`SELECT manager_full_access FROM users WHERE id='new-manager'`).Scan(&full))
			require.False(t, full)
			require.Equal(t, int64(0), scalarCount(t, db, `SELECT COUNT(*) FROM item_grants`))

			for _, query := range []string{
				`INSERT INTO item_grants (id,tenant_id,client_id,user_id,file_id,created_by) VALUES ('bad-user','tenant','client','foreign','file','manager')`,
				`INSERT INTO item_grants (id,tenant_id,client_id,user_id,file_id,created_by) VALUES ('bad-file','tenant','client','member','other-file','manager')`,
				`INSERT INTO item_grants (id,tenant_id,client_id,user_id,file_id,created_by) VALUES ('missing','tenant','client','member','missing','manager')`,
			} {
				_, err := db.Exec(query)
				require.Error(t, err)
			}
			_, err = db.Exec(`INSERT INTO item_grants (id,tenant_id,client_id,user_id,file_id,created_by) VALUES ('good','tenant','client','member','file','removed-creator')`)
			require.NoError(t, err)
			grantExpiry := time.Date(2026, 11, 8, 14, 0, 0, 0, time.FixedZone("EST", -5*3600))
			_, err = db.Exec("UPDATE item_grants SET expires_at="+bindVar(engine, 1)+" WHERE id='good'", grantExpiry)
			require.NoError(t, err)
			var storedExpiry time.Time
			require.NoError(t, db.QueryRow(`SELECT expires_at FROM item_grants WHERE id='good'`).Scan(&storedExpiry))
			require.True(t, storedExpiry.Equal(grantExpiry), "grant expiry must preserve the real instant")
			_, err = db.Exec(`INSERT INTO item_grants (id,tenant_id,client_id,user_id,file_id,created_by) VALUES ('duplicate','tenant','client','member','file','manager')`)
			require.Error(t, err)
			// Revocation retains the same current row, rather than creating a
			// second active grant. History is recorded separately in audit_events.
			_, err = db.Exec(`UPDATE item_grants SET revoked_at=CURRENT_TIMESTAMP WHERE id='good'`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO item_grants (id,tenant_id,client_id,user_id,file_id,created_by) VALUES ('duplicate','tenant','client','member','file','manager')`)
			require.Error(t, err)
			tx, err = db.Begin()
			require.NoError(t, err)
			require.NoError(t, downAddScopedAccess(context.Background(), tx))
			require.NoError(t, tx.Commit())
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM secure_links WHERE token='unchanged-token'`))
		})
	}
}
