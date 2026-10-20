package services

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

type QuotaError struct {
	Code    string
	Message string
}

func (e *QuotaError) Error() string { return e.Message }

var ErrStaffLimit = &QuotaError{"staff_limit_reached", "Staff user limit reached. Remove a staff user or upgrade your plan."}
var ErrStorageLimit = &QuotaError{"storage_limit_reached", "Storage limit reached. Delete files or upgrade your plan."}

// lockQuota must run inside the same transaction as the protected mutation.
// A write lock works on PostgreSQL and SQLite, including across server processes.
func lockQuota(tx *gorm.DB, tenantID string) (*models.TenantEntitlement, error) {
	if tenantID == "" {
		return nil, errors.New("tenant id is required")
	}
	result := tx.Model(&models.TenantEntitlement{}).Where("tenant_id = ?", tenantID).
		UpdateColumn("max_users", gorm.Expr("max_users"))
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, errors.New("tenant entitlements are missing")
	}
	var entitlement models.TenantEntitlement
	if err := tx.Where("tenant_id = ?", tenantID).First(&entitlement).Error; err != nil {
		return nil, err
	}
	return &entitlement, nil
}

// CheckStaffQuota includes pending invitations, which already have user records.
// Existing staff may still be edited when a tenant is over its new limit.
func CheckStaffQuota(tx *gorm.DB, user *models.User) error {
	if user.Role != "admin" && user.Role != "manager" {
		return nil
	}
	entitlement, err := lockQuota(tx, user.TenantID)
	if err != nil {
		return err
	}
	if entitlement.MaxUsers == 0 {
		return nil
	}
	var existing int64
	if user.ID != "" {
		if err := tx.Model(&models.User{}).Where("tenant_id = ? AND id = ? AND role IN ?", user.TenantID, user.ID, []string{"admin", "manager"}).Count(&existing).Error; err != nil {
			return err
		}
	}
	if existing > 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&models.User{}).Where("tenant_id = ? AND role IN ?", user.TenantID, []string{"admin", "manager"}).Count(&count).Error; err != nil {
		return err
	}
	if count >= entitlement.MaxUsers {
		return ErrStaffLimit
	}
	return nil
}

type TenantUsage struct {
	StaffUsers      int64 `json:"staff_users"`
	MaxUsers        int64 `json:"max_users"`
	StorageBytes    int64 `json:"storage_bytes"`
	ReservedBytes   int64 `json:"reserved_bytes"`
	MaxStorageBytes int64 `json:"max_storage_bytes"`
}

func tenantUsage(tx *gorm.DB, tenantID string, entitlement *models.TenantEntitlement) (*TenantUsage, error) {
	u := &TenantUsage{MaxUsers: entitlement.MaxUsers, MaxStorageBytes: entitlement.MaxStorageBytes}
	if err := tx.Model(&models.User{}).Where("tenant_id = ? AND role IN ?", tenantID, []string{"admin", "manager"}).Count(&u.StaffUsers).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&models.File{}).Where("tenant_id = ? AND type = 'file' AND deleted_at IS NULL", tenantID).Select("COALESCE(SUM(size), 0)").Scan(&u.StorageBytes).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&models.StorageReservation{}).Where("tenant_id = ? AND expires_at > ?", tenantID, time.Now()).Select("COALESCE(SUM(bytes), 0)").Scan(&u.ReservedBytes).Error; err != nil {
		return nil, err
	}
	return u, nil
}

func GetTenantUsage(ctx context.Context, db *gorm.DB, tenantID string) (*TenantUsage, error) {
	var u *TenantUsage
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := lockQuota(tx, tenantID)
		if err != nil {
			return err
		}
		u, err = tenantUsage(tx, tenantID, e)
		return err
	})
	return u, err
}

func (s *TenantProvisioningService) Usage(ctx context.Context, tenantID string) (*TenantUsage, error) {
	return GetTenantUsage(ctx, s.db, tenantID)
}

func (s *TenantSettingsService) Usage(ctx context.Context, tenantID string) (*TenantUsage, error) {
	return GetTenantUsage(ctx, s.DB, tenantID)
}

func ReserveStorage(ctx context.Context, db *gorm.DB, tenantID string, size int64) (*models.StorageReservation, error) {
	if size < 0 {
		return nil, errors.New("invalid file size")
	}
	r := &models.StorageReservation{ID: ids.NewULID(), TenantID: tenantID, Bytes: size, ExpiresAt: time.Now().Add(24 * time.Hour)}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := lockQuota(tx, tenantID)
		if err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND expires_at <= ?", tenantID, time.Now()).Delete(&models.StorageReservation{}).Error; err != nil {
			return err
		}
		u, err := tenantUsage(tx, tenantID, e)
		if err != nil {
			return err
		}
		if e.MaxStorageBytes > 0 && (u.StorageBytes > e.MaxStorageBytes || u.ReservedBytes > e.MaxStorageBytes-u.StorageBytes || size > e.MaxStorageBytes-u.StorageBytes-u.ReservedBytes) {
			return ErrStorageLimit
		}
		return tx.Create(r).Error
	})
	return r, err
}

func FinalizeStorage(ctx context.Context, db *gorm.DB, reservation *models.StorageReservation, file *models.File) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := lockQuota(tx, reservation.TenantID)
		if err != nil {
			return err
		}
		var r models.StorageReservation
		if err := tx.Where("id = ? AND tenant_id = ? AND expires_at > ?", reservation.ID, reservation.TenantID, time.Now()).First(&r).Error; err != nil {
			return err
		}
		if file.TenantID != r.TenantID || file.Size < 0 || file.Size > r.Bytes {
			return errors.New("uploaded size exceeds reservation")
		}
		u, err := tenantUsage(tx, r.TenantID, e)
		if err != nil {
			return err
		}
		if e.MaxStorageBytes > 0 && (u.StorageBytes > e.MaxStorageBytes || file.Size > e.MaxStorageBytes-u.StorageBytes) {
			return ErrStorageLimit
		}
		if err := tx.Create(file).Error; err != nil {
			return err
		}
		return tx.Delete(&r).Error
	})
}

func ReleaseStorage(db *gorm.DB, r *models.StorageReservation) error {
	return db.Where("id = ? AND tenant_id = ?", r.ID, r.TenantID).Delete(&models.StorageReservation{}).Error
}

// UploadFile reserves quota before writing and commits measured bytes atomically.
// Multipart sizes are measured by the server; the bounded reader also prevents
// a mismatched stream from exceeding its reservation.
func UploadFile(ctx context.Context, db *gorm.DB, store storage.Storage, file *models.File, src io.Reader, size int64) error {
	r, err := ReserveStorage(ctx, db, file.TenantID, size)
	if err != nil {
		return err
	}
	defer ReleaseStorage(db, r)
	reader := &io.LimitedReader{R: src, N: size}
	file.Size, err = store.Upload(file.Path, reader)
	if err != nil {
		// Storage owns cleanup of failed writes. Deleting here could remove an
		// existing file when the upload failed before creating anything.
		return err
	}
	var extra [1]byte
	n, readErr := src.Read(extra[:])
	if n != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		_ = store.Delete(file.Path)
		return errors.New("upload exceeds declared size or could not be read")
	}
	if err := FinalizeStorage(ctx, db, r, file); err != nil {
		cleanupErr := store.Delete(file.Path)
		return errors.Join(err, cleanupErr)
	}
	return nil
}
