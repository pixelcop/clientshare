package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	apphandlers "github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/storage"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/stretchr/testify/require"
)

type unavailableQueue struct{ email.EmailQueue }

func (unavailableQueue) Queue(email.Email, *time.Time) (*models.QueuedEmail, error) {
	return nil, errors.New("queue failure")
}

func TestLinkCreationRetryPreservesTokenAndExactExpiry(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	expiry := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Millisecond)
	payload := map[string]any{"access_type": "read", "expiry_days": 1, "expires_at": expiry}
	var first models.SecureLink
	for i := 0; i < 3; i++ {
		req := jsonAuthRequest(http.MethodPost, "/api/clients/"+env.client.ID+"/links", env.adminToken, payload)
		req.Header.Set("Idempotency-Key", "creation-retry-key")
		resp, err := env.app.Test(req)
		require.NoError(t, err)
		require.Equal(t, 201, resp.StatusCode)
		var link models.SecureLink
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&link))
		resp.Body.Close()
		if i == 0 {
			first = link
		} else {
			require.Equal(t, first.ID, link.ID)
			require.Equal(t, first.Token, link.Token)
		}
		require.True(t, expiry.Equal(link.ExpiresAt))
	}
	payload["access_type"] = "readwrite"
	req := jsonAuthRequest(http.MethodPost, "/api/clients/"+env.client.ID+"/links", env.adminToken, payload)
	req.Header.Set("Idempotency-Key", "creation-retry-key")
	resp, err := env.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 409, resp.StatusCode)
	resp.Body.Close()
	var count int64
	require.NoError(t, env.db.Model(&models.SecureLink{}).Where("creation_key IS NOT NULL").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestEmailExistingLinkRetriesAndTruthfulFailures(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, Token: "delivery-token", AccessType: "read", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, env.db.Create(&link).Error)
	queue := email.NewDBEmailQueue(env.db, nil, time.Hour)
	appFor := func(q email.EmailQueue, role string) *fiber.App {
		app := fiber.New()
		app.Use(func(c fiber.Ctx) error {
			tenantctx.SetLocal(c, tenantctx.RequestContext{ID: env.client.TenantID})
			c.Locals("role", role)
			c.Locals("user_id", env.adminUser.ID)
			return c.Next()
		})
		h := apphandlers.NewLinkHandler(env.db, env.signingKey, storage.NewLocalStorage(env.storageRoot), q, handlersTestPublicBaseURL, nil)
		app.Post("/api/links/:id/email", middleware.RoleRequired("admin"), h.EmailSecureLink)
		return app
	}
	request := func(app *fiber.App, key, recipient string) (int, map[string]any) {
		req := jsonRequest(http.MethodPost, "/api/links/"+link.ID+"/email", map[string]any{"email": recipient})
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		resp, err := app.Test(req)
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		resp.Body.Close()
		return resp.StatusCode, result
	}
	for i := 0; i < 3; i++ {
		// New queue/handler instances exercise persistence across server restarts.
		status, result := request(appFor(email.NewDBEmailQueue(env.db, nil, time.Hour), "admin"), "email-retry-key", "recipient@example.test")
		require.Equal(t, 202, status)
		require.Equal(t, "queued", result["status"])
	}
	var messages []models.QueuedEmail
	require.NoError(t, env.db.Where("secure_link_id = ?", link.ID).Find(&messages).Error)
	require.Len(t, messages, 1)
	require.Nil(t, messages[0].SentAt)
	require.Contains(t, messages[0].Body, "/links/"+link.Token)
	status, _ := request(appFor(queue, "admin"), "email-retry-key", "other@example.test")
	require.Equal(t, 409, status)
	status, _ = request(appFor(queue, "admin"), "", "recipient@example.test")
	require.Equal(t, 400, status)
	status, _ = request(appFor(queue, "manager"), "new-request-key", "recipient@example.test")
	require.Equal(t, 403, status)
	status, _ = request(appFor(queue, "link"), "new-request-key", "recipient@example.test")
	require.Equal(t, 403, status)
	status, result := request(appFor(nil, "admin"), "unavailable-key", "recipient@example.test")
	require.Equal(t, 503, status)
	require.Equal(t, "unavailable", result["status"])
	status, result = request(appFor(unavailableQueue{}, "admin"), "failed-request-key", "recipient@example.test")
	require.Equal(t, 503, status)
	require.Equal(t, "failed", result["status"])
	status, _ = request(appFor(queue, "admin"), "invalid-request-key", "not an email")
	require.Equal(t, 400, status)
	require.NoError(t, env.db.Model(&link).Update("tenant_id", "other-tenant").Error)
	status, _ = request(appFor(queue, "admin"), "tenant-request-key", "recipient@example.test")
	require.Equal(t, 404, status)
}

func TestRevocationRetainsHistoryAndDeniesOpenSessions(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/clients/"+env.client.ID+"/links", env.adminToken, map[string]any{"access_type": "read", "expiry_days": 1}))
	require.NoError(t, err)
	var link models.SecureLink
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&link))
	resp.Body.Close()
	resp, err = env.app.Test(jsonRequest(http.MethodGet, "/links/"+link.Token, nil))
	require.NoError(t, err)
	cookies := resp.Cookies()
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodDelete, "/api/links/"+link.ID, env.adminToken, nil))
	require.NoError(t, err)
	require.Equal(t, 204, resp.StatusCode)
	resp.Body.Close()
	require.NoError(t, env.db.First(&link, "id = ?", link.ID).Error)
	require.NotNil(t, link.RevokedAt)
	req := jsonRequest(http.MethodGet, "/api/folders/"+*env.client.RootFolderID, nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp, err = env.app.Test(req)
	require.NoError(t, err)
	require.Contains(t, []int{401, 403}, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonRequest(http.MethodGet, "/links/"+link.Token, nil))
	require.NoError(t, err)
	require.Equal(t, 403, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, "/api/clients/"+env.client.ID+"/links", env.adminToken, nil))
	require.NoError(t, err)
	var history []models.SecureLink
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&history))
	resp.Body.Close()
	for _, row := range history {
		if row.ID == link.ID {
			require.NotNil(t, row.RevokedAt)
			require.Empty(t, row.Token)
			require.Empty(t, row.LinkURL)
			return
		}
	}
	t.Fatal("revoked history missing")
}

func TestScopedLinkCannotUseLegacyClientEndpoints(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/clients/"+env.client.ID+"/links", env.adminToken, map[string]any{"access_type": "read", "expiry_days": 1}))
	require.NoError(t, err)
	var link models.SecureLink
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&link))
	resp.Body.Close()
	require.NoError(t, env.db.Model(&link).Update("file_id", env.client.RootFolderID).Error)
	link.FileID = env.client.RootFolderID
	require.ErrorIs(t, services.PersistSecureLink(t.Context(), env.db, &link), services.ErrScopedSharingUnavailable)
}
