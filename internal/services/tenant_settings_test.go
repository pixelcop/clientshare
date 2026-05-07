package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTenantSettingsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.TenantSettings{}, &models.TenantDomain{}); err != nil {
		t.Fatalf("failed to migrate tenant settings tables: %v", err)
	}
	return db
}

func TestTenantSettingsService_UpdateSyncsAndClearsPublicBaseURLDomain(t *testing.T) {
	db := setupTenantSettingsTestDB(t)
	service := NewTenantSettingsService(db)

	publicBaseURL := "https://Portal.Example.Test/"
	settings, err := service.Update(context.Background(), tenantctx.DefaultTenantID, TenantSettingsUpdate{PublicBaseURL: &publicBaseURL})
	if err != nil {
		t.Fatalf("failed updating tenant settings: %v", err)
	}
	if settings.PublicBaseURL != "https://Portal.Example.Test" {
		t.Fatalf("expected normalized public base url, got %q", settings.PublicBaseURL)
	}

	var domain models.TenantDomain
	if err := db.Where("tenant_id = ?", tenantctx.DefaultTenantID).First(&domain).Error; err != nil {
		t.Fatalf("expected synchronized tenant domain: %v", err)
	}
	if domain.Domain != "portal.example.test" {
		t.Fatalf("expected tenant domain hostname portal.example.test, got %q", domain.Domain)
	}
	if domain.Kind != tenantctx.DomainKindPublicBaseURL {
		t.Fatalf("expected tenant domain kind %q, got %q", tenantctx.DomainKindPublicBaseURL, domain.Kind)
	}

	emptyPublicBaseURL := "  "
	settings, err = service.Update(context.Background(), tenantctx.DefaultTenantID, TenantSettingsUpdate{PublicBaseURL: &emptyPublicBaseURL})
	if err != nil {
		t.Fatalf("failed clearing tenant public base url: %v", err)
	}
	if settings.PublicBaseURL != "" {
		t.Fatalf("expected cleared public base url, got %q", settings.PublicBaseURL)
	}

	var count int64
	if err := db.Model(&models.TenantDomain{}).Where("tenant_id = ?", tenantctx.DefaultTenantID).Count(&count).Error; err != nil {
		t.Fatalf("failed counting tenant domains: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected derived tenant domain to be removed, found %d rows", count)
	}
}

func TestTenantSettingsService_UpdateRejectsDuplicatePublicBaseURLHost(t *testing.T) {
	db := setupTenantSettingsTestDB(t)
	service := NewTenantSettingsService(db)
	now := time.Now()
	if err := db.Create(&models.TenantDomain{TenantID: "other-tenant", Domain: "portal.example.test", Kind: tenantctx.DomainKindPublicBaseURL, IsPrimary: true, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("failed seeding conflicting tenant domain: %v", err)
	}

	publicBaseURL := "https://portal.example.test"
	_, err := service.Update(context.Background(), tenantctx.DefaultTenantID, TenantSettingsUpdate{PublicBaseURL: &publicBaseURL})
	if err == nil {
		t.Fatal("expected duplicate tenant public base url error")
	}
	if err != ErrTenantPublicBaseURLConflict {
		t.Fatalf("expected ErrTenantPublicBaseURLConflict, got %v", err)
	}

	settings, getErr := service.Get(context.Background(), tenantctx.DefaultTenantID)
	if getErr != nil {
		t.Fatalf("failed loading tenant settings after conflict: %v", getErr)
	}
	if settings.PublicBaseURL != "" {
		t.Fatalf("expected failed update to leave public base url empty, got %q", settings.PublicBaseURL)
	}
}
