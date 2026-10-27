package services

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
)

func TestHostedHostnameActivationPreservesLinksAndOwnership(t *testing.T) {
	db := setupTenantSettingsTestDB(t)
	if err := db.AutoMigrate(&models.Tenant{}, &models.AuditEvent{}); err != nil {
		t.Fatal(err)
	}
	tenant := models.Tenant{Slug: "original", Name: "Example"}
	other := models.Tenant{Slug: "other", Name: "Other"}
	for _, item := range []*models.Tenant{&tenant, &other} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewTenantProvisioningService(db, "", 0, nil, "")
	register := func(host string) *models.TenantDomain {
		t.Helper()
		d, err := svc.RegisterDomain(tenant.ID, RegisterDomainInput{Domain: host, Kind: "subdomain"})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	old := register("old.portal.test")
	if again := register(old.Domain); again.ID != old.ID {
		t.Fatal("retry created another domain")
	}
	if _, err := svc.RegisterDomain(other.ID, RegisterDomainInput{Domain: old.Domain, Kind: "subdomain"}); !errors.Is(err, ErrTenantPublicBaseURLConflict) {
		t.Fatalf("hostname ownership not protected: %v", err)
	}
	register("new.portal.test")
	app := fiber.New()
	app.Use(middleware.NewTenantResolver(db, middleware.TenantResolverConfig{Mode: "hosted"}))
	app.Use(func(c fiber.Ctx) error { return c.SendString("portal") })
	check := func(method, host, path, location string) {
		t.Helper()
		req := httptest.NewRequest(method, "https://"+host+path, strings.NewReader("body"))
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		wantStatus := 200
		if location != "" {
			wantStatus = 308
		}
		if res.StatusCode != wantStatus || res.Header.Get("Location") != location {
			t.Fatalf("%s %s: status=%d location=%q", method, host, res.StatusCode, res.Header.Get("Location"))
		}
	}
	path := "/s/a%2Fb?token=x%2By&download=1&download=2"
	check("GET", old.Domain, path, "") // registration alone never redirects
	if err := svc.ActivateHostedDomain(tenant.ID, "new.portal.test"); err != nil {
		t.Fatal(err)
	}
	check("GET", old.Domain, path, "https://new.portal.test"+path)
	check("POST", old.Domain, "/api/files?client=123", "https://new.portal.test/api/files?client=123")
	check("GET", "new.portal.test", path, "")
	register("latest.portal.test")
	if err := svc.ActivateHostedDomain(tenant.ID, "latest.portal.test"); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{old.Domain, "new.portal.test"} {
		check("GET", host, path, "https://latest.portal.test"+path)
	}
	check("GET", "latest.portal.test", path, "")
	if err := svc.ActivateHostedDomain(other.ID, "latest.portal.test"); err == nil {
		t.Fatal("cross-tenant activation succeeded")
	}
	var settings models.TenantSettings
	if err := db.First(&settings, "tenant_id = ?", tenant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if settings.PublicBaseURL != "https://latest.portal.test" {
		t.Fatalf("outgoing links still use %s", settings.PublicBaseURL)
	}
}
