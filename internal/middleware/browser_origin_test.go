package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestBrowserOriginRequired(t *testing.T) {
	app := fiber.New()
	app.Use(BrowserOriginRequired)
	app.Post("/api/login", func(c fiber.Ctx) error { return c.SendStatus(204) })
	for _, tc := range []struct {
		origin string
		status int
	}{
		{"http://portal.test", 204},
		{"http://sibling.test", 403},
		{"https://portal.test", 403},
		{"null", 403},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://portal.test/api/login", nil)
			req.Header.Set("Origin", tc.origin)
			res, err := app.Test(req)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, tc.status, res.StatusCode)
		})
	}
}
