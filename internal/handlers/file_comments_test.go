package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestFileCommentsHTTPIsolationValidationAndLifecycle(t *testing.T) {
	env := setupFileTestEnv(t)
	require.NoError(t, env.db.AutoMigrate(&models.FileComment{}, &models.FileCommentRevision{}))
	file := createFileRecord(t, env, env.rootFolder, "discussion.txt", "test")
	base := "/api/files/" + file.ID + "/comments"
	for _, token := range []string{env.otherToken, env.linkToken} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, base, token, nil))
		require.NoError(t, err)
		require.Equal(t, 404, resp.StatusCode)
		resp.Body.Close()
		resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, base+"/count", token, nil))
		require.NoError(t, err)
		require.Equal(t, 404, resp.StatusCode)
		resp.Body.Close()
		resp, err = env.app.Test(jsonAuthRequest(http.MethodPost, base+"/preview", token, map[string]any{"body": "**Private**", "visibility": "shared"}))
		require.NoError(t, err)
		require.Equal(t, 404, resp.StatusCode)
		resp.Body.Close()
		resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, base+"/mentions?search=Admin", token, nil))
		require.NoError(t, err)
		require.Equal(t, 404, resp.StatusCode)
		resp.Body.Close()
	}
	resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, base+"/preview", env.clientToken, map[string]any{"body": "**Ready**", "visibility": "shared"}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	var preview struct {
		BodyHTML string `json:"body_html"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&preview))
	resp.Body.Close()
	require.Contains(t, preview.BodyHTML, "<strong>Ready</strong>")
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPost, base+"/preview", env.clientToken, map[string]any{"body": "**Ready**", "visibility": "team"}))
	require.NoError(t, err)
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPost, base+"/preview", env.clientToken, map[string]any{"body": "[@Nobody](mention:unknown)", "visibility": "shared"}))
	require.NoError(t, err)
	require.Equal(t, 400, resp.StatusCode)
	resp.Body.Close()
	var commentCount int64
	require.NoError(t, env.db.Model(&models.FileComment{}).Count(&commentCount).Error)
	require.Zero(t, commentCount, "previews never create comments")
	req := jsonAuthRequest(http.MethodPost, base, env.clientToken, map[string]any{"body": "A question", "visibility": "shared"})
	req.Header.Set("Idempotency-Key", "question")
	resp, err = env.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)
	var comment models.FileComment
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&comment))
	resp.Body.Close()
	require.True(t, comment.CanEdit)
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, base+"/count", env.clientToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	var count struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&count))
	resp.Body.Close()
	require.Equal(t, 1, count.Count)
	require.NotEmpty(t, comment.AuthorName)
	path := base + "/" + comment.ID
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPut, path, env.clientToken, map[string]any{"body": "Publish it", "version": 1, "visibility": "team"}))
	require.NoError(t, err)
	require.Equal(t, 400, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPut, path, env.adminToken, map[string]any{"body": "Overwrite", "version": 1}))
	require.NoError(t, err)
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPut, path, env.clientToken, map[string]any{"body": "Edited question", "version": 1}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodDelete, path, env.clientToken, map[string]any{"version": 1}))
	require.NoError(t, err)
	require.Equal(t, 409, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodDelete, path, env.clientToken, map[string]any{"version": 2}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, base, env.adminToken, nil))
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	require.NotContains(t, string(body), "Edited question")
	require.NotContains(t, string(body), "creation_key")
	require.NotContains(t, string(body), "tenant_id")
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, path+"/history", env.clientToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, path+"/history", env.adminToken, nil))
	require.NoError(t, err)
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
}
