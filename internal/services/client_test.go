package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type storageStub struct {
	createFolderErr error
	moveErr         error
}

func (s storageStub) Tenant(string) storage.Storage           { return s }
func (s storageStub) List(string) ([]storage.FileInfo, error) { return nil, nil }
func (s storageStub) Upload(string, io.Reader) (int64, error) { return 0, nil }
func (s storageStub) Download(string) (io.ReadCloser, error)  { return nil, nil }
func (s storageStub) Delete(string) error                     { return nil }
func (s storageStub) CreateFolder(string) error               { return s.createFolderErr }
func (s storageStub) Move(string, string) error               { return s.moveErr }
func (s storageStub) ZipFolder(string, string) error          { return nil }

func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.AutoMigrate(&models.Client{}, &models.File{}, &models.FileActivity{})
	return db, func() {}
}

func TestClientCRUD(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	tmpDir := t.TempDir()
	service := &ClientService{DB: db, Storage: storage.NewLocalStorage(tmpDir)}
	tenantID := tenantctx.DefaultTenantID
	tenantRoot := filepath.Join(tmpDir, "tenants", tenantID)

	// Create
	client, err := service.CreateClient(context.Background(), tenantID, "Acme")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if client.Name != "Acme" {
		t.Errorf("expected name Acme, got %s", client.Name)
	}
	if _, err := os.Stat(filepath.Join(tenantRoot, "Acme")); err != nil {
		t.Errorf("folder not created: %v", err)
	}

	// List
	clients, err := service.ListClients(context.Background(), tenantID, "", "admin")
	if err != nil || len(clients) != 1 {
		t.Errorf("list failed: %v", err)
	}

	// Get
	got, err := service.GetClient(context.Background(), tenantID, client.ID)
	if err != nil || got.Name != "Acme" {
		t.Errorf("get failed: %v", err)
	}

	// Update
	updated, err := service.UpdateClient(context.Background(), tenantID, client.ID, "Beta")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "Beta" {
		t.Errorf("expected name Beta, got %s", updated.Name)
	}
	if _, err := os.Stat(filepath.Join(tenantRoot, "Beta")); err != nil {
		t.Errorf("folder not renamed: %v", err)
	}

	// Delete
	if err := service.DeleteClient(context.Background(), tenantID, client.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tenantRoot, "trash", "Beta")); err != nil {
		t.Errorf("folder not moved to trash: %v", err)
	}
	if _, err := service.GetClient(context.Background(), tenantID, client.ID); err == nil {
		t.Errorf("client not deleted from db")
	}
}

func TestListClientsPaginated(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	service := &ClientService{DB: db, Storage: storage.NewLocalStorage(t.TempDir())}
	tenantID := tenantctx.DefaultTenantID

	for _, name := range []string{"Acme", "Beta", "Charlie"} {
		if _, err := service.CreateClient(context.Background(), tenantID, name); err != nil {
			t.Fatalf("create %s failed: %v", name, err)
		}
	}

	pagination, clients, err := service.ListClientsPaginated(context.Background(), tenantID, "", "admin", "a", 1, 2)
	if err != nil {
		t.Fatalf("paginated list failed: %v", err)
	}
	if pagination.TotalItems != 3 {
		t.Fatalf("expected 3 total items, got %d", pagination.TotalItems)
	}
	if pagination.TotalPages != 2 {
		t.Fatalf("expected 2 total pages, got %d", pagination.TotalPages)
	}
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients on first page, got %d", len(clients))
	}
	if clients[0].Name != "Acme" || clients[1].Name != "Beta" {
		t.Fatalf("unexpected client order: %+v", []string{clients[0].Name, clients[1].Name})
	}

	pagination, clients, err = service.ListClientsPaginated(context.Background(), tenantID, "", "admin", "char", 1, 2)
	if err != nil {
		t.Fatalf("search list failed: %v", err)
	}
	if pagination.TotalItems != 1 {
		t.Fatalf("expected 1 filtered item, got %d", pagination.TotalItems)
	}
	if len(clients) != 1 || clients[0].Name != "Charlie" {
		t.Fatalf("expected only Charlie in filtered results")
	}
}

func TestCreateClientRollsBackOnStorageError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := &ClientService{DB: db, Storage: storageStub{createFolderErr: os.ErrPermission}}
	tenantID := tenantctx.DefaultTenantID

	client, err := service.CreateClient(context.Background(), tenantID, "Acme")
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected permission error, got %v", err)
	}
	if client != nil {
		t.Fatalf("expected no client, got %+v", client)
	}

	var clientCount int64
	if err := db.Model(&models.Client{}).Where("tenant_id = ?", tenantID).Count(&clientCount).Error; err != nil {
		t.Fatalf("count clients failed: %v", err)
	}
	if clientCount != 0 {
		t.Fatalf("expected no clients after rollback, got %d", clientCount)
	}

	var fileCount int64
	if err := db.Model(&models.File{}).Where("tenant_id = ?", tenantID).Count(&fileCount).Error; err != nil {
		t.Fatalf("count files failed: %v", err)
	}
	if fileCount != 0 {
		t.Fatalf("expected no files after rollback, got %d", fileCount)
	}
}

func TestUpdateClientRestoresFolderOnDBError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tmpDir := t.TempDir()
	service := &ClientService{DB: db, Storage: storage.NewLocalStorage(tmpDir)}
	tenantID := tenantctx.DefaultTenantID
	tenantRoot := filepath.Join(tmpDir, "tenants", tenantID)

	client, err := service.CreateClient(context.Background(), tenantID, "Acme")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	callbackName := "test:fail_client_update"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "clients" {
			tx.AddError(errors.New("forced client update failure"))
		}
	}); err != nil {
		t.Fatalf("register update callback failed: %v", err)
	}
	defer db.Callback().Update().Remove(callbackName)

	_, err = service.UpdateClient(context.Background(), tenantID, client.ID, "Beta")
	if err == nil || !strings.Contains(err.Error(), "forced client update failure") {
		t.Fatalf("expected forced update failure, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(tenantRoot, "Acme")); err != nil {
		t.Fatalf("expected original folder to be restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tenantRoot, "Beta")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected renamed folder to be rolled back, got %v", err)
	}

	reloaded, err := service.GetClient(context.Background(), tenantID, client.ID)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if reloaded.Name != "Acme" || reloaded.FolderPath != "Acme" {
		t.Fatalf("expected client to remain unchanged, got name=%s folder=%s", reloaded.Name, reloaded.FolderPath)
	}
}

func TestDeleteClientRestoresFolderOnDBError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tmpDir := t.TempDir()
	service := &ClientService{DB: db, Storage: storage.NewLocalStorage(tmpDir)}
	tenantID := tenantctx.DefaultTenantID
	tenantRoot := filepath.Join(tmpDir, "tenants", tenantID)

	client, err := service.CreateClient(context.Background(), tenantID, "Acme")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	callbackName := "test:fail_client_delete"
	if err := db.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "clients" {
			tx.AddError(errors.New("forced client delete failure"))
		}
	}); err != nil {
		t.Fatalf("register delete callback failed: %v", err)
	}
	defer db.Callback().Delete().Remove(callbackName)

	err = service.DeleteClient(context.Background(), tenantID, client.ID)
	if err == nil || !strings.Contains(err.Error(), "forced client delete failure") {
		t.Fatalf("expected forced delete failure, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(tenantRoot, "Acme")); err != nil {
		t.Fatalf("expected original folder to be restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tenantRoot, "trash", "Acme")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected trash move to be rolled back, got %v", err)
	}

	if _, err := service.GetClient(context.Background(), tenantID, client.ID); err != nil {
		t.Fatalf("expected client to remain in db: %v", err)
	}
}

func TestFileOperationsLifecycle(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	tmpDir := t.TempDir()
	tenantID := tenantctx.DefaultTenantID
	store := storage.NewLocalStorage(tmpDir).Tenant(tenantID)
	tenantRoot := filepath.Join(tmpDir, "tenants", tenantID)
	client := &models.Client{Name: "Acme", FolderPath: "Acme"}
	db.Create(client)
	os.MkdirAll(filepath.Join(tenantRoot, "Acme"), 0755)

	// Upload
	f, err := os.Create(filepath.Join(tenantRoot, "Acme", "test.txt"))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	f.WriteString("hello world")
	f.Close()

	// List root
	files, err := store.List("Acme")
	if err != nil || len(files) == 0 {
		t.Errorf("list root failed: %v", err)
	}

	// Create subfolder
	if err := store.CreateFolder("Acme/sub"); err != nil {
		t.Errorf("create subfolder failed: %v", err)
	}

	// List nested
	files, err = store.List("Acme/sub")
	if err != nil {
		t.Errorf("list nested failed: %v", err)
	}

	// Download
	reader, err := store.Download("Acme/test.txt")
	if err != nil {
		t.Errorf("download failed: %v", err)
	}
	reader.Close()

	// Zip folder
	zipPath := tmpDir + "/Acme.zip"
	if err := store.ZipFolder("Acme", zipPath); err != nil {
		t.Errorf("zip failed: %v", err)
	}
	os.Remove(zipPath)

	// Delete file
	if err := store.Delete("Acme/test.txt"); err != nil {
		t.Errorf("delete file failed: %v", err)
	}

	// Delete folder
	if err := store.Delete("Acme/sub"); err != nil {
		t.Errorf("delete folder failed: %v", err)
	}
}
