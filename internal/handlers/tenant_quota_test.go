package handlers_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestQuotaStaffInvitationAndPromotionHTTP(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	require.NoError(t, env.db.Model(&models.TenantEntitlement{}).Where("tenant_id = ?", env.adminUser.TenantID).Update("max_users", 2).Error)
	for _, request := range []struct {
		method, path string
		body         map[string]any
	}{
		{http.MethodPost, "/api/users", map[string]any{"email": "new@test.invalid", "role": "manager", "name": "New"}},
		{http.MethodPut, "/api/users/" + env.clientUser.ID, map[string]any{"role": "manager"}},
	} {
		resp, err := env.app.Test(jsonAuthRequest(request.method, request.path, env.adminToken, request.body))
		require.NoError(t, err)
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		_ = resp.Body.Close()
		require.Equal(t, "staff_limit_reached", body["code"])
	}
	resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{"email": "client2@test.invalid", "role": "client", "client_ids": []string{env.client.ID}}))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestQuotaUploadsIncludingPublicLinksHTTP(t *testing.T) {
	env := setupFileTestEnv(t)
	tenantID := env.adminUser.TenantID
	require.NoError(t, env.db.Model(&models.TenantEntitlement{}).Where("tenant_id = ?", tenantID).Update("max_storage_bytes", 4).Error)
	linkToken, err := auth.GenerateLinkJWT(env.client.ID, "999", "upload", tenantID)
	require.NoError(t, err)
	for _, token := range []string{env.adminToken, env.clientToken, linkToken} {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("files", "quota.txt")
		require.NoError(t, err)
		_, err = part.Write([]byte("hello"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", token, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err := env.app.Test(req)
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		_ = resp.Body.Close()
		require.Equal(t, http.StatusConflict, resp.StatusCode, "%+v", result)
		require.Equal(t, "storage_limit_reached", result["code"])
	}
	var count int64
	require.NoError(t, env.db.Model(&models.File{}).Where("type = 'file'").Count(&count).Error)
	require.Zero(t, count)
}
