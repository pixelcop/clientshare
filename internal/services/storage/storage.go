package storage

import (
	"archive/zip"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileInfo struct {
	Name    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

type Storage interface {
	Tenant(tenantID string) Storage
	List(path string) ([]FileInfo, error)
	Upload(dst string, src io.Reader) (int64, error)
	Download(src string) (io.ReadCloser, error)
	Delete(path string) error
	CreateFolder(path string) error
	Move(src string, dst string) error
	ZipFolder(path string, dst string) error
}

type LocalStorage struct {
	Root   string
	prefix string
}

func NewLocalStorage(root string) *LocalStorage {
	return &LocalStorage{Root: root}
}

func (s *LocalStorage) Tenant(tenantID string) Storage {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return s
	}
	return &LocalStorage{
		Root:   s.Root,
		prefix: joinScopedPath(s.prefix, tenantRootPath(tenantID)),
	}
}

func (s *LocalStorage) scopedRoot() string {
	if s.prefix == "" {
		return s.Root
	}
	return filepath.Join(s.Root, s.prefix)
}

func (s *LocalStorage) resolvePath(path string) (string, error) {
	return sanitizePath(s.scopedRoot(), path)
}

func (s *LocalStorage) List(path string) ([]FileInfo, error) {
	absPath, err := s.resolvePath(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}
	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    entry.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
		})
	}
	return files, nil
}

func (s *LocalStorage) Upload(dst string, src io.Reader) (int64, error) {
	absPath, err := s.resolvePath(dst)
	if err != nil {
		return 0, err
	}
	err = os.MkdirAll(filepath.Dir(absPath), 0755)
	if err != nil {
		return 0, err
	}
	f, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	size, err := io.Copy(f, src)
	return size, err
}

func (s *LocalStorage) Download(path string) (io.ReadCloser, error) {
	absPath, err := s.resolvePath(path)
	if err != nil {
		return nil, err
	}
	return os.Open(absPath)
}

func (s *LocalStorage) Delete(path string) error {
	absPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(absPath)
}

func (s *LocalStorage) CreateFolder(path string) error {
	absPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(absPath, 0755)
}

func (s *LocalStorage) Move(src string, dst string) error {
	absSrc, err := s.resolvePath(src)
	if err != nil {
		return err
	}
	absDst, err := s.resolvePath(dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absDst), 0755); err != nil {
		return err
	}
	return os.Rename(absSrc, absDst)
}

func (s *LocalStorage) ZipFolder(path string, dst string) error {
	absPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	zipFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer zipFile.Close()
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()
	return filepath.Walk(absPath, func(file string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(absPath, file)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		w, err := zipWriter.Create(rel)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		return err
	})
}

func tenantRootPath(tenantID string) string {
	return filepath.Join("tenants", tenantID)
}

func joinScopedPath(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		filtered = append(filtered, part)
	}
	if len(filtered) == 0 {
		return ""
	}
	return filepath.Join(filtered...)
}

// sanitizePath ensures the path is safe and within the storage root
func sanitizePath(root, p string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	cleaned := filepath.Clean(p)
	abs := filepath.Join(rootAbs, cleaned)

	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", os.ErrPermission
	}
	return abs, nil
}

// GetFileMetadata returns file size, mod time, and mime type
func GetFileMetadata(path string) (size int64, modTime time.Time, mimeType string, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, time.Time{}, "", err
	}
	size = info.Size()
	modTime = info.ModTime()
	mimeType = mime.TypeByExtension(filepath.Ext(path))
	return
}

func ClientFolderPath(name string) string {
	return filepath.Clean(name)
}

func TrashPath(path string) string {
	return filepath.Join("trash", filepath.Clean(path))
}
