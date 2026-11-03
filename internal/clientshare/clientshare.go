package clientshare

import (
	"context"
	"fmt"
	"os"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
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
	var legacy int64
	if err := db.Model(&models.File{}).Where("type <> 'folder' AND disk_deleted_at IS NULL AND storage_missing = ? AND (storage_key IS NULL OR storage_key = '')", false).Count(&legacy).Error; err != nil {
		return err
	}
	if legacy > 0 {
		return fmt.Errorf("%d retained files require offline storage migration before startup", legacy)
	}
	if cfg.Storage.Layout != "" && cfg.Storage.Layout != "immutable-v1" {
		return fmt.Errorf("unsupported storage.layout %q", cfg.Storage.Layout)
	}
	views, err := services.NewReadableViews(db, cfg.Storage.Local.Root, cfg.Storage.Local.ReadableView.Root, cfg.Storage.Local.ReadableView.Mode)
	if err != nil {
		return err
	}
	if cfg.Storage.Type != StorageLocal && views.Mode != "none" {
		return fmt.Errorf("readable views require local storage")
	}
	if views.Mode == "none" || cfg.Storage.Local.ReadableView.Owner {
		if views.Mode != "none" {
			lock, err := views.Lock()
			if err != nil {
				return err
			}
			defer lock.Close()
			if err := views.Probe(); err != nil {
				return err
			}
		}
		viewCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go views.Run(viewCtx, os.Stderr)
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
