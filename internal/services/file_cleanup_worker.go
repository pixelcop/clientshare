package services

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/pixelcop/clientshare/internal/services/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	DefaultFileCleanupInterval  = 24 * time.Hour
	DefaultFileCleanupRetention = 30 * 24 * time.Hour
	defaultFileCleanupBatchSize = 100
)

type FileCleanupWorker struct {
	fileService  *FileService
	store        storage.Storage
	logger       *zap.Logger
	stopCh       chan struct{}
	stopOnce     sync.Once
	pollInterval time.Duration
	retention    time.Duration
	batchSize    int
	now          func() time.Time
}

func NewFileCleanupWorker(db *gorm.DB, store storage.Storage, logger *zap.Logger) *FileCleanupWorker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &FileCleanupWorker{
		fileService:  NewFileService(db),
		store:        store,
		logger:       logger,
		stopCh:       make(chan struct{}),
		pollInterval: DefaultFileCleanupInterval,
		retention:    DefaultFileCleanupRetention,
		batchSize:    defaultFileCleanupBatchSize,
		now:          time.Now,
	}
}

func (w *FileCleanupWorker) Start() {
	go func() {
		w.RunOnce(context.Background())

		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				w.RunOnce(context.Background())
			case <-w.stopCh:
				return
			}
		}
	}()
}

func (w *FileCleanupWorker) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
}

func (w *FileCleanupWorker) RunOnce(ctx context.Context) {
	if w == nil || w.fileService == nil || w.store == nil {
		return
	}

	for {
		cutoff := w.now().Add(-w.retention)
		files, err := w.fileService.ListFilesPendingDiskDelete(ctx, cutoff, w.batchSize)
		if err != nil {
			w.logger.Error("list files pending disk deletion", zap.Error(err))
			return
		}
		if len(files) == 0 {
			return
		}

		for _, file := range files {
			if err := w.store.Tenant(file.TenantID).Delete(file.Path); err != nil {
				w.logger.Warn("delete file from storage", zap.Error(err), zap.String("tenant_id", file.TenantID), zap.String("file_id", file.ID), zap.String("path", file.Path))
				continue
			}
			deletedAt := w.now()
			if err := w.fileService.MarkFileDiskDeleted(ctx, file.TenantID, file.ID, deletedAt); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				w.logger.Error("mark file disk deleted", zap.Error(err), zap.String("tenant_id", file.TenantID), zap.String("file_id", file.ID), zap.String("path", file.Path))
			}
		}

		if len(files) < w.batchSize {
			return
		}
	}
}
