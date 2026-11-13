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
	"unicode"
	"unicode/utf8"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
)

const (
	ViewModeNone     = "none"
	ViewModeHardlink = "hardlink"
	ViewModeSymlink  = "symlink"
	ViewModeExport   = "export"
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
		mode = ViewModeNone
	}
	if mode != ViewModeNone && mode != ViewModeHardlink && mode != ViewModeSymlink && mode != ViewModeExport {
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
	if v.Mode != ViewModeNone {
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
	if v.Mode == ViewModeNone {
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
	case ViewModeHardlink:
		return os.Link(source.Name(), target.Name())
	case ViewModeSymlink:
		rel, err := filepath.Rel(v.Root, source.Name())
		if err != nil {
			return err
		}
		if err := os.Symlink(rel, target.Name()); err != nil {
			return err
		}
		_, err = os.Stat(target.Name())
		return err
	case ViewModeExport:
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

// viewName is a one-way, filesystem-independent display mapping. Never use it
// for portal names or object keys. Reject structural corruption before mapping.
func viewName(name string) (string, error) {
	if err := LogicalName(name); err != nil {
		return "", fmt.Errorf("unsafe projection name %q: %w", name, err)
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("invalid UTF-8 projection name %q", name)
	}
	trailing := len(strings.TrimRight(name, " ."))
	var mapped strings.Builder
	for i, c := range name {
		if !unicode.IsPrint(c) || strings.ContainsRune(":*?\"<>|", c) || i >= trailing {
			mapped.WriteByte('_')
		} else {
			mapped.WriteRune(c)
		}
	}
	result := mapped.String()
	base := strings.ToUpper(strings.TrimRight(strings.SplitN(result, ".", 2)[0], " "))
	deviceNumber := strings.TrimPrefix(base, "COM")
	if strings.HasPrefix(base, "LPT") {
		deviceNumber = strings.TrimPrefix(base, "LPT")
	}
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" ||
		((strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && strings.Contains("123456789¹²³", deviceNumber) && utf8.RuneCountInString(deviceNumber) == 1) {
		result = "_" + result
	}
	if len(result) > 255 {
		return "", fmt.Errorf("projection name %q exceeds 255 bytes after mapping; no truncation is performed", name)
	}
	return result, nil
}

// Compare conservatively even on case-sensitive POSIX filesystems. SMB clients
// and normalization-insensitive filesystems must not expose ambiguous aliases.
func viewCanonical(name string) string {
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(name)))
}

func (v *ReadableViews) expected(ctx context.Context, state models.ReadableView, output io.Writer) ([]ViewEntry, error) {
	if v.Mode == ViewModeNone || v.Clear {
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
	if err := v.DB.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND deleted_at IS NULL", state.TenantID, state.ClientID).Order("id").Find(&files).Error; err != nil {
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
	for _, f := range files {
		if _, err := viewName(f.Filename); err != nil {
			return nil, fmt.Errorf("client %s entry %s: %w", state.ClientID, f.ID, err)
		}
	}
	// Client roots share a tenant namespace, including clients not yet applied.
	var roots []models.File
	if err := v.DB.WithContext(ctx).Table("files").Select("files.*").Joins("JOIN clients ON clients.root_folder_id = files.id AND clients.tenant_id = files.tenant_id").Where("files.tenant_id = ? AND files.deleted_at IS NULL", state.TenantID).Order("files.id").Find(&roots).Error; err != nil {
		return nil, err
	}
	rootName := ""
	for _, f := range files {
		if f.ID == *client.RootFolderID {
			rootName = f.Filename
		}
	}
	projectedRoot, err := viewName(rootName)
	if err != nil {
		return nil, err
	}
	for _, other := range roots {
		if other.ID == *client.RootFolderID {
			continue
		}
		name, err := viewName(other.Filename)
		if err == nil && viewCanonical(name) == viewCanonical(projectedRoot) {
			return nil, fmt.Errorf("projection root collision: client %s name %q and client %s name %q map to %q / %q; no names were changed", client.ID, rootName, other.ClientID, other.Filename, projectedRoot, name)
		}
	}
	paths, err := LogicalPaths(files)
	if err != nil {
		return nil, err
	}
	entries := make([]ViewEntry, 0, len(files))
	seen := map[string]string{}
	var problems []error
	for _, f := range files {
		if f.StorageMissing {
			continue
		}
		logical := paths[f.ID]
		parts := strings.Split(logical, "/")
		for i, part := range parts {
			mapped, err := viewName(part)
			if err != nil {
				return nil, err
			}
			parts[i] = mapped
		}
		p := state.TenantID + "/" + strings.Join(parts, "/")
		if logical != strings.Join(parts, "/") {
			fmt.Fprintf(output, "map client %s entry %s: %q -> %q\n", state.ClientID, f.ID, logical, p)
		}
		canonical := viewCanonical(p)
		if previous, ok := seen[canonical]; ok {
			problems = append(problems, fmt.Errorf("projection collision: %s and entry %s name %q map to %q (case/Unicode-insensitive comparison); no names were changed", previous, f.ID, logical, p))
		}
		seen[canonical] = fmt.Sprintf("entry %s name %q", f.ID, logical)
		e := ViewEntry{Path: p, Directory: f.Type == "folder", Checksum: f.Checksum, Size: f.Size}
		if !e.Directory {
			if f.StorageKey == nil {
				return nil, errors.New("unmigrated file")
			}
			e.Key = *f.StorageKey
		}
		entries = append(entries, e)
	}
	if err := errors.Join(problems...); err != nil {
		return nil, err
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
	case ViewModeSymlink:
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
	case ViewModeHardlink:
		sourceInfo, err := os.Stat(source)
		if err != nil {
			return false, err
		}
		if !info.Mode().IsRegular() || !os.SameFile(info, sourceInfo) {
			return false, fmt.Errorf("modified hardlink: %s", p)
		}
	case ViewModeExport:
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
	case ViewModeHardlink:
		return os.Link(source, p)
	case ViewModeSymlink:
		rel, err := filepath.Rel(filepath.Dir(p), source)
		if err != nil {
			return err
		}
		return os.Symlink(rel, p)
	case ViewModeExport:
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
	// Find predictable conflicts before changing owned entries or the manifest.
	if !dryRun {
		if err := v.Reconcile(ctx, state, true, io.Discard); err != nil {
			return err
		}
	}
	manifestRoot := v.Root
	if v.Mode == ViewModeNone {
		manifestRoot = ""
	}
	manifest := ViewManifest{Mode: v.Mode, Root: manifestRoot}
	if state.Manifest != "" {
		if err := json.Unmarshal([]byte(state.Manifest), &manifest); err != nil {
			return err
		}
	}
	if v.Mode == ViewModeNone && manifest.Mode == ViewModeNone && len(manifest.Entries) == 0 {
		manifest.Root = ""
	}
	if manifest.Mode != v.Mode || manifest.Root != manifestRoot {
		return errors.New("view mode/root changed: reconcile old mode with deleted desired entries before explicit rebuild")
	}
	expected, err := v.expected(ctx, state, output)
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
	// Case/normalization-only renames and replacing the object at the same name
	// cannot coexist on a case-insensitive filesystem. Verify and remove those
	// owned entries first; ordinary renames still create before removing.
	replace := map[string]bool{}
	replacementPaths := map[string]bool{}
	replacementLeaves := map[string]bool{}
	nextByName := map[string]ViewEntry{}
	for _, next := range expected {
		nextByName[viewCanonical(next.Path)] = next
	}
	for _, old := range manifest.Entries {
		canonical := viewCanonical(old.Path)
		if next, ok := nextByName[canonical]; ok && old != next {
			replace[old.Path] = true
			replacementPaths[canonical] = true
			replacementLeaves[viewCanonical(filepath.Dir(old.Path))+"/"+filepath.Base(old.Path)] = true
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
	if v.Mode != ViewModeNone && !dryRun {
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
			plannedReplacement := dryRun && replacementPaths[viewCanonical(e.Path)]
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
					if viewCanonical(entry.Name()) == viewCanonical(filepath.Base(p)) {
						// The parent may itself be renamed, but only the
						// exact owned leaf is excused, never a second alias.
						if dryRun && replacementLeaves[viewCanonical(filepath.Dir(e.Path))+"/"+entry.Name()] {
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
		if dryRun && replacementPaths[viewCanonical(e.Path)] {
			exists, err = false, nil
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
	logger := zap.L().With(zap.String("job", "readable_views"))
	for {
		started := time.Now()
		logger.Debug("Background job run started")
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
		logger.Debug("Background job run finished", zap.Duration("elapsed", time.Since(started)))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
