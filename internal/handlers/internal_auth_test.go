package handlers_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
)

func TestInternalAuthHandlerHostedLogin(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	app := fiber.New()
	secret := "12345678901234567890123456789012"
	hostedLoginSvc := services.NewHostedLoginService(env.db, handlersTestPublicBaseURL, "reset-secret")
	handlers.RegisterInternalAuthRoutes(app.Group("/internal", middleware.InternalAuthRequired(secret)), hostedLoginSvc)

	req := jsonRequest(http.MethodPost, "/internal/auth/hosted-login", map[string]any{
		"email":    env.adminUser.Email,
		"password": "admin-pass",
	})
	req.Header.Set("X-Internal-Token", secret)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body services.HostedLoginResult
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.HandoffCode == "" {
		t.Fatal("handoff_code = empty, want issued code")
	}
	if body.RedirectURL != "https://frontend.local" {
		t.Fatalf("redirect_url = %q, want primary tenant domain", body.RedirectURL)
	}
	if body.TenantID != env.adminUser.TenantID {
		t.Fatalf("tenant_id = %q, want %q", body.TenantID, env.adminUser.TenantID)
	}
	if body.TenantSlug != tenantctx.DefaultTenantSlug {
		t.Fatalf("tenant_slug = %q, want %q", body.TenantSlug, tenantctx.DefaultTenantSlug)
	}
	if body.UserID != env.adminUser.ID {
		t.Fatalf("user_id = %q, want %q", body.UserID, env.adminUser.ID)
	}
	if body.Role != env.adminUser.Role {
		t.Fatalf("role = %q, want %q", body.Role, env.adminUser.Role)
	}
	var handoff models.HostedLoginHandoff
	if err := env.db.Where("tenant_id = ? AND user_id = ?", env.adminUser.TenantID, env.adminUser.ID).First(&handoff).Error; err != nil {
		t.Fatalf("expected stored hosted login handoff: %v", err)
	}
}

func TestInternalAuthHandlerHostedPasskeyOptions(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	credential := webauthn.Credential{ID: []byte("hosted-passkey-credential"), PublicKey: []byte("test-public-key")}
	credentialData, err := json.Marshal(credential)
	if err != nil {
		t.Fatalf("marshal passkey credential: %v", err)
	}
	if err := env.db.Create(&models.PasskeyCredential{
		TenantID:         env.adminUser.TenantID,
		UserID:           env.adminUser.ID,
		CredentialID:     base64.RawURLEncoding.EncodeToString(credential.ID),
		CredentialIDHash: "hosted-passkey-credential-hash",
		CredentialData:   credentialData,
		RPID:             handlersTestPublicDomain,
		Name:             "Laptop",
		CreatedAt:        time.Now(),
	}).Error; err != nil {
		t.Fatalf("create passkey credential: %v", err)
	}

	app := fiber.New()
	secret := "12345678901234567890123456789012"
	hostedLoginSvc := services.NewHostedLoginService(env.db, handlersTestPublicBaseURL, "reset-secret")
	passkeyAuth := handlers.NewAuthHandler(
		env.db,
		env.signingKey,
		env.queue,
		handlersTestPublicBaseURL,
		"reset-secret",
		"invite-secret",
		30*time.Minute,
		72*time.Hour,
		"https://clientshare.app",
		hostedLoginSvc,
		services.NewTenantSettingsService(env.db),
	)
	handlers.RegisterInternalAuthRoutes(app.Group("/internal", middleware.InternalAuthRequired(secret)), hostedLoginSvc, passkeyAuth)

	req := jsonRequest(http.MethodPost, "/internal/auth/hosted-login/passkey/options", map[string]any{
		"email": env.adminUser.Email,
	})
	req.Header.Set("X-Internal-Token", secret)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Passkey bool `json:"passkey"`
		Options []struct {
			ChallengeID string `json:"challenge_id"`
			PublicKey   any    `json:"public_key"`
		} `json:"options"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !body.Passkey || len(body.Options) != 1 || body.Options[0].ChallengeID == "" || body.Options[0].PublicKey == nil {
		t.Fatalf("expected hosted passkey options, got %#v", body)
	}
	var challenge models.WebAuthnChallenge
	if err := env.db.Where("id = ?", body.Options[0].ChallengeID).First(&challenge).Error; err != nil {
		t.Fatalf("load hosted passkey challenge: %v", err)
	}
	if challenge.Purpose != "hosted_login" || challenge.RPID != handlersTestPublicDomain {
		t.Fatalf("unexpected hosted challenge: %#v", challenge)
	}
}

func TestInternalAuthHandlerHostedLoginRejectsDuplicateEmails(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	passwordHash, err := auth.HashPassword("duplicate-pass")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	otherTenant := &models.Tenant{Slug: "other-tenant", Name: "Other Tenant", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := env.db.Create(otherTenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := env.db.Create(&models.TenantEntitlement{TenantID: otherTenant.ID, Status: tenantctx.StatusActive, PlanCode: "growth"}).Error; err != nil {
		t.Fatalf("create entitlement: %v", err)
	}
	if err := env.db.Create(&models.User{TenantID: otherTenant.ID, Email: env.adminUser.Email, PasswordHash: passwordHash, Role: "admin", Name: "Duplicate", CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create duplicate user: %v", err)
	}

	app := fiber.New()
	secret := "12345678901234567890123456789012"
	handlers.RegisterInternalAuthRoutes(app.Group("/internal", middleware.InternalAuthRequired(secret)), services.NewHostedLoginService(env.db, handlersTestPublicBaseURL, "reset-secret"))

	req := jsonRequest(http.MethodPost, "/internal/auth/hosted-login", map[string]any{
		"email":    env.adminUser.Email,
		"password": "admin-pass",
	})
	req.Header.Set("X-Internal-Token", secret)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}
