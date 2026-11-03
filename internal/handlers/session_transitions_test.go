package handlers_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/links"
	"github.com/stretchr/testify/require"
)

func TestRecipientSwitchRequiresCurrentSessionConfirmation(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	store := &auth.SessionStore{DB: env.db}
	identity, err := store.Validate(t.Context(), env.adminToken, auth.PortalAudience, env.client.TenantID)
	require.NoError(t, err)
	token, err := links.GenerateSecureToken(env.signingKey)
	require.NoError(t, err)
	link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, Token: token, AccessType: "view", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, env.db.Create(&link).Error)
	path := "/links/" + token
	response, err := env.app.Test(jsonAuthRequest(http.MethodGet, path, env.adminToken, nil))
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	require.NoError(t, err)
	require.Contains(t, string(body), "Open this link as a recipient?")
	_, err = store.Validate(t.Context(), env.adminToken, auth.PortalAudience, env.client.TenantID)
	require.NoError(t, err, "GET must not replace the current identity")
	for _, id := range []string{"stale-session", identity.SessionID} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(url.Values{"session_id": {id}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: env.adminToken})
		response, err = env.app.Test(req)
		require.NoError(t, err)
		response.Body.Close()
		require.Equal(t, 200, response.StatusCode)
		if id == "stale-session" {
			_, err = store.Validate(t.Context(), env.adminToken, auth.PortalAudience, env.client.TenantID)
			require.NoError(t, err)
			continue
		}
		var raw string
		for _, cookie := range response.Cookies() {
			if cookie.Name == auth.SessionCookie {
				raw = cookie.Value
			}
		}
		recipient, err := store.Validate(t.Context(), raw, auth.PortalAudience, env.client.TenantID)
		require.NoError(t, err)
		require.Equal(t, link.ID, recipient.LinkID)
		_, err = store.Validate(t.Context(), env.adminToken, auth.PortalAudience, env.client.TenantID)
		require.ErrorIs(t, err, auth.ErrSessionInvalid)
	}
}
