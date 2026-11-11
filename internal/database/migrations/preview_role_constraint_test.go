package main

import (
	"context"
	"github.com/pixelcop/clientshare/internal/models"
	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPreviewRoleCheckRebuildPreservesAssignments(t *testing.T) {
	db := openMigrationTestDB(t)
	migration.SetMigrationDriver(dbpkg.DatabaseDriverSQLite)
	for _, q := range []string{
		`CREATE TABLE access_roles (id TEXT PRIMARY KEY,tenant_id TEXT,allow_file BOOLEAN,can_upload BOOLEAN,CHECK(NOT allow_file OR NOT can_upload),UNIQUE(tenant_id,id))`,
		`CREATE TABLE role_assignments (id TEXT PRIMARY KEY,tenant_id TEXT,role_id TEXT,user_id TEXT,group_id TEXT,client_id TEXT,file_id TEXT,source_kind TEXT,source_id TEXT,version BIGINT,expires_at TIMESTAMP,revoked_at TIMESTAMP,created_at TIMESTAMP,updated_at TIMESTAMP,FOREIGN KEY(tenant_id,role_id) REFERENCES access_roles(tenant_id,id) ON DELETE CASCADE)`,
		`INSERT INTO access_roles VALUES ('role','tenant',FALSE,TRUE)`,
		`INSERT INTO role_assignments (id,tenant_id,role_id,user_id,client_id,source_kind,version) VALUES ('assignment','tenant','role','user','client','group',9)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	gdb, err := migration.MigrationTxGormDB(tx)
	require.NoError(t, err)
	require.NoError(t, removePreviewFileUploadConstraint(gdb))
	require.NoError(t, gdb.Exec("UPDATE access_roles SET allow_file=TRUE").Error)
	var a models.RoleAssignment
	require.NoError(t, gdb.First(&a, "id = ?", "assignment").Error)
	require.EqualValues(t, 9, a.Version)
	require.Equal(t, "role", a.RoleID)
	require.NoError(t, tx.Commit())
	require.Equal(t, int64(1), scalarCount(t, db, `SELECT COUNT(*) FROM role_assignments WHERE id='assignment' AND version=9`))
	rows, err := db.Query("PRAGMA foreign_key_check")
	require.NoError(t, err)
	defer rows.Close()
	require.False(t, rows.Next())
}
