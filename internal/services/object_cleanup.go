package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"gorm.io/gorm"
)

// CleanupOrphanObjects is explicit and local-only. Upload staging is excluded;
// the grace period exceeds quota reservation lifetimes and request deadlines.
func CleanupOrphanObjects(ctx context.Context, db *gorm.DB, root string, grace time.Duration, apply bool, out io.Writer) error {
	if grace < 24*time.Hour {
		return fmt.Errorf("orphan grace must be at least 24h")
	}
	dir, err := storage.SafePath(root, "objects")
	if err != nil {
		return err
	}
	return filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 || strings.HasPrefix(parts[2], ".") {
			return nil
		}
		store := storage.NewLocalStorage(root).Tenant(parts[0])
		local := store.(*storage.LocalStorage)
		expected, err := local.ObjectPath(parts[2])
		if err != nil {
			return err
		}
		if filepath.Clean(expected) != filepath.Clean(p) {
			return fmt.Errorf("unexpected object bucket: %s", p)
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected object entry: %s", p)
		}
		if time.Since(info.ModTime()) < grace {
			return nil
		}
		var count int64
		if err := db.WithContext(ctx).Model(&models.File{}).Where("tenant_id = ? AND storage_key = ?", parts[0], parts[2]).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return nil
		}
		fmt.Fprintf(out, "orphan %s\n", rel)
		if apply {
			return store.DeleteObject(ctx, parts[2])
		}
		return nil
	})
}

func CleanupLegacyObjects(ctx context.Context, db *gorm.DB, root string, plan ObjectMigrationPlan, apply bool, out io.Writer) error {
	if plan.Version != 1 || len(plan.Problems) > 0 {
		return fmt.Errorf("invalid migration manifest")
	}
	for _, e := range plan.Entries {
		if e.Missing {
			continue
		}
		var file models.File
		if err := db.WithContext(ctx).Where("tenant_id = ? AND id = ?", e.TenantID, e.FileID).First(&file).Error; err != nil {
			return err
		}
		if file.StorageKey == nil || *file.StorageKey != e.Key || file.Checksum != e.Checksum {
			return fmt.Errorf("record no longer matches manifest: %s", e.FileID)
		}
		if !strings.HasPrefix(e.Source, "tenants/"+e.TenantID+"/") {
			return fmt.Errorf("source outside tenant")
		}
		p, err := storage.SafePath(root, e.Source)
		if err != nil {
			return err
		}
		source, err := hashRegularFile(ctx, p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if source.Size != e.Size || source.Checksum != e.Checksum {
			return fmt.Errorf("legacy source changed: %s", e.Source)
		}
		// Even after retention, cleanup requires an independently verified backup
		// or object; this command never assumes a deleted object can be recovered.
		dest, err := storage.NewLocalStorage(root).Tenant(e.TenantID).Stat(ctx, e.Key)
		if err != nil {
			return err
		}
		if dest != source {
			return fmt.Errorf("object mismatch: %s", e.FileID)
		}
		fmt.Fprintf(out, "legacy %s\n", e.Source)
		if apply {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
	}
	return nil
}
