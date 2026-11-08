package main

import (
	"context"
	"database/sql"
	"testing"

	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/stretchr/testify/require"
)

func TestAccessInvitationMigrationPreservesGrantsAndIdentityTokens(t *testing.T) {
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
				`CREATE TABLE item_grants (id TEXT PRIMARY KEY)`,
				`CREATE TABLE invite_tokens (id TEXT PRIMARY KEY, token_hash TEXT NOT NULL)`,
				`INSERT INTO item_grants VALUES ('legacy-grant')`,
				`INSERT INTO invite_tokens VALUES ('legacy-identity','unchanged-identity-hash')`,
			} {
				_, err := db.Exec(query)
				require.NoError(t, err)
			}
			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, upAddAccessInvitations(context.Background(), tx))
			require.NoError(t, tx.Commit())
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT version FROM item_grants WHERE id='legacy-grant'`))
			var hash string
			var scope sql.NullString
			require.NoError(t, db.QueryRow(`SELECT token_hash,access_invitation_id FROM invite_tokens WHERE id='legacy-identity'`).Scan(&hash, &scope))
			require.Equal(t, "unchanged-identity-hash", hash)
			require.False(t, scope.Valid)
			_, err = db.Exec(`INSERT INTO access_invitations (id,tenant_id,client_id,email,token_hash,created_by,expires_at) VALUES ('one','tenant','client','recipient@example.test','one','creator','2027-01-01T00:00:00Z')`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO access_invitations (id,tenant_id,client_id,email,token_hash,created_by,expires_at) VALUES ('two','tenant','client','recipient@example.test','two','creator','2027-01-01T00:00:00Z')`)
			require.Error(t, err, "only one pending invitation per normalized recipient and target")
			_, err = db.Exec(`UPDATE access_invitations SET accepted_at=CURRENT_TIMESTAMP WHERE id='one'`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO access_invitations (id,tenant_id,client_id,email,token_hash,created_by,expires_at) VALUES ('two','tenant','client','recipient@example.test','two','creator','2027-01-01T00:00:00Z')`)
			require.NoError(t, err)
			tx, err = db.Begin()
			require.NoError(t, err)
			require.NoError(t, downAddAccessInvitations(context.Background(), tx))
			require.NoError(t, tx.Commit())
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM item_grants WHERE id='legacy-grant'`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM invite_tokens WHERE token_hash='unchanged-identity-hash'`))
		})
	}
}
