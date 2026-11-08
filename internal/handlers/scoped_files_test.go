package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/stretchr/testify/require"
)

func TestNamedItemHTTPIsolationAndRedactedNavigation(t *testing.T) {
	for _, scope := range []string{"file", "folder"} {
		t.Run(scope, func(t *testing.T) {
			env := setupFileTestEnv(t)
			folder := &models.File{TenantID: env.client.TenantID, ClientID: env.client.ID, FolderID: &env.rootFolder.ID, Filename: "Shared", Path: filepath.Join(env.rootFolder.Path, "Shared"), Type: "folder"}
			_, err := services.NewFileService(env.db).CreateFolderRecord(t.Context(), folder)
			require.NoError(t, err)
			file := createFileRecord(t, env, folder, "shared.txt", "shared content")
			sibling := createFileRecord(t, env, env.rootFolder, "private.txt", "private content")
			target := file.ID
			if scope == "folder" {
				target = folder.ID
			}
			grant := &models.ItemGrant{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: target, UserID: env.otherClientUser.ID}
			grant.CanRead, grant.CreatedBy = true, "removed-creator"
			require.NoError(t, env.db.Create(grant).Error)
			check := func(path string, status int) []byte {
				t.Helper()
				resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, path, env.otherToken, nil))
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, status, resp.StatusCode, path)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				return body
			}
			body := check("/api/files/"+file.ID, 200)
			var dto models.File
			require.NoError(t, json.Unmarshal(body, &dto))
			require.NotNil(t, dto.Capabilities)
			require.True(t, dto.Capabilities.CanRead)
			require.False(t, dto.Capabilities.CanShare)
			require.NotContains(t, dto.Path, env.rootFolder.Filename)
			require.Equal(t, target, dto.NavigationRootID)
			if scope == "file" {
				require.Nil(t, dto.FolderID)
			}
			require.Equal(t, "shared content", string(check("/api/files/"+file.ID+"/download", 200)))
			check("/api/files/"+sibling.ID, 404)
			check("/api/folders/"+env.rootFolder.ID, 404)
			check("/api/folders/"+env.rootFolder.ID+"/breadcrumbs", 404)
			check("/api/folders/"+env.rootFolder.ID+"/download-zip", 404)
			if scope == "folder" {
				check("/api/folders/"+folder.ID, 200)
				crumbs := check("/api/folders/"+folder.ID+"/breadcrumbs", 200)
				require.NotContains(t, string(crumbs), env.rootFolder.ID)
				check("/api/folders/"+folder.ID+"/download-zip", 200)
			} else {
				check("/api/folders/"+folder.ID, 404)
			}
			resp, err := env.app.Test(jsonAuthRequest(http.MethodDelete, "/api/files/"+file.ID, env.otherToken, nil))
			require.NoError(t, err)
			require.Equal(t, 404, resp.StatusCode)
			resp.Body.Close()
			require.NoError(t, env.db.Model(grant).Update("revoked_at", time.Now()).Error)
			check("/api/files/"+file.ID, 404) // already-open named session
		})
	}
}

func TestPrivateCollectionCannotProbeOrReuseExistingFolders(t *testing.T) {
	env := setupFileTestEnv(t)
	private := &models.File{TenantID: env.client.TenantID, ClientID: env.client.ID, FolderID: &env.rootFolder.ID, Filename: "private-directory", Path: filepath.Join(env.rootFolder.Path, "private-directory"), Type: "folder"}
	_, err := services.NewFileService(env.db).CreateFolderRecord(t.Context(), private)
	require.NoError(t, err)
	link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, Token: "collection", AccessType: "write", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, env.db.Create(&link).Error)
	token, err := (&auth.SessionStore{DB: env.db}).Create(t.Context(), env.client.TenantID, "", link.ID, auth.PortalAudience, "", "")
	require.NoError(t, err)
	for _, path := range []string{"/api/folders/" + private.ID, "/api/folders/" + private.ID + "/breadcrumbs", "/api/folders/" + private.ID + "/download-zip"} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, path, token, nil))
		require.NoError(t, err)
		require.Equal(t, 404, resp.StatusCode)
		resp.Body.Close()
	}
	for _, kind := range []string{"file", "folder"} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/folders/"+env.rootFolder.ID+"?type="+kind, token, nil))
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, err)
		require.NotContains(t, string(body), private.Filename)
		require.NotContains(t, string(body), private.ID)
	}
	for _, name := range []string{private.Filename, "a-new-directory"} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID, token, map[string]any{"name": name}))
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		var created models.File
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
		resp.Body.Close()
		require.NotEqual(t, private.ID, created.ID)
		require.Empty(t, created.Path)
		var stored models.File
		require.NoError(t, env.db.First(&stored, "id = ?", created.ID).Error)
		require.True(t, strings.HasPrefix(stored.Filename, name+"-"))
		// The same session can continue the newly created directory tree.
		resp, err = env.app.Test(jsonAuthRequest(http.MethodPost, "/api/folders/"+created.ID, token, map[string]any{"name": "nested"}))
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		resp.Body.Close()
	}
	resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/exists", token, map[string]any{"names": []string{private.Filename}}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	var result struct {
		Existing []string `json:"existing"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	resp.Body.Close()
	require.Empty(t, result.Existing)
}

func TestScopedBearerSessionsCannotEscapeItemOrAcquireSharingAuthority(t *testing.T) {
	for _, mode := range []string{"read", "view", "write", "readwrite"} {
		t.Run(mode, func(t *testing.T) {
			env := setupFileTestEnv(t)
			folder := &models.File{TenantID: env.client.TenantID, ClientID: env.client.ID, FolderID: &env.rootFolder.ID, Filename: "Shared", Path: filepath.Join(env.rootFolder.Path, "Shared"), Type: "folder"}
			_, err := services.NewFileService(env.db).CreateFolderRecord(t.Context(), folder)
			require.NoError(t, err)
			file := createFileRecord(t, env, folder, "visible.txt", "content")
			private := createFileRecord(t, env, env.rootFolder, "private.txt", "private")
			link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: &folder.ID, Token: "scoped-session", AccessType: mode, ExpiresAt: time.Now().Add(time.Hour)}
			require.NoError(t, env.db.Create(&link).Error)
			token, err := (&auth.SessionStore{DB: env.db}).Create(t.Context(), env.client.TenantID, "", link.ID, auth.PortalAudience, "", "")
			require.NoError(t, err)
			get := func(path string, status int) []byte {
				t.Helper()
				resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, path, token, nil))
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, status, resp.StatusCode, path)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				return body
			}
			body := get("/api/folders", 200)
			require.NotContains(t, string(body), env.rootFolder.ID)
			require.NotContains(t, string(body), private.Filename)
			get("/api/folders/"+env.rootFolder.ID, 404)
			get("/api/files/"+private.ID, 404)
			get("/api/files/"+file.ID+"/access", 404)
			get("/api/shared-with-me", 404)
			status := 200
			if mode == "write" {
				status = 404
				require.NotContains(t, string(body), file.Filename)
			}
			get("/api/files/"+file.ID+"/download", status)
			get("/api/folders/"+folder.ID+"/download-zip", status)
			require.NoError(t, env.db.Model(&models.File{}).Where("id = ?", env.rootFolder.ID).Update("deleted_at", time.Now()).Error)
			get("/api/folders/"+folder.ID, 401)
		})
	}
}
