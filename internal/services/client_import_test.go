package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/storage"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recordingEmailSender struct {
	emails []emailpkg.Email
}

func (s *recordingEmailSender) Send(email emailpkg.Email) error {
	s.emails = append(s.emails, email)
	return nil
}

func setupImportTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.Client{}, &models.File{}, &models.FileActivity{}); err != nil {
		t.Fatalf("failed to migrate client tables: %v", err)
	}
	return db, func() {}
}

func TestClientImportService_ImportCSV(t *testing.T) {
	db, cleanup := setupImportTestDB(t)
	defer cleanup()
	if err := db.AutoMigrate(&models.User{}, &models.UserClient{}, &models.InviteToken{}); err != nil {
		t.Fatalf("failed to migrate user tables: %v", err)
	}

	tmpDir := t.TempDir()
	store := storage.NewLocalStorage(tmpDir)
	clientService := &ClientService{DB: db, Storage: store}
	tenantID := tenantctx.DefaultTenantID
	acme, err := clientService.CreateClient(context.Background(), tenantID, "Acme")
	if err != nil {
		t.Fatalf("failed to create seed client: %v", err)
	}
	_ = acme
	other, err := clientService.CreateClient(context.Background(), tenantID, "Other")
	if err != nil {
		t.Fatalf("failed to create other client: %v", err)
	}

	otherHash, err := auth.HashPassword("other-pass")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	existingUser := models.User{TenantID: tenantID, Email: "client@other.test", PasswordHash: otherHash, Role: "client", Name: "Other Client", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&existingUser).Error; err != nil {
		t.Fatalf("failed to create existing user: %v", err)
	}
	if err := db.Create(&models.UserClient{TenantID: tenantID, UserID: existingUser.ID, ClientID: other.ID, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("failed to create seed user-client assignment: %v", err)
	}

	sender := &recordingEmailSender{}
	queue := emailpkg.NewInMemoryEmailQueue(sender, time.Minute)
	importService := &ClientImportService{
		DB:          db,
		Storage:     store,
		EmailQueue:  queue,
		TenantID:    tenantID,
		BaseURL:     "https://frontend.local",
		TokenSecret: "invite-secret",
		InviteTTL:   72 * time.Hour,
	}

	result, err := importService.ImportCSV(strings.NewReader(strings.Join([]string{
		"CLIENTNAME,EMAIL",
		"Acme,new-user@test.local",
		"Acme,second-user@test.local",
		"Beta,new-user@test.local",
		"Beta,new-user@test.local",
		"Beta,client@other.test",
		"Gamma,",
		"Beta,",
	}, "\n")), ClientImportOptions{InvitedBy: "cli-import"})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.RowsRead != 7 {
		t.Fatalf("expected 7 rows read, got %d", result.RowsRead)
	}
	if result.CreatedClients != 2 {
		t.Fatalf("expected 2 created clients, got %d", result.CreatedClients)
	}
	if result.ExistingClients != 1 {
		t.Fatalf("expected 1 existing client, got %d", result.ExistingClients)
	}
	if result.CreatedUsers != 2 {
		t.Fatalf("expected 2 created users, got %d", result.CreatedUsers)
	}
	if result.UpdatedUsers != 1 {
		t.Fatalf("expected 1 updated user, got %d", result.UpdatedUsers)
	}
	if result.InvitedUsers != 2 {
		t.Fatalf("expected 2 invited users, got %d", result.InvitedUsers)
	}
	if len(result.InviteEmailIDs) != 2 {
		t.Fatalf("expected 2 queued invite email ids, got %d", len(result.InviteEmailIDs))
	}

	for _, inviteEmailID := range result.InviteEmailIDs {
		if err := queue.SendNow(tenantID, inviteEmailID); err != nil {
			t.Fatalf("failed to send queued invite email: %v", err)
		}
	}
	if len(sender.emails) != 2 {
		t.Fatalf("expected 2 sent invite emails, got %d", len(sender.emails))
	}
	recipients := []string{sender.emails[0].To, sender.emails[1].To}
	if !(contains(recipients, "new-user@test.local") && contains(recipients, "second-user@test.local")) {
		t.Fatalf("expected invites sent to both new users, got %v", recipients)
	}

	assertClientExistsByName(t, db, "Beta")
	assertClientExistsByName(t, db, "Gamma")

	assertUserAssignments(t, db, "new-user@test.local", 2)
	assertUserAssignments(t, db, "second-user@test.local", 1)
	assertUserAssignments(t, db, "client@other.test", 2)

	var invites []models.InviteToken
	if err := db.Where("tenant_id = ? AND email IN ?", tenantID, []string{"new-user@test.local", "second-user@test.local"}).Find(&invites).Error; err != nil {
		t.Fatalf("failed to load invite tokens: %v", err)
	}
	if len(invites) != 2 {
		t.Fatalf("expected 2 invite tokens for imported users, got %d", len(invites))
	}
	for _, invite := range invites {
		if invite.InvitedBy != "cli-import" {
			t.Fatalf("expected invited_by cli-import, got %q", invite.InvitedBy)
		}
	}
}

func TestClientImportService_ImportCSV_MultipleEmailsPerRow(t *testing.T) {
	db, cleanup := setupImportTestDB(t)
	defer cleanup()
	if err := db.AutoMigrate(&models.User{}, &models.UserClient{}, &models.InviteToken{}); err != nil {
		t.Fatalf("failed to migrate user tables: %v", err)
	}

	importService := &ClientImportService{
		DB:          db,
		Storage:     storage.NewLocalStorage(t.TempDir()),
		EmailQueue:  emailpkg.NewInMemoryEmailQueue(&recordingEmailSender{}, time.Minute),
		TenantID:    tenantctx.DefaultTenantID,
		BaseURL:     "https://frontend.local",
		TokenSecret: "invite-secret",
		InviteTTL:   72 * time.Hour,
	}

	result, err := importService.ImportCSV(strings.NewReader(strings.Join([]string{
		"CLIENTNAME,EMAIL",
		"Foobar,\"user1@gmail.com, user2@example.com, user3@bar.com\"",
		"No Email,",
	}, "\n")), ClientImportOptions{InvitedBy: "cli-import"})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.RowsRead != 2 {
		t.Fatalf("expected 2 rows read, got %d", result.RowsRead)
	}
	if result.CreatedClients != 2 {
		t.Fatalf("expected 2 created clients, got %d", result.CreatedClients)
	}
	if result.CreatedUsers != 3 {
		t.Fatalf("expected 3 created users, got %d", result.CreatedUsers)
	}
	if result.InvitedUsers != 3 {
		t.Fatalf("expected 3 invited users, got %d", result.InvitedUsers)
	}

	assertClientExistsByName(t, db, "Foobar")
	assertClientExistsByName(t, db, "No Email")
	assertUserAssignments(t, db, "user1@gmail.com", 1)
	assertUserAssignments(t, db, "user2@example.com", 1)
	assertUserAssignments(t, db, "user3@bar.com", 1)
	assertNoUserByEmail(t, db, "")
	if len(result.InviteEmailIDs) != 3 {
		t.Fatalf("expected 3 queued invite emails, got %d", len(result.InviteEmailIDs))
	}
	var inviteCount int64
	if err := db.Model(&models.InviteToken{}).Count(&inviteCount).Error; err != nil {
		t.Fatalf("failed to count invite tokens: %v", err)
	}
	if inviteCount != 3 {
		t.Fatalf("expected 3 invite tokens, got %d", inviteCount)
	}
}

func assertClientExistsByName(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	var client models.Client
	if err := db.Where("tenant_id = ? AND name = ?", tenantctx.DefaultTenantID, name).First(&client).Error; err != nil {
		t.Fatalf("expected client %s to exist: %v", name, err)
	}
}

func assertUserAssignments(t *testing.T, db *gorm.DB, email string, expected int) {
	t.Helper()
	var user models.User
	if err := db.Where("tenant_id = ? AND email = ?", tenantctx.DefaultTenantID, email).First(&user).Error; err != nil {
		t.Fatalf("expected user %s to exist: %v", email, err)
	}
	var assignments []models.UserClient
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantctx.DefaultTenantID, user.ID).Find(&assignments).Error; err != nil {
		t.Fatalf("failed to load assignments for %s: %v", email, err)
	}
	if len(assignments) != expected {
		t.Fatalf("expected %d assignments for %s, got %d", expected, email, len(assignments))
	}
}

func assertNoUserByEmail(t *testing.T, db *gorm.DB, email string) {
	t.Helper()
	var count int64
	if err := db.Model(&models.User{}).Where("tenant_id = ? AND email = ?", tenantctx.DefaultTenantID, email).Count(&count).Error; err != nil {
		t.Fatalf("failed to count users for %q: %v", email, err)
	}
	if count != 0 {
		t.Fatalf("expected no user for %q, got %d", email, count)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
