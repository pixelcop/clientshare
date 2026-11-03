package services

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/stretchr/testify/require"
)

func TestShardFlatObjects(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(map[bool]string{false: "flat only", true: "both names after interruption"}[interrupted], func(t *testing.T) {
			db, root, _, file := migrationFixture(t)
			ctx := context.Background()
			plan, err := InventoryObjects(ctx, db, root, nil)
			require.NoError(t, err)
			require.NoError(t, ApplyObjects(ctx, db, root, plan))
			require.NoError(t, db.First(file, "id = ?", file.ID).Error)
			before := *file
			sharded := immutableObjectPathForTest(t, root, file.TenantID, *file.StorageKey)
			flat := filepath.Join(root, "objects", file.TenantID, *file.StorageKey)
			require.NoError(t, os.Link(sharded, flat))
			if !interrupted {
				require.NoError(t, os.Remove(sharded))
				// Reads must not probe the old flat location.
				_, err = storage.NewLocalStorage(root).Tenant(file.TenantID).Open(ctx, *file.StorageKey)
				require.ErrorIs(t, err, os.ErrNotExist)
			}
			require.NoError(t, ShardFlatObjects(ctx, db, root, false, io.Discard))
			require.FileExists(t, flat)
			if !interrupted {
				require.NoFileExists(t, sharded)
			}
			inode, err := os.Stat(flat)
			require.NoError(t, err)
			require.NoError(t, ShardFlatObjects(ctx, db, root, true, io.Discard))
			require.NoError(t, ShardFlatObjects(ctx, db, root, true, io.Discard))
			require.NoFileExists(t, flat)
			after, err := os.Stat(sharded)
			require.NoError(t, err)
			require.True(t, os.SameFile(inode, after), "conversion should relocate without copying bytes")
			require.FileExists(t, filepath.Join(root, "tenants", file.TenantID, "Acme", "report.txt"))
			require.NoError(t, db.First(file, "id = ?", file.ID).Error)
			require.Equal(t, before, *file, "conversion must not change metadata or keys")
		})
	}
}

func TestShardFlatObjectsRefusesCorruptionAndManagedViews(t *testing.T) {
	for _, failure := range []string{"source corruption", "destination conflict", "managed view", "missing object"} {
		t.Run(failure, func(t *testing.T) {
			db, root, _, file := migrationFixture(t)
			ctx := context.Background()
			plan, err := InventoryObjects(ctx, db, root, nil)
			require.NoError(t, err)
			require.NoError(t, ApplyObjects(ctx, db, root, plan))
			sharded := immutableObjectPathForTest(t, root, file.TenantID, file.ID)
			flat := filepath.Join(root, "objects", file.TenantID, file.ID)
			// Independent files make conflict checks meaningful.
			require.NoError(t, os.WriteFile(flat, []byte("hello"), 0600))
			switch failure {
			case "source corruption":
				require.NoError(t, os.WriteFile(flat, []byte("wrong"), 0600))
			case "destination conflict":
				require.NoError(t, os.WriteFile(sharded, []byte("wrong"), 0600))
			case "managed view":
				require.NoError(t, db.Model(&models.ReadableView{}).Where("tenant_id = ?", file.TenantID).Update("manifest", `{"Mode":"export","Entries":[{"Path":"report.txt"}]}`).Error)
			case "missing object":
				require.NoError(t, os.Remove(flat))
				require.NoError(t, os.Remove(sharded))
			}
			require.Error(t, ShardFlatObjects(ctx, db, root, true, io.Discard))
			if failure != "missing object" {
				require.FileExists(t, flat)
			}
		})
	}
}

func TestOrphanCleanupRejectsWrongBucketAndPreservesFlatFiles(t *testing.T) {
	db, root, _, _ := migrationFixture(t)
	flat := filepath.Join(root, "objects", "tenant", "old-flat")
	require.NoError(t, os.MkdirAll(filepath.Dir(flat), 0700))
	require.NoError(t, os.WriteFile(flat, []byte("unmanaged flat bytes"), 0600))
	require.NoError(t, CleanupOrphanObjects(context.Background(), db, root, 24*time.Hour, true, io.Discard))
	require.FileExists(t, flat)
	wrong := filepath.Join(root, "objects", "tenant", "00", "object1")
	require.NoError(t, os.MkdirAll(filepath.Dir(wrong), 0700))
	require.NoError(t, os.WriteFile(wrong, []byte("wrong bucket"), 0600))
	err := CleanupOrphanObjects(context.Background(), db, root, 24*time.Hour, true, io.Discard)
	require.ErrorContains(t, err, "unexpected object bucket")
	require.FileExists(t, wrong)
}

func TestShardFlatObjectsRejectsLegacyFullPathRecords(t *testing.T) {
	db, root, _, _ := migrationFixture(t)
	require.ErrorContains(t, ShardFlatObjects(context.Background(), db, root, true, io.Discard), "use storage inventory/migrate")
}
