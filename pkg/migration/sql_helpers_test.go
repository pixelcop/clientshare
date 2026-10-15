package migration

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLiteMigrationDDLStaysInTransaction(t *testing.T) {
	originalDriver := migrationDriver
	SetMigrationDriver("sqlite")
	t.Cleanup(func() { migrationDriver = originalDriver })

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "migration.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec("CREATE TABLE widgets (id INTEGER PRIMARY KEY, obsolete TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO widgets (id, obsolete) VALUES (1, 'original')")
	require.NoError(t, err)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	gdb, err := MigrationTxGormDB(tx)
	require.NoError(t, err)
	require.NoError(t, gdb.Migrator().DropColumn("widgets", "obsolete"))
	require.False(t, gdb.Migrator().HasColumn("widgets", "obsolete"))
	require.NoError(t, tx.Rollback())

	var value string
	require.NoError(t, db.QueryRow("SELECT obsolete FROM widgets WHERE id = 1").Scan(&value))
	require.Equal(t, "original", value, "rolling back must restore both schema and data")
}
