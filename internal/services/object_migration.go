package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"gorm.io/gorm"
)

type ObjectMigrationEntry struct {
	TenantID     string `json:"tenant_id"`
	FileID       string `json:"file_id"`
	RecordedPath string `json:"recorded_path"`
	Source       string `json:"source"` // relative to storage root, tenant-scoped
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	Checksum     string `json:"sha256"`
	Missing      bool   `json:"acknowledged_missing,omitempty"`
}

type ObjectMigrationPlan struct {
	Version           int                    `json:"version"`
	CreatedAt         time.Time              `json:"created_at"`
	Entries           []ObjectMigrationEntry `json:"entries"`
	Problems          []string               `json:"problems"`
	Orphans           []string               `json:"unreferenced_legacy_entries"`
	Folders           int                    `json:"folders"`
	PhysicallyDeleted int                    `json:"physically_deleted"`
	RequiredBytes     int64                  `json:"required_copy_bytes"`
	AvailableBytes    uint64                 `json:"available_bytes"`
}

// Inventory is read-only. Overrides are explicit tenant/file -> relative source
// mappings for operator-reviewed stale paths. No basename search is performed.
func InventoryObjects(ctx context.Context, db *gorm.DB, root string, overrides map[string]string, acknowledgedMissing ...string) (ObjectMigrationPlan, error) {
	p := ObjectMigrationPlan{Version: 1, CreatedAt: time.Now().UTC()}
	var files []models.File
	if err := db.WithContext(ctx).Order("tenant_id, id").Find(&files).Error; err != nil {
		return p, err
	}
	used := map[string]string{}
	missing := map[string]bool{}
	for _, id := range acknowledgedMissing {
		missing[id] = true
	}
	for _, f := range files {
		if f.Type == "folder" {
			p.Folders++
			continue
		}
		if f.DiskDeletedAt != nil {
			p.PhysicallyDeleted++
			continue
		}
		if f.StorageKey != nil && *f.StorageKey != "" {
			continue
		}
		identity := f.TenantID + "/" + f.ID
		if missing[identity] || f.StorageMissing {
			p.Entries = append(p.Entries, ObjectMigrationEntry{TenantID: f.TenantID, FileID: f.ID, RecordedPath: f.Path, Size: f.Size, Missing: true})
			delete(missing, identity)
			continue
		}
		rel := filepath.ToSlash(filepath.Join("tenants", f.TenantID, f.Path))
		if source, ok := overrides[identity]; ok {
			rel = source
		}
		tenantPrefix := "tenants/" + f.TenantID + "/"
		if !strings.HasPrefix(rel, tenantPrefix) {
			p.Problems = append(p.Problems, identity+": source is outside tenant")
			continue
		}
		source, err := storage.SafePath(root, rel)
		if err != nil {
			p.Problems = append(p.Problems, identity+": "+err.Error())
			continue
		}
		if prior, ok := used[source]; ok {
			p.Problems = append(p.Problems, identity+": duplicate source with "+prior)
			continue
		}
		used[source] = identity
		info, err := hashRegularFile(ctx, source)
		if err != nil {
			p.Problems = append(p.Problems, identity+": "+err.Error()+"; supply reviewed source override for stale paths")
			continue
		}
		if info.Size != f.Size || (f.Checksum != "" && f.Checksum != info.Checksum) {
			p.Problems = append(p.Problems, identity+": source size/checksum mismatch")
			continue
		}
		p.Entries = append(p.Entries, ObjectMigrationEntry{TenantID: f.TenantID, FileID: f.ID, RecordedPath: f.Path, Source: rel, Key: f.ID, Size: info.Size, Checksum: info.Checksum})
	}
	for id := range missing {
		p.Problems = append(p.Problems, "unknown/non-retained missing-file acknowledgment: "+id)
	}
	for _, e := range p.Entries {
		if e.Missing {
			continue
		}
		actual, err := storage.NewLocalStorage(root).Tenant(e.TenantID).Stat(ctx, e.Key)
		if errors.Is(err, os.ErrNotExist) {
			p.RequiredBytes += e.Size
			continue
		}
		if err != nil || actual.Size != e.Size || actual.Checksum != e.Checksum {
			p.Problems = append(p.Problems, e.TenantID+"/"+e.FileID+": existing destination does not match inventory")
		}
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(root, &fs); err != nil {
		return p, err
	}
	p.AvailableBytes = uint64(fs.Bavail) * uint64(fs.Bsize)
	if uint64(p.RequiredBytes) > p.AvailableBytes {
		p.Problems = append(p.Problems, "insufficient free space for immutable copies")
	}
	legacy, err := storage.SafePath(root, "tenants")
	if err != nil {
		return p, err
	}
	err = filepath.WalkDir(legacy, func(path string, entry os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.IsDir() && used[path] == "" {
			rel, _ := filepath.Rel(root, path)
			p.Orphans = append(p.Orphans, filepath.ToSlash(rel))
		}
		return nil
	})
	return p, err
}

func hashRegularFile(ctx context.Context, path string) (storage.ObjectInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	if !info.Mode().IsRegular() {
		return storage.ObjectInfo{}, fmt.Errorf("not a regular file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	defer f.Close()
	return storage.Hash(ctx, f)
}

// verifyMigrationCopies checks all sources/destinations and optionally publishes
// missing destinations without replacing existing bytes.
func verifyMigrationCopies(ctx context.Context, root string, plan ObjectMigrationPlan, copyMissing bool) error {
	if plan.Version != 1 || len(plan.Problems) > 0 {
		return errors.New("migration inventory has unresolved problems or unsupported version")
	}
	store := storage.NewLocalStorage(root)
	seen := map[string]bool{}
	for _, e := range plan.Entries {
		identity := e.TenantID + "/" + e.FileID
		if seen[identity] || (!e.Missing && e.Key != e.FileID) {
			return fmt.Errorf("duplicate or invalid manifest identity %s", identity)
		}
		seen[identity] = true
		if e.Missing {
			continue
		}
		if !strings.HasPrefix(e.Source, "tenants/"+e.TenantID+"/") {
			return errors.New("source outside tenant")
		}
		source, err := storage.SafePath(root, e.Source)
		if err != nil {
			return err
		}
		info, err := hashRegularFile(ctx, source)
		if err != nil {
			return err
		}
		if info.Size != e.Size || info.Checksum != e.Checksum {
			return fmt.Errorf("source changed: %s", identity)
		}
		tenant := store.Tenant(e.TenantID)
		dest, err := tenant.Stat(ctx, e.Key)
		if errors.Is(err, os.ErrNotExist) {
			if !copyMissing {
				continue
			}
			var f *os.File
			f, err = os.Open(source)
			if err != nil {
				return err
			}
			_, err = tenant.PutNew(ctx, e.Key, f)
			err = errors.Join(err, f.Close())
			if err != nil {
				return err
			}
			dest, err = tenant.Stat(ctx, e.Key)
		}
		if err != nil {
			return err
		}
		if dest != info {
			return fmt.Errorf("destination verification failed: %s", identity)
		}
	}
	return nil
}

func VerifyMigrationPlan(ctx context.Context, root string, plan ObjectMigrationPlan) error {
	return verifyMigrationCopies(ctx, root, plan, false)
}

// ApplyObjects uses the manifest and verified objects as its checkpoint. Only
// after all copies verify does one transaction publish storage identities.
func ApplyObjects(ctx context.Context, db *gorm.DB, root string, plan ObjectMigrationPlan) error {
	if err := VerifyMigrationPlan(ctx, root, plan); err != nil {
		return err
	}
	if err := verifyMigrationCopies(ctx, root, plan, true); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, e := range plan.Entries {
			var f models.File
			if err := tx.Where("tenant_id = ? AND id = ?", e.TenantID, e.FileID).First(&f).Error; err != nil {
				return err
			}
			if e.Missing {
				if f.StorageKey != nil || f.Type == "folder" || f.DiskDeletedAt != nil || (!f.StorageMissing && f.Path != e.RecordedPath) {
					return fmt.Errorf("cannot acknowledge changed/object-backed file %s", f.ID)
				}
				if err := tx.Model(&f).Update("storage_missing", true).Error; err != nil {
					return err
				}
				if err := models.DirtyView(tx, f.TenantID, f.ClientID); err != nil {
					return err
				}
				continue
			}
			if f.Type == "folder" || f.DiskDeletedAt != nil || f.Size != e.Size || (f.StorageKey == nil && f.Path != e.RecordedPath) || (f.StorageKey != nil && *f.StorageKey != e.Key) {
				return fmt.Errorf("record changed since inventory: %s", e.FileID)
			}
			if err := tx.Model(&f).Updates(map[string]interface{}{"storage_key": e.Key, "checksum": e.Checksum, "size": e.Size}).Error; err != nil {
				return err
			}
			if err := models.DirtyView(tx, e.TenantID, f.ClientID); err != nil {
				return err
			}
		}
		var missing int64
		if err := tx.Model(&models.File{}).Where("type <> 'folder' AND disk_deleted_at IS NULL AND storage_missing = ? AND (storage_key IS NULL OR storage_key = '')", false).Count(&missing).Error; err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("cutover blocked: %d retained files are not migrated", missing)
		}
		var retained []models.File
		if err := tx.Where("type <> 'folder' AND disk_deleted_at IS NULL AND storage_missing = ?", false).Find(&retained).Error; err != nil {
			return err
		}
		for _, f := range retained {
			if f.StorageKey == nil {
				return fmt.Errorf("missing object key: %s", f.ID)
			}
			info, err := storage.NewLocalStorage(root).Tenant(f.TenantID).Stat(ctx, *f.StorageKey)
			if err != nil {
				return err
			}
			if info.Size != f.Size || info.Checksum != f.Checksum {
				return fmt.Errorf("retained object verification failed: %s/%s", f.TenantID, f.ID)
			}
		}
		var clients []models.Client
		if err := tx.Find(&clients).Error; err != nil {
			return err
		}
		for _, c := range clients {
			if c.RootFolderID != nil {
				if err := tx.Model(&models.File{}).Where("tenant_id = ? AND id = ?", c.TenantID, *c.RootFolderID).Update("filename", c.FolderPath).Error; err != nil {
					return err
				}
			}
			if err := RefreshLogicalPaths(tx, c.TenantID, c.ID); err != nil {
				return err
			}
			if err := models.DirtyView(tx, c.TenantID, c.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
