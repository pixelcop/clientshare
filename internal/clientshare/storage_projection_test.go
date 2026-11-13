package clientshare

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStorageProjectionCLIIsolatedRehearsal(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "rehearsal.db")
	objectRoot := filepath.Join(root, "storage")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.File{}, &models.ReadableView{}))
	config := strings.ReplaceAll(mockConfig, "\t", "  ")
	config = strings.ReplaceAll(config, "/tmp/test.db", dbPath)
	config = strings.ReplaceAll(config, "/tmp/storage", objectRoot)
	config = strings.Replace(config, "storage:\n", "storage:\n  layout: immutable-v2\n", 1)
	config = strings.Replace(config, "    root: ", "    readable_view:\n      mode: hardlink\n      owner: true\n    root: ", 1)
	configPath := filepath.Join(root, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0600))
	ctx := context.Background()
	// Existing data includes client roots ending in periods and spaces.
	for i, name := range []string{"Example Consulting P.C.", "Trailing Space "} {
		client, err := (&services.ClientService{DB: db}).CreateClient(ctx, "tenant", fmt.Sprintf("seed-%d", i))
		require.NoError(t, err)
		require.NoError(t, db.Model(client).Updates(map[string]interface{}{"name": name, "folder_path": name}).Error)
		require.NoError(t, db.Model(&models.File{}).Where("id = ?", *client.RootFolderID).Update("filename", name).Error)
		file := &models.File{TenantID: "tenant", ClientID: client.ID, FolderID: client.RootFolderID, Filename: "report.txt", Path: name + "/report.txt", Type: "file", Size: 5}
		require.NoError(t, db.Create(file).Error)
		_, err = storage.NewLocalStorage(objectRoot).Tenant("tenant").(storage.LegacyStorage).Upload(file.Path, strings.NewReader("hello"))
		require.NoError(t, err)
	}
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := StorageCommand(ctx, append(args, "--config", configPath), &out)
		return out.String(), err
	}
	output, err := run("inventory")
	require.NoError(t, err)
	manifestPath := filepath.Join(root, "inventory.json")
	require.NoError(t, os.WriteFile(manifestPath, []byte(output), 0600))
	_, err = run("migrate", "--manifest", manifestPath, "--view-mode", "hardlink", "--apply")
	require.NoError(t, err)
	output, err = run("verify")
	require.NoError(t, err)
	require.Contains(t, output, "Verified 2 retained objects")
	output, err = run("view-reconcile")
	require.NoError(t, err)
	require.Contains(t, output, `"tenant/Example Consulting P.C_"`)
	require.Contains(t, output, `"tenant/Trailing Space_"`)
	require.NoDirExists(t, filepath.Join(objectRoot, "readable/tenant"))
	_, err = run("view-reconcile", "--apply")
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(objectRoot, "readable/tenant/Example Consulting P.C_/report.txt"))
	require.FileExists(t, filepath.Join(objectRoot, "readable/tenant/Trailing Space_/report.txt"))
	_, err = run("verify")
	require.NoError(t, err)
	// Every affected client is reported; a later conflict must prevent an
	// otherwise clean earlier client from being partially applied by the CLI.
	clean, err := (&services.ClientService{DB: db}).CreateClient(ctx, "tenant", "Clean.")
	require.NoError(t, err)
	for _, name := range []string{"Collision.", "Collision_"} {
		_, err := (&services.ClientService{DB: db}).CreateClient(ctx, "tenant", name)
		require.NoError(t, err)
	}
	for _, apply := range []bool{false, true} {
		args := []string{"view-reconcile"}
		if apply {
			args = append(args, "--apply")
		}
		output, err = run(args...)
		require.ErrorContains(t, err, "projection root collision")
		require.Equal(t, 2, strings.Count(err.Error(), "projection root collision"))
		if !apply {
			require.Contains(t, output, clean.ID)
		}
		require.NoDirExists(t, filepath.Join(objectRoot, "readable/tenant/Clean_"))
		var state models.ReadableView
		require.NoError(t, db.First(&state, "client_id = ?", clean.ID).Error)
		require.Empty(t, state.Manifest)
	}
}
