package web

import (
	"errors"
	"strings"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"go.uber.org/zap"

	sharedmw "github.com/pixelcop/clientshare/pkg/web/middleware"
)

func CreateApp(maxBodyMB int, logger *zap.Logger, useColor bool) *fiber.App {
	if maxBodyMB <= 0 {
		maxBodyMB = 32
	}
	app := fiber.New(fiber.Config{
		ProxyHeader: fiber.HeaderXForwardedFor,
		TrustProxy:  true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			LinkLocal: true,
			Loopback:  true,
			Private:   true,
		},
		BodyLimit: maxBodyMB * 1024 * 1024,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			status := fiber.StatusInternalServerError
			msg := "internal server error"
			if fe, ok := err.(*fiber.Error); ok {
				status = fe.Code
				if fe.Message != "" {
					msg = fe.Message
				}
			}
			if errors.Is(err, fiber.ErrRequestEntityTooLarge) || status == fiber.StatusRequestEntityTooLarge {
				msg = "request body too large"
			}

			c.Response().SetStatusCode(status)
			sharedmw.LogWebRequest(c, nil, nil, false, false, err)

			// These responses won't actually make it back as connection is reset by fiber
			if strings.HasPrefix(c.Path(), "/api") {
				return c.Status(status).JSON(fiber.Map{"error": msg})
			}
			return c.Status(status).SendString(msg)
		},
	})

	if sentry.CurrentHub().Client() != nil {
		sentryHandler := sentryfiber.New(sentryfiber.Options{
			Repanic:         true,
			WaitForDelivery: true,
		})
		app.Use(sentryHandler)
	}

	app.Use(requestid.New())
	app.Use(sharedmw.NewAccessLogger(logger, false, useColor))
	app.Use(compress.New())

	return app
}
