package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestPrivateNoStore(t *testing.T) {
	for _, status := range []int{200, 302, 401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			app := fiber.New()
			app.Use(PrivateNoStore)
			app.Get("/", func(c fiber.Ctx) error {
				c.Set(fiber.HeaderCacheControl, "public, max-age=3600")
				if status == 500 {
					return fiber.ErrInternalServerError
				}
				return c.SendStatus(status)
			})
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, status)
			}
			if got := resp.Header.Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control = %q, want private, no-store", got)
			}
		})
	}
}
