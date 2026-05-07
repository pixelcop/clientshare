package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

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
	hostedLoginSvc := services.NewHostedLoginService(env.db, handlersTestPublicBaseURL)
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
	if body.Token == "" {
		t.Fatal("token = empty, want issued token")
	}
	if body.RedirectURL != handlersTestPublicBaseURL {
		t.Fatalf("redirect_url = %q, want %q", body.RedirectURL, handlersTestPublicBaseURL)
	}
	if body.TenantID != tenantctx.DefaultTenantID {
		t.Fatalf("tenant_id = %q, want %q", body.TenantID, tenantctx.DefaultTenantID)
	}

	token, err := auth.ParseJWT(body.Token)
	if err != nil || !token.Valid {
		t.Fatalf("ParseJWT() err = %v, want valid token", err)
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
	handlers.RegisterInternalAuthRoutes(app.Group("/internal", middleware.InternalAuthRequired(secret)), services.NewHostedLoginService(env.db, handlersTestPublicBaseURL))

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