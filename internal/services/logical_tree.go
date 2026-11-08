package services

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func LogicalName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return errors.New("invalid logical name")
	}
	return nil
}

func lockLogicalParent(tx *gorm.DB, file *models.File) error {
	if file.FolderID == nil {
		return nil
	}
	var parent models.File
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND client_id = ? AND id = ? AND type = 'folder' AND deleted_at IS NULL", file.TenantID, file.ClientID, *file.FolderID).First(&parent).Error
}

// LogicalPaths derives paths from relationships, never from legacy Path values.
func LogicalPaths(files []models.File) (map[string]string, error) {
	byID := map[string]models.File{}
	for _, f := range files {
		byID[f.ID] = f
	}
	paths, visiting := map[string]string{}, map[string]bool{}
	var resolve func(string) (string, error)
	resolve = func(id string) (string, error) {
		if p, ok := paths[id]; ok {
			return p, nil
		}
		f, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("missing parent %s", id)
		}
		if visiting[id] {
			return "", errors.New("folder cycle")
		}
		visiting[id] = true
		if err := LogicalName(f.Filename); err != nil {
			return "", err
		}
		p := f.Filename
		if f.FolderID != nil && *f.FolderID != "" {
			parent, ok := byID[*f.FolderID]
			if !ok || parent.Type != "folder" {
				return "", fmt.Errorf("invalid parent for %s", id)
			}
			base, err := resolve(*f.FolderID)
			if err != nil {
				return "", err
			}
			p = path.Join(base, p)
		}
		visiting[id] = false
		paths[id] = p
		return p, nil
	}
	for id := range byID {
		if _, err := resolve(id); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func RefreshLogicalPaths(tx *gorm.DB, tenantID, clientID string) error {
	var files []models.File
	if err := tx.Where("tenant_id = ? AND client_id = ?", tenantID, clientID).Find(&files).Error; err != nil {
		return err
	}
	paths, err := LogicalPaths(files)
	if err != nil {
		return err
	}
	for id, p := range paths {
		if err := tx.Model(&models.File{}).Where("tenant_id = ? AND id = ?", tenantID, id).Update("path", p).Error; err != nil {
			return err
		}
	}
	return nil
}

func OpenFile(ctx context.Context, store storage.Storage, file *models.File) (io.ReadCloser, error) {
	if file.StorageMissing {
		return nil, fmt.Errorf("%w: acknowledged pre-migration missing file", os.ErrNotExist)
	}
	if file.StorageKey == nil || *file.StorageKey == "" {
		return nil, errors.New("file requires immutable storage migration")
	}
	return store.Open(ctx, *file.StorageKey)
}

// RenameMove changes the logical tree only, including descendant display paths.
// Authorization belongs to the caller; a move cannot cross tenant/client scope.
func (s *FileService) RenameMove(ctx context.Context, tenantID, fileID, name, parentID string) error {
	if err := LogicalName(name); err != nil {
		return err
	}
	var clientID string
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var f models.File
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantID, fileID).First(&f).Error; err != nil {
			return err
		}
		clientID = f.ClientID
		if f.FolderID == nil {
			return errors.New("rename client roots through ClientService")
		}
		var parent models.File
		if err := tx.Where("tenant_id = ? AND client_id = ? AND id = ? AND type = 'folder' AND deleted_at IS NULL", tenantID, clientID, parentID).First(&parent).Error; err != nil {
			return err
		}
		var duplicate int64
		if err := tx.Model(&models.File{}).Where("tenant_id = ? AND client_id = ? AND folder_id = ? AND filename = ? AND id <> ? AND deleted_at IS NULL", tenantID, clientID, parentID, name, fileID).Count(&duplicate).Error; err != nil {
			return err
		}
		if duplicate > 0 {
			return os.ErrExist
		}
		if err := tx.Model(&f).Updates(map[string]interface{}{"filename": name, "folder_id": parentID}).Error; err != nil {
			return err
		}
		// Resolving the entire parent graph rejects moving a folder into itself.
		if err := RefreshLogicalPaths(tx, tenantID, clientID); err != nil {
			return err
		}
		return models.DirtyView(tx, tenantID, clientID)
	})
	if err == nil {
		s.invalidateFolderHierarchyCacheByClient(clientID)
	}
	return err
}

// WriteFolderZip is the internal trusted-storage helper. HTTP delivery must use
// WriteAuthorizedFolderZip, which checks each archive entry with the resolver.
func (s *FileService) WriteFolderZip(ctx context.Context, store storage.Storage, folder *models.File, out io.Writer) error {
	return s.writeFolderZip(ctx, store, folder, out, nil)
}

func (s *FileService) WriteAuthorizedFolderZip(ctx context.Context, store storage.Storage, folder *models.File, out io.Writer, req AccessRequest) error {
	req.TenantID, req.ClientID, req.FileID, req.Operation = folder.TenantID, folder.ClientID, folder.ID, AccessZIP
	decision, err := (&AccessService{DB: s.DB}).Resolve(ctx, req)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return gorm.ErrRecordNotFound
	}
	return s.writeFolderZip(ctx, store, folder, out, &req)
}

func (s *FileService) writeFolderZip(ctx context.Context, store storage.Storage, folder *models.File, out io.Writer, req *AccessRequest) error {
	var files []models.File
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND deleted_at IS NULL AND disk_deleted_at IS NULL", folder.TenantID, folder.ClientID).Find(&files).Error; err != nil {
		return err
	}
	if req != nil {
		allowed := make(map[string]bool)
		request := *req
		request.Operation = AccessRead
		for start := 0; start < len(files); start += maxAccessItems {
			ids := make([]string, 0, maxAccessItems)
			for _, f := range files[start:min(start+maxAccessItems, len(files))] {
				ids = append(ids, f.ID)
			}
			decisions, err := (&AccessService{DB: s.DB}).ResolveItems(ctx, request, ids)
			if err != nil {
				return err
			}
			for id, decision := range decisions {
				allowed[id] = decision.Allowed
			}
		}
		filtered := make([]models.File, 0, len(files))
		for _, f := range files {
			if allowed[f.ID] {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}
	children := map[string][]models.File{}
	for _, f := range files {
		if f.FolderID != nil {
			children[*f.FolderID] = append(children[*f.FolderID], f)
		}
	}
	z := zip.NewWriter(out)
	seen, names := map[string]bool{}, map[string]bool{}
	var walk func(string, string) error
	walk = func(id, base string) error {
		if seen[id] {
			return errors.New("folder cycle")
		}
		seen[id] = true
		for _, f := range children[id] {
			if f.Type != "file" && f.Type != "folder" {
				return errors.New("invalid archive item type")
			}
			if err := LogicalName(f.Filename); err != nil {
				return err
			}
			name := path.Join(base, f.Filename)
			if names[strings.ToLower(name)] {
				return fmt.Errorf("archive name collision: %s", name)
			}
			names[strings.ToLower(name)] = true
			if f.Type == "folder" {
				if _, err := z.Create(name + "/"); err != nil {
					return err
				}
				if err := walk(f.ID, name); err != nil {
					return err
				}
				continue
			}
			r, err := OpenFile(ctx, store, &f)
			if err != nil {
				return err
			}
			w, err := z.Create(name)
			if err == nil {
				_, err = io.Copy(w, r)
			}
			err = errors.Join(err, r.Close())
			if err != nil {
				return err
			}
		}
		return nil
	}
	return errors.Join(walk(folder.ID, ""), z.Close())
}
