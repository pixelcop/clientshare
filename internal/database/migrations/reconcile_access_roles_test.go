package main

import (
	"context"
	"testing"

	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestRoleReconciliationRepairsCheckpointsWithoutOverwritingCustomizations(t *testing.T) {
	db := openMigrationTestDB(t)
	p := newMigrationProviderForTest(t, db, goose.DialectSQLite3, dbpkg.DatabaseConfig{Driver: dbpkg.DatabaseDriverSQLite})
	ctx := context.Background()
	_, err := p.UpTo(ctx, 56)
	require.NoError(t, err)
	for _, query := range []string{
		`DROP TABLE access_role_catalogs`,
		`UPDATE access_roles SET allow_file=FALSE WHERE template_key='workspace-manager'`,
		`UPDATE access_roles SET name='Our document team',allow_file=FALSE,version=2 WHERE template_key='file-manager'`,
		`UPDATE access_roles SET archived_at=CURRENT_TIMESTAMP WHERE template_key='viewer'`,
		`INSERT INTO users (id,tenant_id,email,password_hash,role) SELECT 'standard-member',id,'standard@example.test','','manager' FROM tenants LIMIT 1`,
		`INSERT INTO access_profiles (tenant_id,user_id,user_type,organization_role,can_manage_users,all_workspaces) SELECT tenant_id,id,'internal','member',FALSE,FALSE FROM users WHERE id='standard-member'`,
		`INSERT INTO users (id,tenant_id,email,password_hash,role) SELECT 'legacy-manager',id,'legacy@example.test','','manager' FROM tenants LIMIT 1`,
		`INSERT INTO clients (id,tenant_id,name,folder_path) SELECT 'workspace',id,'Workspace','workspace' FROM tenants LIMIT 1`,
		`INSERT INTO user_clients (tenant_id,user_id,client_id,role_id) SELECT tenant_id,'legacy-manager','workspace',id FROM access_roles WHERE template_key='workspace-manager'`,
		`INSERT INTO role_assignments (id,tenant_id,role_id,user_id,client_id,source_kind,source_id) SELECT 'old-snapshot',tenant_id,id,'legacy-manager','workspace','membership','legacy-manager:workspace' FROM access_roles WHERE template_key='workspace-manager'`,
		`INSERT INTO access_groups (id,tenant_id,name) SELECT 'real-group',id,'Our group' FROM tenants LIMIT 1`,
		`INSERT INTO role_assignments (id,tenant_id,role_id,group_id,client_id,source_kind) SELECT 'real-grant',tenant_id,id,'real-group','workspace','group' FROM access_roles WHERE template_key='contributor'`,
	} {
		_, err := db.Exec(query)
		require.NoError(t, err)
	}
	_, err = p.Up(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_role_catalogs`))
	require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_roles WHERE name='Our document team' AND NOT allow_file AND version=2`))
	require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_roles WHERE template_key='workspace-manager' AND allow_file AND version=2`))
	require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_roles WHERE template_key='viewer' AND archived_at IS NOT NULL`))
	require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_profiles WHERE user_id='standard-member' AND NOT can_manage_users AND NOT all_workspaces`))
	require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM access_profiles WHERE user_id='legacy-manager' AND can_manage_users`))
	require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM user_clients uc JOIN access_roles ar ON ar.id=uc.role_id WHERE uc.user_id='legacy-manager' AND ar.legacy_sharing AND NOT uc.capabilities_set`))
	require.Equal(t, int64(0), scalarCount(t, db, `SELECT COUNT(*) FROM role_assignments WHERE source_kind='membership'`))
	require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM role_assignments WHERE id='real-grant' AND source_kind='group'`))
}
