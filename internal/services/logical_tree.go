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

// WriteFolderZip follows active child relationships under the authorized folder.
// The current authorization model grants access to a client's entire tree.
func (s *FileService) WriteFolderZip(ctx context.Context, store storage.Storage, folder *models.File, out io.Writer) error {
	var files []models.File
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND deleted_at IS NULL", folder.TenantID, folder.ClientID).Find(&files).Error; err != nil {
		return err
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
