package main

import (
	"context"
	"database/sql"
	"testing"

	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestActivateRolesUsesCurrentSourcesAndPreservesLegacySharing(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var db *sql.DB
			cfg := dbpkg.DatabaseConfig{Driver: dbpkg.DatabaseDriverSQLite}
			dialect := goose.DialectSQLite3
			if engine == "postgres" {
				db, cfg = openPostgresMigrationTestDB(t)
				dialect = goose.DialectPostgres
			} else {
				db = openMigrationTestDB(t)
			}
			p := newMigrationProviderForTest(t, db, dialect, cfg)
			ctx := context.Background()
			_, err := p.UpTo(ctx, 54)
			require.NoError(t, err)
			for _, query := range []string{
				`INSERT INTO users (id,tenant_id,email,password_hash,role) SELECT 'legacy-manager',id,'manager@example.test','','manager' FROM tenants LIMIT 1`,
				`INSERT INTO users (id,tenant_id,email,password_hash,role) SELECT 'legacy-client',id,'client@example.test','','client' FROM tenants LIMIT 1`,
				`INSERT INTO clients (id,tenant_id,name,folder_path) SELECT 'workspace',id,'Workspace','workspace' FROM tenants LIMIT 1`,
				`INSERT INTO files (id,tenant_id,client_id,filename,path,type,size) SELECT 'folder',id,'workspace','Folder','folder','folder',0 FROM tenants LIMIT 1`,
				`INSERT INTO user_clients (tenant_id,user_id,client_id) SELECT tenant_id,id,'workspace' FROM users WHERE id IN ('legacy-manager','legacy-client')`,
				`INSERT INTO item_grants (id,tenant_id,client_id,user_id,file_id,can_read,can_share,created_by,version) SELECT 'grant',id,'workspace','legacy-client','folder',TRUE,TRUE,'legacy-manager',7 FROM tenants LIMIT 1`,
				`INSERT INTO access_invitations (id,tenant_id,client_id,file_id,email,token_hash,created_by,can_read,can_upload,can_share,expires_at) SELECT 'invitation',id,'workspace','folder','client@example.test','unchanged-hash','legacy-manager',TRUE,TRUE,TRUE,'2027-01-01 00:00:00' FROM tenants LIMIT 1`,
			} {
				_, err := db.Exec(query)
				require.NoError(t, err)
			}
			_, err = p.UpTo(ctx, 55)
			require.NoError(t, err)
			// Simulate changes while only the additive checkpoint was installed.
			_, err = db.Exec(`UPDATE user_clients SET capabilities_set=TRUE,can_read=TRUE,can_upload=FALSE,can_delete=FALSE,can_share=FALSE WHERE user_id='legacy-client'`)
			require.NoError(t, err)
			_, err = db.Exec(`UPDATE users SET manager_full_access=TRUE WHERE id='legacy-manager'`)
			require.NoError(t, err)
			_, err = p.Up(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM user_clients uc JOIN access_roles ar ON ar.id=uc.role_id AND ar.tenant_id=uc.tenant_id WHERE uc.user_id='legacy-client' AND ar.template_key='viewer'`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_profiles WHERE user_id='legacy-manager' AND all_workspaces AND can_manage_users`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM user_clients uc JOIN access_roles ar ON ar.id=uc.role_id WHERE uc.user_id='legacy-manager' AND ar.legacy_sharing`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM item_grants g JOIN access_roles ar ON ar.id=g.role_id WHERE g.id='grant' AND g.version=7 AND ar.legacy_sharing`))
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_invitations i JOIN access_roles ar ON ar.id=i.role_id WHERE i.token_hash='unchanged-hash' AND ar.legacy_sharing`))
			require.Equal(t, int64(0), scalarCount(t, db, `SELECT COUNT(*) FROM role_assignments`))
			// Refuse a lossy downgrade instead of expanding partial sharing rights
			// back into the old all-or-nothing share permission.
			var previousRole string
			require.NoError(t, db.QueryRow(`SELECT role_id FROM user_clients WHERE user_id='legacy-client'`).Scan(&previousRole))
			_, err = db.Exec(`UPDATE user_clients SET role_id=(SELECT id FROM access_roles WHERE template_key='client-contact' LIMIT 1) WHERE user_id='legacy-client'`)
			require.NoError(t, err)
			_, err = p.DownTo(ctx, 55)
			require.ErrorContains(t, err, "independent sharing rights")
			_, err = db.Exec("UPDATE user_clients SET role_id="+bindVar(engine, 1)+" WHERE user_id='legacy-client'", previousRole)
			require.NoError(t, err)
			_, err = p.DownTo(ctx, 55)
			require.NoError(t, err)
			require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_invitations WHERE token_hash='unchanged-hash'`))
		})
	}
}
