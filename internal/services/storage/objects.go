package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ObjectStore contains only immutable byte operations. Keys are opaque IDs;
// tenant scope is supplied by Tenant, never by a caller's logical file path.
type ObjectStore interface {
	PutNew(context.Context, string, io.Reader) (ObjectInfo, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Stat(context.Context, string) (ObjectInfo, error)
	DeleteObject(context.Context, string) error
}

// ImmutableLayout identifies the hash-sharded physical layout. Object IDs in
// database records remain unchanged; placement is derived only from the ID.
const ImmutableLayout = "immutable-v2"

type ObjectInfo struct {
	Size     int64
	Checksum string // lowercase hexadecimal SHA-256, never an S3 ETag
}

func validIdentity(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func objectPath(tenant, key string) (string, error) {
	if !validIdentity(tenant) || !validIdentity(key) {
		return "", fmt.Errorf("%w: invalid object or tenant identity", os.ErrPermission)
	}
	digest := sha256.Sum256([]byte(key))
	bucket := hex.EncodeToString(digest[:1])
	return "objects/" + tenant + "/" + bucket + "/" + key, nil
}

// SafePath refuses symlinks in every existing component, including the root.
// Storage roots must be application-owned; external writers are unsupported.
func SafePath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) || strings.Contains(relative, "\\") {
		return "", os.ErrPermission
	}
	for _, part := range strings.Split(relative, "/") {
		if part == ".." {
			return "", os.ErrPermission
		}
	}
	p, err := sanitizePath(root, relative)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	for current := p; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: symlink at %s", os.ErrPermission, current)
		}
		if current == rootAbs || filepath.Dir(current) == current {
			break
		}
	}
	return p, nil
}

func (s *LocalStorage) ObjectPath(key string) (string, error) {
	rel, err := objectPath(s.tenantID, key)
	if err != nil {
		return "", err
	}
	return SafePath(s.Root, rel)
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

func Hash(ctx context.Context, r io.Reader) (ObjectInfo, error) {
	h := sha256.New()
	n, err := io.Copy(h, contextReader{ctx, r})
	return ObjectInfo{n, hex.EncodeToString(h.Sum(nil))}, err
}

func (s *LocalStorage) PutNew(ctx context.Context, key string, src io.Reader) (ObjectInfo, error) {
	p, err := s.ObjectPath(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return ObjectInfo{}, err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return ObjectInfo{}, err
	}
	defer os.Remove(f.Name())
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), contextReader{ctx, src})
	err = errors.Join(copyErr, f.Sync(), f.Close())
	if err != nil {
		return ObjectInfo{}, err
	}
	if err = ctx.Err(); err != nil {
		return ObjectInfo{}, err
	}
	// link is an atomic, no-replace publication on supported filesystems. Fail
	// explicitly on mounts that cannot provide it; never fall back to rename.
	if err = os.Link(f.Name(), p); err != nil {
		return ObjectInfo{}, fmt.Errorf("publish immutable object (hardlink support required): %w", err)
	}
	dir, err := os.Open(filepath.Dir(p))
	if err == nil {
		err = errors.Join(dir.Sync(), dir.Close())
	}
	if err != nil {
		return ObjectInfo{}, err
	} // published orphan is recoverable
	return ObjectInfo{n, hex.EncodeToString(h.Sum(nil))}, nil
}

func (s *LocalStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := s.ObjectPath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("object is not a regular file: %s", key)
	}
	return f, nil
}

func (s *LocalStorage) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	f, err := s.Open(ctx, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	defer f.Close()
	return Hash(ctx, f)
}

func (s *LocalStorage) DeleteObject(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := s.ObjectPath(key)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(p); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("object is not a regular file: %s", key)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
