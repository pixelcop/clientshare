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

func TestAccessRoleFoundationPreservesLegacyAuthority(t *testing.T) {
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
			// Frozen source schema, rather than AutoMigrate of new Go models.
			for _, query := range []string{
				`CREATE TABLE tenants (id TEXT PRIMARY KEY)`,
				`CREATE TABLE users (id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,role TEXT NOT NULL,manager_full_access BOOLEAN NOT NULL DEFAULT FALSE)`,
				`CREATE TABLE clients (id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL)`,
				`CREATE TABLE files (id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,client_id TEXT NOT NULL,type TEXT NOT NULL)`,
				`CREATE TABLE user_clients (tenant_id TEXT NOT NULL,user_id TEXT NOT NULL,client_id TEXT NOT NULL,capabilities_set BOOLEAN NOT NULL DEFAULT FALSE,can_read BOOLEAN NOT NULL DEFAULT FALSE,can_upload BOOLEAN NOT NULL DEFAULT FALSE,can_delete BOOLEAN NOT NULL DEFAULT FALSE,can_share BOOLEAN NOT NULL DEFAULT FALSE,PRIMARY KEY(user_id,client_id))`,
				`CREATE TABLE item_grants (id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,user_id TEXT NOT NULL,client_id TEXT NOT NULL,file_id TEXT NOT NULL,can_read BOOLEAN NOT NULL DEFAULT FALSE,can_upload BOOLEAN NOT NULL DEFAULT FALSE,can_delete BOOLEAN NOT NULL DEFAULT FALSE,can_share BOOLEAN NOT NULL DEFAULT FALSE,version BIGINT NOT NULL DEFAULT 1,expires_at TIMESTAMP,revoked_at TIMESTAMP)`,
				`CREATE TABLE secure_links (id TEXT PRIMARY KEY,token TEXT NOT NULL)`,
				`CREATE TABLE access_invitations (id TEXT PRIMARY KEY,token_hash TEXT NOT NULL)`,
				`INSERT INTO tenants VALUES ('tenant'),('other')`,
				`INSERT INTO users VALUES ('admin','tenant','admin',FALSE),('manager','tenant','manager',FALSE),('full','tenant','manager',TRUE),('client','tenant','client',FALSE),('customer','tenant','customer',FALSE),('foreign','other','client',FALSE)`,
				`INSERT INTO clients VALUES ('workspace','tenant'),('other-workspace','other')`,
				`INSERT INTO files VALUES ('file','tenant','workspace','file'),('foreign-file','other','other-workspace','file')`,
				`INSERT INTO user_clients (tenant_id,user_id,client_id) VALUES ('tenant','client','workspace'),('tenant','manager','workspace')`,
				`INSERT INTO user_clients (tenant_id,user_id,client_id,capabilities_set,can_read) VALUES ('tenant','customer','workspace',TRUE,TRUE)`,
				`INSERT INTO item_grants (id,tenant_id,user_id,client_id,file_id,can_read,version,expires_at,revoked_at) VALUES ('grant','tenant','client','workspace','file',TRUE,7,'2027-01-01 12:00:00','2026-01-01 12:00:00')`,
				`INSERT INTO secure_links VALUES ('link','unchanged-bearer')`,
				`INSERT INTO access_invitations VALUES ('invite','unchanged-hash')`,
			} {
				_, err := db.Exec(query)
				require.NoError(t, err)
			}
			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, upAddAccessRoleFoundation(context.Background(), tx))
			require.NoError(t, tx.Commit())
			require.Equal(t, int64(6), scalarCount(t, db, `SELECT COUNT(*) FROM access_roles WHERE tenant_id='other'`))
			require.Equal(t, int64(6), scalarCount(t, db, `SELECT COUNT(*) FROM access_profiles`))
			for id, expected := range map[string]struct {
				kind, role   string
				manage, full bool
			}{
				"admin":    {"internal", "administrator", true, true},
				"manager":  {"internal", "member", true, false},
				"full":     {"internal", "member", true, true},
				"client":   {"external", "none", false, false},
				"customer": {"external", "none", false, false},
			} {
				var kind, role string
				var manage, full bool
				require.NoError(t, db.QueryRow(`SELECT user_type,organization_role,can_manage_users,all_workspaces FROM access_profiles WHERE user_id=`+bindVar(engine, 1), id).Scan(&kind, &role, &manage, &full))
				require.Equal(t, expected.kind, kind)
				require.Equal(t, expected.role, role)
				require.Equal(t, expected.manage, manage)
				require.Equal(t, expected.full, full)
			}
			var name string
			var read, upload, del, invite, links, manage bool
			require.NoError(t, db.QueryRow(`SELECT r.name,r.can_read,r.can_upload,r.can_delete,r.can_invite_external,r.can_create_links,r.can_manage_access FROM access_roles r JOIN role_assignments a ON a.role_id=r.id WHERE a.source_kind='membership' AND a.user_id='client'`).Scan(&name, &read, &upload, &del, &invite, &links, &manage))
			require.Equal(t, "Client contact (legacy)", name)
			require.True(t, read && upload && invite && links)
			require.False(t, del || manage)
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM role_assignments a JOIN access_roles r ON r.id=a.role_id WHERE a.user_id='customer' AND r.template_key='viewer'`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM role_assignments a JOIN access_roles r ON r.id=a.role_id WHERE a.user_id='manager' AND r.template_key='workspace-manager'`))
			var expires, revoked time.Time
			var version int
			require.NoError(t, db.QueryRow(`SELECT version,expires_at,revoked_at FROM role_assignments WHERE source_kind='grant'`).Scan(&version, &expires, &revoked))
			require.Equal(t, 7, version)
			require.Equal(t, 2027, expires.Year())
			require.Equal(t, 2026, revoked.Year())
			// Existing data is untouched, including implicit legacy membership.
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM user_clients WHERE user_id='client' AND NOT capabilities_set AND NOT can_read AND NOT can_upload AND NOT can_share`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM secure_links WHERE token='unchanged-bearer'`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_invitations WHERE token_hash='unchanged-hash'`))
			_, err = db.Exec(`INSERT INTO access_groups (id,tenant_id,name) VALUES ('team','tenant','Accounting'),('foreign-team','other','Accounting')`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO access_group_members (tenant_id,group_id,user_id) VALUES ('tenant','team','client')`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO role_assignments (id,tenant_id,role_id,group_id,client_id) SELECT 'group-access','tenant',id,'team','workspace' FROM access_roles WHERE tenant_id='tenant' AND template_key='viewer'`)
			require.NoError(t, err)
			for _, query := range []string{
				`INSERT INTO access_group_members (tenant_id,group_id,user_id) VALUES ('tenant','missing','client')`,
				`INSERT INTO access_group_members (tenant_id,group_id,user_id) VALUES ('tenant','team','foreign')`,
				`INSERT INTO access_group_members (tenant_id,group_id,user_id) VALUES ('tenant','foreign-team','client')`,
				`INSERT INTO role_assignments (id,tenant_id,role_id,user_id,group_id,client_id) SELECT 'two-principals','tenant',id,'client','team','workspace' FROM access_roles WHERE tenant_id='tenant' AND template_key='viewer'`,
				`UPDATE access_profiles SET organization_role='administrator' WHERE user_id='client'`,
				`UPDATE access_profiles SET all_workspaces=TRUE WHERE user_id='client'`,
				`INSERT INTO role_assignments (id,tenant_id,role_id,user_id,client_id) SELECT 'bad-user','tenant',id,'foreign','workspace' FROM access_roles WHERE tenant_id='tenant' AND template_key='viewer'`,
				`INSERT INTO role_assignments (id,tenant_id,role_id,user_id,client_id,file_id) SELECT 'bad-file','tenant',id,'client','workspace','foreign-file' FROM access_roles WHERE tenant_id='tenant' AND template_key='viewer'`,
				`INSERT INTO role_assignments (id,tenant_id,role_id,user_id,client_id) SELECT 'bad-role','tenant',id,'client','workspace' FROM access_roles WHERE tenant_id='other' AND template_key='viewer'`,
				`INSERT INTO role_assignments (id,tenant_id,role_id,client_id) SELECT 'no-principal','tenant',id,'workspace' FROM access_roles WHERE tenant_id='tenant' AND template_key='viewer'`,
			} {
				_, err := db.Exec(query)
				require.Error(t, err)
			}
			tx, err = db.Begin()
			require.NoError(t, err)
			require.NoError(t, downAddAccessRoleFoundation(context.Background(), tx))
			require.NoError(t, tx.Commit())
			require.Equal(t, int64(3), scalarCount(t, db, `SELECT COUNT(*) FROM user_clients`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM item_grants WHERE version=7 AND revoked_at IS NOT NULL`))
		})
	}
}
