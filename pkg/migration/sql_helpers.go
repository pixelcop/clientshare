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
		dialector = sqlite.New(sqlite.Config{Conn: tx})
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
