package services

import (
	"testing"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTenantProvisioningServiceCreateAdminUserSetsPasswordHashWithoutInvite(t *testing.T) {
	db := openTenantProvisioningTestDB(t)
	svc := NewTenantProvisioningService(db, "invite-secret", 0, nil, "https://app.test")
	passwordHash, err := auth.HashPassword("super-secret-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	user, err := svc.CreateAdminUser("tenant-123", CreateAdminUserInput{
		Email:        "owner@example.test",
		Name:         "Owner",
		PasswordHash: passwordHash,
	})
	if err != nil {
		t.Fatalf("CreateAdminUser() error = %v", err)
	}
	if user.PasswordHash == "" {
		t.Fatal("PasswordHash = empty, want bcrypt hash")
	}
	if user.PasswordHash != passwordHash {
		t.Fatal("PasswordHash mismatch, want stored hash to match provided hash")
	}
	if !auth.CheckPasswordHash("super-secret-password", user.PasswordHash) {
		t.Fatal("stored password hash does not match original password")
	}

	var inviteCount int64
	if err := db.Model(&models.InviteToken{}).Count(&inviteCount).Error; err != nil {
		t.Fatalf("count invite tokens: %v", err)
	}
	if inviteCount != 0 {
		t.Fatalf("inviteCount = %d, want 0", inviteCount)
	}

	var auditCount int64
	if err := db.Model(&models.AuditEvent{}).Where("action = ?", "admin_user.created").Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("auditCount = %d, want 1", auditCount)
	}
}

func TestTenantProvisioningServiceCheckAvailabilityDetectsExistingSlugAndDomain(t *testing.T) {
	db := openTenantProvisioningTestDB(t)
	svc := NewTenantProvisioningService(db, "invite-secret", 0, nil, "https://app.test")

	if err := db.Create(&models.Tenant{ID: "tenant-1", Slug: "acme", Name: "Acme"}).Error; err != nil {
		t.Fatalf("Create(tenant) error = %v", err)
	}
	if err := db.Create(&models.TenantDomain{ID: "domain-1", TenantID: "tenant-1", Domain: "portal.example.test", Kind: "custom"}).Error; err != nil {
		t.Fatalf("Create(domain) error = %v", err)
	}

	result, err := svc.CheckAvailability("acme", "portal.example.test")
	if err != nil {
		t.Fatalf("CheckAvailability() error = %v", err)
	}
	if result.Available {
		t.Fatal("Available = true, want false")
	}
	if result.Reason != "slug_taken" {
		t.Fatalf("Reason = %q, want slug_taken", result.Reason)
	}

	result, err = svc.CheckAvailability("fresh", "portal.example.test")
	if err != nil {
		t.Fatalf("CheckAvailability() error = %v", err)
	}
	if result.Available {
		t.Fatal("Available = true, want false for existing domain")
	}
	if result.Reason != "domain_taken" {
		t.Fatalf("Reason = %q, want domain_taken", result.Reason)
	}

	result, err = svc.CheckAvailability("fresh", "fresh.example.test")
	if err != nil {
		t.Fatalf("CheckAvailability() error = %v", err)
	}
	if !result.Available {
		t.Fatalf("Available = false, want true (reason %q)", result.Reason)
	}
}

func openTenantProvisioningTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.InviteToken{}, &models.AuditEvent{}, &models.Tenant{}, &models.TenantDomain{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	return db
}
