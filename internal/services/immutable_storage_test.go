package services

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func migrationFixture(t *testing.T) (*gorm.DB, string, *models.Client, *models.File) {
	t.Helper()
	db, _ := setupTestDB(t)
	root := t.TempDir()
	ctx := context.Background()
	client, err := (&ClientService{DB: db}).CreateClient(ctx, "tenant", "Acme")
	require.NoError(t, err)
	f := &models.File{TenantID: "tenant", ClientID: client.ID, FolderID: client.RootFolderID, Filename: "report.txt", Path: "Acme/report.txt", Type: "file", Size: 5}
	require.NoError(t, db.Create(f).Error)
	legacy := storage.NewLocalStorage(root).Tenant("tenant").(storage.LegacyStorage)
	_, err = legacy.Upload(f.Path, strings.NewReader("hello"))
	require.NoError(t, err)
	return db, root, client, f
}

func TestOfflineMigrationResumesVerifiesAndPreservesLegacy(t *testing.T) {
	db, root, client, file := migrationFixture(t)
	ctx := context.Background()
	plan, err := InventoryObjects(ctx, db, root, nil)
	require.NoError(t, err)
	require.Empty(t, plan.Problems)
	require.Len(t, plan.Entries, 1)
	require.NoDirExists(t, filepath.Join(root, "objects"))
	// Simulate interruption after publication and before database commit.
	store := storage.NewLocalStorage(root).Tenant("tenant")
	_, err = store.PutNew(ctx, file.ID, strings.NewReader("hello"))
	require.NoError(t, err)
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	require.NoError(t, db.First(file, "id = ?", file.ID).Error)
	require.Equal(t, file.ID, *file.StorageKey)
	require.FileExists(t, filepath.Join(root, "tenants", "tenant", "Acme", "report.txt"))
	_, err = (&ClientService{DB: db}).UpdateClient(ctx, "tenant", client.ID, "Renamed")
	require.NoError(t, err)
	require.NoError(t, db.First(file, "id = ?", file.ID).Error)
	require.Equal(t, "Renamed/report.txt", file.Path)
	r, err := OpenFile(ctx, store, file)
	require.NoError(t, err)
	body, err := io.ReadAll(r)
	r.Close()
	require.NoError(t, err)
	require.Equal(t, "hello", string(body))
	var archive bytes.Buffer
	var folder models.File
	require.NoError(t, db.First(&folder, "id = ?", *client.RootFolderID).Error)
	require.NoError(t, NewFileService(db).WriteFolderZip(ctx, store, &folder, &archive))
	z, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	require.NoError(t, err)
	require.Len(t, z.File, 1)
	require.Equal(t, "report.txt", z.File[0].Name)
	// A tampered destination must block even a repeated apply.
	require.NoError(t, os.WriteFile(filepath.Join(root, "objects", "tenant", file.ID), []byte("wrong"), 0600))
	require.ErrorContains(t, ApplyObjects(ctx, db, root, plan), "verification failed")
}

func TestMigrationStaleMissingAndRetainedFiles(t *testing.T) {
	db, root, _, file := migrationFixture(t)
	ctx := context.Background()
	now := time.Now()
	require.NoError(t, db.Model(file).Update("deleted_at", now).Error)
	require.NoError(t, os.Rename(filepath.Join(root, "tenants", "tenant", "Acme"), filepath.Join(root, "tenants", "tenant", "Renamed")))
	plan, err := InventoryObjects(ctx, db, root, nil)
	require.NoError(t, err)
	require.NotEmpty(t, plan.Problems)
	require.Error(t, ApplyObjects(ctx, db, root, plan))
	plan, err = InventoryObjects(ctx, db, root, map[string]string{"tenant/" + file.ID: "tenants/tenant/Renamed/report.txt"})
	require.NoError(t, err)
	require.Empty(t, plan.Problems)
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	require.NoError(t, db.First(file, "id = ?", file.ID).Error)
	require.NotNil(t, file.DeletedAt)
}

func TestReadableViewsModesRecoveryAndConflicts(t *testing.T) {
	for _, mode := range []string{"none", "hardlink", "symlink", "export"} {
		t.Run(mode, func(t *testing.T) {
			db, root, client, file := migrationFixture(t)
			ctx := context.Background()
			plan, err := InventoryObjects(ctx, db, root, nil)
			require.NoError(t, err)
			require.NoError(t, ApplyObjects(ctx, db, root, plan))
			view, err := NewReadableViews(db, root, "", mode)
			require.NoError(t, err)
			require.NoError(t, view.Probe())
			lock, err := view.Lock()
			require.NoError(t, err)
			defer lock.Close()
			_, err = view.Lock()
			require.Error(t, err)
			var state models.ReadableView
			reload := func() {
				require.NoError(t, db.First(&state, "tenant_id = ? AND client_id = ?", "tenant", client.ID).Error)
			}
			reload()
			require.NoError(t, view.Reconcile(ctx, state, true, io.Discard))
			require.NoFileExists(t, filepath.Join(view.Root, "tenant", "Acme", "report.txt"))
			require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
			reload()
			require.Equal(t, state.DesiredRevision, state.AppliedRevision)
			if mode == "none" {
				require.NoDirExists(t, view.Root)
				return
			}
			path := filepath.Join(view.Root, "tenant", "Acme", "report.txt")
			b, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "hello", string(b))
			// Simulate a crash with persisted intent but missing generated entry.
			require.NoError(t, os.Remove(path))
			require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
			if mode == "export" {
				require.NoError(t, os.WriteFile(path, []byte("edited"), 0600))
				require.ErrorContains(t, view.Reconcile(ctx, state, false, io.Discard), "modified export")
				require.NoError(t, os.WriteFile(path, []byte("hello"), 0600))
			}
			for _, name := range []string{"Second", "second", "Third"} {
				_, err := (&ClientService{DB: db}).UpdateClient(ctx, "tenant", client.ID, name)
				require.NoError(t, err)
				reload()
				require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
			}
			require.NoFileExists(t, path)
			require.FileExists(t, filepath.Join(view.Root, "tenant", "Third", "report.txt"))
			require.NoError(t, NewFileService(db).SoftDeleteFile(ctx, "tenant", file))
			reload()
			require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
			require.NoFileExists(t, filepath.Join(view.Root, "tenant", "Third", "report.txt"))
			require.FileExists(t, filepath.Join(root, "objects", "tenant", file.ID))
			var manifest ViewManifest
			reload()
			require.NoError(t, json.Unmarshal([]byte(state.Manifest), &manifest))
			require.Len(t, manifest.Entries, 1)
		})
	}
}

func TestMissingAcknowledgmentAndOrphanGrace(t *testing.T) {
	db, root, _, file := migrationFixture(t)
	ctx := context.Background()
	require.NoError(t, os.Remove(filepath.Join(root, "tenants", "tenant", "Acme", "report.txt")))
	plan, err := InventoryObjects(ctx, db, root, nil, "tenant/"+file.ID)
	require.NoError(t, err)
	require.Empty(t, plan.Problems)
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	require.NoError(t, db.First(file, "id = ?", file.ID).Error)
	require.True(t, file.StorageMissing)
	store := storage.NewLocalStorage(root).Tenant("tenant")
	_, err = OpenFile(ctx, store, file)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = store.PutNew(ctx, "old-orphan", strings.NewReader("abandoned"))
	require.NoError(t, err)
	_, err = store.PutNew(ctx, "new-orphan", strings.NewReader("in flight"))
	require.NoError(t, err)
	old := filepath.Join(root, "objects", "tenant", "old-orphan")
	past := time.Now().Add(-8 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(old, past, past))
	require.NoError(t, CleanupOrphanObjects(ctx, db, root, 7*24*time.Hour, false, io.Discard))
	require.FileExists(t, old)
	require.NoError(t, CleanupOrphanObjects(ctx, db, root, 7*24*time.Hour, true, io.Discard))
	require.NoFileExists(t, old)
	require.FileExists(t, filepath.Join(root, "objects", "tenant", "new-orphan"))
}

func TestUploadDatabaseFailureCleansOnlyNewObject(t *testing.T) {
	db := quotaTestDB(t)
	ctx := context.Background()
	store := storage.NewLocalStorage(t.TempDir()).Tenant("a")
	_, err := store.PutNew(ctx, "previous", strings.NewReader("prior"))
	require.NoError(t, err)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_file", func(tx *gorm.DB) {
		if tx.Statement.Table == "files" {
			tx.AddError(errors.New("forced persistence failure"))
		}
	}))
	defer db.Callback().Create().Remove("test:fail_file")
	file := &models.File{TenantID: "a", ClientID: "client", Filename: "new.txt", Path: "new.txt", Type: "file"}
	require.ErrorContains(t, UploadFile(ctx, db, store, file, strings.NewReader("hello"), 5), "forced persistence failure")
	_, err = store.Stat(ctx, *file.StorageKey)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = store.Stat(ctx, "previous")
	require.NoError(t, err)
}

func TestViewRevisionAndRetentionWaitForManagedCleanup(t *testing.T) {
	db, root, client, file := migrationFixture(t)
	ctx := context.Background()
	plan, err := InventoryObjects(ctx, db, root, nil)
	require.NoError(t, err)
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	view, err := NewReadableViews(db, root, "", "export")
	require.NoError(t, err)
	require.NoError(t, view.Probe())
	var state models.ReadableView
	reload := func() {
		require.NoError(t, db.First(&state, "tenant_id = ? AND client_id = ?", "tenant", client.ID).Error)
	}
	reload()
	require.NoError(t, models.DirtyView(db, "tenant", client.ID))
	require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
	reload()
	require.NotEqual(t, state.DesiredRevision, state.AppliedRevision)
	require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
	require.NoError(t, NewFileService(db).SoftDeleteFile(ctx, "tenant", file))
	require.NoError(t, db.Model(file).Update("deleted_at", time.Now().Add(-31*24*time.Hour)).Error)
	copyPath := filepath.Join(view.Root, "tenant", "Acme", "report.txt")
	require.NoError(t, os.WriteFile(copyPath, []byte("external edit"), 0600))
	reload()
	require.Error(t, view.Reconcile(ctx, state, false, io.Discard))
	worker := NewFileCleanupWorker(db, storage.NewLocalStorage(root), zap.NewNop())
	worker.RunOnce(ctx)
	require.FileExists(t, filepath.Join(root, "objects", "tenant", file.ID))
	require.NoError(t, os.WriteFile(copyPath, []byte("hello"), 0600))
	reload()
	require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
	worker.RunOnce(ctx)
	require.NoFileExists(t, copyPath)
	require.NoFileExists(t, filepath.Join(root, "objects", "tenant", file.ID))
	require.NoError(t, db.First(file, "id = ?", file.ID).Error)
	require.NotNil(t, file.DiskDeletedAt)
}

func TestReadableViewUnmanagedCollision(t *testing.T) {
	db, root, client, _ := migrationFixture(t)
	ctx := context.Background()
	plan, err := InventoryObjects(ctx, db, root, nil)
	require.NoError(t, err)
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	view, err := NewReadableViews(db, root, "", "export")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(view.Root, "tenant", "acme"), 0700))
	var state models.ReadableView
	require.NoError(t, db.First(&state, "tenant_id = ? AND client_id = ?", "tenant", client.ID).Error)
	require.ErrorContains(t, view.Reconcile(ctx, state, false, io.Discard), "unmanaged view collision")
}
