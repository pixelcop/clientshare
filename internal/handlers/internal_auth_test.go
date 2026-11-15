package handlers_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
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

func TestAccountSessionHandoff(t *testing.T) {
	for _, scenario := range []string{"valid", "missing", "tampered", "portal-audience", "expired", "revoked", "suspended", "deleted-user"} {
		t.Run(scenario, func(t *testing.T) {
			env := setupOtherHandlersEnv(t)
			store := &auth.SessionStore{DB: env.db}
			var originalSessions int64
			if err := env.db.Model(&models.BrowserSession{}).Count(&originalSessions).Error; err != nil {
				t.Fatal(err)
			}
			audience := auth.AccountAudience
			if scenario == "portal-audience" {
				audience = auth.PortalAudience
			}
			token, err := store.Create(t.Context(), env.adminUser.TenantID, env.adminUser.ID, "", audience, "", "")
			if err != nil {
				t.Fatal(err)
			}
			var session models.BrowserSession
			if err := env.db.First(&session, "token_hash = ?", auth.SessionHash(token)).Error; err != nil {
				t.Fatal(err)
			}
			expectedStatus := 401
			switch scenario {
			case "valid":
				expectedStatus = 200
			case "missing":
				token = ""
			case "tampered":
				token += "x"
			case "expired":
				if err := env.db.Model(&session).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
					t.Fatal(err)
				}
			case "revoked":
				if err := store.Revoke(t.Context(), token, audience, true); err != nil {
					t.Fatal(err)
				}
			case "suspended":
				if err := env.db.Model(&models.TenantEntitlement{}).Where("tenant_id = ?", env.adminUser.TenantID).Update("status", "suspended").Error; err != nil {
					t.Fatal(err)
				}
				expectedStatus = 403
			case "deleted-user":
				if err := env.db.Delete(&env.adminUser).Error; err != nil {
					t.Fatal(err)
				}
			}
			svc := services.NewHostedLoginService(env.db, handlersTestPublicBaseURL, "handoff-secret")
			app := fiber.New()
			secret := "12345678901234567890123456789012"
			handlers.RegisterInternalAuthRoutes(app.Group("/internal", middleware.InternalAuthRequired(secret)), svc)
			req := jsonRequest(http.MethodPost, "/internal/auth/sessions/handoff", map[string]any{"token": token})
			req.Header.Set("X-Internal-Token", secret)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != expectedStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, expectedStatus)
			}
			if scenario != "valid" {
				return
			}
			var result services.HostedLoginResult
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result.HandoffCode == "" || result.AccountSession != "" || result.UserID != env.adminUser.ID || result.TenantID != env.adminUser.TenantID || result.RedirectURL != "https://frontend.local" {
				t.Fatalf("unexpected handoff: %+v", result)
			}
			var count int64
			if err := env.db.Model(&models.BrowserSession{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != originalSessions+1 {
				t.Fatalf("created an extra browser session: %d", count)
			}
			if _, err := svc.ConsumeHandoff("another-tenant", result.HandoffCode); !errors.Is(err, services.ErrHostedLoginHandoffInvalid) {
				t.Fatalf("cross-tenant handoff: %v", err)
			}
			identity, err := svc.ConsumeHandoff(env.adminUser.TenantID, result.HandoffCode)
			if err != nil {
				t.Fatal(err)
			}
			if identity.SessionGroupID != session.GroupID {
				t.Fatal("handoff lost existing session group")
			}
			if _, err := svc.ConsumeHandoff(env.adminUser.TenantID, result.HandoffCode); !errors.Is(err, services.ErrHostedLoginHandoffInvalid) {
				t.Fatalf("replayed handoff: %v", err)
			}
			for _, invalidate := range []string{"expiry", "revocation"} {
				fresh, err := svc.HandoffFromAccountSession(t.Context(), token)
				if err != nil {
					t.Fatal(err)
				}
				if invalidate == "expiry" {
					if err := env.db.Model(&models.HostedLoginHandoff{}).Where("session_group_id = ?", session.GroupID).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
						t.Fatal(err)
					}
				} else if err := store.Revoke(t.Context(), token, auth.AccountAudience, true); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.ConsumeHandoff(env.adminUser.TenantID, fresh.HandoffCode); !errors.Is(err, services.ErrHostedLoginHandoffInvalid) {
					t.Fatalf("%s did not invalidate handoff: %v", invalidate, err)
				}
			}
		})
	}
}

func TestInternalAuthHandlerHostedPasskeyOptions(t *testing.T) {
	for _, passwordAvailable := range []bool{true, false} {
		t.Run(map[bool]string{true: "password-and-passkey", false: "passkey-only"}[passwordAvailable], func(t *testing.T) {
			env := setupOtherHandlersEnv(t)
			if !passwordAvailable {
				if err := env.db.Model(&env.adminUser).Update("password_hash", "").Error; err != nil {
					t.Fatal(err)
				}
			}
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
				Passkey           bool `json:"passkey"`
				PasswordAvailable bool `json:"password_available"`
				Options           []struct {
					ChallengeID string `json:"challenge_id"`
					PublicKey   any    `json:"public_key"`
				} `json:"options"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if !body.Passkey || body.PasswordAvailable != passwordAvailable || len(body.Options) != 1 || body.Options[0].ChallengeID == "" || body.Options[0].PublicKey == nil {
				t.Fatalf("expected hosted passkey options, got %#v", body)
			}
			var challenge models.WebAuthnChallenge
			if err := env.db.Where("id = ?", body.Options[0].ChallengeID).First(&challenge).Error; err != nil {
				t.Fatalf("load hosted passkey challenge: %v", err)
			}
			if challenge.Purpose != "hosted_login" || challenge.RPID != handlersTestPublicDomain {
				t.Fatalf("unexpected hosted challenge: %#v", challenge)
			}
		})
	}
}

func TestHostedOptionsReflectPasswordAvailability(t *testing.T) {
	for _, password := range []bool{true, false} {
		t.Run(map[bool]string{true: "password", false: "passkey-only"}[password], func(t *testing.T) {
			env := setupOtherHandlersEnv(t)
			if !password {
				if err := env.db.Model(&env.adminUser).Update("password_hash", "").Error; err != nil {
					t.Fatal(err)
				}
			}
			svc := services.NewHostedLoginService(env.db, handlersTestPublicBaseURL, "reset-secret")
			h := handlers.NewAuthHandler(env.db, env.signingKey, env.queue, handlersTestPublicBaseURL, "reset-secret", "invite-secret", 30*time.Minute, 72*time.Hour, "https://clientshare.app", svc, services.NewTenantSettingsService(env.db))
			app := fiber.New()
			handlers.RegisterInternalAuthRoutes(app.Group("/internal"), svc, h)
			response, err := app.Test(jsonRequest(http.MethodPost, "/internal/auth/hosted-login/passkey/options", map[string]any{"email": env.adminUser.Email}))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var body struct {
				Passkey           bool `json:"passkey"`
				PasswordAvailable bool `json:"password_available"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != 200 || body.Passkey || body.PasswordAvailable != password {
				t.Fatalf("incorrect authentication capabilities: %d %+v", response.StatusCode, body)
			}
		})
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
