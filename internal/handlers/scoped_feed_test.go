package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestFeedFiltersGroupedItemsCountsAndReadMutationsByCurrentAccess(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	visible := createFileRecord(t, env, env.rootFolder, "visible-feed.txt", "visible")
	private := createFileRecord(t, env, env.rootFolder, "private-feed.txt", "private")
	grant := models.ItemGrant{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: visible.ID, UserID: env.otherClientUser.ID, CreatedBy: "removed-creator", AccessCapabilities: models.AccessCapabilities{CanRead: true}}
	require.NoError(t, env.db.Create(&grant).Error)
	batch := "test-batch"
	for i, file := range []*models.File{visible, private} {
		require.NoError(t, env.db.Create(&models.FileActivity{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: &file.ID, UploadBatchID: &batch, Action: "upload", FilePath: "private-ancestor/" + file.Filename, CreatedAt: time.Now().Add(time.Duration(i) * time.Second)}).Error)
	}
	grouped := models.FeedEvent{TenantID: env.client.TenantID, UserID: env.otherClientUser.ID, EventType: models.FeedEventTypeFileUploaded, ClientID: &env.client.ID, ActorUserID: &env.adminUser.ID, SourceType: models.FeedEventSourceTypeFileUploadBatch, SourceID: batch, FileID: &visible.ID, FilePath: "private-ancestor/visible-feed.txt"}
	privateEvent := models.FeedEvent{TenantID: env.client.TenantID, UserID: env.otherClientUser.ID, EventType: models.FeedEventTypeFileUploaded, ClientID: &env.client.ID, ActorUserID: &env.adminUser.ID, SourceType: models.FeedEventSourceTypeFileActivity, SourceID: "private-activity", FileID: &private.ID, FilePath: "private-ancestor/private-feed.txt"}
	for _, event := range []*models.FeedEvent{&grouped, &privateEvent} {
		require.NoError(t, env.db.Create(event).Error)
	}
	readFeed := func() models.FeedListResponse {
		t.Helper()
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/feed?state=all&page_size=1", env.otherToken, nil))
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		var result models.FeedListResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		resp.Body.Close()
		return result
	}
	result := readFeed()
	require.EqualValues(t, 1, result.Pagination.TotalItems)
	require.EqualValues(t, 1, result.Summary.UnreadCount)
	require.False(t, result.Pagination.HasMore)
	require.Len(t, result.Items, 1)
	event := result.Items[0]
	require.Equal(t, grouped.ID, event.ID)
	require.Equal(t, 1, event.FileCount)
	require.Equal(t, []models.FeedEventFile{{ID: visible.ID, Path: visible.Filename}}, event.Files)
	require.Equal(t, visible.Filename, event.FilePath)
	require.Empty(t, event.ClientName)
	require.Empty(t, event.ActorEmail)
	require.Nil(t, event.ActorUserID)
	resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/feed/"+privateEvent.ID+"/read", env.otherToken, nil))
	require.NoError(t, err)
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPost, "/api/feed/read-all", env.otherToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	require.NoError(t, env.db.First(&privateEvent, "id = ?", privateEvent.ID).Error)
	require.False(t, privateEvent.IsRead)
	require.NoError(t, env.db.Model(&grant).Update("revoked_at", time.Now()).Error)
	result = readFeed()
	require.Empty(t, result.Items)
	require.Zero(t, result.Pagination.TotalItems)
	require.Zero(t, result.Summary.ReadCount)
	require.Zero(t, result.Summary.UnreadCount)
}
