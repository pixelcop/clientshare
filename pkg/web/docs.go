package web

import (
	"io/fs"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/etag"
	staticmw "github.com/gofiber/fiber/v3/middleware/static"
)

// SetupDocsServing mounts the built guide before the app's SPA/dev proxy fallback.
// Documentation is public; missing pages and assets must not return the app shell.
func SetupDocsServing(app *fiber.App, docsFS fs.FS) {
	app.Get("/docs", func(c fiber.Ctx) error {
		if c.Path() == "/docs" {
			return c.Redirect().Status(fiber.StatusPermanentRedirect).To("/docs/")
		}
		return c.Next()
	})
	app.Get("/docs/*", etag.New(), staticmw.New("", staticmw.Config{
		FS:         docsFS,
		Browse:     false,
		IndexNames: []string{"index.html"},
		Compress:   true,
		ModifyResponse: func(c fiber.Ctx) error {
			cacheControl := CacheControlDocuments
			if strings.HasPrefix(c.Path(), "/docs/assets/") {
				cacheControl = CacheControlImmutableAssets
			}
			c.Set(fiber.HeaderCacheControl, cacheControl)
			return nil
		},
	}), func(c fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, CacheControlDocuments)
		if body, err := fs.ReadFile(docsFS, "404.html"); err == nil && !strings.HasPrefix(c.Path(), "/docs/assets/") {
			c.Type("html", "utf-8")
			return c.Status(fiber.StatusNotFound).Send(body)
		}
		return c.SendStatus(fiber.StatusNotFound)
	})
}
