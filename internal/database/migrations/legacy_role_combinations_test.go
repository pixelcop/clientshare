package main

import (
	"github.com/pixelcop/clientshare/internal/models"
	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLegacyRoleConversionPreservesEveryEffectiveCombination(t *testing.T) {
	sqlDB := openMigrationTestDB(t)
	migration.SetMigrationDriver(dbpkg.DatabaseDriverSQLite)
	tx, err := sqlDB.Begin()
	require.NoError(t, err)
	defer tx.Rollback()
	db, err := migration.MigrationTxGormDB(tx)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AccessRole{}))
	for _, role := range models.DefaultAccessRoles("tenant") {
		require.NoError(t, db.Create(&role).Error)
	}
	for _, audience := range []string{"internal", "external"} {
		for _, scope := range []string{"workspace", "folder", "file"} {
			for bits := 0; bits < 16; bits++ {
				caps := models.AccessCapabilities{CanRead: bits&1 != 0, CanUpload: bits&2 != 0, CanDelete: bits&4 != 0, CanShare: bits&8 != 0}
				id, err := migrationAccessRole(db, "tenant", audience, scope, caps)
				require.NoError(t, err)
				var role models.AccessRole
				require.NoError(t, db.First(&role, "id=?", id).Error)
				require.True(t, role.Supports(audience, scope))
				p, valid := role.PermissionsAt(scope)
				require.True(t, valid)
				require.Equal(t, caps.CanRead, p.CanRead)
				require.Equal(t, caps.CanDelete, p.CanDelete)
				require.Equal(t, caps.CanUpload && scope != "file", p.CanUpload)
				require.Equal(t, caps.CanShare, p.CanInviteExternal || p.CanCreateLinks || p.CanManageAccess)
				if caps.CanShare {
					require.True(t, role.LegacySharing, "legacy provenance contract must remain active")
				}
				again, err := migrationAccessRole(db, "tenant", audience, scope, caps)
				require.NoError(t, err)
				require.Equal(t, id, again, "reuse stable tenant-local roles rather than duplicating them")
			}
		}
	}
}
