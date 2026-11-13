package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/pixelcop/clientshare/pkg/utils"
	"github.com/stretchr/testify/require"
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

func TestAccessLoggerIncludesRequestContext(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core).With(zap.String("commit_hash", "abc123"))
	app := fiber.New()
	app.Use(requestid.New())
	app.Use(NewAccessLogger(logger, false, false))
	app.Use(func(c fiber.Ctx) error {
		c.Locals("tenant_id", "resolved-tenant")
		if c.Path() == "/private" {
			c.Locals("user_id", "verified-user")
		}
		return c.Next()
	})
	app.Get("/*", func(c fiber.Ctx) error { return c.SendStatus(200) })
	for _, path := range []string{"/private", "/public"} {
		req := httptest.NewRequest("GET", "https://portal.example"+path, nil)
		req.Header.Set("User-Agent", "test-browser/1.0")
		req.Header.Set("X-User-ID", "untrusted-user")
		req.Header.Set("X-Tenant-ID", "untrusted-tenant")
		req.Header.Set("Cookie", "clientshare_session=never-log-this")
		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()
		entry := observed.All()[observed.Len()-1]
		fields := entry.ContextMap()
		require.Equal(t, "abc123", fields["commit_hash"])
		require.Equal(t, resp.Header.Get("X-Request-ID"), fields["requestId"])
		require.NotEmpty(t, fields["requestId"])
		require.Equal(t, "resolved-tenant", fields["tenant_id"])
		if path == "/private" {
			require.Equal(t, "verified-user", fields["user_id"])
		} else {
			require.NotContains(t, fields, "user_id")
		}
		request := fields["req"].(map[string]any)
		require.Equal(t, "portal.example", request["hostname"])
		require.Equal(t, "test-browser/1.0", request["userAgent"])
		require.NotContains(t, request, "cookie")
		require.NotContains(t, request, "session_cookie_present")
	}
	require.Equal(t, 2, observed.Len())
}

func TestAccessLoggerOmitsUnknownIdentity(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	app := fiber.New()
	app.Use(NewAccessLogger(zap.New(core), false, false))
	app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(200) })
	req := httptest.NewRequest("GET", "/?user_id=untrusted&tenant_id=untrusted", nil)
	req.Header.Set("X-User-ID", "untrusted")
	req.Header.Set("X-Tenant-ID", "untrusted")
	resp, err := app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 1, observed.Len())
	fields := observed.All()[0].ContextMap()
	require.NotContains(t, fields, "tenant_id")
	require.NotContains(t, fields, "user_id")
}

func TestAccessLogUserAgentIsBoundedUTF8(t *testing.T) {
	for _, value := range []string{"", "Opera/140", strings.Repeat("x", 2048), strings.Repeat("€", 500), "invalid\xff"} {
		got := accessLogUserAgent(value)
		require.LessOrEqual(t, len(got), 1024)
		require.True(t, utf8.ValidString(got))
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
