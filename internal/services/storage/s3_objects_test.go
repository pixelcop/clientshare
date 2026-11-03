package storage_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestS3ImmutableObjects(t *testing.T) {
	ctx := context.Background()
	base := newFakeS3Storage(t)
	a, b := base.Tenant("a"), base.Tenant("b")
	info, err := a.PutNew(ctx, "object1", strings.NewReader("hello"))
	require.NoError(t, err)
	got, err := a.Stat(ctx, "object1")
	require.NoError(t, err)
	require.Equal(t, info, got)
	_, err = a.PutNew(ctx, "object1", strings.NewReader("replace"))
	require.ErrorIs(t, err, os.ErrExist)
	_, err = b.Open(ctx, "object1")
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = a.Open(ctx, "../object1")
	require.ErrorIs(t, err, os.ErrPermission)
	_, err = base.Tenant("../a").Open(ctx, "object1")
	require.ErrorIs(t, err, os.ErrPermission)
	r, err := a.Tenant("a").Open(ctx, "object1")
	require.NoError(t, err)
	content, err := io.ReadAll(r)
	r.Close()
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
	require.NoError(t, a.DeleteObject(ctx, "object1"))
	_, err = a.Open(ctx, "object1")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestS3RetriesTransientButNotPermanentFailures(t *testing.T) {
	var puts, gets atomic.Int32
	base := newFakeS3Storage(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "PUT" && strings.Contains(r.URL.Path, "/objects/") && puts.Add(1) < 3 {
				w.WriteHeader(503)
				io.WriteString(w, "<Error><Code>SlowDown</Code></Error>")
				return
			}
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/objects/") {
				gets.Add(1)
				w.WriteHeader(403)
				io.WriteString(w, "<Error><Code>AccessDenied</Code></Error>")
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	store := base.Tenant("tenant")
	_, err := store.PutNew(context.Background(), "key", strings.NewReader("hello"))
	require.NoError(t, err)
	require.EqualValues(t, 3, puts.Load())
	_, err = store.Open(context.Background(), "key")
	require.Error(t, err)
	require.EqualValues(t, 1, gets.Load())
}
