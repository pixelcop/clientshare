package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/pixelcop/clientshare/internal/tenant"
	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestMigrationProviderUpCreatesCurrentSchemaAndSeedsTenant(t *testing.T) {
	db := openMigrationTestDB(t)
	provider := newMigrationProviderForTest(t, db, goose.DialectSQLite3, dbpkg.DatabaseConfig{Driver: dbpkg.DatabaseDriverSQLite})

	results, err := provider.Up(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 4)

	requireDefaultTenantSeeded(t, db, dbpkg.DatabaseDriverSQLite)
	requireGooseVersionsApplied(t, db, dbpkg.DatabaseDriverSQLite, 34, 35, 36, 37)
	requireColumnPresent(t, db, "files", "disk_deleted_at")
}

func TestMigrationProviderUpCreatesCurrentSchemaAndSeedsTenantPostgres(t *testing.T) {
	db, cfg := openPostgresMigrationTestDB(t)
	provider := newMigrationProviderForTest(t, db, goose.DialectPostgres, cfg)

	results, err := provider.Up(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 4)

	requireDefaultTenantSeeded(t, db, dbpkg.DatabaseDriverPostgres)
	requireGooseVersionsApplied(t, db, dbpkg.DatabaseDriverPostgres, 34, 35, 36, 37)
	requireColumnPresent(t, db, "files", "disk_deleted_at")
}

func TestMigrationProviderUpIsIdempotent(t *testing.T) {
	db := openMigrationTestDB(t)
	provider := newMigrationProviderForTest(t, db, goose.DialectSQLite3, dbpkg.DatabaseConfig{Driver: dbpkg.DatabaseDriverSQLite})

	_, err := provider.Up(context.Background())
	require.NoError(t, err)

	results, err := provider.Up(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 0)

	requireDefaultTenantSeeded(t, db, dbpkg.DatabaseDriverSQLite)
}

func TestMigrationProviderUpIsIdempotentPostgres(t *testing.T) {
	db, cfg := openPostgresMigrationTestDB(t)
	provider := newMigrationProviderForTest(t, db, goose.DialectPostgres, cfg)

	_, err := provider.Up(context.Background())
	require.NoError(t, err)

	results, err := provider.Up(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 0)

	requireDefaultTenantSeeded(t, db, dbpkg.DatabaseDriverPostgres)
}

func TestMigrationProviderResetDropsManagedTables(t *testing.T) {
	db := openMigrationTestDB(t)
	provider := newMigrationProviderForTest(t, db, goose.DialectSQLite3, dbpkg.DatabaseConfig{Driver: dbpkg.DatabaseDriverSQLite})

	_, err := provider.Up(context.Background())
	require.NoError(t, err)

	results, err := provider.DownTo(context.Background(), 0)
	require.NoError(t, err)
	require.Len(t, results, 4)

	requireManagedTablesMissing(t, db, dbpkg.DatabaseDriverSQLite)
}

func TestMigrationProviderResetDropsManagedTablesPostgres(t *testing.T) {
	db, cfg := openPostgresMigrationTestDB(t)
	provider := newMigrationProviderForTest(t, db, goose.DialectPostgres, cfg)

	_, err := provider.Up(context.Background())
	require.NoError(t, err)

	results, err := provider.DownTo(context.Background(), 0)
	require.NoError(t, err)
	require.Len(t, results, 4)

	requireManagedTablesMissing(t, db, dbpkg.DatabaseDriverPostgres)
}

func newMigrationProviderForTest(t *testing.T, db *sql.DB, dialect goose.Dialect, cfg dbpkg.DatabaseConfig) *goose.Provider {
	t.Helper()

	driver, err := cfg.ResolvedDriver()
	require.NoError(t, err)
	migration.SetMigrationDriver(driver)
	provider, migrationDir, err := migration.NewMigrationProvider(
		db,
		dialect,
		cfg,
		workingMigrationsDir(t),
		[]*goose.Migration{
			migration.NewGoMigration(34, upCreateCurrentSchema, downCreateCurrentSchema),
			migration.NewGoMigration(35, upSeedDefaultTenant, downSeedDefaultTenant),
			migration.NewGoMigration(36, upBackfillTenantDomainsFromPublicBaseURLs, downBackfillTenantDomainsFromPublicBaseURLs),
			migration.NewGoMigration(37, upAddFilesDiskDeletedAt, downAddFilesDiskDeletedAt),
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, migrationDir)
	return provider
}

func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	require.NoError(t, goose.SetDialect("sqlite"))
	dbPath := filepath.Join(t.TempDir(), "tenant-foundation.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	_, err = db.Exec(`PRAGMA foreign_keys = ON;`)
	require.NoError(t, err)
	return db
}

func openPostgresMigrationTestDB(t *testing.T) (*sql.DB, dbpkg.DatabaseConfig) {
	t.Helper()

	port := availableTCPPort(t)
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	dataDir := filepath.Join(t.TempDir(), "data")
	cacheDir := filepath.Join(t.TempDir(), "cache")

	postgres := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Port(port).
			Database("postgres").
			Username("postgres").
			Password("postgres").
			RuntimePath(runtimeDir).
			DataPath(dataDir).
			CachePath(cacheDir).
			StartTimeout(45 * time.Second).
			Logger(io.Discard),
	)
	require.NoError(t, postgres.Start())
	t.Cleanup(func() {
		require.NoError(t, postgres.Stop())
	})

	cfg := dbpkg.DatabaseConfig{
		Driver: dbpkg.DatabaseDriverPostgres,
		DSN:    fmt.Sprintf("postgresql://postgres:postgres@localhost:%d/postgres?sslmode=disable", port),
	}
	sqlDriver, err := cfg.SQLDriverName()
	require.NoError(t, err)
	runtimeDSN, err := cfg.RuntimeDSN()
	require.NoError(t, err)
	db, err := sql.Open(sqlDriver, runtimeDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	require.NoError(t, db.Ping())

	return db, cfg
}

func workingMigrationsDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	return dir
}

func availableTCPPort(t *testing.T) uint32 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, listener.Close())
	}()
	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.Positive(t, addr.Port)
	return uint32(addr.Port)
}

func scalarCount(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.QueryRow(query, args...).Scan(&count))
	return count
}

func requireDefaultTenantSeeded(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	require.Equal(t, int64(1), scalarCount(t, db, countByValueQuery(driver, "tenants", "id"), tenant.DefaultTenantID))
	require.Equal(t, int64(1), scalarCount(t, db, countByValueQuery(driver, "tenant_settings", "tenant_id"), tenant.DefaultTenantID))
	require.Equal(t, int64(1), scalarCount(t, db, countByValueQuery(driver, "tenant_entitlements", "tenant_id"), tenant.DefaultTenantID))
}

func requireGooseVersionsApplied(t *testing.T, db *sql.DB, driver string, versions ...int64) {
	t.Helper()
	query := fmt.Sprintf("SELECT COUNT(*) FROM goose_db_version WHERE version_id = %s AND is_applied", bindVar(driver, 1))
	for _, version := range versions {
		require.Equal(t, int64(1), scalarCount(t, db, query, version))
	}
}

func requireColumnPresent(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	_, err := db.Exec("SELECT " + column + " FROM " + table + " LIMIT 1")
	require.NoError(t, err)
}

func requireManagedTablesMissing(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	requireTableMissing(t, db, driver, "audit_events")
	requireTableMissing(t, db, driver, "feed_events")
	requireTableMissing(t, db, driver, "user_clients")
	requireTableMissing(t, db, driver, "invite_tokens")
	requireTableMissing(t, db, driver, "password_reset_tokens")
	requireTableMissing(t, db, driver, "email_queue")
	requireTableMissing(t, db, driver, "file_activity")
	requireTableMissing(t, db, driver, "secure_links")
	requireTableMissing(t, db, driver, "files")
	requireTableMissing(t, db, driver, "users")
	requireTableMissing(t, db, driver, "clients")
	requireTableMissing(t, db, driver, "tenant_entitlements")
	requireTableMissing(t, db, driver, "tenant_settings")
	requireTableMissing(t, db, driver, "tenant_domains")
	requireTableMissing(t, db, driver, "tenants")
}

func countByValueQuery(driver, table, column string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = %s", table, column, bindVar(driver, 1))
}

func bindVar(driver string, position int) string {
	if driver == dbpkg.DatabaseDriverPostgres {
		return fmt.Sprintf("$%d", position)
	}
	return "?"
}

func requireTableMissing(t *testing.T, db *sql.DB, driver, table string) {
	t.Helper()
	_, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1")
	require.Error(t, err)
	switch driver {
	case dbpkg.DatabaseDriverPostgres:
		require.Contains(t, err.Error(), fmt.Sprintf("relation %q does not exist", table))
	default:
		require.Contains(t, err.Error(), "no such table")
	}
}
