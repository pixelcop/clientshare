package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/links"
)

func TestReadOnlyLinkFileAccess(t *testing.T) {
	for _, access := range []string{"read", "view"} {
		t.Run(access, func(t *testing.T) {
			env := setupFileTestEnv(t)
			file := createFileRecord(t, env, env.rootFolder, "readme.txt", "read only")
			link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, Token: "readonly", AccessType: access, ExpiresAt: time.Now().Add(time.Hour)}
			if err := env.db.Create(&link).Error; err != nil {
				t.Fatal(err)
			}
			// A stale JWT with broader permissions must not override the database.
			token, err := auth.GenerateLinkJWT(env.client.ID, link.ID, "readwrite", env.client.TenantID)
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				method, path string
				status       int
			}{
				{http.MethodGet, "/api/folders", 200},
				{http.MethodGet, "/api/folders/" + env.rootFolder.ID, 200},
				{http.MethodGet, "/api/folders/" + env.rootFolder.ID + "/breadcrumbs", 200},
				{http.MethodGet, "/api/files/" + file.ID, 200},
				{http.MethodGet, "/api/files/" + file.ID + "/download", 200},
				{http.MethodGet, "/api/folders/" + env.rootFolder.ID + "/download-zip", 200},
				{http.MethodPost, "/api/folders/" + env.rootFolder.ID, 403},
				{http.MethodPost, "/api/folders/" + env.rootFolder.ID + "/upload", 403},
				{http.MethodDelete, "/api/files/" + file.ID, 403},
				{http.MethodDelete, "/api/files/" + env.rootFolder.ID, 403},
			} {
				resp, err := env.app.Test(newAuthRequest(tc.method, tc.path, token, nil))
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if resp.StatusCode != tc.status {
					t.Errorf("%s %s: want %d, got %d", tc.method, tc.path, tc.status, resp.StatusCode)
				}
			}
			if err := env.db.Model(&link).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			resp, err := env.app.Test(newAuthRequest(http.MethodGet, "/api/files/"+file.ID+"/download", token, nil))
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 403 {
				t.Fatalf("expired session: got %d", resp.StatusCode)
			}
		})
	}
}

func TestRestrictedLinkCannotBecomeFullAccessAccount(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	for _, access := range []string{"read", "view", "write", "unknown"} {
		t.Run(access, func(t *testing.T) {
			token, err := links.GenerateSecureToken(env.signingKey)
			if err != nil {
				t.Fatal(err)
			}
			link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, Token: token, AccessType: access, ExpiresAt: time.Now().Add(time.Hour)}
			if err := env.db.Create(&link).Error; err != nil {
				t.Fatal(err)
			}
			resp, err := env.app.Test(jsonRequest(http.MethodPost, "/api/auth/register", map[string]any{"token": token, "email": access + "@test.local", "name": "Viewer", "password": "password123"}))
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 403 {
				t.Fatalf("want 403, got %d", resp.StatusCode)
			}
			var count int64
			env.db.Model(&models.User{}).Where("email = ?", access+"@test.local").Count(&count)
			if count != 0 {
				t.Fatal("restricted link created a user")
			}
		})
	}
}

func TestCreateSecureLinkRejectsUnsupportedAccess(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	for _, access := range []string{"", "readonly", "admin"} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/clients/"+env.client.ID+"/links", env.adminToken, map[string]any{"access_type": access}))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("access %q: want 400, got %d", access, resp.StatusCode)
		}
	}
}
