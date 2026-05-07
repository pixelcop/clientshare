package db

import (
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	sqlmysql "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const DatabaseDriverSQLite = "sqlite"
const DatabaseDriverPostgres = "postgres"
const DatabaseDriverMySQL = "mysql"

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	Path   string `mapstructure:"path"`
	DSN    string `mapstructure:"dsn"`
}

func (c DatabaseConfig) ResolvedDriver() (string, error) {
	driver := strings.ToLower(strings.TrimSpace(c.Driver))
	dsn := strings.TrimSpace(c.DSN)
	path := strings.TrimSpace(c.Path)

	switch driver {
	case DatabaseDriverSQLite, "sqlite3":
		return DatabaseDriverSQLite, nil
	case DatabaseDriverPostgres, "postgresql", "pgx":
		return DatabaseDriverPostgres, nil
	case DatabaseDriverMySQL, "mariadb":
		return DatabaseDriverMySQL, nil
	case "":
		if path != "" {
			return DatabaseDriverSQLite, nil
		}
		if strings.HasPrefix(strings.ToLower(dsn), "postgres://") || strings.HasPrefix(strings.ToLower(dsn), "postgresql://") {
			return DatabaseDriverPostgres, nil
		}
		if dsn == "" {
			return DatabaseDriverSQLite, nil
		}
		return "", fmt.Errorf("database.driver is required when using database.dsn for non-SQLite databases")
	default:
		return "", fmt.Errorf("unsupported database driver: %s", c.Driver)
	}
}

func (c DatabaseConfig) RuntimeDSN() (string, error) {
	driver, err := c.ResolvedDriver()
	if err != nil {
		return "", err
	}

	switch driver {
	case DatabaseDriverSQLite:
		if dsn := strings.TrimSpace(c.DSN); dsn != "" {
			// remove schema, seems to break in modernc.org/sqlite
			return strings.TrimPrefix(dsn, "sqlite://"), nil
		}
		if path := strings.TrimSpace(c.Path); path != "" {
			return path, nil
		}
		return "", fmt.Errorf("database.path or database.dsn is required for sqlite")
	case DatabaseDriverPostgres:
		if dsn := strings.TrimSpace(c.DSN); dsn != "" {
			return dsn, nil
		}
		return "", fmt.Errorf("database.dsn is required for postgres")
	case DatabaseDriverMySQL:
		if strings.TrimSpace(c.DSN) == "" {
			return "", fmt.Errorf("database.dsn is required for mysql")
		}
		return normalizeMySQLDSN(c.DSN, false)
	default:
		return "", fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func (c DatabaseConfig) GormDialector() (gorm.Dialector, error) {
	runtimeDSN, err := c.RuntimeDSN()
	if err != nil {
		return nil, err
	}
	driver, err := c.ResolvedDriver()
	if err != nil {
		return nil, err
	}

	switch driver {
	case DatabaseDriverSQLite:
		return sqlite.Open(runtimeDSN), nil
	case DatabaseDriverPostgres:
		return postgres.Open(runtimeDSN), nil
	case DatabaseDriverMySQL:
		return mysql.Open(runtimeDSN), nil
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

// SQLDriverName returns the driver name to be used with sql.Open for the given database configuration.
func (c DatabaseConfig) SQLDriverName() (string, error) {
	driver, err := c.ResolvedDriver()
	if err != nil {
		return "", err
	}

	switch driver {
	case DatabaseDriverSQLite:
		return "sqlite", nil
	case DatabaseDriverPostgres:
		return "pgx", nil
	case DatabaseDriverMySQL:
		return "mysql", nil
	default:
		return "", fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func normalizeMySQLDSN(dsn string, enableMultiStatements bool) (string, error) {
	parsed, err := sqlmysql.ParseDSN(strings.TrimSpace(dsn))
	if err != nil {
		return "", fmt.Errorf("parse mysql dsn: %w", err)
	}
	parsed.ParseTime = true
	if enableMultiStatements {
		parsed.MultiStatements = true
	}
	return parsed.FormatDSN(), nil
}
