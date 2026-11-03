package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"gorm.io/gorm"
)

type ViewEntry struct {
	Path      string
	Key       string
	Checksum  string
	Size      int64
	Directory bool
}
type ViewManifest struct {
	Mode    string
	Root    string
	Entries []ViewEntry
}
type ReadableViews struct {
	DB         *gorm.DB
	ObjectRoot string
	Root       string
	Mode       string
	Clear      bool // explicit removal/rebuild operation, never used by the worker
}

func NewReadableViews(db *gorm.DB, objectRoot, root, mode string) (*ReadableViews, error) {
	if mode == "" {
		mode = "none"
	}
	if mode != "none" && mode != "hardlink" && mode != "symlink" && mode != "export" {
		return nil, fmt.Errorf("unknown readable view mode %q", mode)
	}
	objectRoot, err := filepath.Abs(objectRoot)
	if err != nil {
		return nil, err
	}
	if root == "" {
		root = filepath.Join(objectRoot, "readable")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	for _, reserved := range []string{objectRoot, filepath.Join(objectRoot, "objects"), filepath.Join(objectRoot, "staging"), filepath.Join(objectRoot, "view-state"), filepath.Join(objectRoot, "tenants")} {
		if root == reserved || strings.HasPrefix(reserved, root+string(os.PathSeparator)) || (reserved != objectRoot && strings.HasPrefix(root, reserved+string(os.PathSeparator))) {
			return nil, errors.New("readable root overlaps authoritative, legacy or state storage")
		}
	}
	if _, err := storage.SafePath(root, ""); err != nil {
		return nil, err
	}
	return &ReadableViews{DB: db, ObjectRoot: objectRoot, Root: root, Mode: mode}, nil
}

// Lock is held by exactly one process per root. A second owner fails explicitly.
func (v *ReadableViews) Lock() (*os.File, error) {
	dir, err := storage.SafePath(v.ObjectRoot, "view-state")
	if err != nil {
		return nil, err
	}
	name := "reconciler.lock"
	if v.Mode != "none" {
		dir = filepath.Dir(v.Root)
		name = "." + filepath.Base(v.Root) + ".clientshare.lock"
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	p, err := storage.SafePath(dir, name)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("readable view already has an owner: %w", err)
	}
	return f, nil
}

// Probe never substitutes a different mode. Both publication and view linking
// must be tested on the actual mounted filesystem before production cutover.
func (v *ReadableViews) Probe() error {
	if v.Mode == "none" {
		return nil
	}
	if err := os.MkdirAll(v.Root, 0700); err != nil {
		return err
	}
	dir, err := storage.SafePath(v.ObjectRoot, "staging")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	source, err := os.CreateTemp(dir, "view-probe-*")
	if err != nil {
		return err
	}
	source.Close()
	defer os.Remove(source.Name())
	target, err := os.CreateTemp(v.Root, ".probe-*")
	if err != nil {
		return err
	}
	target.Close()
	os.Remove(target.Name())
	defer os.Remove(target.Name())
	switch v.Mode {
	case "hardlink":
		return os.Link(source.Name(), target.Name())
	case "symlink":
		rel, err := filepath.Rel(v.Root, source.Name())
		if err != nil {
			return err
		}
		if err := os.Symlink(rel, target.Name()); err != nil {
			return err
		}
		_, err = os.Stat(target.Name())
		return err
	case "export":
		// Export publication uses an atomic no-replace hardlink within its own filesystem.
		f, err := os.CreateTemp(v.Root, ".probe-copy-*")
		if err != nil {
			return err
		}
		f.Close()
		defer os.Remove(f.Name())
		return os.Link(f.Name(), target.Name())
	}
	return nil
}

func viewName(name string) error {
	if err := LogicalName(name); err != nil {
		return err
	}
	if strings.TrimRight(name, " .") != name || strings.ContainsAny(name, ":*?\"<>|") {
		return fmt.Errorf("unsupported NAS name %q", name)
	}
	for _, c := range name {
		if c < 32 {
			return fmt.Errorf("unsupported NAS name %q", name)
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return fmt.Errorf("reserved NAS name %q", name)
	}
	return nil
}

func (v *ReadableViews) expected(ctx context.Context, state models.ReadableView) ([]ViewEntry, error) {
	if v.Mode == "none" || v.Clear {
		return nil, nil
	}
	var client models.Client
	err := v.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", state.TenantID, state.ClientID).First(&client).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []models.File
	if err := v.DB.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND deleted_at IS NULL", state.TenantID, state.ClientID).Find(&files).Error; err != nil {
		return nil, err
	}
	// Only export records reachable from the active client root. Legacy data
	// can retain active children beneath an already-deleted parent.
	if client.RootFolderID == nil {
		return nil, errors.New("client has no logical root")
	}
	reachable := map[string]bool{*client.RootFolderID: true}
	for changed := true; changed; {
		changed = false
		for _, f := range files {
			if !reachable[f.ID] && f.FolderID != nil && reachable[*f.FolderID] {
				reachable[f.ID] = true
				changed = true
			}
		}
	}
	rootActive := false
	for _, f := range files {
		if f.ID == *client.RootFolderID {
			rootActive = true
		}
	}
	if !rootActive {
		return nil, nil
	}
	active := make([]models.File, 0, len(files))
	for _, f := range files {
		if reachable[f.ID] {
			active = append(active, f)
		}
	}
	files = active
	paths, err := LogicalPaths(files)
	if err != nil {
		return nil, err
	}
	entries := make([]ViewEntry, 0, len(files))
	seen := map[string]bool{}
	for _, f := range files {
		if f.StorageMissing {
			continue
		}
		p := paths[f.ID]
		for _, part := range strings.Split(p, "/") {
			if err := viewName(part); err != nil {
				return nil, err
			}
		}
		p = state.TenantID + "/" + p
		if seen[strings.ToLower(p)] {
			return nil, fmt.Errorf("case-insensitive view collision: %s", p)
		}
		seen[strings.ToLower(p)] = true
		e := ViewEntry{Path: p, Directory: f.Type == "folder", Checksum: f.Checksum, Size: f.Size}
		if !e.Directory {
			if f.StorageKey == nil {
				return nil, errors.New("unmigrated file")
			}
			e.Key = *f.StorageKey
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (v *ReadableViews) save(state models.ReadableView, manifest ViewManifest) error {
	b, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return v.DB.Model(&models.ReadableView{}).Where("tenant_id = ? AND client_id = ?", state.TenantID, state.ClientID).Update("manifest", string(b)).Error
}

// entryPath permits a managed leaf symlink, but never directory symlinks.
func (v *ReadableViews) entryPath(e ViewEntry) (string, error) {
	dir, err := storage.SafePath(v.Root, filepath.ToSlash(filepath.Dir(e.Path)))
	if err != nil {
		return "", err
	}
	if err := LogicalName(filepath.Base(e.Path)); err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(e.Path)), nil
}

func (v *ReadableViews) matches(ctx context.Context, tenant string, e ViewEntry) (bool, error) {
	p, err := v.entryPath(e)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if e.Directory {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("changed managed directory: %s", p)
		}
		return true, nil
	}
	source, err := storage.NewLocalStorage(v.ObjectRoot).Tenant(tenant).(*storage.LocalStorage).ObjectPath(e.Key)
	if err != nil {
		return false, err
	}
	switch v.Mode {
	case "symlink":
		target, err := os.Readlink(p)
		if err != nil {
			return false, err
		}
		expected, err := filepath.Rel(filepath.Dir(p), source)
		if err != nil {
			return false, err
		}
		if target != expected {
			return false, fmt.Errorf("modified symlink: %s", p)
		}
	case "hardlink":
		sourceInfo, err := os.Stat(source)
		if err != nil {
			return false, err
		}
		if !info.Mode().IsRegular() || !os.SameFile(info, sourceInfo) {
			return false, fmt.Errorf("modified hardlink: %s", p)
		}
	case "export":
		actual, err := hashRegularFile(ctx, p)
		if err != nil {
			return false, err
		}
		if actual.Size != e.Size || actual.Checksum != e.Checksum {
			return false, fmt.Errorf("modified export: %s", p)
		}
	}
	return true, nil
}

func (v *ReadableViews) create(ctx context.Context, tenant string, e ViewEntry) error {
	p, err := v.entryPath(e)
	if err != nil {
		return err
	}
	if e.Directory {
		return os.Mkdir(p, 0700)
	}
	source, err := storage.NewLocalStorage(v.ObjectRoot).Tenant(tenant).(*storage.LocalStorage).ObjectPath(e.Key)
	if err != nil {
		return err
	}
	switch v.Mode {
	case "hardlink":
		return os.Link(source, p)
	case "symlink":
		rel, err := filepath.Rel(filepath.Dir(p), source)
		if err != nil {
			return err
		}
		return os.Symlink(rel, p)
	case "export":
		in, err := os.Open(source)
		if err != nil {
			return err
		}
		defer in.Close()
		// Keep interrupted copies outside the readable namespace, on the same
		// filesystem as the destination. They cannot block directory cleanup.
		staging, err := storage.SafePath(v.Root, ".clientshare-staging")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(staging, 0700); err != nil {
			return err
		}
		out, err := os.CreateTemp(staging, "export-*")
		if err != nil {
			return err
		}
		defer os.Remove(out.Name())
		info, copyErr := storage.Hash(ctx, io.TeeReader(in, out))
		err = errors.Join(copyErr, out.Sync(), out.Close())
		if err != nil {
			return err
		}
		if info.Size != e.Size || info.Checksum != e.Checksum {
			return errors.New("source checksum mismatch")
		}
		return os.Link(out.Name(), p)
	}
	return errors.New("view mode does not create files")
}

// Reconcile persists creation intent before publishing entries and removes only
// verified owned entries. Interrupted operations are therefore repeatable.
// Caller must hold Lock, including for explicit full verification/rebuilds.
func (v *ReadableViews) Reconcile(ctx context.Context, state models.ReadableView, dryRun bool, output io.Writer) (result error) {
	defer func() {
		if result != nil && !dryRun {
			v.DB.Model(&models.ReadableView{}).Where("tenant_id = ? AND client_id = ?", state.TenantID, state.ClientID).Update("last_error", result.Error())
		}
	}()
	manifest := ViewManifest{Mode: v.Mode, Root: v.Root}
	if state.Manifest != "" {
		if err := json.Unmarshal([]byte(state.Manifest), &manifest); err != nil {
			return err
		}
	}
	if manifest.Mode != v.Mode || manifest.Root != v.Root {
		return errors.New("view mode/root changed: reconcile old mode with deleted desired entries before explicit rebuild")
	}
	expected, err := v.expected(ctx, state)
	if err != nil {
		return err
	}
	owned := map[string]ViewEntry{}
	wanted := map[string]bool{}
	for _, e := range manifest.Entries {
		if !strings.HasPrefix(e.Path, state.TenantID+"/") {
			return errors.New("manifest entry outside tenant scope")
		}
		owned[e.Path] = e
	}
	// Case-only renames and replacing the object at the same logical name
	// cannot coexist on a case-insensitive filesystem. Verify and remove those
	// owned entries first; ordinary renames still create before removing.
	replace := map[string]bool{}
	for _, old := range manifest.Entries {
		for _, next := range expected {
			if strings.EqualFold(old.Path, next.Path) && old != next {
				replace[old.Path] = true
			}
		}
	}
	sort.Slice(manifest.Entries, func(i, j int) bool { return manifest.Entries[i].Path > manifest.Entries[j].Path })
	kept := make([]ViewEntry, 0, len(manifest.Entries))
	for _, old := range manifest.Entries {
		if !replace[old.Path] {
			kept = append(kept, old)
			continue
		}
		exists, err := v.matches(ctx, state.TenantID, old)
		if err != nil {
			return err
		}
		if exists {
			fmt.Fprintf(output, "remove %s (replacement)\n", old.Path)
			if !dryRun {
				p, err := v.entryPath(old)
				if err != nil {
					return err
				}
				if err := os.Remove(p); err != nil {
					return err
				}
			}
		}
		delete(owned, old.Path)
	}
	if len(replace) > 0 {
		manifest.Entries = kept
		if !dryRun {
			if err := v.save(state, manifest); err != nil {
				return err
			}
		}
	}
	if v.Mode != "none" && !dryRun {
		tenantDir, err := storage.SafePath(v.Root, state.TenantID)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(tenantDir, 0700); err != nil {
			return err
		}
	}
	for _, e := range expected {
		wanted[e.Path] = true
		old, known := owned[e.Path]
		if known && old != e {
			return fmt.Errorf("managed path identity changed: %s; remove old view first", e.Path)
		}
		p, err := v.entryPath(e)
		if err != nil {
			return err
		}
		if !known {
			plannedReplacement := false
			if dryRun {
				for old := range replace {
					if strings.EqualFold(old, e.Path) {
						plannedReplacement = true
					}
				}
			}
			// Ask the filesystem too: Unicode-normalized aliases need not compare
			// equal as Go strings, but must never become newly claimed ownership.
			if _, statErr := os.Lstat(p); statErr == nil && !plannedReplacement {
				return fmt.Errorf("unmanaged view collision: %s", e.Path)
			} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			// Refuse even a matching unmanaged entry, and case aliases on Unix.
			if entries, err := os.ReadDir(filepath.Dir(p)); err == nil {
				for _, entry := range entries {
					if strings.EqualFold(entry.Name(), filepath.Base(p)) {
						if dryRun && replace[filepath.ToSlash(filepath.Join(filepath.Dir(e.Path), entry.Name()))] {
							continue
						}
						return fmt.Errorf("unmanaged view collision: %s", e.Path)
					}
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if !dryRun {
				manifest.Entries = append(manifest.Entries, e)
				if err := v.save(state, manifest); err != nil {
					return err
				}
			}
		}
		exists, err := v.matches(ctx, state.TenantID, e)
		if dryRun {
			for old := range replace {
				if strings.EqualFold(old, e.Path) {
					exists, err = false, nil
				}
			}
		}
		if err != nil {
			return err
		}
		if !exists {
			fmt.Fprintf(output, "create %s\n", e.Path)
			if !dryRun {
				if err := v.create(ctx, state.TenantID, e); err != nil {
					return err
				}
			}
		}
	}
	// Reverse depth order preserves directories until all managed children go.
	sort.Slice(manifest.Entries, func(i, j int) bool { return manifest.Entries[i].Path > manifest.Entries[j].Path })
	for _, e := range manifest.Entries {
		if wanted[e.Path] {
			continue
		}
		exists, err := v.matches(ctx, state.TenantID, e)
		if err != nil {
			return err
		}
		if exists {
			fmt.Fprintf(output, "remove %s\n", e.Path)
			if !dryRun {
				p, err := v.entryPath(e)
				if err != nil {
					return err
				}
				if err := os.Remove(p); err != nil {
					return err
				}
			}
		}
	}
	if dryRun {
		return nil
	}
	manifest.Entries = expected
	if err := v.save(state, manifest); err != nil {
		return err
	}
	return v.DB.Model(&models.ReadableView{}).Where("tenant_id = ? AND client_id = ? AND desired_revision = ?", state.TenantID, state.ClientID, state.DesiredRevision).Updates(map[string]interface{}{"applied_revision": state.DesiredRevision, "last_error": ""}).Error
}

func (v *ReadableViews) Run(ctx context.Context, output io.Writer) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	type retryState struct {
		delay time.Duration
		next  time.Time
	}
	retries := map[string]retryState{}
	for {
		var states []models.ReadableView
		if err := v.DB.WithContext(ctx).Where("desired_revision <> applied_revision").Order("tenant_id, client_id").Find(&states).Error; err != nil {
			fmt.Fprintln(output, err)
		} else {
			for _, state := range states {
				key := state.TenantID + "/" + state.ClientID
				retry := retries[key]
				if time.Now().Before(retry.next) {
					continue
				}
				if err := v.Reconcile(ctx, state, false, output); err != nil {
					fmt.Fprintf(output, "view %s/%s: %v\n", state.TenantID, state.ClientID, err)
					if retry.delay == 0 {
						retry.delay = 30 * time.Second
					} else {
						retry.delay *= 2
					}
					if retry.delay > 5*time.Minute {
						retry.delay = 5 * time.Minute
					}
					retry.next = time.Now().Add(retry.delay)
					retries[key] = retry
				} else {
					delete(retries, key)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
