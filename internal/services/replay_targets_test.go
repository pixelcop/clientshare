package services

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReplayDirectorySearchAndIDValidation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/directory.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Tenant{}, &models.User{}, &models.ReplayRequest{}))
	require.NoError(t, db.Create(&[]models.Tenant{{ID: "tenant-a", Name: "Acme", Slug: "acme"}, {ID: "tenant-b", Name: "Beta", Slug: "beta"}}).Error)
	require.NoError(t, db.Create(&[]models.User{{ID: "user-a", TenantID: "tenant-a", Name: "Alice", Email: "alice@example.com"}, {ID: "user-b", TenantID: "tenant-b", Name: "Bob", Email: "bob@example.com"}}).Error)
	svc := NewReplayRequestService(db)
	ctx := t.Context()
	options, err := svc.SearchTargets(ctx, "tenants", "ACM", "")
	require.NoError(t, err)
	require.Len(t, options, 1)
	require.Equal(t, "tenant-a", options[0].ID)
	options, err = svc.SearchTargets(ctx, "users", "example.com", "tenant-b")
	require.NoError(t, err)
	require.Len(t, options, 1)
	require.Equal(t, "user-b", options[0].ID)
	require.Equal(t, "Beta", options[0].TenantName)
	options, err = svc.SearchTargets(ctx, "users", "%", "")
	require.NoError(t, err)
	require.Empty(t, options, "wildcards are searched literally")
	_, err = svc.SearchTargets(ctx, "unknown", "", "")
	require.ErrorIs(t, err, ErrReplayRequest)
	for _, target := range []models.ReplayTarget{{TenantID: "missing"}, {UserID: "missing"}, {TenantID: "tenant-a", UserID: "user-b"}} {
		_, err = svc.Create(ctx, models.ReplayRequest{Label: "Invalid", Targets: []models.ReplayTarget{target}, ExpiresAt: time.Now().Add(time.Hour)})
		require.ErrorIs(t, err, ErrReplayRequest)
	}
	for _, target := range []models.ReplayTarget{{TenantID: "tenant-a", UserID: "user-a"}, {UserID: "user-b"}, {UserEmail: "unknown@example.com"}} {
		_, err = svc.Create(ctx, models.ReplayRequest{Label: "Valid", Targets: []models.ReplayTarget{target}, ExpiresAt: time.Now().Add(time.Hour)})
		require.NoError(t, err)
	}
}
