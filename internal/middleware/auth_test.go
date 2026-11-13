package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	sharedmw "github.com/pixelcop/clientshare/pkg/web/middleware"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuthRequiredAccessLogOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, cookie, outcome string
		store, migrate        bool
		status                int
	}{
		{"missing cookie", "", "missing_cookie", false, false, 401},
		{"missing store", "opaque-token", "validation_unavailable", false, false, 503},
		{"malformed session", "opaque-token", "invalid_session", true, true, 401},
		{"unknown session", strings.Repeat("a", 43), "invalid_session", true, true, 401},
		{"database error", strings.Repeat("a", 43), "validation_unavailable", true, false, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)
			app := fiber.New()
			app.Use(sharedmw.NewAccessLogger(zap.New(core), false, false))
			if tc.store {
				db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
				require.NoError(t, err)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				t.Cleanup(func() { sqlDB.Close() })
				if tc.migrate {
					require.NoError(t, db.AutoMigrate(&models.BrowserSession{}))
				}
				app.Use(func(c fiber.Ctx) error {
					c.Locals("session_store", &auth.SessionStore{DB: db})
					return c.Next()
				})
			}
			app.Get("/api/auth/me", AuthRequired, func(c fiber.Ctx) error {
				t.Error("unauthenticated request reached handler")
				return c.SendStatus(200)
			})
			req := httptest.NewRequest("GET", "/api/auth/me", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: tc.cookie})
			}
			resp, err := app.Test(req)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, tc.status, resp.StatusCode)
			require.Equal(t, 1, logs.Len())
			fields := logs.All()[0].ContextMap()
			require.Equal(t, tc.outcome, fields["auth_outcome"])
			require.NotContains(t, fields, "user_id")
		})
	}
}
