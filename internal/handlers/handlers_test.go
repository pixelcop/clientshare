package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/models"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestApp() (*fiber.App, *gorm.DB) {
	db, _ := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	db.AutoMigrate(&models.User{}, &models.UserClient{}, &models.InviteToken{})
	app := fiber.New()
	// Middleware to inject admin role for all requests
	app.Use(func(c fiber.Ctx) error {
		tenantctx.SetLocal(c, tenantctx.RequestContext{ID: tenantctx.DefaultTenantID, Slug: tenantctx.DefaultTenantSlug, Name: tenantctx.DefaultTenantName, Resolution: "test"})
		c.Locals("role", "admin")
		return c.Next()
	})
	h := NewUsersHandler(db, nil, "", "secret", "reset-secret", 72*time.Hour, 30*time.Minute, nil)
	app.Get("/api/users", h.ListUsersHandler)
	app.Post("/api/users", h.CreateUserHandler)
	app.Put("/api/users/:id", h.UpdateUserHandler)
	app.Delete("/api/users/:id", h.DeleteUserHandler)
	return app, db
}

func TestUserCRUDLifecycle(t *testing.T) {
	app, db := setupTestApp()
	db.Create(&models.User{Email: "admin@x.com", PasswordHash: "x", Role: "admin", Name: "Admin", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	user := map[string]interface{}{"email": "bob@x.com", "role": "manager", "name": "Bob"}
	body, _ := json.Marshal(user)
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 201 {
		t.Fatalf("create user failed: %d", resp.StatusCode)
	}

	respBody := make([]byte, resp.ContentLength)
	resp.Body.Read(respBody)
	var user2 models.User
	json.Unmarshal(respBody, &user2)
	assert.NotEmpty(t, user2.ID)

	req = httptest.NewRequest(http.MethodGet, "/api/users", nil)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("list users failed: %d", resp.StatusCode)
	}

	patch := map[string]interface{}{"name": "Bobby"}
	body, _ = json.Marshal(patch)
	req = httptest.NewRequest(http.MethodPut, "/api/users/"+user2.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("update user failed: %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/users/"+user2.ID, nil)
	resp, _ = app.Test(req)
	if resp.StatusCode != 204 {
		t.Fatalf("delete user failed: %d", resp.StatusCode)
	}
}
