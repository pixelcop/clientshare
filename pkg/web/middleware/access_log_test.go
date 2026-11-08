package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestAccessLogURLRedactsCredentials(t *testing.T) {
	for _, input := range []string{
		"/links/bearer-secret?token=query-secret",
		"/api/link/bearer-secret",
		"/api/link/%62earer-secret",
		"https://name:password@portal.example/links/bearer-secret#token=fragment-secret",
		"/accept-access?code=query-secret#token=fragment-secret",
		"https://portal.example/accept-invite#token=fragment-secret",
	} {
		got := accessLogURL(input)
		for _, secret := range []string{"bearer-secret", "query-secret", "fragment-secret", "password"} {
			if strings.Contains(got, secret) {
				t.Fatalf("URL retained a credential: %q", got)
			}
		}
	}
	if got := accessLogURL("/api/files/item-id/download?inline=true"); got != "/api/files/item-id/download" {
		t.Fatalf("lost safe route context: %q", got)
	}
}

func TestAccessLoggerRedactsRequestAndRedirect(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(utils.LoggerContextKey, logger)
		return c.Next()
	})
	app.Use(NewAccessLogger(logger, false, false))
	app.Get("/links/:token", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderLocation, "https://portal.example/accept-access?code=redirect-secret#token=fragment-secret")
		return c.SendStatus(303)
	})
	res, err := app.Test(httptest.NewRequest("GET", "/links/bearer-secret?token=query-secret", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if observed.Len() != 1 {
		t.Fatalf("expected one access log, got %d", observed.Len())
	}
	fields := observed.All()[0].ContextMap()
	req := fields["req"].(map[string]any)
	response := fields["res"].(map[string]any)
	if req["url"] != "/links/%5Bredacted%5D" || response["location"] != "https://portal.example/accept-access" {
		t.Fatalf("unexpected sanitized fields: %#v", fields)
	}
}
