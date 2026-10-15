package web

import (
	"io"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func TestDocsServing(t *testing.T) {
	app := fiber.New()
	SetupDocsServing(app, fstest.MapFS{
		"index.html":      {Data: []byte("guide home")},
		"task/start.html": {Data: []byte("guide task")},
		"assets/app.js":   {Data: []byte("console.log('guide')")},
		"404.html":        {Data: []byte("guide not found")},
	})
	app.Get("*", func(c fiber.Ctx) error { return c.SendString("app shell") })
	for _, tc := range []struct {
		path, body, cache string
		status            int
	}{
		{"/docs/", "guide home", CacheControlDocuments, 200},
		{"/docs/task/start.html", "guide task", CacheControlDocuments, 200},
		{"/docs/assets/app.js", "console.log('guide')", CacheControlImmutableAssets, 200},
		{"/docs/missing", "guide not found", CacheControlDocuments, 404},
		{"/docs/assets/missing.js", "Not Found", CacheControlDocuments, 404},
		{"/clients", "app shell", "", 200},
		{"/docs-other", "app shell", "", 200},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", tc.path, nil))
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.status, resp.StatusCode)
			require.Equal(t, tc.body, string(body))
			require.Equal(t, tc.cache, resp.Header.Get("Cache-Control"))
		})
	}

	resp, err := app.Test(httptest.NewRequest("GET", "/docs", nil))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 308, resp.StatusCode)
	require.Equal(t, "/docs/", resp.Header.Get("Location"))

	head, err := app.Test(httptest.NewRequest("HEAD", "/docs/task/start.html", nil))
	require.NoError(t, err)
	defer head.Body.Close()
	require.Equal(t, 200, head.StatusCode)
	body, err := io.ReadAll(head.Body)
	require.NoError(t, err)
	require.Empty(t, body)
}
