package db

import (
	dbpkg "github.com/pixelcop/clientshare/internal/db"
	"go.uber.org/zap"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func GormConfig(logger *zap.Logger) *gorm.Config {
	logLevel := glogger.Warn
	if true {
		logLevel = glogger.Info
	}

	gl := NewZapLogger(logger, int(logLevel))
	gl.IgnoreRecordNotFoundError = true
	gl.SetAsDefault()
	return &gorm.Config{
		Logger: gl,
	}
}

func InitDB(cfg DatabaseConfig, logger *zap.Logger) (*gorm.DB, error) {
	dialector, err := cfg.GormDialector()
	if err != nil {
		logger.Fatal("failed to resolve database config", zap.Error(err))
		return nil, err
	}

	db, err := gorm.Open(dialector, GormConfig(logger))
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
		return nil, err
	}

	dbpkg.SetDB(db)
	return db, nil
}
