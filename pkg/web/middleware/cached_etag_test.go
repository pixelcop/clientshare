package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestNewCachedETag_ShortCircuitsCachedMatch(t *testing.T) {
	t.Helper()

	version := "v1"
	handlerCalls := 0
	app := fiber.New()
	app.Get("/asset.txt",
		NewCachedETag(CachedETagConfig{
			Resolve: func(c fiber.Ctx) (CachedETagResource, bool) {
				return CachedETagResource{Key: "asset.txt", Version: version}, true
			},
		}),
		func(c fiber.Ctx) error {
			handlerCalls++
			c.Set(fiber.HeaderETag, `"etag-v1"`)
			c.Set(fiber.HeaderCacheControl, "public, max-age=60")
			return c.SendString("payload")
		},
	)

	firstReq := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	firstResp, err := app.Test(firstReq)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("expected first status 200, got %d", firstResp.StatusCode)
	}
	if handlerCalls != 1 {
		t.Fatalf("expected handler to run once, got %d", handlerCalls)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	secondReq.Header.Set(fiber.HeaderIfNoneMatch, `"etag-v1"`)
	secondResp, err := app.Test(secondReq)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if secondResp.StatusCode != http.StatusNotModified {
		t.Fatalf("expected second status 304, got %d", secondResp.StatusCode)
	}
	if handlerCalls != 1 {
		t.Fatalf("expected cached hit to skip handler, got %d calls", handlerCalls)
	}
	if got := secondResp.Header.Get(fiber.HeaderETag); got != `"etag-v1"` {
		t.Fatalf("expected cached ETag, got %q", got)
	}
	if got := secondResp.Header.Get(fiber.HeaderCacheControl); got != "public, max-age=60" {
		t.Fatalf("expected cached Cache-Control, got %q", got)
	}
}

func TestNewCachedETag_InvalidatesOnVersionChange(t *testing.T) {
	t.Helper()

	version := "v1"
	handlerCalls := 0
	app := fiber.New()
	app.Get("/asset.txt",
		NewCachedETag(CachedETagConfig{
			Resolve: func(c fiber.Ctx) (CachedETagResource, bool) {
				return CachedETagResource{Key: "asset.txt", Version: version}, true
			},
		}),
		func(c fiber.Ctx) error {
			handlerCalls++
			c.Set(fiber.HeaderETag, `"etag-`+version+`"`)
			return c.SendString("payload-" + version)
		},
	)

	firstReq := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	firstResp, err := app.Test(firstReq)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if got := firstResp.Header.Get(fiber.HeaderETag); got != `"etag-v1"` {
		t.Fatalf("expected first ETag, got %q", got)
	}

	version = "v2"
	secondReq := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	secondReq.Header.Set(fiber.HeaderIfNoneMatch, `"etag-v1"`)
	secondResp, err := app.Test(secondReq)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("expected second status 200 after invalidation, got %d", secondResp.StatusCode)
	}
	if handlerCalls != 2 {
		t.Fatalf("expected handler to rerun after invalidation, got %d calls", handlerCalls)
	}
	if got := secondResp.Header.Get(fiber.HeaderETag); got != `"etag-v2"` {
		t.Fatalf("expected second ETag, got %q", got)
	}

	thirdReq := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	thirdReq.Header.Set(fiber.HeaderIfNoneMatch, `"etag-v2"`)
	thirdResp, err := app.Test(thirdReq)
	if err != nil {
		t.Fatalf("third request failed: %v", err)
	}
	if thirdResp.StatusCode != http.StatusNotModified {
		t.Fatalf("expected third status 304, got %d", thirdResp.StatusCode)
	}
	if handlerCalls != 2 {
		t.Fatalf("expected cached hit after refresh, got %d calls", handlerCalls)
	}
}
