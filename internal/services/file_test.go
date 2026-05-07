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
