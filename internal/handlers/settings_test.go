package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	handlerspkg "github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pixelcop/clientshare/pkg/utils"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSettingsHandlers_GetAndUpdate(t *testing.T) {
	tempRoot := testStorageRoot
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	workingDir, err := os.MkdirTemp(tempRoot, "settings-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(workingDir)

	publicDir := filepath.Join(workingDir, "public")
	if err := os.MkdirAll(publicDir, 0o755); err != nil {
		t.Fatalf("failed creating public dir: %v", err)
	}

	db, err := gorm.Open(sqlite.Open("file:settings-handlers-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := db.AutoMigrate(&models.TenantSettings{}, &models.TenantDomain{}); err != nil {
		t.Fatalf("failed to migrate tenant settings: %v", err)
	}
	service := services.NewTenantSettingsService(db)
	fallbackBaseURL := "https://fallback.example.test"
	handler := handlerspkg.NewSettingsHandler(service, publicDir, fallbackBaseURL, tenantctx.ModeSingle)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		tenantctx.SetLocal(c, tenantctx.RequestContext{ID: tenantctx.DefaultTenantID, Slug: tenantctx.DefaultTenantSlug, Name: tenantctx.DefaultTenantName, Resolution: "test"})
		return c.Next()
	})
	app.Get("/branding", handler.GetBranding)
	app.Put("/branding", handler.UpdateBranding)
	app.Get("/tenant", handler.GetTenantSettings)
	app.Put("/tenant", handler.UpdateTenantSettings)

	getReq := jsonRequest(http.MethodGet, "/branding", nil)
	getReq.Host = "tenant.example.test"
	getResp, err := app.Test(getReq)
	if err != nil {
		t.Fatalf("get branding request failed: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}
	var brandingBody map[string]any
	if err := json.NewDecoder(getResp.Body).Decode(&brandingBody); err != nil {
		t.Fatalf("failed decoding branding response: %v", err)
	}
	if brandingBody["effective_public_base_url"] != "http://tenant.example.test" {
		t.Fatalf("expected effective branding url to use request host, got %#v", brandingBody["effective_public_base_url"])
	}

	invalidColorReq := jsonRequest(http.MethodPut, "/branding", map[string]any{"primary_color": "blue"})
	invalidColorResp, err := app.Test(invalidColorReq)
	if err != nil {
		t.Fatalf("invalid color request failed: %v", err)
	}
	if invalidColorResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", invalidColorResp.StatusCode)
	}

	updateReq := jsonRequest(http.MethodPut, "/branding", map[string]any{"site_title": "ClientShare Pro", "primary_color": "#aabbcc"})
	updateResp, err := app.Test(updateReq)
	if err != nil {
		t.Fatalf("update branding request failed: %v", err)
	}
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateResp.StatusCode)
	}

	settings, err := service.Get(context.Background(), tenantctx.DefaultTenantID)
	if err != nil {
		t.Fatalf("failed loading tenant settings: %v", err)
	}
	if settings.SiteTitle != "ClientShare Pro" {
		t.Fatalf("expected updated site title, got %q", settings.SiteTitle)
	}
	if settings.PrimaryColor != "#aabbcc" {
		t.Fatalf("expected updated color, got %q", settings.PrimaryColor)
	}

	tenantUpdateReq := jsonRequest(http.MethodPut, "/tenant", map[string]any{
		"site_title":                      "ClientShare Pro",
		"primary_color":                   "#aabbcc",
		"public_base_url":                 "https://portal.example.test",
		"invite_welcome_text":             "Welcome aboard",
		"secure_link_default_expiry_days": 45,
	})
	tenantUpdateResp, err := app.Test(tenantUpdateReq)
	if err != nil {
		t.Fatalf("tenant update request failed: %v", err)
	}
	if tenantUpdateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", tenantUpdateResp.StatusCode)
	}

	settings, err = service.Get(context.Background(), tenantctx.DefaultTenantID)
	if err != nil {
		t.Fatalf("failed loading tenant settings after tenant update: %v", err)
	}
	if settings.PublicBaseURL != "https://portal.example.test" {
		t.Fatalf("expected updated public base url, got %q", settings.PublicBaseURL)
	}
	if settings.InviteWelcomeText != "Welcome aboard" {
		t.Fatalf("expected updated invite welcome text, got %q", settings.InviteWelcomeText)
	}
	if settings.SecureLinkDefaultExpiryDays != 45 {
		t.Fatalf("expected updated secure link expiry days, got %d", settings.SecureLinkDefaultExpiryDays)
	}

	tenantGetReq := jsonRequest(http.MethodGet, "/tenant", nil)
	tenantGetReq.Host = "tenant.example.test"
	tenantGetResp, err := app.Test(tenantGetReq)
	if err != nil {
		t.Fatalf("tenant get request failed: %v", err)
	}
	if tenantGetResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", tenantGetResp.StatusCode)
	}
	var tenantBody map[string]any
	if err := json.NewDecoder(tenantGetResp.Body).Decode(&tenantBody); err != nil {
		t.Fatalf("failed decoding tenant response: %v", err)
	}
	if tenantBody["effective_public_base_url"] != "https://portal.example.test" {
		t.Fatalf("expected effective tenant url from stored public_base_url, got %#v", tenantBody["effective_public_base_url"])
	}

	var tenantDomain models.TenantDomain
	if err := db.Where("tenant_id = ?", tenantctx.DefaultTenantID).First(&tenantDomain).Error; err != nil {
		t.Fatalf("expected synchronized tenant domain: %v", err)
	}
	if tenantDomain.Domain != "portal.example.test" {
		t.Fatalf("expected synchronized tenant domain hostname, got %q", tenantDomain.Domain)
	}
	if tenantDomain.Kind != tenantctx.DomainKindPublicBaseURL {
		t.Fatalf("expected synchronized tenant domain kind %q, got %q", tenantctx.DomainKindPublicBaseURL, tenantDomain.Kind)
	}
}

func TestSettingsHandlers_RejectDuplicateTenantPublicBaseURLHost(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-handlers-conflict-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := db.AutoMigrate(&models.TenantSettings{}, &models.TenantDomain{}); err != nil {
		t.Fatalf("failed to migrate tenant settings tables: %v", err)
	}
	service := services.NewTenantSettingsService(db)
	now := time.Now()
	if err := db.Create(&models.TenantDomain{TenantID: "other-tenant", Domain: "portal.example.test", Kind: tenantctx.DomainKindPublicBaseURL, IsPrimary: true, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("failed to seed conflicting tenant domain: %v", err)
	}

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		tenantctx.SetLocal(c, tenantctx.RequestContext{ID: tenantctx.DefaultTenantID, Slug: tenantctx.DefaultTenantSlug, Name: tenantctx.DefaultTenantName, Resolution: "test"})
		return c.Next()
	})
	handler := handlerspkg.NewSettingsHandler(service, t.TempDir(), "https://fallback.example.test", tenantctx.ModeSingle)
	app.Put("/tenant", handler.UpdateTenantSettings)

	req := jsonRequest(http.MethodPut, "/tenant", map[string]any{
		"site_title":                      "ClientShare Pro",
		"primary_color":                   "#aabbcc",
		"public_base_url":                 "https://portal.example.test",
		"invite_welcome_text":             "Welcome aboard",
		"secure_link_default_expiry_days": 45,
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("duplicate host update request failed: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestSettingsHandlers_SystemSettingsSingleTenant(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	previousLevel := utils.CurrentLogLevel()
	t.Cleanup(func() {
		if err := utils.SetLogLevel(previousLevel); err != nil {
			t.Fatalf("failed to restore previous log level: %v", err)
		}
	})

	if err := utils.SetLogLevel("info"); err != nil {
		t.Fatalf("failed setting initial log level: %v", err)
	}

	unauthorizedReq := jsonRequest(http.MethodGet, "/api/settings/system", nil)
	unauthorizedResp, err := env.app.Test(unauthorizedReq)
	if err != nil {
		t.Fatalf("unauthorized get request failed: %v", err)
	}
	if unauthorizedResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorizedResp.StatusCode)
	}

	forbiddenReq := jsonAuthRequest(http.MethodPut, "/api/settings/system", env.managerToken, map[string]any{"level": "debug"})
	forbiddenResp, err := env.app.Test(forbiddenReq)
	if err != nil {
		t.Fatalf("forbidden update request failed: %v", err)
	}
	if forbiddenResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", forbiddenResp.StatusCode)
	}

	getReq := jsonAuthRequest(http.MethodGet, "/api/settings/system", env.adminToken, nil)
	getResp, err := env.app.Test(getReq)
	if err != nil {
		t.Fatalf("get system settings request failed: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	var getBody struct {
		TenancyMode string `json:"tenancy_mode"`
		LogLevel    *struct {
			Level   string   `json:"level"`
			Options []string `json:"options"`
		} `json:"log_level"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&getBody); err != nil {
		t.Fatalf("failed decoding get response: %v", err)
	}
	if getBody.TenancyMode != tenantctx.ModeSingle {
		t.Fatalf("expected single tenancy mode, got %q", getBody.TenancyMode)
	}
	if getBody.LogLevel == nil {
		t.Fatalf("expected log level settings in single-tenant response")
	}
	if getBody.LogLevel.Level != "info" {
		t.Fatalf("expected info level, got %q", getBody.LogLevel.Level)
	}
	if len(getBody.LogLevel.Options) == 0 {
		t.Fatalf("expected log level options in response")
	}

	invalidReq := jsonAuthRequest(http.MethodPut, "/api/settings/system", env.adminToken, map[string]any{"level": "verbose"})
	invalidResp, err := env.app.Test(invalidReq)
	if err != nil {
		t.Fatalf("invalid update request failed: %v", err)
	}
	if invalidResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", invalidResp.StatusCode)
	}

	updateReq := jsonAuthRequest(http.MethodPut, "/api/settings/system", env.adminToken, map[string]any{"level": "debug"})
	updateResp, err := env.app.Test(updateReq)
	if err != nil {
		t.Fatalf("update system settings request failed: %v", err)
	}
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateResp.StatusCode)
	}
	if got := utils.CurrentLogLevel(); got != "debug" {
		t.Fatalf("expected runtime log level debug, got %q", got)
	}

	var updateBody struct {
		TenancyMode string `json:"tenancy_mode"`
		LogLevel    *struct {
			Level string `json:"level"`
		} `json:"log_level"`
	}
	if err := json.NewDecoder(updateResp.Body).Decode(&updateBody); err != nil {
		t.Fatalf("failed decoding update response: %v", err)
	}
	if updateBody.TenancyMode != tenantctx.ModeSingle {
		t.Fatalf("expected single tenancy mode after update, got %q", updateBody.TenancyMode)
	}
	if updateBody.LogLevel == nil {
		t.Fatalf("expected log level payload after update")
	}
	if updateBody.LogLevel.Level != "debug" {
		t.Fatalf("expected updated level debug, got %q", updateBody.LogLevel.Level)
	}
}

func TestSettingsHandlers_SystemSettingsHostedModeOmitsLogLevel(t *testing.T) {
	env := setupOtherHandlersEnvWithOptions(t, handlersTestOptions{disableAuthRateLimit: true, tenancyMode: tenantctx.ModeHosted})
	previousLevel := utils.CurrentLogLevel()
	t.Cleanup(func() {
		if err := utils.SetLogLevel(previousLevel); err != nil {
			t.Fatalf("failed to restore previous log level: %v", err)
		}
	})

	if err := utils.SetLogLevel("info"); err != nil {
		t.Fatalf("failed setting initial log level: %v", err)
	}

	getReq := jsonAuthRequest(http.MethodGet, "/api/settings/system", env.adminToken, nil)
	getReq.Host = handlersTestPublicDomain
	getResp, err := env.app.Test(getReq)
	if err != nil {
		t.Fatalf("get hosted system settings request failed: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	var getBody struct {
		TenancyMode string `json:"tenancy_mode"`
		LogLevel    any    `json:"log_level"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&getBody); err != nil {
		t.Fatalf("failed decoding hosted get response: %v", err)
	}
	if getBody.TenancyMode != tenantctx.ModeHosted {
		t.Fatalf("expected hosted tenancy mode, got %q", getBody.TenancyMode)
	}
	if getBody.LogLevel != nil {
		t.Fatalf("expected hosted response to omit log level, got %#v", getBody.LogLevel)
	}

	updateReq := jsonAuthRequest(http.MethodPut, "/api/settings/system", env.adminToken, map[string]any{"level": "debug"})
	updateReq.Host = handlersTestPublicDomain
	updateResp, err := env.app.Test(updateReq)
	if err != nil {
		t.Fatalf("hosted update request failed: %v", err)
	}
	if updateResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", updateResp.StatusCode)
	}
	if got := utils.CurrentLogLevel(); got != "info" {
		t.Fatalf("expected runtime log level to remain info, got %q", got)
	}
}
