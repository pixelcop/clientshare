package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/links"
)

func TestAuthHandlers_LoginMeAndLogout(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	loginReq := jsonRequest(http.MethodPost, "/api/auth/login", map[string]any{"email": env.adminUser.Email, "password": "admin-pass"})
	loginResp, err := env.app.Test(loginReq)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", loginResp.StatusCode)
	}
	var loginBody map[string]any
	if err := json.NewDecoder(loginResp.Body).Decode(&loginBody); err != nil {
		t.Fatalf("decode login failed: %v", err)
	}
	if loginBody["authenticated"] != true {
		t.Fatalf("expected authenticated response, got %#v", loginBody)
	}
	loginCookie := loginResp.Header.Get("Set-Cookie")
	if !strings.Contains(loginCookie, "jwt=") {
		t.Fatalf("expected login to set jwt cookie, got %q", loginCookie)
	}

	meReq := jsonAuthRequest(http.MethodGet, "/api/auth/me", env.adminToken, nil)
	meResp, err := env.app.Test(meReq)
	if err != nil {
		t.Fatalf("me request failed: %v", err)
	}
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", meResp.StatusCode)
	}
	setCookie := meResp.Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, "jwt=") {
		t.Fatalf("expected /auth/me to install jwt cookie, got %q", setCookie)
	}
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Fatalf("expected HttpOnly cookie, got %q", setCookie)
	}
	if !strings.Contains(strings.ToLower(setCookie), "secure") {
		t.Fatalf("expected Secure cookie, got %q", setCookie)
	}
	if !strings.Contains(setCookie, "SameSite=Lax") {
		t.Fatalf("expected SameSite=Lax cookie, got %q", setCookie)
	}

	logoutReq := jsonRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutResp, err := env.app.Test(logoutReq)
	if err != nil {
		t.Fatalf("logout request failed: %v", err)
	}
	if logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", logoutResp.StatusCode)
	}
	clearCookie := logoutResp.Header.Get("Set-Cookie")
	if !strings.Contains(clearCookie, "jwt=") {
		t.Fatalf("expected logout to clear jwt cookie, got %q", clearCookie)
	}
}

func TestAuthHandlers_MeRequiresAuth(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	req := jsonRequest(http.MethodGet, "/api/auth/me", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuthHandlers_RegisterForgotAndReset(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	invalidRegister := jsonRequest(http.MethodPost, "/api/auth/register", map[string]any{
		"token":    "bad-token",
		"email":    "new-client@test.local",
		"name":     "New Client",
		"password": "abc12345",
	})
	invalidRegisterResp, err := env.app.Test(invalidRegister)
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	if invalidRegisterResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", invalidRegisterResp.StatusCode)
	}

	secureToken, err := links.GenerateSecureToken(env.signingKey)
	if err != nil {
		t.Fatalf("failed generating secure token: %v", err)
	}
	if err := env.db.Create(&models.SecureLink{ClientID: env.client.ID, Token: secureToken, ExpiresAt: time.Now().Add(time.Hour), AccessType: "view", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("failed creating secure link: %v", err)
	}

	registerReq := jsonRequest(http.MethodPost, "/api/auth/register", map[string]any{
		"token":    secureToken,
		"email":    "new-client@test.local",
		"name":     "New Client",
		"password": "abc12345",
	})
	registerResp, err := env.app.Test(registerReq)
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	if registerResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", registerResp.StatusCode)
	}
	var registerBody map[string]any
	if err := json.NewDecoder(registerResp.Body).Decode(&registerBody); err != nil {
		t.Fatalf("decode register failed: %v", err)
	}
	if _, ok := registerBody["token"]; ok {
		t.Fatalf("expected register response without token, got %#v", registerBody)
	}

	forgotMissingEmail := jsonRequest(http.MethodPost, "/api/auth/forgot-password", map[string]any{})
	forgotMissingEmailResp, err := env.app.Test(forgotMissingEmail)
	if err != nil {
		t.Fatalf("forgot request failed: %v", err)
	}
	if forgotMissingEmailResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", forgotMissingEmailResp.StatusCode)
	}

	forgotReq := jsonRequest(http.MethodPost, "/api/auth/forgot-password", map[string]any{"email": env.clientUser.Email})
	forgotResp, err := env.app.Test(forgotReq)
	if err != nil {
		t.Fatalf("forgot request failed: %v", err)
	}
	if forgotResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", forgotResp.StatusCode)
	}
	if len(env.queue.items) == 0 {
		t.Fatalf("expected password reset email queued")
	}

	resetMissing := jsonRequest(http.MethodPost, "/api/auth/reset-password", map[string]any{"token": "", "password": ""})
	resetMissingResp, err := env.app.Test(resetMissing)
	if err != nil {
		t.Fatalf("reset missing request failed: %v", err)
	}
	if resetMissingResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resetMissingResp.StatusCode)
	}

	resetInvalid := jsonRequest(http.MethodPost, "/api/auth/reset-password", map[string]any{"token": "invalid", "password": "new-pass"})
	resetInvalidResp, err := env.app.Test(resetInvalid)
	if err != nil {
		t.Fatalf("reset invalid request failed: %v", err)
	}
	if resetInvalidResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resetInvalidResp.StatusCode)
	}

	rawToken := "raw-reset-token"
	record := models.PasswordResetToken{
		UserID:    env.clientUser.ID,
		TokenHash: resetTokenHash("reset-secret", rawToken),
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
	if err := env.db.Create(&record).Error; err != nil {
		t.Fatalf("failed creating password reset token: %v", err)
	}
	resetReq := jsonRequest(http.MethodPost, "/api/auth/reset-password", map[string]any{"token": rawToken, "password": "new-client-pass"})
	resetResp, err := env.app.Test(resetReq)
	if err != nil {
		t.Fatalf("reset request failed: %v", err)
	}
	if resetResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resetResp.StatusCode)
	}

	reloginReq := jsonRequest(http.MethodPost, "/api/auth/login", map[string]any{"email": env.clientUser.Email, "password": "new-client-pass"})
	reloginResp, err := env.app.Test(reloginReq)
	if err != nil {
		t.Fatalf("relogin request failed: %v", err)
	}
	if reloginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", reloginResp.StatusCode)
	}
}

func TestAuthHandlers_AcceptInvite(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	createReq := jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{
		"email":      "invited-user@test.local",
		"name":       "Invited User",
		"role":       "client",
		"client_ids": []string{env.client.ID},
	})
	createResp, err := env.app.Test(createReq)
	if err != nil {
		t.Fatalf("create invited user request failed: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	if len(env.queue.items) == 0 {
		t.Fatalf("expected invite email queued")
	}
	inviteBody := env.queue.items[len(env.queue.items)-1].Body
	start := "#token="
	idx := strings.Index(inviteBody, start)
	if idx == -1 {
		t.Fatalf("expected invite body to include token")
	}
	tokenSection := inviteBody[idx+len(start):]
	token := strings.Fields(tokenSection)[0]
	if strings.TrimSpace(token) == "" {
		t.Fatalf("expected non-empty invite token")
	}

	acceptReq := jsonRequest(http.MethodPost, "/api/auth/accept-invite", map[string]any{
		"token":    token,
		"password": "invite-pass-123",
	})
	acceptResp, err := env.app.Test(acceptReq)
	if err != nil {
		t.Fatalf("accept invite request failed: %v", err)
	}
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", acceptResp.StatusCode)
	}
	acceptCookie := acceptResp.Header.Get("Set-Cookie")
	if !strings.Contains(acceptCookie, "jwt=") {
		t.Fatalf("expected accept invite to set jwt cookie, got %q", acceptCookie)
	}
	var acceptBody map[string]any
	if err := json.NewDecoder(acceptResp.Body).Decode(&acceptBody); err != nil {
		t.Fatalf("decode accept invite failed: %v", err)
	}
	if _, ok := acceptBody["token"]; ok {
		t.Fatalf("expected accept invite response without token, got %#v", acceptBody)
	}

	reloginReq := jsonRequest(http.MethodPost, "/api/auth/login", map[string]any{"email": "invited-user@test.local", "password": "invite-pass-123"})
	reloginResp, err := env.app.Test(reloginReq)
	if err != nil {
		t.Fatalf("relogin request failed: %v", err)
	}
	if reloginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", reloginResp.StatusCode)
	}

	reuseReq := jsonRequest(http.MethodPost, "/api/auth/accept-invite", map[string]any{
		"token":    token,
		"password": "newer-pass-123",
	})
	reuseResp, err := env.app.Test(reuseReq)
	if err != nil {
		t.Fatalf("reuse invite request failed: %v", err)
	}
	if reuseResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", reuseResp.StatusCode)
	}
}
