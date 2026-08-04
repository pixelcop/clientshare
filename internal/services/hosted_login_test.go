package services

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHostedLoginRedirectUsesPrimaryTenantDomain(t *testing.T) {
	db := openHostedLoginTestDB(t)
	now := time.Now().UTC()
	if err := db.Create([]models.TenantDomain{
		{TenantID: "tenant-1", Domain: "old.clientshare.example", IsPrimary: false, CreatedAt: now.Add(-time.Hour)},
		{TenantID: "tenant-1", Domain: "ClientShare.Hub.Bixby.io", IsPrimary: true, CreatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed tenant domains: %v", err)
	}

	service := NewHostedLoginService(db, "https://fallback.example.test", "handoff-secret")
	redirectURL, err := service.resolveRedirectURL("tenant-1")
	if err != nil {
		t.Fatalf("resolveRedirectURL() error = %v", err)
	}
	if redirectURL != "https://clientshare.hub.bixby.io" {
		t.Fatalf("redirectURL = %q", redirectURL)
	}
}

func TestHostedLoginRedirectDoesNotConstructDomainFromTenantSlug(t *testing.T) {
	db := openHostedLoginTestDB(t)
	service := NewHostedLoginService(db, "https://clientshare.hub.bixby.io", "handoff-secret")

	_, err := service.resolveRedirectURL("default")
	if !errors.Is(err, ErrHostedLoginRedirectUnavailable) {
		t.Fatalf("resolveRedirectURL() error = %v, want %v", err, ErrHostedLoginRedirectUnavailable)
	}
}

func openHostedLoginTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.TenantDomain{}); err != nil {
		t.Fatalf("migrate tenant domains: %v", err)
	}
	return db
}
