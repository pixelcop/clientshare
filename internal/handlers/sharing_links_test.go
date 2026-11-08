package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"github.com/stretchr/testify/require"
)

type sharingMailSender struct{ messages []emailpkg.Email }

func (s *sharingMailSender) Send(message emailpkg.Email) error {
	s.messages = append(s.messages, message)
	return nil
}

func TestPublicScopedLinkLandingAndInspectionRedactPrivateContext(t *testing.T) {
	for _, kind := range []string{"file", "folder"} {
		t.Run(kind, func(t *testing.T) {
			env := setupFileTestEnv(t)
			target := &models.File{TenantID: env.client.TenantID, ClientID: env.client.ID, FolderID: &env.rootFolder.ID, Type: kind, Filename: "Shared target", Path: env.rootFolder.Path + "/Shared target"}
			require.NoError(t, env.db.Create(target).Error)
			service := &services.SharingService{DB: env.db, LinkSigningKey: env.signingKey}
			link, err := service.CreateLink(t.Context(), services.AccessRequest{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: target.ID, Actor: services.AccessActor{UserID: env.adminUser.ID}}, services.LinkInput{AccessType: "read", ExpiryDays: 7, Email: "private-contact@example.test"}, "public-item-link")
			require.NoError(t, err)
			resp, err := env.app.Test(jsonRequest(http.MethodGet, "/links/"+link.Token, nil))
			require.NoError(t, err)
			require.Equal(t, 200, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)
			redirect := "/files/" + target.ID
			if kind == "folder" {
				redirect = "/folder/" + target.ID
			}
			require.Contains(t, string(body), redirect)
			require.NotContains(t, string(body), env.rootFolder.ID)
			cookie := ""
			for _, item := range resp.Cookies() {
				if item.Name == "clientshare_session" {
					cookie = item.Value
				}
			}
			require.NotEmpty(t, cookie)
			resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, "/api/link/"+link.Token, cookie, nil))
			require.NoError(t, err)
			require.Equal(t, 200, resp.StatusCode)
			body, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			require.NoError(t, err)
			for _, secret := range []string{env.rootFolder.ID, env.rootFolder.Path, env.client.Name, link.CreatedBy, link.Email, "created_by", "last_accessed"} {
				require.NotContains(t, string(body), secret)
			}
			var result struct {
				Target models.File `json:"target"`
			}
			require.NoError(t, json.Unmarshal(body, &result))
			require.Equal(t, target.ID, result.Target.ID)
			require.Nil(t, result.Target.FolderID)
			require.Equal(t, target.Filename, result.Target.Path)
			require.NoError(t, env.db.Model(link).Update("revoked_at", time.Now()).Error)
			resp, err = env.app.Test(jsonRequest(http.MethodGet, "/links/"+link.Token, nil))
			require.NoError(t, err)
			require.Equal(t, 403, resp.StatusCode)
			resp.Body.Close()
		})
	}
}

func TestScopedLinkLifecycleHTTPUsesExactTargetAndRevalidatesMail(t *testing.T) {
	env := setupFileTestEnv(t)
	sender := &sharingMailSender{}
	queue := emailpkg.NewDBEmailQueue(env.db, sender, time.Hour)
	app := lifecycleApp(env, queue)
	file := createFileRecord(t, env, env.rootFolder, "shared-file.txt", "content")
	call := func(method, path, token string, input any, status int) []byte {
		t.Helper()
		req := jsonAuthRequest(method, path, token, input)
		req.Header.Set("Idempotency-Key", "scoped-link-lifecycle")
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, status, resp.StatusCode, path)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return body
	}
	path := "/api/files/" + file.ID + "/links"
	input := map[string]any{"access_type": "read", "expiry_days": 7}
	call(http.MethodPost, path, env.linkToken, input, 404)
	call(http.MethodPost, path, env.otherToken, input, 404)
	data := call(http.MethodPost, path, env.clientToken, input, 201)
	var link models.SecureLink
	require.NoError(t, json.Unmarshal(data, &link))
	require.Equal(t, file.ID, *link.FileID)
	require.Contains(t, link.LinkURL, "/links/"+link.Token)
	data = call(http.MethodPost, path, env.clientToken, input, 201)
	var replay models.SecureLink
	require.NoError(t, json.Unmarshal(data, &replay))
	require.Equal(t, link.ID, replay.ID)
	call(http.MethodPost, path, env.clientToken, map[string]any{"access_type": "write", "expiry_days": 7}, 403)
	grant := models.ItemGrant{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: file.ID, UserID: env.otherClientUser.ID, AccessCapabilities: models.AccessCapabilities{CanRead: true, CanShare: true}}
	require.NoError(t, env.db.Create(&grant).Error)
	data = call(http.MethodGet, path, env.otherToken, nil, 200)
	require.Contains(t, string(data), link.Token)
	call(http.MethodGet, "/api/clients/"+env.client.ID+"/links", env.otherToken, nil, 404)
	call(http.MethodPost, "/api/links/"+link.ID+"/email", env.otherToken, map[string]any{"email": "reader@example.test"}, 202)
	call(http.MethodPost, "/api/links/"+link.ID+"/email", env.otherToken, map[string]any{"email": "reader@example.test"}, 202)
	var jobs []models.QueuedEmail
	require.NoError(t, env.db.Where("secure_link_id = ?", link.ID).Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.NoError(t, queue.SendNow(env.client.TenantID, jobs[0].ID))
	require.Len(t, sender.messages, 1)
	require.Contains(t, sender.messages[0].Body, file.Filename)
	require.NotContains(t, sender.messages[0].Body, env.client.Name)
	call(http.MethodDelete, "/api/links/"+link.ID, env.otherToken, nil, 204)
	require.NoError(t, env.db.First(&link, "id = ?", link.ID).Error)
	require.NotNil(t, link.RevokedAt)
	call(http.MethodPost, "/api/links/"+link.ID+"/email", env.clientToken, map[string]any{"email": "reader@example.test"}, 404)
}

func TestClientLinkManagementAndQueuedCredentialsRespectCurrentCeiling(t *testing.T) {
	env := setupFileTestEnv(t)
	resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/clients/"+env.client.ID+"/links", env.clientToken, map[string]any{"access_type": "readwrite", "expiry_days": 7, "email": "recipient@example.test"}))
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)
	var link models.SecureLink
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&link))
	resp.Body.Close()
	require.NoError(t, env.db.Model(&models.UserClient{}).Where("user_id = ? AND client_id = ?", env.managerUser.ID, env.client.ID).Updates(map[string]any{"capabilities_set": true, "can_read": true, "can_share": true}).Error)
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, "/api/clients/"+env.client.ID+"/links", env.managerToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	require.NotContains(t, string(body), link.Token, "link listing cannot reveal a stronger upload credential")
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, "/api/email/queue", env.managerToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	body, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	require.NotContains(t, string(body), link.Token, "queue inspection cannot bypass the same ceiling")
	for _, path := range []string{"/api/clients/" + env.otherClient.ID + "/links", "/api/clients/" + env.otherClient.ID + "/collection?since=" + time.Now().UTC().Format(time.RFC3339)} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, path, env.managerToken, nil))
		require.NoError(t, err)
		require.Equal(t, 404, resp.StatusCode)
		resp.Body.Close()
	}
}
