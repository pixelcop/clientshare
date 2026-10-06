package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/stretchr/testify/require"
)

func TestSignupPasskeyPortalRedirectAndLastCredentialProtection(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	require.NoError(t, env.db.Model(&env.adminUser).Update("password_hash", "").Error)
	data, err := json.Marshal(webauthn.Credential{ID: []byte("signup-key"), PublicKey: []byte("key")})
	require.NoError(t, err)
	credential := models.PasskeyCredential{TenantID: env.adminUser.TenantID, UserID: env.adminUser.ID, CredentialID: "c2lnbnVwLWtleQ", CredentialIDHash: "signup-key-hash", CredentialData: data, RPID: "clientshare.app", Name: "Signup passkey"}
	require.NoError(t, env.db.Create(&credential).Error)
	h := handlers.NewAuthHandler(env.db, env.signingKey, env.queue, handlersTestPublicBaseURL, "reset-secret", "invite-secret", 30*time.Minute, 72*time.Hour, "https://clientshare.app", nil, services.NewTenantSettingsService(env.db))
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		tenantctx.SetLocal(c, tenantctx.RequestContext{ID: env.adminUser.TenantID, Resolution: "test"})
		return c.Next()
	})
	app.Post("/options", h.BeginPasskeyLoginHandler)
	response, err := app.Test(jsonRequest(http.MethodPost, "/options", map[string]any{"email": env.adminUser.Email}))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var body struct {
		Passkey     bool   `json:"passkey"`
		RedirectURL string `json:"redirect_url"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.True(t, body.Passkey)
	require.Equal(t, "https://clientshare.app/login#email="+url.QueryEscape(env.adminUser.Email), body.RedirectURL)
	response, err = env.app.Test(jsonAuthRequest(http.MethodDelete, "/api/auth/passkeys/"+credential.ID, env.adminToken, nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusConflict, response.StatusCode)
	require.NoError(t, env.db.First(&credential, "id = ?", credential.ID).Error)
	response, err = env.app.Test(jsonAuthRequest(http.MethodGet, "/api/users?page=1&page_size=10&role=admin&invite_status=accepted", env.adminToken, nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var users paginatedUsersResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&users))
	require.Len(t, users.Items, 1)
	require.Equal(t, env.adminUser.ID, users.Items[0].ID)
	require.True(t, users.Items[0].InviteAccepted)
}
