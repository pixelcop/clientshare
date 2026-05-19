package services

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"go.uber.org/zap"
)

type recordingCleanupStorage struct {
	deletedByTenant map[string][]string
	failures        map[string]error
	tenantID        string
}

func newRecordingCleanupStorage() *recordingCleanupStorage {
	return &recordingCleanupStorage{
		deletedByTenant: make(map[string][]string),
		failures:        make(map[string]error),
	}
}

func (s *recordingCleanupStorage) Tenant(tenantID string) storage.Storage {
	return &recordingCleanupStorage{deletedByTenant: s.deletedByTenant, failures: s.failures, tenantID: tenantID}
}

func (s *recordingCleanupStorage) List(path string) ([]storage.FileInfo, error) {
	return nil, nil
}

func (s *recordingCleanupStorage) Upload(dst string, src io.Reader) (int64, error) {
	return 0, nil
}

func (s *recordingCleanupStorage) Download(src string) (io.ReadCloser, error) {
	return nil, nil
}

func (s *recordingCleanupStorage) Delete(path string) error {
	if err := s.failures[path]; err != nil {
		return err
	}
	s.deletedByTenant[s.tenantID] = append(s.deletedByTenant[s.tenantID], path)
	return nil
}

func (s *recordingCleanupStorage) CreateFolder(path string) error {
	return nil
}

func (s *recordingCleanupStorage) Move(src string, dst string) error {
	return nil
}

func (s *recordingCleanupStorage) ZipFolder(path string, dst string) error {
	return nil
}

func TestFileCleanupWorkerRunOnceDeletesEligibleFiles(t *testing.T) {
	db := setupFileServiceTestDB(t)
	now := time.Date(2026, time.May, 19, 12, 0, 0, 0, time.UTC)
	oldDeletedAt := now.Add(-31 * 24 * time.Hour)
	recentDeletedAt := now.Add(-5 * 24 * time.Hour)

	eligible := &models.File{TenantID: tenantctx.DefaultTenantID, ClientID: "client-1", Filename: "old.pdf", Path: "Acme/old.pdf", Type: "file", Size: 1, UploadedAt: now.Add(-60 * 24 * time.Hour), DeletedAt: &oldDeletedAt}
	recent := &models.File{TenantID: tenantctx.DefaultTenantID, ClientID: "client-1", Filename: "recent.pdf", Path: "Acme/recent.pdf", Type: "file", Size: 1, UploadedAt: now.Add(-20 * 24 * time.Hour), DeletedAt: &recentDeletedAt}
	for _, file := range []*models.File{eligible, recent} {
		if err := db.Create(file).Error; err != nil {
			t.Fatalf("failed to seed file %s: %v", file.Filename, err)
		}
	}

	store := newRecordingCleanupStorage()
	worker := NewFileCleanupWorker(db, store, zap.NewNop())
	worker.now = func() time.Time { return now }
	worker.retention = 30 * 24 * time.Hour
	worker.batchSize = 10

	worker.RunOnce(context.Background())

	deleted := store.deletedByTenant[tenantctx.DefaultTenantID]
	if len(deleted) != 1 || deleted[0] != eligible.Path {
		t.Fatalf("expected only %q to be deleted, got %v", eligible.Path, deleted)
	}

	var storedEligible models.File
	if err := db.First(&storedEligible, "tenant_id = ? AND id = ?", tenantctx.DefaultTenantID, eligible.ID).Error; err != nil {
		t.Fatalf("failed to reload eligible file: %v", err)
	}
	if storedEligible.DiskDeletedAt == nil {
		t.Fatal("expected eligible file disk_deleted_at to be set")
	}

	var storedRecent models.File
	if err := db.First(&storedRecent, "tenant_id = ? AND id = ?", tenantctx.DefaultTenantID, recent.ID).Error; err != nil {
		t.Fatalf("failed to reload recent file: %v", err)
	}
	if storedRecent.DiskDeletedAt != nil {
		t.Fatal("expected recent file disk_deleted_at to remain nil")
	}
}

func TestFileCleanupWorkerRunOnceLeavesRecordPendingWhenDeleteFails(t *testing.T) {
	db := setupFileServiceTestDB(t)
	now := time.Date(2026, time.May, 19, 12, 0, 0, 0, time.UTC)
	oldDeletedAt := now.Add(-31 * 24 * time.Hour)
	file := &models.File{TenantID: tenantctx.DefaultTenantID, ClientID: "client-1", Filename: "old.pdf", Path: "Acme/old.pdf", Type: "file", Size: 1, UploadedAt: now.Add(-60 * 24 * time.Hour), DeletedAt: &oldDeletedAt}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	store := newRecordingCleanupStorage()
	store.failures[file.Path] = errors.New("delete failed")
	worker := NewFileCleanupWorker(db, store, zap.NewNop())
	worker.now = func() time.Time { return now }
	worker.retention = 30 * 24 * time.Hour
	worker.batchSize = 10

	worker.RunOnce(context.Background())

	var storedFile models.File
	if err := db.First(&storedFile, "tenant_id = ? AND id = ?", tenantctx.DefaultTenantID, file.ID).Error; err != nil {
		t.Fatalf("failed to reload file: %v", err)
	}
	if storedFile.DiskDeletedAt != nil {
		t.Fatal("expected disk_deleted_at to remain nil after storage delete failure")
	}
}
