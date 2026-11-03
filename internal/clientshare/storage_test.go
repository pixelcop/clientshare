package clientshare

import (
	"context"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/stretchr/testify/require"
)

func TestInitStorageViewsValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      string
		layout    string
		provider  string
		legacy    bool
		wantError string
	}{
		{name: "default", provider: StorageLocal},
		{name: "unmigrated", provider: StorageLocal, legacy: true, wantError: "require offline storage migration"},
		{name: "unknown layout", provider: StorageLocal, layout: "unknown", wantError: "unsupported storage.layout"},
		{name: "unknown mode", provider: StorageLocal, mode: "unknown", wantError: "unknown readable view mode"},
		{name: "S3 view", provider: StorageS3, mode: services.ViewModeExport, wantError: "require local storage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			require.NoError(t, db.AutoMigrate(&models.File{}, &models.ReadableView{}))
			cfg := newTestConfig(t.TempDir())
			cfg.Storage.Type = tc.provider
			cfg.Storage.Layout = tc.layout
			cfg.Storage.Local.ReadableView.Mode = tc.mode
			if tc.legacy {
				require.NoError(t, db.Create(&models.File{TenantID: "tenant", ClientID: "client", Filename: "old.txt", Type: "file"}).Error)
			}
			stop, err := InitStorageViews(context.Background(), cfg, db)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, stop)
				return
			}
			require.NoError(t, err)
			stop()
			stop() // Cleanup is safe to call more than once.
		})
	}
}

func TestInitStorageViewsOwnerCleanup(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.File{}, &models.ReadableView{}))
	cfg := newTestConfig(t.TempDir())
	cfg.Storage.Type = StorageLocal
	cfg.Storage.Local.ReadableView.Mode = services.ViewModeExport
	views, err := services.NewReadableViews(db, cfg.Storage.Local.Root, "", services.ViewModeExport)
	require.NoError(t, err)
	// A non-owner does not acquire the lock or start a worker.
	stop, err := InitStorageViews(context.Background(), cfg, db)
	require.NoError(t, err)
	stop()
	lock, err := views.Lock()
	require.NoError(t, err)
	require.NoError(t, lock.Close())

	cfg.Storage.Local.ReadableView.Owner = true
	stop, err = InitStorageViews(context.Background(), cfg, db)
	require.NoError(t, err)
	t.Cleanup(stop)
	_, err = views.Lock()
	require.Error(t, err)
	stop()
	lock, err = views.Lock()
	require.NoError(t, err)
	require.NoError(t, lock.Close())
}
