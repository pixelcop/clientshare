package web

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/gofiber/contrib/v3/sentry"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

func TestCreateAppSentry(t *testing.T) {
	// Do not parallelize: app initialization reads the global Sentry hub.
	hub := sentry.CurrentHub()
	originalClient := hub.Client()
	t.Cleanup(func() { hub.BindClient(originalClient) })

	for _, enabled := range []bool{false, true} {
		name := "without_client"
		if enabled {
			name = "with_client_without_dsn"
		}
		t.Run(name, func(t *testing.T) {
			hub.BindClient(nil)
			if enabled {
				// A client with no DSN still triggers middleware registration.
				client, err := sentry.NewClient(sentry.ClientOptions{Dsn: "", Transport: &sentry.MockTransport{}})
				if err != nil {
					t.Fatal(err)
				}
				hub.BindClient(client)
			}

			middlewareApp := CreateApp(0, zap.NewNop(), false)
			middlewareApp.Get("/sentry-check", func(c fiber.Ctx) error {
				if got := sentryfiber.GetHubFromContext(c) != nil; got != enabled {
					t.Errorf("request Sentry hub present = %v, want %v", got, enabled)
				}
				return c.SendString("OK")
			})
			resp, err := middlewareApp.Test(httptest.NewRequest("GET", "/sentry-check", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != fiber.StatusOK || string(body) != "OK" {
				t.Fatalf("GET /sentry-check = %d %.100q, want 200 OK", resp.StatusCode, body)
			}
		})
	}
}
