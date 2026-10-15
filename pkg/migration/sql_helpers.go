package migration

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/libtnb/sqlite" // pure-go driver seems to work better for migrations
	"github.com/pixelcop/clientshare/pkg/db"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var migrationDriver = db.DatabaseDriverSQLite

// sqliteMigrationTx keeps GORM's DB() from unwrapping *sql.Tx to its parent
// pool. The SQLite migrator must reuse Goose's transaction rather than acquire
// another connection (which both escapes rollback and locks against the tx).
type sqliteMigrationTx struct {
	*sql.Tx
}

func SetMigrationDriver(driver string) {
	trimmed := strings.TrimSpace(driver)
	if trimmed == "" {
		migrationDriver = db.DatabaseDriverSQLite
		return
	}
	migrationDriver = trimmed
}

// MigrationTxGormDB creates a GORM DB instance from the given migration transaction and the global migrationDriver.
func MigrationTxGormDB(tx *sql.Tx) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch migrationDriver {
	case "", db.DatabaseDriverSQLite:
		dialector = sqlite.New(sqlite.Config{Conn: sqliteMigrationTx{tx}})
	case db.DatabaseDriverPostgres:
		dialector = postgres.New(postgres.Config{Conn: tx})
	case db.DatabaseDriverMySQL:
		dialector = mysql.New(mysql.Config{Conn: tx})
	default:
		return nil, fmt.Errorf("unsupported migration driver: %s", migrationDriver)
	}

	logMode := logger.Warn
	if os.Getenv("DEBUG") == "1" {
		logMode = logger.Info
	}
	l := logger.Default.LogMode(logMode)
	return gorm.Open(dialector, &gorm.Config{Logger: l})
}
