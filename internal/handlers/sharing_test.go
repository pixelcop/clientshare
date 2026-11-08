package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestSharedWithMeHTTPDoesNotGrantClientOrAncestryAccess(t *testing.T) {
	env := setupFileTestEnv(t)
	file := createFileRecord(t, env, env.rootFolder, "shared.txt", "shared content")
	createFileRecord(t, env, env.rootFolder, "private.txt", "private content")
	grant := models.ItemGrant{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: file.ID, UserID: env.otherClientUser.ID, AccessCapabilities: models.AccessCapabilities{CanRead: true}}
	require.NoError(t, env.db.Create(&grant).Error)
	resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/shared-with-me?page_size=1", env.otherToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	require.NotContains(t, string(body), "private.txt")
	require.NotContains(t, string(body), env.rootFolder.ID)
	require.NotContains(t, string(body), env.client.Name)
	var page models.PaginatedResult[models.File]
	require.NoError(t, json.Unmarshal(body, &page))
	require.EqualValues(t, 1, page.Pagination.TotalItems)
	require.Equal(t, file.ID, page.Items[0].ID)
	for _, path := range []string{"/api/folders/" + env.rootFolder.ID, "/api/clients/" + env.client.ID, "/api/users?client_id=" + env.client.ID} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, path, env.otherToken, nil))
		require.NoError(t, err)
		require.NotEqual(t, 200, resp.StatusCode, path)
		resp.Body.Close()
	}
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, "/api/shared-with-me", env.linkToken, nil))
	require.NoError(t, err)
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
	require.NoError(t, env.db.Model(&grant).Update("expires_at", time.Now().Add(-time.Second)).Error)
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, "/api/shared-with-me", env.otherToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&page))
	resp.Body.Close()
	require.Zero(t, page.Pagination.TotalItems)
	require.Empty(t, page.Items)
}

func TestApplicationScopedCreationUsesAuthorizedLifecycle(t *testing.T) {
	env := setupFileTestEnv(t)
	file := createFileRecord(t, env, env.rootFolder, "not-shared.txt", "content")
	for _, suffix := range []string{"grants", "invitations", "links"} {
		req := jsonAuthRequest(http.MethodPost, "/api/files/"+file.ID+"/"+suffix, env.adminToken, map[string]any{"user_id": env.otherClientUser.ID, "email": "recipient@example.test", "capabilities": models.AccessCapabilities{CanRead: true}, "access_type": "read"})
		req.Header.Set("Idempotency-Key", "release-gate-test")
		resp, err := env.app.Test(req)
		require.NoError(t, err)
		require.Equal(t, 201, resp.StatusCode, "creation is enabled only with complete server-side enforcement")
		resp.Body.Close()
	}
	for _, model := range []any{&models.ItemGrant{}, &models.AccessInvitation{}} {
		var count int64
		require.NoError(t, env.db.Model(model).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
	resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/files/"+file.ID+"/download", env.otherToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, "/api/folders/"+env.rootFolder.ID, env.otherToken, nil))
	require.NoError(t, err)
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
}
