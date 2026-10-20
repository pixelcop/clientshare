package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func quotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/quota.db?_busy_timeout=10000&_journal_mode=WAL"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TenantEntitlement{}, &models.User{}, &models.File{}, &models.StorageReservation{}))
	require.NoError(t, db.Create(&models.TenantEntitlement{TenantID: "a", MaxUsers: 1, MaxStorageBytes: 10}).Error)
	require.NoError(t, db.Create(&models.TenantEntitlement{TenantID: "b", MaxUsers: 1, MaxStorageBytes: 10}).Error)
	return db
}

func TestStaffQuotaConcurrentInvitesAndRoleChanges(t *testing.T) {
	db := quotaTestDB(t)
	results := make(chan error, 10)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := models.User{TenantID: "a", Email: fmt.Sprintf("%d@test.invalid", i), Role: "manager"}
			results <- db.Transaction(func(tx *gorm.DB) error {
				if err := CheckStaffQuota(tx, &u); err != nil {
					return err
				}
				return tx.Create(&u).Error
			})
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, ErrStaffLimit)
		}
	}
	require.Equal(t, 1, success)
	var staff models.User
	require.NoError(t, db.Where("tenant_id = ?", "a").First(&staff).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return CheckStaffQuota(tx, &staff) }))
	client := models.User{TenantID: "a", Email: "client@test.invalid", Role: "client"}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := CheckStaffQuota(tx, &client); err != nil {
			return err
		}
		return tx.Create(&client).Error
	}))
	client.Role = "admin"
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error { return CheckStaffQuota(tx, &client) }), ErrStaffLimit)
	other := models.User{TenantID: "b", Role: "admin"}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return CheckStaffQuota(tx, &other) }))
	require.NoError(t, db.Delete(&staff).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return CheckStaffQuota(tx, &client) }))
}

func TestStorageQuotaConcurrentReservationsAndExpiry(t *testing.T) {
	db := quotaTestDB(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := ReserveStorage(ctx, db, "a", 3); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, ErrStorageLimit)
		}
	}
	require.Equal(t, 3, success)
	_, err := ReserveStorage(ctx, db, "b", 10)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.StorageReservation{}).Where("tenant_id = ?", "a").Update("expires_at", time.Now().Add(-time.Hour)).Error)
	r, err := ReserveStorage(ctx, db, "a", 10)
	require.NoError(t, err)
	file := &models.File{TenantID: "a", ClientID: "client", Type: "file", Size: 10}
	// Downgrading during an upload must be rechecked at finalization.
	require.NoError(t, db.Model(&models.TenantEntitlement{}).Where("tenant_id = ?", "a").Update("max_storage_bytes", 5).Error)
	require.ErrorIs(t, FinalizeStorage(ctx, db, r, file), ErrStorageLimit)
	require.NoError(t, ReleaseStorage(db, r))
	u, err := GetTenantUsage(ctx, db, "a")
	require.NoError(t, err)
	require.Zero(t, u.StorageBytes)
	require.Zero(t, u.ReservedBytes)
}

type failedUploadStore struct{ storage.Storage }

func (s failedUploadStore) Upload(path string, src io.Reader) (int64, error) {
	n, _ := s.Storage.Upload(path, io.LimitReader(src, 2))
	_ = s.Storage.Delete(path)
	return n, errors.New("storage unavailable")
}

func TestStorageUploadCleanupMeasuredBytesAndDeletion(t *testing.T) {
	db := quotaTestDB(t)
	ctx := context.Background()
	store := storage.NewLocalStorage(t.TempDir()).Tenant("a")
	file := &models.File{TenantID: "a", ClientID: "client", Path: "report.txt", Filename: "report.txt", Type: "file"}
	require.Error(t, UploadFile(ctx, db, failedUploadStore{store}, file, strings.NewReader("hello"), 5))
	_, err := store.Download(file.Path)
	require.Error(t, err)
	u, err := GetTenantUsage(ctx, db, "a")
	require.NoError(t, err)
	require.Zero(t, u.ReservedBytes)
	require.Error(t, UploadFile(ctx, db, store, file, strings.NewReader("too long"), 2))
	require.NoError(t, UploadFile(ctx, db, store, file, strings.NewReader("hello"), 8))
	require.EqualValues(t, 5, file.Size)
	duplicate := &models.File{TenantID: "a", ClientID: "client", Path: file.Path, Type: "file"}
	require.Error(t, UploadFile(ctx, db, store, duplicate, strings.NewReader("x"), 1))
	reader, err := store.Download(file.Path)
	require.NoError(t, err)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, "hello", string(content))
	_, err = ReserveStorage(ctx, db, "a", 6)
	require.ErrorIs(t, err, ErrStorageLimit)
	require.NoError(t, db.Model(file).Update("deleted_at", time.Now()).Error)
	_, err = ReserveStorage(ctx, db, "a", 10)
	require.NoError(t, err)
}
