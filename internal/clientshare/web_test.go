package clientshare

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pixelcop/clientshare/pkg/web"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const cacheControlFallbackDocuments = "no-cache, must-revalidate"

func newTestConfig(tempRoot string) *Config {
	cfg := &Config{}
	cfg.Server.MaxBodyMB = 4
	cfg.Server.BaseURL = "https://frontend.local"
	cfg.Storage.Local.Root = tempRoot
	cfg.SecureLinks.SigningKey = "test-signing-key"
	cfg.Auth.JWTSecret = "test-reset-secret"
	cfg.Auth.InviteTokenSecret = "test-invite-secret"
	cfg.Branding.SiteTitle = "ClientShare"
	cfg.Branding.PrimaryColor = "#112233"
	return cfg
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := database.AutoMigrate(&models.Tenant{}, &models.TenantSettings{}, &models.TenantDomain{}); err != nil {
		t.Fatalf("failed to migrate tenant tables: %v", err)
	}
	if err := database.Create(&models.Tenant{ID: tenantctx.DefaultTenantID, Slug: tenantctx.DefaultTenantSlug, Name: tenantctx.DefaultTenantName}).Error; err != nil {
		t.Fatalf("failed to seed default tenant: %v", err)
	}
	if err := database.Create(&models.TenantSettings{TenantID: tenantctx.DefaultTenantID, PublicBaseURL: "https://frontend.local"}).Error; err != nil {
		t.Fatalf("failed to seed default tenant settings: %v", err)
	}
	if err := database.Create(&models.TenantDomain{TenantID: tenantctx.DefaultTenantID, Domain: "frontend.local", Kind: tenantctx.DomainKindPublicBaseURL, IsPrimary: true}).Error; err != nil {
		t.Fatalf("failed to seed default tenant domain: %v", err)
	}

	return database
}

func firstEmbeddedAssetPath(t *testing.T) string {
	t.Helper()

	_, filePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}

	assetDir := filepath.Join(filepath.Dir(filePath), "..", "static", "dist", "assets")
	entries, err := os.ReadDir(assetDir)
	if err != nil {
		t.Fatalf("failed to read embedded assets: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			return "/assets/" + entry.Name()
		}
	}

	t.Fatal("expected at least one embedded asset")
	return ""
}

func TestNewWebApp_UsesProductionRouteSetup(t *testing.T) {
	tempRoot, err := os.MkdirTemp("", "clientshare-web-test-*")
	if err != nil {
		t.Fatalf("failed to create temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempRoot)
	})

	cfg := newTestConfig(tempRoot)
	cfg.DevMode = true

	app, err := NewWebApp(cfg, newTestDB(t), zap.NewNop(), nil, storage.NewLocalStorage(tempRoot))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	healthResp, err := app.Test(healthReq)
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", healthResp.StatusCode)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Accept", "application/json")
	loginResp, err := app.Test(loginReq)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if loginResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", loginResp.StatusCode)
	}

	protectedReq := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	protectedReq.Header.Set("Accept", "application/json")
	protectedResp, err := app.Test(protectedReq)
	if err != nil {
		t.Fatalf("protected request failed: %v", err)
	}
	if protectedResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", protectedResp.StatusCode)
	}
}

func TestNewWebAppServesWebAuthnRelatedOrigins(t *testing.T) {
	tempRoot, err := os.MkdirTemp("", "clientshare-web-test-*")
	if err != nil {
		t.Fatalf("create temp root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempRoot) })

	cfg := newTestConfig(tempRoot)
	cfg.DevMode = true
	cfg.Auth.HostedPasskeyOrigin = "https://clientshare.app"
	cfg.InternalAPI.Enabled = true
	cfg.InternalAPI.Secret = "12345678901234567890123456789012"
	app, err := NewWebApp(cfg, newTestDB(t), zap.NewNop(), nil, storage.NewLocalStorage(tempRoot))
	if err != nil {
		t.Fatalf("NewWebApp() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/.well-known/webauthn", nil)
	req.Host = "frontend.local"
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body struct {
		Origins []string `json:"origins"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Origins) != 1 || body.Origins[0] != "https://clientshare.app" {
		t.Fatalf("origins = %#v, want ClientShare SaaS origin", body.Origins)
	}
}

func TestNewWebApp_ProductionStaticCacheHeaders(t *testing.T) {
	tempRoot, err := os.MkdirTemp("", "clientshare-web-cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempRoot)
	})

	cfg := newTestConfig(tempRoot)

	app, err := NewWebApp(cfg, newTestDB(t), zap.NewNop(), nil, storage.NewLocalStorage(tempRoot))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	assetPath := firstEmbeddedAssetPath(t)

	tests := []struct {
		name         string
		path         string
		wantStatus   int
		wantCacheCtl string
		wantETag     bool
	}{
		{
			name:         "hashed assets are immutable",
			path:         assetPath,
			wantStatus:   http.StatusOK,
			wantCacheCtl: web.CacheControlImmutableAssets,
			wantETag:     true,
		},
		{
			name:         "root icons use one day cache",
			path:         "/favicon.ico",
			wantStatus:   http.StatusOK,
			wantCacheCtl: web.CacheControlRootAssets,
			wantETag:     true,
		},
		{
			name:         "spa route is no-cache",
			path:         "/clients",
			wantStatus:   http.StatusOK,
			wantCacheCtl: cacheControlFallbackDocuments,
			wantETag:     true,
		},
		{
			name:         "index document is no-cache",
			path:         "/",
			wantStatus:   http.StatusOK,
			wantCacheCtl: cacheControlFallbackDocuments,
			wantETag:     true,
		},
		{
			name:         "api routes keep their own headers",
			path:         "/api/healthz",
			wantStatus:   http.StatusOK,
			wantCacheCtl: "",
			wantETag:     false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, testCase.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if resp.StatusCode != testCase.wantStatus {
				t.Fatalf("expected status %d, got %d", testCase.wantStatus, resp.StatusCode)
			}

			if got := resp.Header.Get("Cache-Control"); got != testCase.wantCacheCtl {
				t.Fatalf("expected Cache-Control %q, got %q", testCase.wantCacheCtl, got)
			}

			gotETag := resp.Header.Get("ETag") != ""
			if gotETag != testCase.wantETag {
				t.Fatalf("expected ETag present=%t, got %t", testCase.wantETag, gotETag)
			}
		})
	}
}

func TestNewWebApp_ProductionStaticETagShortCircuit(t *testing.T) {
	tempRoot, err := os.MkdirTemp("", "clientshare-web-etag-test-*")
	if err != nil {
		t.Fatalf("failed to create temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempRoot)
	})

	cfg := newTestConfig(tempRoot)
	app, err := NewWebApp(cfg, newTestDB(t), zap.NewNop(), nil, storage.NewLocalStorage(tempRoot))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	assetPath := firstEmbeddedAssetPath(t)
	firstReq := httptest.NewRequest(http.MethodGet, assetPath, nil)
	firstResp, err := app.Test(firstReq)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("expected first status 200, got %d", firstResp.StatusCode)
	}

	etag := firstResp.Header.Get(fiber.HeaderETag)
	if etag == "" {
		t.Fatal("expected static asset ETag on first response")
	}

	secondReq := httptest.NewRequest(http.MethodGet, assetPath, nil)
	secondReq.Header.Set(fiber.HeaderIfNoneMatch, etag)
	secondResp, err := app.Test(secondReq)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if secondResp.StatusCode != http.StatusNotModified {
		t.Fatalf("expected second status 304, got %d", secondResp.StatusCode)
	}
	if got := secondResp.Header.Get(fiber.HeaderETag); got != etag {
		t.Fatalf("expected cached ETag %q, got %q", etag, got)
	}
	if got := secondResp.Header.Get(fiber.HeaderCacheControl); got != web.CacheControlImmutableAssets {
		t.Fatalf("expected cached Cache-Control %q, got %q", web.CacheControlImmutableAssets, got)
	}
}

func TestNewWebApp_SPAFallbackReusesCachedETagAcrossRoutes(t *testing.T) {
	tempRoot, err := os.MkdirTemp("", "clientshare-web-spa-etag-test-*")
	if err != nil {
		t.Fatalf("failed to create temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempRoot)
	})

	cfg := newTestConfig(tempRoot)
	app, err := NewWebApp(cfg, newTestDB(t), zap.NewNop(), nil, storage.NewLocalStorage(tempRoot))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	firstReq := httptest.NewRequest(http.MethodGet, "/clients", nil)
	firstResp, err := app.Test(firstReq)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("expected first status 200, got %d", firstResp.StatusCode)
	}

	etag := firstResp.Header.Get(fiber.HeaderETag)
	if etag == "" {
		t.Fatal("expected SPA fallback ETag on first response")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/settings", nil)
	secondReq.Header.Set(fiber.HeaderIfNoneMatch, etag)
	secondResp, err := app.Test(secondReq)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if secondResp.StatusCode != http.StatusNotModified {
		t.Fatalf("expected second status 304, got %d", secondResp.StatusCode)
	}
	if got := secondResp.Header.Get(fiber.HeaderCacheControl); got != cacheControlFallbackDocuments {
		t.Fatalf("expected SPA Cache-Control %q, got %q", cacheControlFallbackDocuments, got)
	}
}

func TestNewWebApp_PublicStaticETagInvalidatesOnFileChange(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	tempDir, err := os.MkdirTemp("", "clientshare-web-public-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(repoRoot)
		_ = os.RemoveAll(tempDir)
	})

	if err := os.MkdirAll(filepath.Join(tempDir, "public"), 0o755); err != nil {
		t.Fatalf("failed to create public dir: %v", err)
	}

	publicFile := filepath.Join(tempDir, "public", "favicon.ico")
	if err := os.WriteFile(publicFile, []byte("version-one"), 0o644); err != nil {
		t.Fatalf("failed to create public file: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	tempRoot, err := os.MkdirTemp("", "clientshare-storage-root-*")
	if err != nil {
		t.Fatalf("failed to create temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempRoot)
	})

	cfg := newTestConfig(tempRoot)
	app, err := NewWebApp(cfg, newTestDB(t), zap.NewNop(), nil, storage.NewLocalStorage(tempRoot))
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	firstReq := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	firstResp, err := app.Test(firstReq)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	etag := firstResp.Header.Get(fiber.HeaderETag)
	if etag == "" {
		t.Fatal("expected ETag for disk-backed public asset")
	}

	updatedModTime := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(publicFile, []byte("version-two"), 0o644); err != nil {
		t.Fatalf("failed to update public file: %v", err)
	}
	if err := os.Chtimes(publicFile, updatedModTime, updatedModTime); err != nil {
		t.Fatalf("failed to update modtime: %v", err)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	secondReq.Header.Set(fiber.HeaderIfNoneMatch, etag)
	secondResp, err := app.Test(secondReq)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("expected second status 200 after file change, got %d", secondResp.StatusCode)
	}
	if got := secondResp.Header.Get(fiber.HeaderETag); got == "" || got == etag {
		t.Fatalf("expected refreshed ETag after file change, got %q", got)
	}
}
