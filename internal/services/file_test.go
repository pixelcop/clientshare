package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFileServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := database.AutoMigrate(&models.File{}); err != nil {
		t.Fatalf("failed to migrate files table: %v", err)
	}
	return database
}

func createFolderRecord(t *testing.T, db *gorm.DB, clientID string, parentID *string, name, path string) *models.File {
	t.Helper()
	folder := &models.File{
		TenantID:   tenantctx.DefaultTenantID,
		ClientID:   clientID,
		FolderID:   parentID,
		Filename:   name,
		Path:       path,
		Type:       "folder",
		Size:       0,
		UploadedAt: time.Now(),
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("failed to create folder %s: %v", name, err)
	}
	return folder
}

func TestGetFolderHierarchyByID(t *testing.T) {
	db := setupFileServiceTestDB(t)
	service := NewFileService(db)

	root := createFolderRecord(t, db, "client-1", nil, "Acme", "Acme")
	parent := createFolderRecord(t, db, "client-1", &root.ID, "Reports", "Acme/Reports")
	leaf := createFolderRecord(t, db, "client-1", &parent.ID, "2026", "Acme/Reports/2026")

	hierarchy, err := service.GetFolderHierarchyByID(context.Background(), tenantctx.DefaultTenantID, leaf.ClientID, leaf.ID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(hierarchy) != 3 {
		t.Fatalf("expected 3 folders in hierarchy, got %d", len(hierarchy))
	}

	if hierarchy[0].ID != root.ID {
		t.Fatalf("expected first folder to be root %s, got %s", root.ID, hierarchy[0].ID)
	}
	if hierarchy[1].ID != parent.ID {
		t.Fatalf("expected second folder to be parent %s, got %s", parent.ID, hierarchy[1].ID)
	}
	if hierarchy[2].ID != leaf.ID {
		t.Fatalf("expected last folder to be leaf %s, got %s", leaf.ID, hierarchy[2].ID)
	}
}

func TestGetFolderHierarchyByIDNotFound(t *testing.T) {
	db := setupFileServiceTestDB(t)
	service := NewFileService(db)

	_, err := service.GetFolderHierarchyByID(context.Background(), tenantctx.DefaultTenantID, "client-1", "does-not-exist")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestGetFolderHierarchyByIDCacheInvalidatedOnFolderCreate(t *testing.T) {
	db := setupFileServiceTestDB(t)
	service := NewFileService(db)

	root := createFolderRecord(t, db, "client-1", nil, "Acme", "Acme")
	parent := createFolderRecord(t, db, "client-1", &root.ID, "Reports", "Acme/Reports")
	leaf := createFolderRecord(t, db, "client-1", &parent.ID, "2026", "Acme/Reports/2026")

	hierarchy, err := service.GetFolderHierarchyByID(context.Background(), tenantctx.DefaultTenantID, leaf.ClientID, leaf.ID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(hierarchy) != 3 {
		t.Fatalf("expected 3 folders in hierarchy, got %d", len(hierarchy))
	}

	now := time.Now()
	if err := db.Model(&models.File{}).Where("tenant_id = ? AND id = ?", tenantctx.DefaultTenantID, root.ID).Update("deleted_at", &now).Error; err != nil {
		t.Fatalf("failed to soft delete root folder: %v", err)
	}

	cachedHierarchy, err := service.GetFolderHierarchyByID(context.Background(), tenantctx.DefaultTenantID, leaf.ClientID, leaf.ID)
	if err != nil {
		t.Fatalf("expected no error on cached hierarchy, got: %v", err)
	}
	if len(cachedHierarchy) != 3 {
		t.Fatalf("expected cached hierarchy of 3 folders, got %d", len(cachedHierarchy))
	}

	newFolder := &models.File{
		TenantID:   tenantctx.DefaultTenantID,
		ClientID:   root.ClientID,
		FolderID:   &parent.ID,
		Filename:   "Q1",
		Path:       "Acme/Reports/Q1",
		Type:       "folder",
		Size:       0,
		UploadedAt: time.Now(),
	}
	if _, err := service.CreateFolderRecord(context.Background(), newFolder); err != nil {
		t.Fatalf("expected folder create to succeed, got: %v", err)
	}

	recomputedHierarchy, err := service.GetFolderHierarchyByID(context.Background(), tenantctx.DefaultTenantID, leaf.ClientID, leaf.ID)
	if err != nil {
		t.Fatalf("expected no error after cache invalidation, got: %v", err)
	}
	if len(recomputedHierarchy) != 2 {
		t.Fatalf("expected recomputed hierarchy of 2 folders after root deletion, got %d", len(recomputedHierarchy))
	}
}

func TestListFilesPendingDiskDeleteOnlyReturnsEligibleRows(t *testing.T) {
	db := setupFileServiceTestDB(t)
	service := NewFileService(db)
	now := time.Now()
	oldDeletedAt := now.Add(-31 * 24 * time.Hour)
	recentDeletedAt := now.Add(-7 * 24 * time.Hour)
	alreadyDiskDeletedAt := now.Add(-2 * 24 * time.Hour)

	eligible := &models.File{TenantID: tenantctx.DefaultTenantID, ClientID: "client-1", Filename: "eligible.pdf", Path: "Acme/eligible.pdf", Type: "file", Size: 1, UploadedAt: now.Add(-40 * 24 * time.Hour), DeletedAt: &oldDeletedAt}
	recent := &models.File{TenantID: tenantctx.DefaultTenantID, ClientID: "client-1", Filename: "recent.pdf", Path: "Acme/recent.pdf", Type: "file", Size: 1, UploadedAt: now.Add(-10 * 24 * time.Hour), DeletedAt: &recentDeletedAt}
	alreadyDeleted := &models.File{TenantID: tenantctx.DefaultTenantID, ClientID: "client-1", Filename: "done.pdf", Path: "Acme/done.pdf", Type: "file", Size: 1, UploadedAt: now.Add(-50 * 24 * time.Hour), DeletedAt: &oldDeletedAt, DiskDeletedAt: &alreadyDiskDeletedAt}
	active := &models.File{TenantID: tenantctx.DefaultTenantID, ClientID: "client-1", Filename: "active.pdf", Path: "Acme/active.pdf", Type: "file", Size: 1, UploadedAt: now}

	for _, file := range []*models.File{eligible, recent, alreadyDeleted, active} {
		if err := db.Create(file).Error; err != nil {
			t.Fatalf("failed to seed file %s: %v", file.Filename, err)
		}
	}

	files, err := service.ListFilesPendingDiskDelete(context.Background(), now.Add(-30*24*time.Hour), 10)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 eligible file, got %d", len(files))
	}
	if files[0].ID != eligible.ID {
		t.Fatalf("expected eligible file %s, got %s", eligible.ID, files[0].ID)
	}
}

func TestMarkFileDiskDeletedSetsTimestamp(t *testing.T) {
	db := setupFileServiceTestDB(t)
	service := NewFileService(db)
	now := time.Now().UTC().Truncate(time.Second)
	deletedAt := now.Add(-31 * 24 * time.Hour)
	file := &models.File{TenantID: tenantctx.DefaultTenantID, ClientID: "client-1", Filename: "eligible.pdf", Path: "Acme/eligible.pdf", Type: "file", Size: 1, UploadedAt: now.Add(-40 * 24 * time.Hour), DeletedAt: &deletedAt}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	markAt := now.Add(5 * time.Minute)
	if err := service.MarkFileDiskDeleted(context.Background(), tenantctx.DefaultTenantID, file.ID, markAt); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var stored models.File
	if err := db.First(&stored, "tenant_id = ? AND id = ?", tenantctx.DefaultTenantID, file.ID).Error; err != nil {
		t.Fatalf("failed to reload file: %v", err)
	}
	if stored.DiskDeletedAt == nil {
		t.Fatal("expected disk_deleted_at to be set")
	}
	if !stored.DiskDeletedAt.Equal(markAt) {
		t.Fatalf("expected disk_deleted_at %v, got %v", markAt, stored.DiskDeletedAt)
	}
}
