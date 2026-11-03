package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type interruptedReader struct{}

func (interruptedReader) Read([]byte) (int, error) { return 0, errors.New("interrupted") }

func TestImmutableLocalObjects(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	base := NewLocalStorage(root)
	a, b := base.Tenant("tenant-a"), base.Tenant("tenant-b")
	info, err := a.PutNew(ctx, "object1", strings.NewReader("hello"))
	require.NoError(t, err)
	require.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", info.Checksum)
	_, err = a.PutNew(ctx, "object1", strings.NewReader("overwritten"))
	require.ErrorIs(t, err, os.ErrExist)
	_, err = b.Open(ctx, "object1")
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = a.PutNew(ctx, "failed", interruptedReader{})
	require.Error(t, err)
	_, err = a.Stat(ctx, "failed")
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, key := range []string{"", "..", "../other", "a/b", "a\\b", "/absolute"} {
		_, err = a.PutNew(ctx, key, strings.NewReader("bad"))
		require.ErrorIs(t, err, os.ErrPermission)
	}
	_, err = base.Tenant("../escape").PutNew(ctx, "object1", strings.NewReader("bad"))
	require.ErrorIs(t, err, os.ErrPermission)
	// Scoping again selects the same tenant, rather than nesting it twice.
	r, err := a.Tenant("tenant-a").Open(ctx, "object1")
	require.NoError(t, err)
	content, err := io.ReadAll(r)
	require.NoError(t, err)
	r.Close()
	require.Equal(t, "hello", string(content))
	require.FileExists(t, filepath.Join(root, "objects", "tenant-a", "object1"))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, "objects", "escaped")))
	_, err = base.Tenant("escaped").PutNew(ctx, "object2", strings.NewReader("bad"))
	require.ErrorIs(t, err, os.ErrPermission)
	require.NoError(t, a.DeleteObject(ctx, "object1"))
	require.NoError(t, a.DeleteObject(ctx, "object1"))
}
