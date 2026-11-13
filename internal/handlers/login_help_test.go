package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/models"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
)

func TestLoginHelp(t *testing.T) {
	env := setupSharedHandlersEnv(t)
	if err := env.db.AutoMigrate(&models.SupportMessage{}); err != nil {
		t.Fatal(err)
	}
	newApp := func(hosted bool) *fiber.App {
		app := fiber.New()
		app.Use(func(c fiber.Ctx) error {
			tenantctx.SetLocal(c, tenantctx.RequestContext{ID: tenantctx.DefaultTenantID})
			return c.Next()
		})
		handlers.RegisterLoginHelpRoutes(app.Group("/api"), env.db, handlers.HelpOptions{Hosted: hosted, Provider: "clientshare", Recipient: "support@example.invalid", BookingURL: "https://booking.example.invalid"})
		return app
	}
	app := newApp(true)
	res, err := app.Test(jsonAuthRequest("GET", "/api/help/login", "", nil))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	json.NewDecoder(res.Body).Decode(&config)
	res.Body.Close()
	if res.StatusCode != 200 || config["provider"] != "clientshare" {
		t.Fatal(res.StatusCode, config)
	}
	for _, field := range []string{"booking_url", "reply_email", "requester_name", "recipient"} {
		if _, ok := config[field]; ok {
			t.Fatal("public capability leaked", field)
		}
	}
	input := map[string]any{"id": uuid.NewString(), "message": "I cannot sign in", "email": "visitor@example.invalid"}
	post := func(app *fiber.App, want int) map[string]any {
		t.Helper()
		res, err := app.Test(jsonAuthRequest("POST", "/api/help/login/messages", "", input))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var data map[string]any
		json.NewDecoder(res.Body).Decode(&data)
		if res.StatusCode != want {
			t.Fatalf("got %d want %d: %v", res.StatusCode, want, data)
		}
		return data
	}
	first := post(app, 202)
	if second := post(app, 202); second["id"] != first["id"] {
		t.Fatal("duplicate receipt", second)
	}
	var row models.SupportMessage
	if err := env.db.First(&row, "id = ?", first["id"]).Error; err != nil {
		t.Fatal(err)
	}
	if row.Role != "anonymous" || row.Screen != "login" || row.Email != input["email"] || row.TenantID != tenantctx.DefaultTenantID {
		t.Fatal(row)
	}
	input["email"] = "not-an-email"
	post(app, 400)
	input["email"] = "visitor@example.invalid"
	input["requester_id"] = env.adminUser.ID
	post(app, 400)
	delete(input, "requester_id")
	input["message"] = "Changed"
	post(app, 409)
	post(app, 429)
	post(newApp(false), 403)
}
