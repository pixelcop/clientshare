package web

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/etag"
	"github.com/gofiber/fiber/v3/middleware/proxy"
	staticmw "github.com/gofiber/fiber/v3/middleware/static"
	"github.com/pixelcop/clientshare/pkg/web/middleware"
	"go.uber.org/zap"
)

const (
	CacheControlImmutableAssets = "public, max-age=31536000, immutable" // 1 year
	CacheControlRootAssets      = "public, max-age=86400"               // 1 day
	CacheControlDocuments       = "no-cache, must-revalidate"
)

// SetupStaticFileServing configures the Fiber app to serve static files.
// In dev mode, it proxies all requests to the Vite dev server (with an optional fallback to ./public).
// In production, it serves embedded static assets from the provided fs.FS, with support for user-provided assets in ./public that take precedence over embedded ones.
//
// webFS should be an fs.FS containing the embedded frontend assets (if in prod mode)
func SetupStaticFileServing(app *fiber.App, devMode bool, vitePort int, webFS fs.FS) error {
	if devMode {
		return staticDevProxy(app, vitePort)
	}
	return staticProd(app, webFS)
}

func staticDevProxy(app *fiber.App, vitePort int) error {
	if vitePort == 0 {
		vitePort = 5173
	}
	viteURL := fmt.Sprintf("http://127.0.0.1:%d", vitePort)

	if info, err := os.Stat("public"); err == nil && info.IsDir() {
		zap.L().Info("serving public assets from ./public")
		app.Use("/*", staticmw.New("./public", staticmw.Config{Browse: false}))
	}

	// Proxy all requests to Vite (including websockets)
	app.Use(func(c fiber.Ctx) error {
		return proxy.Do(c, viteURL+c.OriginalURL())
	})

	return nil
}

func staticProd(app *fiber.App, webFS fs.FS) error {
	// Serve user-provided public assets from disk
	if info, err := os.Stat("public"); err == nil && info.IsDir() {
		zap.L().Info("serving public assets from ./public")
		app.Get("/*",
			middleware.NewCachedETag(middleware.CachedETagConfig{Resolve: diskStaticETagResolver("public", "public:")}),
			etag.New(),
			staticmw.New("./public", staticmw.Config{Browse: false, Compress: true, MaxAge: 86400}),
		)
	}

	staticRouteSkip := func(c fiber.Ctx) bool {
		path := c.Path()
		return path == "/" || strings.HasPrefix(path, "/api")
	}

	app.Get("/*",
		middleware.NewCachedETag(middleware.CachedETagConfig{
			Next:    staticRouteSkip,
			Resolve: fsStaticETagResolver(webFS, "embedded:"),
		}),
		etag.New(),
		staticmw.New("", staticmw.Config{
			FS:         webFS,
			Browse:     false,
			IndexNames: []string{"index.html"},
			Compress:   true,
			ModifyResponse: func(c fiber.Ctx) error {
				c.Set(fiber.HeaderCacheControl, cacheControlForProdSPAPath(c.Path()))
				return nil
			},
			Next: staticRouteSkip,
		}))

	registerPrerenderedHTMLRoutes(app, webFS)

	// fallback for any non-API paths to serve index.html (for client-side routing)
	app.Get("*",
		middleware.NewCachedETag(middleware.CachedETagConfig{Resolve: spaFallbackETagResolver(webFS)}),
		etag.New(),
		staticmw.New("index.html", staticmw.Config{
			FS:       webFS,
			Compress: true,
			Next: func(c fiber.Ctx) bool {
				// make sure to serve 404 instead of fallback
				path := c.Path()
				return strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/assets")
			},
			ModifyResponse: func(c fiber.Ctx) error {
				c.Set(fiber.HeaderCacheControl, CacheControlDocuments)
				return nil
			},
		}),
	)

	return nil
}

func cacheControlForProdSPAPath(path string) string {
	if strings.HasPrefix(path, "/assets/") {
		return CacheControlImmutableAssets
	}

	if isRootLevelStaticAsset(path) {
		return CacheControlRootAssets
	}

	return CacheControlDocuments
}

func isRootLevelStaticAsset(path string) bool {
	if path == "/site.webmanifest" {
		return true
	}

	if !strings.HasPrefix(path, "/") {
		return false
	}

	relativePath := strings.TrimPrefix(path, "/")
	if relativePath == "" || strings.Contains(relativePath, "/") {
		return false
	}

	lowerPath := strings.ToLower(relativePath)
	return strings.HasSuffix(lowerPath, ".png") || strings.HasSuffix(lowerPath, ".ico")
}

func diskStaticETagResolver(root string, keyPrefix string) middleware.CachedETagResolver {
	return func(c fiber.Ctx) (middleware.CachedETagResource, bool) {
		relativePath, ok := cacheableStaticRequestPath(c.Path())
		if !ok {
			return middleware.CachedETagResource{}, false
		}

		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(relativePath)))
		if err != nil || info.IsDir() {
			return middleware.CachedETagResource{}, false
		}

		return middleware.CachedETagResource{
			Key:     keyPrefix + relativePath,
			Version: staticResourceVersion(info),
		}, true
	}
}

func fsStaticETagResolver(filesystem fs.FS, keyPrefix string) middleware.CachedETagResolver {
	return func(c fiber.Ctx) (middleware.CachedETagResource, bool) {
		relativePath, ok := cacheableStaticRequestPath(c.Path())
		if !ok {
			return middleware.CachedETagResource{}, false
		}

		info, err := fs.Stat(filesystem, relativePath)
		if err != nil || info.IsDir() {
			return middleware.CachedETagResource{}, false
		}

		return middleware.CachedETagResource{
			Key:     keyPrefix + relativePath,
			Version: staticResourceVersion(info),
		}, true
	}
}

func spaFallbackETagResolver(filesystem fs.FS) middleware.CachedETagResolver {
	return func(c fiber.Ctx) (middleware.CachedETagResource, bool) {
		if strings.HasPrefix(c.Path(), "/api") {
			return middleware.CachedETagResource{}, false
		}

		info, err := fs.Stat(filesystem, "index.html")
		if err != nil || info.IsDir() {
			return middleware.CachedETagResource{}, false
		}

		return middleware.CachedETagResource{
			Key:     "embedded:index.html",
			Version: staticResourceVersion(info),
		}, true
	}
}

func cacheableStaticRequestPath(requestPath string) (string, bool) {
	if requestPath == "" {
		return "", false
	}

	normalizedPath := strings.ReplaceAll(requestPath, "\\", "/")
	parts := strings.Split(strings.TrimPrefix(normalizedPath, "/"), "/")
	for _, part := range parts {
		if part == ".." {
			return "", false
		}
	}

	cleanPath := path.Clean("/" + strings.TrimPrefix(normalizedPath, "/"))
	relativePath := strings.TrimPrefix(cleanPath, "/")
	if relativePath == "" || relativePath == "." {
		return "", false
	}

	return relativePath, true
}

func staticResourceVersion(info fs.FileInfo) string {
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UTC().UnixNano())
}

// registerPrerenderedHTMLRoutes registers routes for serving prerendered HTML files for specific
// paths (e.g. /pricing, /docs) in production mode.
//
// For example, when accessing /pricing, try serving either /pricing.html or /pricing/index.html
// from the embedded filesystem. This allows us to have SEO-friendly, prerendered pages for
// important routes while still serving the rest of the app as a SPA.
func registerPrerenderedHTMLRoutes(app *fiber.App, webFS fs.FS) {
	app.Get("*", func(c fiber.Ctx) error {
		htmlPath, ok := prerenderedHTMLPath(webFS, c.Path())
		if !ok {
			return c.Next()
		}

		body, err := fs.ReadFile(webFS, htmlPath)
		if err != nil {
			return c.Next()
		}

		c.Set(fiber.HeaderCacheControl, CacheControlDocuments)
		c.Type("html", "utf-8")
		return c.Send(body)
	})
}

func prerenderedHTMLPath(webFS fs.FS, requestPath string) (string, bool) {
	if strings.HasPrefix(requestPath, "/api") || requestPath == "/" || requestPath == "" {
		return "", false
	}

	normalizedPath := strings.ReplaceAll(requestPath, "\\", "/")
	cleanPath := path.Clean("/" + strings.TrimPrefix(normalizedPath, "/"))
	relativePath := strings.TrimPrefix(cleanPath, "/")
	if relativePath == "" || relativePath == "." || path.Ext(relativePath) != "" {
		return "", false
	}

	candidates := []string{
		relativePath + ".html",
		path.Join(relativePath, "index.html"),
	}

	for _, candidate := range candidates {
		info, err := fs.Stat(webFS, candidate)
		if err == nil && !info.IsDir() {
			return candidate, true
		}
	}

	return "", false
}
