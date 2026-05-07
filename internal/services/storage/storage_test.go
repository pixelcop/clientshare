package storage_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pixelcop/clientshare/internal/services/storage"
)

func TestLocalStorage(t *testing.T) {
	tmpDir := t.TempDir()
	ls := storage.NewLocalStorage(tmpDir)

	// Write a temp file
	filePath := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	// Test List
	files, err := ls.List("")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(files) == 0 {
		t.Error("List: expected at least one file")
	}

	// Test Download
	r, err := ls.Download("file.txt")
	if err != nil {
		t.Fatalf("Download error: %v", err)
	}
	defer r.Close()
	buf := new(strings.Builder)
	_, err = io.Copy(buf, r)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if buf.String() != "hello world\n" {
		t.Errorf("File contents: got %q, want 'hello world\\n'", buf.String())
	}

	// Test Upload
	f, _ := os.Open(filePath)
	_, err = ls.Upload("copy.txt", f)
	if err != nil {
		t.Errorf("Upload error: %v", err)
	}
	f.Close()

	// Test Delete
	err = ls.Delete("copy.txt")
	if err != nil {
		t.Errorf("Delete error: %v", err)
	}
}

func TestLocalStorageTenantIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	rootStore := storage.NewLocalStorage(tmpDir)
	tenantA := rootStore.Tenant("tenant-a")
	tenantB := rootStore.Tenant("tenant-b")

	if _, err := tenantA.Upload("Acme/shared.txt", strings.NewReader("tenant-a")); err != nil {
		t.Fatalf("tenant A upload failed: %v", err)
	}
	if _, err := tenantB.Upload("Acme/shared.txt", strings.NewReader("tenant-b")); err != nil {
		t.Fatalf("tenant B upload failed: %v", err)
	}

	readFile := func(t *testing.T, store storage.Storage) string {
		t.Helper()
		reader, err := store.Download("Acme/shared.txt")
		if err != nil {
			t.Fatalf("download failed: %v", err)
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		return string(data)
	}

	if got := readFile(t, tenantA); got != "tenant-a" {
		t.Fatalf("tenant A read mismatch: got %q", got)
	}
	if got := readFile(t, tenantB); got != "tenant-b" {
		t.Fatalf("tenant B read mismatch: got %q", got)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "tenants", "tenant-a", "Acme", "shared.txt")); err != nil {
		t.Fatalf("expected tenant A file on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "tenants", "tenant-b", "Acme", "shared.txt")); err != nil {
		t.Fatalf("expected tenant B file on disk: %v", err)
	}
}

func TestLocalStorageRejectsPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	ls := storage.NewLocalStorage(tmpDir).Tenant("tenant-a")

	if _, err := ls.Upload(filepath.Join("..", "escape.txt"), strings.NewReader("nope")); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected upload traversal to be rejected, got %v", err)
	}
	if err := ls.CreateFolder(filepath.Join("..", "escape")); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected folder traversal to be rejected, got %v", err)
	}
	if _, err := ls.Download(filepath.Join("..", "escape.txt")); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected download traversal to be rejected, got %v", err)
	}
	if err := ls.Delete(filepath.Join("..", "escape.txt")); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected delete traversal to be rejected, got %v", err)
	}
	if err := ls.Move(filepath.Join("..", "from"), "target"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected move traversal to be rejected, got %v", err)
	}
}
