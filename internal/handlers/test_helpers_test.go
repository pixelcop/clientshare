package handlers_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/clientshare"
	"github.com/pixelcop/clientshare/internal/db"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/storage"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type memoryEmailQueue struct {
	items  []*models.QueuedEmail
	nextID int
}

var testStorageRoot string

const (
	handlersTestPublicBaseURL = "https://frontend.local"
	handlersTestPublicDomain  = "frontend.local"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "clientshare-tests-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create test temp dir:", err)
		os.Exit(1)
	}
	testStorageRoot = root
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}

func (q *memoryEmailQueue) Pending(tenantID, clientID string) []*models.QueuedEmail {
	pending := make([]*models.QueuedEmail, 0, len(q.items))
	for _, item := range q.items {
		if item.SentAt != nil {
			continue
		}
		if tenantID != "" && item.TenantID != tenantID {
			continue
		}
		if clientID == "" || (item.ClientID != nil && *item.ClientID == clientID) {
			pending = append(pending, item)
		}
	}
	return pending
}

func (q *memoryEmailQueue) Queue(emailItem email.Email, sendAt *time.Time) (*models.QueuedEmail, error) {
	scheduled := time.Now()
	if sendAt != nil {
		scheduled = *sendAt
	}
	item := &models.QueuedEmail{
		TenantID:     emailItem.TenantID,
		ID:           ids.NewULID(),
		Recipient:    emailItem.To,
		Subject:      emailItem.Subject,
		Body:         emailItem.Body,
		FileIDsCSV:   emailItem.FileIDsCSV,
		ClientID:     emailItem.ClientID,
		EventType:    "test",
		ScheduledFor: scheduled,
	}
	if item.TenantID == "" {
		item.TenantID = tenantctx.DefaultTenantID
	}
	q.items = append(q.items, item)
	return item, nil
}

func (q *memoryEmailQueue) Update(tenantID, id, recipient, subject, body string) error {
	for _, item := range q.items {
		if item.ID == id && (tenantID == "" || item.TenantID == tenantID) {
			item.Recipient = recipient
			item.Subject = subject
			item.Body = body
			return nil
		}
	}
	return fmt.Errorf("queued email not found")
}

func (q *memoryEmailQueue) SendNow(tenantID, id string) error {
	for _, item := range q.items {
		if item.ID == id && (tenantID == "" || item.TenantID == tenantID) {
			now := time.Now()
			item.SentAt = &now
			return nil
		}
	}
	return fmt.Errorf("queued email not found")
}

func (q *memoryEmailQueue) ProcessTenant(tenantID string) error {
	now := time.Now()
	for _, item := range q.items {
		if item.SentAt == nil && item.TenantID == tenantID {
			item.SentAt = &now
		}
	}
	return nil
}

func (q *memoryEmailQueue) ProcessBatch() error {
	return q.ProcessAll()
}

func (q *memoryEmailQueue) ProcessAll() error {
	now := time.Now()
	for _, item := range q.items {
		if item.SentAt == nil {
			item.SentAt = &now
		}
	}
	return nil
}

type handlersTestEnv struct {
	app             *fiber.App
	db              *gorm.DB
	storageRoot     string
	emailQueue      *memoryEmailQueue
	queue           *memoryEmailQueue
	signingKey      string
	client          *models.Client
	otherClient     *models.Client
	rootFolder      *models.File
	adminUser       *models.User
	managerUser     *models.User
	clientUser      *models.User
	mismatchUser    *models.User
	otherClientUser *models.User
	adminToken      string
	managerToken    string
	clientToken     string
	mismatchToken   string
	otherToken      string
	linkToken       string
}

type fileTestEnv = handlersTestEnv
type otherHandlersEnv = handlersTestEnv

type handlersTestOptions struct {
	passwordResetDuration string
	inviteTokenDuration   string
	disableAuthRateLimit  bool
	tenancyMode           string
}

func setupSharedHandlersEnv(t *testing.T) *handlersTestEnv {
	return setupSharedHandlersEnvWithOptions(t, handlersTestOptions{disableAuthRateLimit: true})
}

func setupSharedHandlersEnvWithOptions(t *testing.T, options handlersTestOptions) *handlersTestEnv {
	t.Helper()

	storageRoot, err := os.MkdirTemp(testStorageRoot, "handlers-test-*")
	if err != nil {
		t.Fatalf("failed to create temp storage: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(storageRoot)
	})

	database, err := gorm.Open(sqlite.Open(memoryDSN(t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := database.AutoMigrate(
		&models.Tenant{},
		&models.TenantSettings{},
		&models.TenantDomain{},
		&models.TenantEntitlement{},
		&models.User{},
		&models.UserClient{},
		&models.Client{},
		&models.File{},
		&models.FileActivity{},
		&models.FeedEvent{},
		&models.QueuedEmail{},
		&models.SecureLink{},
		&models.PasswordResetToken{},
		&models.InviteToken{},
	); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}
	seedDefaultTenantState(t, database)
	db.SetDB(database)

	clientService := &services.ClientService{DB: database, Storage: storage.NewLocalStorage(storageRoot)}
	tenantID := tenantctx.DefaultTenantID
	client, err := clientService.CreateClient(context.Background(), tenantID, "Acme")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	otherClient, err := clientService.CreateClient(context.Background(), tenantID, "Other")
	if err != nil {
		t.Fatalf("failed to create other client: %v", err)
	}

	rootFolder := &models.File{}
	if err := database.Where("id = ?", *client.RootFolderID).First(rootFolder).Error; err != nil {
		t.Fatalf("failed to load root folder: %v", err)
	}

	adminHash, _ := auth.HashPassword("admin-pass")
	managerHash, _ := auth.HashPassword("manager-pass")
	clientHash, _ := auth.HashPassword("client-pass")
	mismatchHash, _ := auth.HashPassword("mismatch-pass")
	otherHash, _ := auth.HashPassword("other-pass")

	adminUser := &models.User{TenantID: tenantID, Email: "admin@test.local", PasswordHash: adminHash, Role: "admin", Name: "Admin", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	managerUser := &models.User{TenantID: tenantID, Email: "manager@test.local", PasswordHash: managerHash, Role: "manager", Name: "Manager", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	clientUser := &models.User{TenantID: tenantID, Email: "client@test.local", PasswordHash: clientHash, Role: "client", Name: "Client", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	mismatchUser := &models.User{TenantID: tenantID, Email: "mismatch@acme.test", PasswordHash: mismatchHash, Role: "client", Name: "Mismatch", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	otherClientUser := &models.User{TenantID: tenantID, Email: "client@other.test", PasswordHash: otherHash, Role: "client", Name: "Other Client", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	for _, user := range []*models.User{adminUser, managerUser, clientUser, mismatchUser, otherClientUser} {
		if err := database.Create(user).Error; err != nil {
			t.Fatalf("failed to create user %s: %v", user.Email, err)
		}
	}

	assignments := []models.UserClient{
		{TenantID: tenantID, UserID: managerUser.ID, ClientID: client.ID, CreatedAt: time.Now()},
		{TenantID: tenantID, UserID: clientUser.ID, ClientID: client.ID, CreatedAt: time.Now()},
		{TenantID: tenantID, UserID: mismatchUser.ID, ClientID: client.ID, CreatedAt: time.Now()},
		{TenantID: tenantID, UserID: otherClientUser.ID, ClientID: otherClient.ID, CreatedAt: time.Now()},
	}
	for _, rel := range assignments {
		if err := database.Create(&rel).Error; err != nil {
			t.Fatalf("failed to create user-client assignment: %v", err)
		}
	}

	jwtSecret := "reset-secret"
	auth.SetJWTSecret(jwtSecret)

	adminToken, _ := auth.GenerateJWT(adminUser.ID, "admin", tenantID)
	managerToken, _ := auth.GenerateJWT(managerUser.ID, "manager", tenantID)
	clientToken, _ := auth.GenerateJWT(clientUser.ID, "client", tenantID)
	mismatchToken, _ := auth.GenerateJWT(mismatchUser.ID, "client", tenantID)
	otherToken, _ := auth.GenerateJWT(otherClientUser.ID, "client", tenantID)
	linkToken, _ := auth.GenerateLinkJWT(client.ID, "999", "view", tenantID)

	queue := &memoryEmailQueue{}
	signingKey := "test-signing-key"
	cfg := &clientshare.Config{}
	cfg.DevMode = true
	cfg.Server.MaxBodyMB = 8
	cfg.Server.BaseURL = handlersTestPublicBaseURL
	cfg.Tenancy.Mode = tenantctx.ModeSingle
	if options.tenancyMode != "" {
		cfg.Tenancy.Mode = options.tenancyMode
	}
	cfg.Storage.Local.Root = storageRoot
	cfg.SecureLinks.SigningKey = signingKey
	cfg.Auth.JWTSecret = jwtSecret
	cfg.Auth.InviteTokenSecret = "invite-secret"
	cfg.Auth.PasswordResetDuration = "30m"
	if options.passwordResetDuration != "" {
		cfg.Auth.PasswordResetDuration = options.passwordResetDuration
	}
	cfg.Auth.InviteTokenDuration = "72h"
	if options.inviteTokenDuration != "" {
		cfg.Auth.InviteTokenDuration = options.inviteTokenDuration
	}
	if options.disableAuthRateLimit {
		disableAuthRateLimit := false
		cfg.Auth.RateLimitEnabled = &disableAuthRateLimit
	}
	cfg.Branding.SiteTitle = "ClientShare"
	cfg.Branding.PrimaryColor = "#112233"

	app, err := clientshare.NewWebApp(cfg, database, zap.NewNop(), queue, storage.NewLocalStorage(storageRoot))
	if err != nil {
		t.Fatalf("failed to create web app: %v", err)
	}

	return &handlersTestEnv{
		app:             app,
		db:              database,
		storageRoot:     storageRoot,
		emailQueue:      queue,
		queue:           queue,
		signingKey:      signingKey,
		client:          client,
		otherClient:     otherClient,
		rootFolder:      rootFolder,
		adminUser:       adminUser,
		managerUser:     managerUser,
		clientUser:      clientUser,
		mismatchUser:    mismatchUser,
		otherClientUser: otherClientUser,
		adminToken:      adminToken,
		managerToken:    managerToken,
		clientToken:     clientToken,
		mismatchToken:   mismatchToken,
		otherToken:      otherToken,
		linkToken:       linkToken,
	}
}

func setupFileTestEnv(t *testing.T) *fileTestEnv {
	return setupSharedHandlersEnv(t)
}

func setupOtherHandlersEnv(t *testing.T) *otherHandlersEnv {
	return setupSharedHandlersEnv(t)
}

func setupOtherHandlersEnvWithOptions(t *testing.T, options handlersTestOptions) *otherHandlersEnv {
	return setupSharedHandlersEnvWithOptions(t, options)
}

func memoryDSN(t *testing.T) string {
	name := strings.ReplaceAll(t.Name(), "/", "_")
	return fmt.Sprintf("file:%s?mode=memory&cache=shared", name)
}

func seedDefaultTenantState(t *testing.T, database *gorm.DB) {
	t.Helper()

	if err := database.Create(&models.Tenant{
		ID:   tenantctx.DefaultTenantID,
		Slug: tenantctx.DefaultTenantSlug,
		Name: tenantctx.DefaultTenantName,
	}).Error; err != nil {
		t.Fatalf("failed to seed default tenant: %v", err)
	}

	if err := database.Create(&models.TenantSettings{
		TenantID:                    tenantctx.DefaultTenantID,
		SiteTitle:                   "ClientShare",
		PrimaryColor:                "#112233",
		PublicBaseURL:               handlersTestPublicBaseURL,
		SecureLinkDefaultExpiryDays: 90,
	}).Error; err != nil {
		t.Fatalf("failed to seed default tenant settings: %v", err)
	}

	if err := database.Create(&models.TenantDomain{
		TenantID:  tenantctx.DefaultTenantID,
		Domain:    handlersTestPublicDomain,
		Kind:      tenantctx.DomainKindPublicBaseURL,
		IsPrimary: true,
	}).Error; err != nil {
		t.Fatalf("failed to seed default tenant domain: %v", err)
	}

	if err := database.Create(&models.TenantEntitlement{
		TenantID: tenantctx.DefaultTenantID,
		Status:   "active",
		PlanCode: "self_hosted",
	}).Error; err != nil {
		t.Fatalf("failed to seed default tenant entitlement: %v", err)
	}
}

func jsonRequest(method, path string, body any) *http.Request {
	var payload io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		payload = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req
}

func jsonAuthRequest(method, path, token string, body any) *http.Request {
	req := jsonRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func intToString(id any) string {
	return fmt.Sprint(id)
}

func resetTokenHash(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
