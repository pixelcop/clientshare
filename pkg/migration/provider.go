package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pixelcop/clientshare/pkg/db"
	"github.com/pressly/goose/v3"
)

func NewMigrationProvider(db *sql.DB, dialect goose.Dialect, dbCfg db.DatabaseConfig, baseDir string, goMigrations []*goose.Migration) (*goose.Provider, string, error) {
	migrationDir, err := migrationDirForDriver(dbCfg, baseDir)
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(migrationDir); err != nil {
		return nil, "", fmt.Errorf("migration dir %s: %w", migrationDir, err)
	}

	provider, err := goose.NewProvider(
		dialect,
		db,
		os.DirFS(migrationDir),
		goose.WithDisableGlobalRegistry(true),
		goose.WithGoMigrations(goMigrations...),
	)
	if err != nil {
		return nil, "", err
	}

	return provider, migrationDir, nil
}

func migrationDirForDriver(dbCfg db.DatabaseConfig, baseDir string) (string, error) {
	if _, err := dbCfg.ResolvedDriver(); err != nil {
		return "", err
	}

	return filepath.Clean(baseDir), nil
}

func NewGoMigration(version int64, up, down func(context.Context, *sql.Tx) error) *goose.Migration {
	return goose.NewGoMigration(
		version,
		&goose.GoFunc{RunTx: up},
		&goose.GoFunc{RunTx: down},
	)
}

func runMigrationCommand(ctx context.Context, provider *goose.Provider, command string, arguments []string) error {
	switch command {
	case "up":
		if pending, err := provider.HasPending(ctx); err != nil {
			return fmt.Errorf("check pending migrations: %w", err)
		} else if !pending {
			fmt.Println("no pending migrations; schema up to date")
			return nil
		}
		results, err := provider.Up(ctx)
		printMigrationResults(results)
		return err
	case "up-to":
		version, err := parseVersionArg(arguments)
		if err != nil {
			return err
		}
		results, err := provider.UpTo(ctx, version)
		printMigrationResults(results)
		return err
	case "up-by-one":
		result, err := provider.UpByOne(ctx)
		printMigrationResult(result)
		return err
	case "down":
		result, err := provider.Down(ctx)
		printMigrationResult(result)
		return err
	case "down-to":
		version, err := parseVersionArg(arguments)
		if err != nil {
			return err
		}
		results, err := provider.DownTo(ctx, version)
		printMigrationResults(results)
		return err
	case "redo":
		result, err := provider.Down(ctx)
		printMigrationResult(result)
		if err != nil {
			return err
		}
		upResult, err := provider.UpByOne(ctx)
		printMigrationResult(upResult)
		return err
	case "reset":
		results, err := provider.DownTo(ctx, 0)
		printMigrationResults(results)
		return err
	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return err
		}
		printMigrationStatus(statuses)
		return nil
	case "version":
		version, err := provider.GetDBVersion(ctx)
		if err != nil {
			return err
		}
		fmt.Println(version)
		return nil
	default:
		return fmt.Errorf("unsupported command %q", command)
	}
}

func parseVersionArg(arguments []string) (int64, error) {
	if len(arguments) == 0 {
		return 0, fmt.Errorf("missing version argument")
	}
	version, err := strconv.ParseInt(arguments[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse version %q: %w", arguments[0], err)
	}
	return version, nil
}

func printMigrationResults(results []*goose.MigrationResult) {
	for _, result := range results {
		printMigrationResult(result)
	}
}

func printMigrationResult(result *goose.MigrationResult) {
	if result == nil {
		return
	}
	fmt.Println(result.String())
}

func printMigrationStatus(statuses []*goose.MigrationStatus) {
	fmt.Printf("%-24s %-10s %s\n", "Applied At", "Version", "Migration")
	for _, status := range statuses {
		appliedAt := "Pending"
		if status.State != goose.StatePending && !status.AppliedAt.IsZero() {
			appliedAt = status.AppliedAt.UTC().Format(time.RFC3339)
		}
		version := int64(0)
		migration := "unknown"
		if status.Source != nil {
			version = status.Source.Version
			migration = migrationName(status.Source)
		}
		fmt.Printf("%-24s %-10d %s\n", appliedAt, version, migration)
	}
}

func migrationName(source *goose.Source) string {
	if source == nil {
		return "unknown"
	}
	if source.Path != "" {
		return filepath.Base(source.Path)
	}
	return fmt.Sprintf("%05d.go", source.Version)
}
