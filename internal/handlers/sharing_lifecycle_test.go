package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	apphandlers "github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/stretchr/testify/require"
)

func lifecycleApp(env *handlersTestEnv, queue emailpkg.EmailQueue) *fiber.App {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		tenantctx.SetLocal(c, tenantctx.RequestContext{ID: env.client.TenantID})
		c.Locals("session_store", &auth.SessionStore{DB: env.db})
		return c.Next()
	})
	api := app.Group("/api")
	apphandlers.RegisterSharingReadRoutes(api, env.db, handlersTestPublicBaseURL)
	apphandlers.RegisterSharingLifecycleRoutes(api, env.db, "invite-secret", env.signingKey, queue, handlersTestPublicBaseURL, 72*time.Hour)
	apphandlers.RegisterLinkRoutes(app, env.db, env.signingKey, nil, queue, handlersTestPublicBaseURL, nil)
	return app
}

func TestSharingLifecycleHTTPEnforcesExactScopeCeilingsVersionsAndIdentity(t *testing.T) {
	env := setupFileTestEnv(t)
	app := lifecycleApp(env, nil)
	file := createFileRecord(t, env, env.rootFolder, "shared.txt", "content")
	post := func(path, token string, body any, status int) []byte {
		t.Helper()
		req := jsonAuthRequest(http.MethodPost, path, token, body)
		req.Header.Set("Idempotency-Key", "lifecycle-test-key")
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, status, resp.StatusCode, path)
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return data
	}
	path := "/api/files/" + file.ID + "/grants"
	input := map[string]any{"user_id": env.otherClientUser.ID, "capabilities": models.AccessCapabilities{CanRead: true, CanShare: true}}
	post(path, env.linkToken, input, 404)
	post(path, env.otherToken, input, 404)
	data := post(path, env.clientToken, input, 201)
	var grant models.ItemGrant
	require.NoError(t, json.Unmarshal(data, &grant))
	replay := post(path, env.clientToken, input, 201)
	var repeated models.ItemGrant
	require.NoError(t, json.Unmarshal(replay, &repeated))
	require.Equal(t, grant.ID, repeated.ID)
	post(path, env.otherToken, map[string]any{"user_id": env.managerUser.ID, "capabilities": models.AccessCapabilities{CanRead: true, CanDelete: true}}, 403)
	post("/api/files/"+env.rootFolder.ID+"/grants", env.otherToken, input, 404)
	invitationPath := "/api/files/" + file.ID + "/invitations"
	data = post(invitationPath, env.otherToken, map[string]any{"email": "new-recipient@example.test", "capabilities": models.AccessCapabilities{CanRead: true}}, 201)
	var invite models.AccessInvitation
	require.NoError(t, json.Unmarshal(data, &invite))
	require.NotContains(t, string(data), "token")
	post("/api/invitations/"+invite.ID+"/email", env.otherToken, nil, 503)
	var identities int64
	require.NoError(t, env.db.Model(&models.User{}).Where("email = 'new-recipient@example.test'").Count(&identities).Error)
	require.Zero(t, identities, "disabled delivery must not mint/return an identity credential")
	post("/api/clients/"+env.client.ID+"/invitations", env.otherToken, map[string]any{"email": "new-member@example.test", "capabilities": models.AccessCapabilities{CanRead: true}}, 404)
	// Versioned updates cannot overwrite a concurrent change or exceed the
	// current grantee's ceiling, even through a still-open session.
	resp, err := app.Test(jsonAuthRequest(http.MethodPatch, path+"/"+grant.ID, env.otherToken, map[string]any{"version": grant.Version, "capabilities": models.AccessCapabilities{CanRead: true}}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	resp, err = app.Test(jsonAuthRequest(http.MethodDelete, path+"/"+grant.ID, env.clientToken, map[string]any{"version": grant.Version}))
	require.NoError(t, err)
	require.Equal(t, 409, resp.StatusCode)
	resp.Body.Close()
	resp, err = app.Test(jsonAuthRequest(http.MethodDelete, path+"/"+grant.ID, env.clientToken, map[string]any{"version": grant.Version + 1}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	post(invitationPath, env.otherToken, map[string]any{"email": "another@example.test", "capabilities": models.AccessCapabilities{CanRead: true}}, 404)
}

func TestInvitationHTTPDeliveryCredentialsAreEmailOnlyAndQueueInspectionCannotRecoverThem(t *testing.T) {
	env := setupFileTestEnv(t)
	queue := emailpkg.NewDBEmailQueue(env.db, nil, time.Hour)
	app := lifecycleApp(env, queue)
	file := createFileRecord(t, env, env.rootFolder, "shared.txt", "content")
	call := func(path string, body any, status int) []byte {
		t.Helper()
		req := jsonAuthRequest(http.MethodPost, path, env.adminToken, body)
		req.Header.Set("Idempotency-Key", "invitation-delivery-key")
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, status, resp.StatusCode)
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return data
	}
	data := call("/api/files/"+file.ID+"/invitations", map[string]any{"email": "brand-new@example.test", "capabilities": models.AccessCapabilities{CanRead: true}}, 201)
	var invite models.AccessInvitation
	require.NoError(t, json.Unmarshal(data, &invite))
	data = call("/api/invitations/"+invite.ID+"/email", nil, 202)
	require.NotContains(t, string(data), "token")
	data = call("/api/invitations/"+invite.ID+"/email", nil, 202)
	require.Contains(t, string(data), "queued")
	var jobs []models.QueuedEmail
	require.NoError(t, env.db.Where("access_invitation_id = ?", invite.ID).Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.NotNil(t, jobs[0].IdentityInviteID)
	require.Contains(t, jobs[0].Body, "/accept-invite#token=")
	// Add the normal queue routes around the durable queue, with the same real
	// sessions. Neither the inviter nor another admin can inspect credentials.
	apphandlers.RegisterEmailQueueRoutes(app.Group("/api", middleware.AuthRequired), env.db, queue)
	resp, err := app.Test(jsonAuthRequest(http.MethodGet, "/api/email/queue", env.adminToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	data, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	require.NotContains(t, string(data), "brand-new@example.test")
	require.NotContains(t, string(data), "token=")
	// Only the recipient's emailed token can set up their account and grant
	// the item; this does not create a client membership.
	start := strings.Index(jobs[0].Body, "/accept-invite#token=")
	require.GreaterOrEqual(t, start, 0)
	token := strings.Fields(jobs[0].Body[start+len("/accept-invite#token="):])[0]
	token = strings.Trim(token, "<>\"()")
	resp, err = env.app.Test(jsonRequest(http.MethodPost, "/api/auth/accept-invite", map[string]any{"token": token, "password": "recipient-password"}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	var user models.User
	require.NoError(t, env.db.First(&user, "email = 'brand-new@example.test'").Error)
	var count int64
	require.NoError(t, env.db.Model(&models.UserClient{}).Where("user_id = ?", user.ID).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, env.db.Model(&models.ItemGrant{}).Where("user_id = ? AND file_id = ?", user.ID, file.ID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
