package handlers_test

import (
	"encoding/json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"testing"
)

func TestHelpAuthorizationAndReceipt(t *testing.T) {
	env := setupSharedHandlersEnv(t)
	if err := env.db.AutoMigrate(&models.SupportMessage{}); err != nil {
		t.Fatal(err)
	}
	newApp := func(hosted bool, provider string) *fiber.App {
		app := fiber.New()
		app.Use(func(c fiber.Ctx) error {
			tenantctx.SetLocal(c, tenantctx.RequestContext{ID: tenantctx.DefaultTenantID})
			return c.Next()
		})
		api := app.Group("/api", middleware.AuthRequired)
		handlers.RegisterHelpRoutes(api, env.db, handlers.HelpOptions{Hosted: hosted, Provider: provider, GuideURL: "/custom-guide/", Recipient: "support@example.invalid", OperatorURL: "https://operator.invalid", OperatorLabel: "Contact portal operator", Version: "test-version"})
		return app
	}
	for _, tc := range []struct {
		name, token, provider, want string
		hosted                      bool
		status                      int
	}{
		{"administrator", env.adminToken, "clientshare", "clientshare", true, 202},
		{"manager", env.managerToken, "clientshare", "clientshare", true, 202},
		{"client", env.clientToken, "clientshare", "none", true, 403},
		{"anonymous", "", "clientshare", "", true, 401},
		{"self-hosted", env.adminToken, "none", "none", false, 403},
		{"operator", env.adminToken, "operator", "operator", false, 403},
		{"single cannot enable commercial", env.adminToken, "clientshare", "none", false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newApp(tc.hosted, tc.provider)
			res, err := app.Test(jsonAuthRequest("GET", "/api/help", tc.token, nil))
			if err != nil {
				t.Fatal(err)
			}
			if tc.token != "" {
				var config map[string]any
				json.NewDecoder(res.Body).Decode(&config)
				if config["provider"] != tc.want || config["guide_url"] != "/custom-guide/" {
					t.Fatal(config)
				}
				if _, ok := config["recipient"]; ok {
					t.Fatal("destination leaked")
				}
			}
			res.Body.Close()
			res, err = app.Test(jsonAuthRequest("POST", "/api/help/messages", tc.token, map[string]any{"id": uuid.NewString(), "message": "Help me configure my portal", "screen": "/files/private?token=secret"}))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.status {
				t.Fatalf("status %d, want %d", res.StatusCode, tc.status)
			}
		})
	}
	app := newApp(true, "clientshare")
	input := map[string]any{"id": uuid.NewString(), "message": "Retry me", "screen": "settings"}
	var receipt string
	for i := 0; i < 2; i++ {
		res, err := app.Test(jsonAuthRequest("POST", "/api/help/messages", env.adminToken, input))
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		json.NewDecoder(res.Body).Decode(&data)
		res.Body.Close()
		if res.StatusCode != 202 {
			t.Fatal(data)
		}
		if i == 1 && data["id"] != receipt {
			t.Fatal("duplicate receipt")
		}
		receipt = data["id"].(string)
	}
	var row models.SupportMessage
	if err := env.db.First(&row, "id = ?", receipt).Error; err != nil {
		t.Fatal(err)
	}
	if row.TenantID != env.adminUser.TenantID || row.RequesterID != env.adminUser.ID || row.Email != env.adminUser.Email || row.Version != "test-version" {
		t.Fatal(row)
	}
	input["message"] = "Changed"
	check := func(token string, want int) {
		t.Helper()
		res, err := app.Test(jsonAuthRequest("POST", "/api/help/messages", token, input))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("status %d want %d", res.StatusCode, want)
		}
	}
	check(env.adminToken, 409)
	input["tenant_id"] = "spoofed"
	check(env.adminToken, 400)
	delete(input, "tenant_id")
	cross, _ := auth.GenerateJWT(env.adminUser.ID, "admin", "other-tenant")
	check(cross, 401)
	env.db.Model(env.managerUser).Update("role", "client")
	check(env.managerToken, 403)
	for i := 0; i < 10; i++ {
		input["id"] = uuid.NewString()
		res, _ := app.Test(jsonAuthRequest("POST", "/api/help/messages", env.adminToken, input))
		res.Body.Close()
	}
	check(env.adminToken, 429)
	// A new limiter lets us exercise storage failure independently of throttling.
	app = newApp(true, "clientshare")
	env.db.Migrator().DropTable(&models.SupportMessage{})
	check(env.adminToken, 503)
}
