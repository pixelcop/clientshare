package tenant_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBaseURLFromFiber_PrefersTenantURLThenRequestHost(t *testing.T) {
	app := fiber.New()
	app.Get("/tenant-base-url", func(c fiber.Ctx) error {
		tenant.SetLocal(c, tenant.RequestContext{ID: tenant.DefaultTenantID, Slug: tenant.DefaultTenantSlug, Name: tenant.DefaultTenantName, PublicBaseURL: "https://portal.example.test", Resolution: "test"})
		return c.SendString(tenant.BaseURLFromFiber(c, "https://fallback.example.test"))
	})
	app.Get("/request-host", func(c fiber.Ctx) error {
		tenant.SetLocal(c, tenant.RequestContext{ID: tenant.DefaultTenantID, Slug: tenant.DefaultTenantSlug, Name: tenant.DefaultTenantName, Resolution: "test"})
		return c.SendString(tenant.BaseURLFromFiber(c, "https://fallback.example.test"))
	})

	t.Run("tenant public base url wins", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://request.example.test/tenant-base-url", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("tenant base url request failed: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed reading tenant base url response: %v", err)
		}
		if string(body) != "https://portal.example.test" {
			t.Fatalf("expected tenant public base url, got %q", string(body))
		}
	})

	t.Run("request host beats fallback when tenant url missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://request.example.test/request-host", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request-host request failed: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed reading request-host response: %v", err)
		}
		if string(body) != "http://request.example.test" {
			t.Fatalf("expected request host base url, got %q", string(body))
		}
	})
}

func TestResolverPrefersDomainRowsBeforeSettingsFallbackAndSubdomains(t *testing.T) {
	db := openTenantResolverTestDB(t)
	now := time.Now()

	tenantA := models.Tenant{ID: "tenant-a", Slug: "tenant-a", Name: "Tenant A", CreatedAt: now, UpdatedAt: now}
	tenantB := models.Tenant{ID: "tenant-b", Slug: "tenant-b", Name: "Tenant B", CreatedAt: now, UpdatedAt: now}
	tenantC := models.Tenant{ID: "tenant-c", Slug: "tenant-c", Name: "Tenant C", CreatedAt: now, UpdatedAt: now}
	for _, tenantModel := range []models.Tenant{tenantA, tenantB, tenantC} {
		if err := db.Create(&tenantModel).Error; err != nil {
			t.Fatalf("failed to create tenant %s: %v", tenantModel.ID, err)
		}
	}

	settings := []models.TenantSettings{
		{TenantID: tenantA.ID, PublicBaseURL: "https://mapped.example.test", CreatedAt: now, UpdatedAt: now},
		{TenantID: tenantB.ID, PublicBaseURL: "https://mapped.example.test", CreatedAt: now, UpdatedAt: now},
		{TenantID: tenantC.ID, PublicBaseURL: "", CreatedAt: now, UpdatedAt: now},
	}
	for index := range settings {
		if err := db.Create(&settings[index]).Error; err != nil {
			t.Fatalf("failed to create tenant settings for %s: %v", settings[index].TenantID, err)
		}
	}
	if err := db.Model(&models.TenantSettings{}).Where("tenant_id = ?", tenantB.ID).Update("public_base_url", "https://scan.example.test").Error; err != nil {
		t.Fatalf("failed updating tenant B public base url: %v", err)
	}
	if err := db.Create(&models.TenantDomain{TenantID: tenantA.ID, Domain: "mapped.example.test", Kind: tenant.DomainKindPublicBaseURL, IsPrimary: true, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("failed to create tenant domain: %v", err)
	}

	resolver := tenant.NewResolver(db, tenant.ModeHosted, "https://app.example.test", tenant.DefaultInternalTargetHeader)

	t.Run("tenant_domains row wins over settings scan fallback", func(t *testing.T) {
		resolved := resolveTenantRequest(t, resolver, httptest.NewRequest(http.MethodGet, "http://mapped.example.test/api/files", nil))
		if resolved.ID != tenantA.ID {
			t.Fatalf("expected tenant A from tenant_domains row, got %q", resolved.ID)
		}
		if resolved.PublicBaseURL != "https://mapped.example.test" {
			t.Fatalf("expected tenant A public base url, got %q", resolved.PublicBaseURL)
		}
	})

	t.Run("settings hostname scan remains as temporary fallback", func(t *testing.T) {
		resolved := resolveTenantRequest(t, resolver, httptest.NewRequest(http.MethodGet, "http://scan.example.test/api/files", nil))
		if resolved.ID != tenantB.ID {
			t.Fatalf("expected tenant B from settings fallback, got %q", resolved.ID)
		}
		if resolved.PublicBaseURL != "https://scan.example.test" {
			t.Fatalf("expected tenant B public base url, got %q", resolved.PublicBaseURL)
		}
	})

	t.Run("subdomain fallback still resolves by slug", func(t *testing.T) {
		resolved := resolveTenantRequest(t, resolver, httptest.NewRequest(http.MethodGet, "http://tenant-c.app.example.test/api/files", nil))
		if resolved.ID != tenantC.ID {
			t.Fatalf("expected tenant C from subdomain fallback, got %q", resolved.ID)
		}
		if resolved.Resolution != "subdomain" {
			t.Fatalf("expected subdomain resolution, got %q", resolved.Resolution)
		}
		if resolved.PublicBaseURL != "" {
			t.Fatalf("expected no stored public base url for subdomain fallback, got %q", resolved.PublicBaseURL)
		}
	})
}

func TestResolverSingleModeUsesCachedTenantFromDB(t *testing.T) {
	db := openTenantResolverTestDB(t)
	now := time.Now()

	defaultTenant := models.Tenant{
		ID:        tenant.DefaultTenantID,
		Slug:      "self-hosted-acme",
		Name:      "Acme Self Hosted",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(&defaultTenant).Error; err != nil {
		t.Fatalf("failed to create default tenant: %v", err)
	}

	settings := models.TenantSettings{
		TenantID:      defaultTenant.ID,
		PublicBaseURL: "https://portal.single.example.test",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatalf("failed to create default tenant settings: %v", err)
	}

	resolver := tenant.NewResolver(db, tenant.ModeSingle, "https://fallback.example.test", tenant.DefaultInternalTargetHeader)

	t.Run("single mode returns the tenant stored in the database", func(t *testing.T) {
		resolved := resolveTenantRequest(t, resolver, httptest.NewRequest(http.MethodGet, "http://request.example.test/api/files", nil))
		if resolved.ID != defaultTenant.ID {
			t.Fatalf("expected default tenant id %q, got %q", defaultTenant.ID, resolved.ID)
		}
		if resolved.Slug != defaultTenant.Slug {
			t.Fatalf("expected default tenant slug %q, got %q", defaultTenant.Slug, resolved.Slug)
		}
		if resolved.Name != defaultTenant.Name {
			t.Fatalf("expected default tenant name %q, got %q", defaultTenant.Name, resolved.Name)
		}
		if resolved.PublicBaseURL != settings.PublicBaseURL {
			t.Fatalf("expected default tenant public base url %q, got %q", settings.PublicBaseURL, resolved.PublicBaseURL)
		}
		if resolved.Resolution != "single" {
			t.Fatalf("expected single resolution, got %q", resolved.Resolution)
		}
	})

	t.Run("single mode reuses the cached tenant after the first lookup", func(t *testing.T) {
		if err := db.Model(&models.Tenant{}).Where("id = ?", defaultTenant.ID).Updates(map[string]any{
			"slug": "changed-after-cache",
			"name": "Changed After Cache",
		}).Error; err != nil {
			t.Fatalf("failed updating default tenant after cache warmup: %v", err)
		}
		if err := db.Model(&models.TenantSettings{}).Where("tenant_id = ?", defaultTenant.ID).Update("public_base_url", "https://changed-after-cache.example.test").Error; err != nil {
			t.Fatalf("failed updating default tenant settings after cache warmup: %v", err)
		}

		resolved := resolveTenantRequest(t, resolver, httptest.NewRequest(http.MethodGet, "http://request.example.test/api/files", nil))
		if resolved.Slug != defaultTenant.Slug {
			t.Fatalf("expected cached tenant slug %q, got %q", defaultTenant.Slug, resolved.Slug)
		}
		if resolved.Name != defaultTenant.Name {
			t.Fatalf("expected cached tenant name %q, got %q", defaultTenant.Name, resolved.Name)
		}
		if resolved.PublicBaseURL != settings.PublicBaseURL {
			t.Fatalf("expected cached tenant public base url %q, got %q", settings.PublicBaseURL, resolved.PublicBaseURL)
		}
	})
}

func openTenantResolverTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open resolver test db: %v", err)
	}
	if err := db.AutoMigrate(&models.Tenant{}, &models.TenantSettings{}, &models.TenantDomain{}); err != nil {
		t.Fatalf("failed to migrate resolver tables: %v", err)
	}
	return db
}

func resolveTenantRequest(t *testing.T, resolver *tenant.Resolver, req *http.Request) tenant.RequestContext {
	t.Helper()
	app := fiber.New()
	app.Get("/*", func(c fiber.Ctx) error {
		resolved, err := resolver.Resolve(c)
		if err != nil {
			return c.Status(http.StatusNotFound).SendString(err.Error())
		}
		return c.JSON(resolved)
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("resolver request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected resolver status 200, got %d", resp.StatusCode)
	}

	var resolved tenant.RequestContext
	if err := json.NewDecoder(resp.Body).Decode(&resolved); err != nil {
		t.Fatalf("failed decoding resolver response: %v", err)
	}
	return resolved
}
