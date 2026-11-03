package clientshare

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"gorm.io/gorm"
)

func BuildStorage(ctx context.Context, cfg *Config) (storage.Storage, error) {
	switch strings.TrimSpace(cfg.Storage.Type) {
	case StorageLocal:
		if strings.TrimSpace(cfg.Storage.Local.Root) == "" {
			return nil, fmt.Errorf("local storage root path must be set in config")
		}
		root, err := filepath.Abs(cfg.Storage.Local.Root)
		if err != nil {
			return nil, fmt.Errorf("resolve storage root path: %w", err)
		}
		cfg.Storage.Local.Root = root
		if _, err := storage.SafePath(root, ""); err != nil {
			return nil, fmt.Errorf("invalid local storage root: %w", err)
		}
		return storage.NewLocalStorage(root), nil
	case StorageS3:
		store, err := storage.NewS3Storage(ctx, storage.S3Config{
			Bucket:       cfg.Storage.S3.Bucket,
			Region:       cfg.Storage.S3.Region,
			Endpoint:     cfg.Storage.S3.Endpoint,
			AccessKey:    cfg.Storage.S3.AccessKey,
			SecretKey:    cfg.Storage.S3.SecretKey,
			UsePathStyle: cfg.Storage.S3.PathStyle,
		})
		if err != nil {
			return nil, fmt.Errorf("init s3 storage: %w", err)
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}
}

// InitStorageViews validates storage readiness and starts reconciliation when
// this instance owns it. The returned cleanup stops the worker before releasing
// its filesystem lock, including when a later startup routine fails.
func InitStorageViews(ctx context.Context, cfg *Config, db *gorm.DB) (func(), error) {
	var legacy int64
	if err := db.WithContext(ctx).Model(&models.File{}).Where("type <> 'folder' AND disk_deleted_at IS NULL AND storage_missing = ? AND (storage_key IS NULL OR storage_key = '')", false).Count(&legacy).Error; err != nil {
		return nil, err
	}
	if legacy > 0 {
		return nil, fmt.Errorf("%d retained files require offline storage migration before startup", legacy)
	}
	if cfg.Storage.Layout != "" && cfg.Storage.Layout != "immutable-v1" {
		return nil, fmt.Errorf("unsupported storage.layout %q", cfg.Storage.Layout)
	}
	views, err := services.NewReadableViews(db, cfg.Storage.Local.Root, cfg.Storage.Local.ReadableView.Root, cfg.Storage.Local.ReadableView.Mode)
	if err != nil {
		return nil, err
	}
	if cfg.Storage.Type != StorageLocal && views.Mode != services.ViewModeNone {
		return nil, fmt.Errorf("readable views require local storage")
	}
	if views.Mode != services.ViewModeNone && !cfg.Storage.Local.ReadableView.Owner {
		return func() {}, nil
	}
	var lock *os.File
	if views.Mode != services.ViewModeNone {
		lock, err = views.Lock()
		if err != nil {
			return nil, err
		}
		if err := views.Probe(); err != nil {
			lock.Close()
			return nil, err
		}
	}
	viewCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		views.Run(viewCtx, os.Stderr)
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
			if lock != nil {
				lock.Close()
			}
		})
	}, nil
}
