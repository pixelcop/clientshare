package db

import (
	"testing"

	sqlmysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestDatabaseConfigResolvedDriverDefaultsToSQLiteForPath(t *testing.T) {
	cfg := DatabaseConfig{Path: "/tmp/clientshare.db"}

	driver, err := cfg.ResolvedDriver()
	require.NoError(t, err)
	require.Equal(t, DatabaseDriverSQLite, driver)

	runtimeDSN, err := cfg.RuntimeDSN()
	require.NoError(t, err)
	require.Equal(t, "/tmp/clientshare.db", runtimeDSN)
}

func TestDatabaseConfigResolvedDriverDetectsPostgresURL(t *testing.T) {
	cfg := DatabaseConfig{DSN: "postgres://clientshare:secret@localhost:5432/clientshare?sslmode=disable"}

	driver, err := cfg.ResolvedDriver()
	require.NoError(t, err)
	require.Equal(t, DatabaseDriverPostgres, driver)

	runtimeDSN, err := cfg.RuntimeDSN()
	require.NoError(t, err)
	require.Equal(t, cfg.DSN, runtimeDSN)
}

func TestDatabaseConfigNormalizesMySQLDSN(t *testing.T) {
	cfg := DatabaseConfig{
		Driver: "mariadb",
		DSN:    "clientshare:secret@tcp(localhost:3306)/clientshare?charset=utf8mb4",
	}

	driver, err := cfg.ResolvedDriver()
	require.NoError(t, err)
	require.Equal(t, DatabaseDriverMySQL, driver)

	runtimeDSN, err := cfg.RuntimeDSN()
	require.NoError(t, err)
	runtimeParsed, err := sqlmysql.ParseDSN(runtimeDSN)
	require.NoError(t, err)
	require.True(t, runtimeParsed.ParseTime)
	require.False(t, runtimeParsed.MultiStatements)
}

func TestDatabaseConfigRejectsAmbiguousNonSQLiteDSNWithoutDriver(t *testing.T) {
	cfg := DatabaseConfig{DSN: "clientshare:secret@tcp(localhost:3306)/clientshare"}

	_, err := cfg.ResolvedDriver()
	require.Error(t, err)
}
