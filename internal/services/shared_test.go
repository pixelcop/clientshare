package services

import (
	"fmt"
	"math"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSharedDiscoveryFiltersBeforeTotalsAndRetainsStrongerChildRoots(t *testing.T) {
	f, service := sharingFixture(t)
	require.NoError(t, f.db.Model(&models.File{}).Where("id = 'folder'").Update("filename", "Shared folder").Error)
	require.NoError(t, f.db.Model(&models.File{}).Where("id = 'file'").Update("filename", "A stronger file").Error)
	f.grant(t, "folder", "recipient", models.AccessCapabilities{CanRead: true})
	f.grant(t, "nested", "recipient", models.AccessCapabilities{CanRead: true})
	child := f.grant(t, "file", "recipient", models.AccessCapabilities{CanRead: true, CanShare: true})
	expired := f.grant(t, "sibling", "recipient", models.AccessCapabilities{CanRead: true})
	require.NoError(t, f.db.Model(expired).Update("expires_at", accessTestNow).Error)
	foreign := models.ItemGrant{TenantID: "foreign", ClientID: "foreign-client", UserID: "recipient", FileID: "foreign-file", AccessCapabilities: models.AccessCapabilities{CanRead: true}}
	require.NoError(t, f.db.Create(&foreign).Error)
	req := f.req("recipient", "", AccessContext)
	page, err := service.SharedWithMe(t.Context(), req, 1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, page.Pagination.TotalItems)
	require.Equal(t, 2, page.Pagination.TotalPages)
	require.Len(t, page.Items, 1)
	require.Equal(t, "file", page.Items[0].ID)
	require.Nil(t, page.Items[0].FolderID)
	require.Nil(t, page.Items[0].UserID)
	require.Equal(t, "A stronger file", page.Items[0].Path)
	page, err = service.SharedWithMe(t.Context(), req, 2, 1)
	require.NoError(t, err)
	require.Equal(t, "folder", page.Items[0].ID)
	page, err = service.SharedWithMe(t.Context(), req, math.MaxInt, math.MaxInt)
	require.NoError(t, err)
	require.Empty(t, page.Items)
	require.Equal(t, 100, page.Pagination.PageSize)
	// Current state wins over the same already-open session's previous page.
	require.NoError(t, f.db.Model(child).Update("revoked_at", accessTestNow).Error)
	page, err = service.SharedWithMe(t.Context(), req, 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Pagination.TotalItems)
	require.Equal(t, "folder", page.Items[0].ID)
	require.NoError(t, f.db.Model(&models.File{}).Where("id = 'root'").Update("deleted_at", accessTestNow).Error)
	page, err = service.SharedWithMe(t.Context(), req, 1, 20)
	require.NoError(t, err)
	require.Empty(t, page.Items)
	require.Zero(t, page.Pagination.TotalItems)
}

func TestSharedDiscoveryPrivateCollectionIdentityAndBatchBoundaries(t *testing.T) {
	f, service := sharingFixture(t)
	f.grant(t, "folder", "recipient", models.AccessCapabilities{CanUpload: true})
	page, err := service.SharedWithMe(t.Context(), f.req("recipient", "", AccessContext), 1, 20)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "Upload destination", page.Items[0].Filename)
	require.Empty(t, page.Items[0].Path)
	require.Nil(t, page.Items[0].FolderID)
	require.True(t, page.Items[0].UploadedAt.IsZero())
	for _, actor := range []AccessActor{{UserID: "recipient", LinkID: "bearer"}, {UserID: "missing"}, {UserID: "unknown-role"}, {}} {
		req := f.req("", "", AccessContext)
		req.Actor = actor
		page, err := service.SharedWithMe(t.Context(), req, 1, 20)
		require.NoError(t, err)
		require.Empty(t, page.Items)
	}
	for i := 0; i < 501; i++ {
		id := fmt.Sprintf("shared-%03d", i)
		require.NoError(t, f.db.Create(&models.File{ID: id, TenantID: "tenant", ClientID: "client", Filename: id, Type: "file", FolderID: accessPtr("folder")}).Error)
		f.grant(t, id, "recipient", models.AccessCapabilities{CanRead: true})
	}
	queries := 0
	require.NoError(t, f.db.Callback().Query().Before("gorm:query").Register("count-shared", func(*gorm.DB) { queries++ }))
	defer f.db.Callback().Query().Remove("count-shared")
	page, err = service.SharedWithMe(t.Context(), f.req("recipient", "", AccessContext), 1, 100)
	require.NoError(t, err)
	require.EqualValues(t, 502, page.Pagination.TotalItems)
	require.Len(t, page.Items, 100)
	require.Less(t, queries, 40, "batch across the 500-target boundary, never one query per row")
}
