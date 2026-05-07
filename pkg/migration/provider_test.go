package migration

import (
	"path/filepath"
	"testing"

	"github.com/pixelcop/clientshare/pkg/db"
	"github.com/stretchr/testify/require"
)

func TestMigrationDirForDriver(t *testing.T) {
	baseDir := filepath.Join("internal", "database", "migrations")

	t.Run("sqlite", func(t *testing.T) {
		dir, err := migrationDirForDriver(db.DatabaseConfig{Driver: db.DatabaseDriverSQLite}, baseDir)
		require.NoError(t, err)
		require.Equal(t, filepath.Clean(baseDir), dir)
	})

	t.Run("postgres", func(t *testing.T) {
		dir, err := migrationDirForDriver(db.DatabaseConfig{Driver: db.DatabaseDriverPostgres}, baseDir)
		require.NoError(t, err)
		require.Equal(t, filepath.Clean(baseDir), dir)
	})

	t.Run("mysql", func(t *testing.T) {
		dir, err := migrationDirForDriver(db.DatabaseConfig{Driver: db.DatabaseDriverMySQL}, baseDir)
		require.NoError(t, err)
		require.Equal(t, filepath.Clean(baseDir), dir)
	})
}
