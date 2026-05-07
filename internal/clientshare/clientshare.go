package clientshare

import (
	"context"

	"github.com/pixelcop/clientshare/pkg/db"
	"go.uber.org/zap"
)

func Start(ctx context.Context, cfg *Config, logger *zap.Logger) error {
	store, err := BuildStorage(ctx, cfg)
	if err != nil {
		return err
	}

	db, err := db.InitDB(cfg.Database, logger)
	if err != nil {
		return err
	}
	err = bootstrapAdminUser(db, cfg)
	if err != nil {
		return err
	}

	emailQueue, err := InitEmail(cfg, logger, db)
	if err != nil {
		return err
	}
	return StartWebServer(cfg, db, logger, emailQueue, store)
}
