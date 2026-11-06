package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

func hostedEmailTestService(t *testing.T) (*HostedLoginService, models.User) {
	t.Helper()
	db := openHostedLoginTestDB(t)
	if err := db.AutoMigrate(&models.Tenant{}, &models.TenantSettings{}, &models.TenantEntitlement{}, &models.User{}, &models.HostedEmailToken{}, &models.HostedLoginHandoff{}, &models.BrowserSession{}, &models.PasswordResetToken{}, &models.WebAuthnChallenge{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Tenant{ID: "email-tenant", Slug: "email-test", Name: "Email test"}).Error; err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("old-password-123")
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{TenantID: "email-tenant", Email: "owner@example.test", Name: "Owner", Role: "admin", PasswordHash: hash}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TenantDomain{TenantID: user.TenantID, Domain: "portal.example.test", IsPrimary: true}).Error; err != nil {
		t.Fatal(err)
	}
	return NewHostedLoginService(db, "https://example.test", "email-test-secret"), user
}

func TestHostedEmailLoginSingleUseAndPurposeBinding(t *testing.T) {
	for _, destination := range []string{"portal", "account"} {
		t.Run(destination, func(t *testing.T) {
			s, user := hostedEmailTestService(t)
			ctx := context.Background()
			delivery, err := s.IssueEmailToken(ctx, " OWNER@EXAMPLE.TEST ", HostedEmailLogin, destination)
			if err != nil || len(delivery.Token) != 43 {
				t.Fatalf("issue: %v %+v", err, delivery)
			}
			var record models.HostedEmailToken
			if err := s.db.First(&record).Error; err != nil {
				t.Fatal(err)
			}
			if record.TokenHash == delivery.Token || record.CredentialHash == user.PasswordHash {
				t.Fatal("unhashed credentials stored")
			}
			if _, _, err := s.CompleteEmailToken(ctx, delivery.Token, HostedEmailReset, "new-password-123"); !errors.Is(err, ErrHostedEmailTokenInvalid) {
				t.Fatalf("wrong purpose accepted: %v", err)
			}
			result, target, err := s.CompleteEmailToken(ctx, delivery.Token, HostedEmailLogin, "")
			if err != nil || target != destination || result == nil || result.UserID != user.ID || result.HandoffCode == "" || result.AccountSession == "" {
				t.Fatalf("complete: %v %s %+v", err, target, result)
			}
			if _, err := (&auth.SessionStore{DB: s.db}).Validate(ctx, result.AccountSession, auth.AccountAudience, ""); err != nil {
				t.Fatalf("account session: %v", err)
			}
			if _, err := s.ConsumeHandoff(user.TenantID, result.HandoffCode); err != nil {
				t.Fatalf("portal handoff: %v", err)
			}
			if _, _, err := s.CompleteEmailToken(ctx, delivery.Token, HostedEmailLogin, ""); !errors.Is(err, ErrHostedEmailTokenInvalid) {
				t.Fatalf("replay accepted: %v", err)
			}
		})
	}
}

func TestHostedEmailCooldownAndReplacement(t *testing.T) {
	s, user := hostedEmailTestService(t)
	ctx := context.Background()
	first, err := s.IssueEmailToken(ctx, user.Email, HostedEmailLogin, "portal")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.IssueEmailToken(ctx, user.Email, HostedEmailLogin, "account")
	if err != nil || second.Token != "" {
		t.Fatalf("cooldown: %v %+v", err, second)
	}
	if err := s.db.Model(&models.HostedEmailToken{}).Where("user_id = ?", user.ID).Update("created_at", time.Now().Add(-2*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	second, err = s.IssueEmailToken(ctx, user.Email, HostedEmailLogin, "account")
	if err != nil || second.Token == "" || first.Token == second.Token {
		t.Fatalf("replacement: %v %+v", err, second)
	}
	if _, _, err := s.CompleteEmailToken(ctx, first.Token, HostedEmailLogin, ""); !errors.Is(err, ErrHostedEmailTokenInvalid) {
		t.Fatalf("old token accepted: %v", err)
	}
	if _, destination, err := s.CompleteEmailToken(ctx, second.Token, HostedEmailLogin, ""); err != nil || destination != "account" {
		t.Fatalf("replacement completion: %s %v", destination, err)
	}
}

func TestHostedEmailRejectsExpiredOrChangedIdentity(t *testing.T) {
	for _, mutation := range []string{"expired", "password", "email", "deleted", "suspended"} {
		t.Run(mutation, func(t *testing.T) {
			s, user := hostedEmailTestService(t)
			ctx := context.Background()
			delivery, err := s.IssueEmailToken(ctx, user.Email, HostedEmailLogin, "portal")
			if err != nil {
				t.Fatal(err)
			}
			var change *gorm.DB
			switch mutation {
			case "expired":
				change = s.db.Model(&models.HostedEmailToken{}).Where("user_id = ?", user.ID).Update("expires_at", time.Now().Add(-time.Minute))
			case "password":
				change = s.db.Model(&models.User{}).Where("id = ?", user.ID).Update("password_hash", "changed")
			case "email":
				change = s.db.Model(&models.User{}).Where("id = ?", user.ID).Update("email", "changed@example.test")
			case "deleted":
				change = s.db.Delete(&user)
			case "suspended":
				change = s.db.Create(&models.TenantEntitlement{TenantID: user.TenantID, Status: "suspended"})
			}
			if change.Error != nil {
				t.Fatal(change.Error)
			}
			if _, _, err := s.CompleteEmailToken(ctx, delivery.Token, HostedEmailLogin, ""); !errors.Is(err, ErrHostedEmailTokenInvalid) {
				t.Fatalf("%s token accepted: %v", mutation, err)
			}
		})
	}
}

func TestHostedEmailResetRevokesSessionsAndOutstandingTokens(t *testing.T) {
	s, user := hostedEmailTestService(t)
	ctx := context.Background()
	old, err := s.Login(user.Email, "old-password-123")
	if err != nil {
		t.Fatal(err)
	}
	magic, err := s.IssueEmailToken(ctx, user.Email, HostedEmailLogin, "portal")
	if err != nil {
		t.Fatal(err)
	}
	reset, err := s.IssueEmailToken(ctx, user.Email, HostedEmailReset, "account")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CompleteEmailToken(ctx, reset.Token, HostedEmailReset, "short"); err == nil {
		t.Fatal("short password accepted")
	}
	result, destination, err := s.CompleteEmailToken(ctx, reset.Token, HostedEmailReset, "new-password-123")
	if err != nil || result != nil || destination != "account" {
		t.Fatalf("reset: %v %s %+v", err, destination, result)
	}
	if _, err := s.Login(user.Email, "old-password-123"); err == nil {
		t.Fatal("old password works")
	}
	if _, err := s.Login(user.Email, "new-password-123"); err != nil {
		t.Fatalf("new password: %v", err)
	}
	if _, err := (&auth.SessionStore{DB: s.db}).Validate(ctx, old.AccountSession, auth.AccountAudience, ""); err == nil {
		t.Fatal("old session still active")
	}
	if _, err := s.ConsumeHandoff(user.TenantID, old.HandoffCode); err == nil {
		t.Fatal("old handoff still active")
	}
	if _, _, err := s.CompleteEmailToken(ctx, magic.Token, HostedEmailLogin, ""); err == nil {
		t.Fatal("outstanding login link works")
	}
	if _, _, err := s.CompleteEmailToken(ctx, reset.Token, HostedEmailReset, "another-password"); err == nil {
		t.Fatal("reset replay works")
	}
}

func TestHostedEmailUnknownAmbiguousAndUnsafeDestination(t *testing.T) {
	s, user := hostedEmailTestService(t)
	ctx := context.Background()
	delivery, err := s.IssueEmailToken(ctx, "missing@example.test", HostedEmailLogin, "portal")
	if err != nil || delivery.Token != "" {
		t.Fatalf("unknown: %v %+v", err, delivery)
	}
	if err := s.db.Create(&models.Tenant{ID: "other-tenant", Slug: "other", Name: "Other"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&models.User{TenantID: "other-tenant", Email: user.Email, Role: "client"}).Error; err != nil {
		t.Fatal(err)
	}
	delivery, err = s.IssueEmailToken(ctx, user.Email, HostedEmailLogin, "portal")
	if err != nil || delivery.Token != "" {
		t.Fatalf("ambiguous: %v %+v", err, delivery)
	}
	if _, err := s.IssueEmailToken(ctx, user.Email, HostedEmailLogin, "https://evil.test"); err == nil {
		t.Fatal("arbitrary destination accepted")
	}
	if _, _, err := s.CompleteEmailToken(ctx, strings.Repeat("x", 43), HostedEmailLogin, ""); !errors.Is(err, ErrHostedEmailTokenInvalid) {
		t.Fatalf("random token: %v", err)
	}
}
