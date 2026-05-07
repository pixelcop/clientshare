package clientshare

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pixelcop/clientshare/internal/services/storage"
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
