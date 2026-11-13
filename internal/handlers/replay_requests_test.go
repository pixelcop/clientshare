package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/stretchr/testify/require"
)

func TestReplayConfigAnonymousIdentityAndInternalAuthorization(t *testing.T) {
	env := setupSharedHandlersEnv(t)
	require.NoError(t, env.db.AutoMigrate(&models.ReplayRequest{}))
	handler := handlers.NewReplayRequestHandler(env.db)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		tenantctx.SetLocal(c, tenantctx.RequestContext{ID: tenantctx.DefaultTenantID})
		return c.Next()
	})
	handler.RegisterPublic(app.Group("/api"))
	handler.RegisterInternal(app.Group("/internal", middleware.InternalAuthRequired("secret")))
	for _, token := range []string{"", "secret"} {
		req := httptest.NewRequest("GET", "/internal/replay-targets?kind=users&q="+env.adminUser.ID, nil)
		req.Header.Set("X-Internal-Token", token)
		response, err := app.Test(req)
		require.NoError(t, err)
		if token == "" {
			require.Equal(t, 401, response.StatusCode)
		} else {
			require.Equal(t, 200, response.StatusCode)
			var body struct {
				Options []services.ReplayTargetOption `json:"options"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			require.Len(t, body.Options, 1)
			require.Equal(t, env.adminUser.ID, body.Options[0].ID)
		}
		response.Body.Close()
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		path := "/internal/replay-requests"
		if method == "DELETE" {
			path += "/id"
		}
		response, err := app.Test(httptest.NewRequest(method, path, nil))
		require.NoError(t, err)
		require.Equal(t, 401, response.StatusCode)
		response.Body.Close()
	}

	// User-controlled query/header identifiers cannot turn on user targeting.
	rule := models.ReplayRequest{Label: "Private reason", Targets: []models.ReplayTarget{{UserEmail: env.adminUser.Email}}, ExpiresAt: time.Now().UTC().Add(time.Hour)}
	require.NoError(t, env.db.Create(&rule).Error)
	check := func(cookie string, enabled bool) {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/config?user_email="+env.adminUser.Email, nil)
		req.Header.Set("X-User-ID", env.adminUser.ID)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: cookie})
		}
		response, err := app.Test(req)
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, 200, response.StatusCode)
		require.Contains(t, response.Header.Get("Cache-Control"), "no-store")
		var body struct {
			Debug struct {
				Replay struct {
					Enabled bool `json:"enabled"`
				} `json:"replay"`
			} `json:"debug"`
		}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
		require.Equal(t, enabled, body.Debug.Replay.Enabled)
	}
	check("", false)
	check("invalid", false)
	check(env.adminToken, true)
	check(env.clientToken, false)

	request := httptest.NewRequest("POST", "/internal/replay-requests", strings.NewReader(`{"label":"Anonymous browser","targets":[{"hostname":"example.com"}],"start_on_failure":true,"expires_at":"`+time.Now().UTC().Add(time.Hour).Format(time.RFC3339)+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Token", "secret")
	response, err := app.Test(request)
	require.NoError(t, err)
	require.Equal(t, 201, response.StatusCode)
	var created models.ReplayRequest
	require.NoError(t, json.NewDecoder(response.Body).Decode(&created))
	response.Body.Close()
	req := httptest.NewRequest("GET", "https://example.com/api/config", nil)
	response, err = app.Test(req)
	require.NoError(t, err)
	var config map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&config))
	response.Body.Close()
	encoded, err := json.Marshal(config)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"enabled":true`)
	require.Contains(t, string(encoded), `"start_on_failure":true`)
	require.NotContains(t, string(encoded), "Private reason")
	require.NotContains(t, string(encoded), env.adminUser.Email)
	require.NotContains(t, string(encoded), "targets")
	request = httptest.NewRequest("DELETE", "/internal/replay-requests/"+created.ID, nil)
	request.Header.Set("X-Internal-Token", "secret")
	response, err = app.Test(request)
	require.NoError(t, err)
	require.Equal(t, 204, response.StatusCode)
	response.Body.Close()
	check("", false)
}
