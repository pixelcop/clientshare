package services

import (
	"context"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/stretchr/testify/require"
)

func TestUnifiedFolderListingOrderingAndPagination(t *testing.T) {
	db := setupFileServiceTestDB(t)
	service := NewFileService(db)
	parent := "parent"
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	items := []models.File{
		{TenantID: tenantctx.DefaultTenantID, ClientID: "client", FolderID: &parent, Type: "folder", Filename: "Alpha", UploadedAt: newer},
		{TenantID: tenantctx.DefaultTenantID, ClientID: "client", FolderID: &parent, Type: "folder", Filename: "Zulu", UploadedAt: older},
		{TenantID: tenantctx.DefaultTenantID, ClientID: "client", FolderID: &parent, Type: "file", Filename: "apple.txt", UploadedAt: newer, Size: 20},
		{TenantID: tenantctx.DefaultTenantID, ClientID: "client", FolderID: &parent, Type: "file", Filename: "zebra.txt", UploadedAt: older, Size: 10},
	}
	for i := range items {
		require.NoError(t, db.Create(&items[i]).Error)
	}
	foreign := models.File{TenantID: "another-tenant", ClientID: "client", FolderID: &parent, Type: "folder", Filename: "Hidden"}
	require.NoError(t, db.Create(&foreign).Error)
	deleted := models.File{TenantID: tenantctx.DefaultTenantID, ClientID: "client", FolderID: &parent, Type: "file", Filename: "Deleted"}
	require.NoError(t, db.Create(&deleted).Error)
	require.NoError(t, db.Model(&deleted).Update("deleted_at", time.Now()).Error)

	for _, tt := range []struct {
		field, direction string
		names            []string
	}{
		{"filename", "asc", []string{"Alpha", "Zulu", "apple.txt", "zebra.txt"}},
		{"filename", "desc", []string{"Zulu", "Alpha", "zebra.txt", "apple.txt"}},
		{"uploaded_at", "asc", []string{"Zulu", "Alpha", "zebra.txt", "apple.txt"}},
		{"uploaded_at", "desc", []string{"Alpha", "Zulu", "apple.txt", "zebra.txt"}},
		{"size", "asc", []string{"Alpha", "Zulu", "zebra.txt", "apple.txt"}},
		{"size", "desc", []string{"Alpha", "Zulu", "apple.txt", "zebra.txt"}},
	} {
		t.Run(tt.field+"/"+tt.direction, func(t *testing.T) {
			var names []string
			for page := 1; page <= 2; page++ {
				pagination, rows, err := service.ListFilesInFolder(context.Background(), tenantctx.DefaultTenantID, "client", parent, "all", page, 3, tt.field, tt.direction)
				require.NoError(t, err)
				require.Equal(t, int64(4), pagination.TotalItems)
				require.Equal(t, 2, pagination.TotalPages)
				require.Equal(t, page, pagination.Page)
				for _, row := range rows {
					names = append(names, row.Filename)
				}
			}
			require.Equal(t, tt.names, names)
		})
	}

	for _, kind := range []string{"file", "folder"} {
		pagination, rows, err := service.ListFilesInFolder(context.Background(), tenantctx.DefaultTenantID, "client", parent, kind, 1, 20, "filename", "asc")
		require.NoError(t, err)
		require.Equal(t, int64(2), pagination.TotalItems)
		for _, row := range rows {
			require.Equal(t, kind, row.Type)
		}
	}

	pagination, rows, err := service.ListFilesInFolder(context.Background(), tenantctx.DefaultTenantID, "client", parent, "all", 99, 3, "filename", "asc")
	require.NoError(t, err)
	require.Equal(t, 2, pagination.Page)
	require.Len(t, rows, 1)
	require.NoError(t, db.Model(&items[3]).Update("deleted_at", time.Now()).Error)
	pagination, rows, err = service.ListFilesInFolder(context.Background(), tenantctx.DefaultTenantID, "client", parent, "all", 99, 3, "filename", "asc")
	require.NoError(t, err)
	require.Equal(t, 1, pagination.Page)
	require.Len(t, rows, 3)
	pagination, rows, err = service.ListFilesInFolder(context.Background(), tenantctx.DefaultTenantID, "client", "empty", "all", 99, 3, "filename", "asc")
	require.NoError(t, err)
	require.Equal(t, 1, pagination.Page)
	require.Zero(t, pagination.TotalPages)
	require.Empty(t, rows)
}
