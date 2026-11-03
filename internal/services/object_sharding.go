package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"gorm.io/gorm"
)

// ShardFlatObjects is an offline conversion for installations that already used
// flat immutable objects. Legacy full-path installations use ApplyObjects
// directly instead. Database identities and legacy full-path files never change.
// A verified, no-replace hardlink is synced before removing each flat name; a
// rerun accepts either or both names after interruption. Runtime reads never do.
func ShardFlatObjects(ctx context.Context, db *gorm.DB, root string, apply bool, out io.Writer) error {
	var states []models.ReadableView
	if err := db.WithContext(ctx).Find(&states).Error; err != nil {
		return err
	}
	for _, state := range states {
		if state.Manifest == "" {
			continue
		}
		var manifest ViewManifest
		if err := json.Unmarshal([]byte(state.Manifest), &manifest); err != nil {
			return err
		}
		if len(manifest.Entries) != 0 {
			return errors.New("clear readable views using the old version before sharding objects")
		}
	}
	var files []models.File
	if err := db.WithContext(ctx).Where("type <> 'folder' AND disk_deleted_at IS NULL AND storage_missing = ?", false).Order("tenant_id, id").Find(&files).Error; err != nil {
		return err
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if file.StorageKey == nil || *file.StorageKey == "" {
			return fmt.Errorf("unmigrated file %s: use storage inventory/migrate for full-path storage", file.ID)
		}
		store := storage.NewLocalStorage(root).Tenant(file.TenantID).(*storage.LocalStorage)
		dest, err := store.ObjectPath(*file.StorageKey)
		if err != nil {
			return err
		}
		source, err := storage.SafePath(root, filepath.Join("objects", file.TenantID, *file.StorageKey))
		if err != nil {
			return err
		}
		expected := storage.ObjectInfo{Size: file.Size, Checksum: file.Checksum}
		original, sourceErr := hashRegularFile(ctx, source)
		if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
			return sourceErr
		}
		if sourceErr == nil && original != expected {
			return fmt.Errorf("flat object checksum/size mismatch: %s", file.ID)
		}
		sharded, destErr := hashRegularFile(ctx, dest)
		if destErr != nil && !errors.Is(destErr, os.ErrNotExist) {
			return destErr
		}
		if destErr == nil && sharded != expected {
			return fmt.Errorf("sharded object checksum/size mismatch: %s", file.ID)
		}
		if sourceErr != nil && destErr != nil {
			return fmt.Errorf("object missing at both flat and sharded paths: %s", file.ID)
		}
		if sourceErr != nil {
			continue // Previously completed object; verified above.
		}
		fmt.Fprintf(out, "shard %s/%s\n", file.TenantID, *file.StorageKey)
		if !apply {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		if destErr != nil {
			if err := os.Link(source, dest); err != nil {
				return fmt.Errorf("publish sharded object without replacement: %w", err)
			}
		}
		if err := syncObjectDirectory(filepath.Dir(dest)); err != nil {
			return err
		}
		// Persist the new bucket's directory entry before removing the old name.
		if err := syncObjectDirectory(filepath.Dir(source)); err != nil {
			return err
		}
		if err := os.Remove(source); err != nil {
			return err
		}
		if err := syncObjectDirectory(filepath.Dir(source)); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Verified %d objects; apply=%t. After successful apply, set storage.layout: %s before startup.\n", len(files), apply, storage.ImmutableLayout)
	return nil
}

func syncObjectDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
