package migration

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/utils"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

var (
	flags      = flag.NewFlagSet("goose", flag.ExitOnError)
	dir        = flags.String("dir", "internal/database/migrations", "directory with migration files")
	configPath = flags.String("config", "config/config.yaml", "path to config file")
)

type ConfigLoader func(path string) (db.DatabaseConfig, error)

func Run(goMigrations []*goose.Migration, configLoader ConfigLoader) {
	if err := flags.Parse(os.Args[1:]); err != nil {
		log.Fatalf("goose: failed to parse flags: %v", err)
	}
	args := flags.Args()

	if len(args) < 1 {
		flags.Usage()
		return
	}

	dbConfig, err := configLoader(*configPath)
	if err != nil {
		log.Fatalf("goose: failed to load config: %v", err)
	}
	utils.InitLogger()
	defer zap.L().Sync()

	driver, err := dbConfig.ResolvedDriver()
	if err != nil {
		log.Fatalf("goose: invalid database config: %v", err)
	}
	SetMigrationDriver(driver)

	db, dialect, err := OpenMigrationDB(dbConfig)
	if err != nil {
		log.Fatalf("goose: failed to open DB: %v", err)
	}

	defer func() {
		if err := db.Close(); err != nil {
			log.Fatalf("goose: failed to close DB: %v", err)
		}
	}()

	provider, migrationDir, err := NewMigrationProvider(db, dialect, dbConfig, *dir, goMigrations)
	if err != nil {
		log.Fatalf("goose: failed to initialize provider: %v", err)
	}

	ctx := context.Background()
	if err := runMigrationCommand(ctx, provider, args[0], args[1:]); err != nil {
		log.Fatalf("goose %v (%s): %v", args[0], migrationDir, err)
	}
}

func GooseDialect(c db.DatabaseConfig) (goose.Dialect, error) {
	driver, err := c.ResolvedDriver()
	if err != nil {
		return "", err
	}

	switch driver {
	case db.DatabaseDriverSQLite:
		return goose.DialectSQLite3, nil
	case db.DatabaseDriverPostgres:
		return goose.DialectPostgres, nil
	case db.DatabaseDriverMySQL:
		return goose.DialectMySQL, nil
	default:
		return "", fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func OpenMigrationDB(c db.DatabaseConfig) (*sql.DB, goose.Dialect, error) {
	sqlDriver, err := c.SQLDriverName()
	if err != nil {
		return nil, "", err
	}

	dsn, err := c.RuntimeDSN()
	if err != nil {
		return nil, "", err
	}
	dialect, err := GooseDialect(c)
	if err != nil {
		return nil, "", err
	}

	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return nil, "", fmt.Errorf("open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, "", fmt.Errorf("ping database: %w", err)
	}

	return db, dialect, nil
}
