package handlers_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
)

func TestCollectionStatusEvidenceAndPermissions(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	if err := env.db.Where("client_id = ?", env.client.ID).Delete(&models.SecureLink{}).Error; err != nil {
		t.Fatal(err)
	}
	since := time.Now().UTC()
	link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, Token: "private-token", AccessType: "write", CreatedAt: since.Add(-time.Hour), ExpiresAt: since.Add(time.Hour)}
	if err := env.db.Create(&link).Error; err != nil {
		t.Fatal(err)
	}
	path := "/api/clients/" + env.client.ID + "/collection?since=" + url.QueryEscape(since.Format(time.RFC3339Nano))
	for _, token := range []string{env.managerToken, env.clientToken, env.linkToken} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, path, token, nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected forbidden, got %d", resp.StatusCode)
		}
	}
	check := func(want bool, count int) {
		t.Helper()
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, path, env.adminToken, nil))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("status %d", resp.StatusCode)
		}
		var raw json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "private-token") || strings.Contains(string(raw), "link_url") {
			t.Fatal("sensitive link data exposed")
		}
		var items []services.CollectionLink
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatal(err)
		}
		if len(items) != count {
			t.Fatalf("expected %d links: %s", count, raw)
		}
		if count > 0 && items[0].Received != want {
			t.Fatalf("wrong receipt: %s", raw)
		}
	}
	add := func(clientID string, linkID *string, action string, at time.Time) {
		t.Helper()
		if err := env.db.Create(&models.FileActivity{TenantID: env.client.TenantID, ClientID: clientID, SecureLinkID: linkID, Action: action, CreatedAt: at}).Error; err != nil {
			t.Fatal(err)
		}
	}
	add(env.client.ID, &link.ID, "upload", since.Add(-time.Minute))
	add(env.client.ID, nil, "upload", since.Add(time.Second))
	add(env.otherClient.ID, &link.ID, "upload", since.Add(time.Second))
	add(env.client.ID, &link.ID, "download", since.Add(time.Second))
	check(false, 1)
	add(env.client.ID, &link.ID, "upload", since.Add(time.Second))
	check(true, 1)
	if err := env.db.Delete(&link).Error; err != nil {
		t.Fatal(err)
	}
	check(false, 0)
	if err := env.db.Delete(env.client).Error; err != nil {
		t.Fatal(err)
	}
	resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, path, env.adminToken, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("deleted client: %d", resp.StatusCode)
	}
}

func TestCollectionUploadAttributionAndAccess(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, Token: "collection-upload", AccessType: "write", ExpiresAt: time.Now().Add(time.Hour)}
	if err := env.db.Create(&link).Error; err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateLinkJWT(env.client.ID, link.ID, "write", env.client.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, err := w.CreateFormFile("files", "collection.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("fixture")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", token, body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("upload: %d", resp.StatusCode)
	}
	var activity models.FileActivity
	if err := env.db.Where("client_id = ? AND action = 'upload'", env.client.ID).First(&activity).Error; err != nil {
		t.Fatal(err)
	}
	if activity.SecureLinkID == nil || *activity.SecureLinkID != link.ID {
		t.Fatal("missing authenticated link attribution")
	}
	check := func(method, path string, status int) {
		t.Helper()
		resp, err := env.app.Test(jsonAuthRequest(method, path, token, nil))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != status {
			t.Fatalf("%s %s: got %d want %d", method, path, resp.StatusCode, status)
		}
	}
	check("GET", "/api/files/"+*activity.FileID, 403)
	check("GET", "/api/files/"+*activity.FileID+"/download", 403)
	check("GET", "/api/folders/"+env.rootFolder.ID+"/download-zip", 403)
	check("GET", "/api/files/"+env.rootFolder.ID, 200)
	check("DELETE", "/api/files/"+*activity.FileID, 403)
	if err := env.db.Model(&link).Update("access_type", "read").Error; err != nil {
		t.Fatal(err)
	}
	check("POST", "/api/folders/"+env.rootFolder.ID+"/upload", 403)
	check("GET", "/api/files/"+*activity.FileID, 200)
	if err := env.db.Model(&link).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/api/files/"+*activity.FileID, 403)
	if err := env.db.Delete(&link).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/api/folders/"+env.rootFolder.ID, 403)
}
