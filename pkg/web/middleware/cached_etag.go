package middleware

import (
	"strings"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/utils/v2"
)

// CachedETagResource identifies a cacheable downstream response.
//
// Key must be stable for the logical resource across requests, while Version
// must change whenever that resource's body would produce a different ETag.
// Together they let the middleware reuse a previously emitted validator without
// keying the cache directly by request URL.
type CachedETagResource struct {
	Key     string
	Version string
}

// CachedETagResolver resolves the logical resource behind the current request.
//
// Returning ok=false skips this middleware for the request.
type CachedETagResolver func(c fiber.Ctx) (CachedETagResource, bool)

// CachedETagConfig configures NewCachedETag.
type CachedETagConfig struct {
	// Next skips the middleware when it returns true.
	Next func(c fiber.Ctx) bool
	// Resolve identifies the cacheable resource for the current request.
	Resolve CachedETagResolver
}

type cachedETagEntry struct {
	Version      string
	ETag         string
	CacheControl string
	LastModified string
}

// NewCachedETag returns a middleware that reuses previously observed ETags and
// can short-circuit with 304 Not Modified before downstream response generation.
//
// This middleware is intended to sit in front of Fiber's standard etag
// middleware, not replace it. On a cache miss it calls the next handler,
// captures the emitted ETag and related cache headers, and stores them in an
// in-memory cache scoped to this middleware instance. On a hit it compares the
// request's If-None-Match header against the cached ETag and returns 304 before
// downstream handlers run.
//
// The cache is keyed by CachedETagResource.Key and guarded by
// CachedETagResource.Version. If the version changes, the cached validator is
// treated as stale and the downstream chain is allowed to regenerate the
// response and its ETag.
func NewCachedETag(config CachedETagConfig) fiber.Handler {
	var (
		mu      sync.RWMutex
		entries = make(map[string]cachedETagEntry)
	)

	return func(c fiber.Ctx) error {
		if config.Next != nil && config.Next(c) {
			return c.Next()
		}

		if config.Resolve == nil {
			return c.Next()
		}

		method := c.Method()
		if method != fiber.MethodGet && method != fiber.MethodHead {
			return c.Next()
		}

		resource, ok := config.Resolve(c)
		if !ok || resource.Key == "" || resource.Version == "" {
			return c.Next()
		}

		ifNoneMatch := c.Get(fiber.HeaderIfNoneMatch)
		if ifNoneMatch != "" {
			mu.RLock()
			entry, found := entries[resource.Key]
			mu.RUnlock()

			if found && entry.Version == resource.Version && etagMatches(ifNoneMatch, entry.ETag) {
				restoreCachedETagHeaders(c, entry)
				return c.SendStatus(fiber.StatusNotModified)
			}
		}

		if err := c.Next(); err != nil {
			return err
		}

		if c.Response().StatusCode() != fiber.StatusOK {
			return nil
		}

		etag := utils.CopyString(c.GetRespHeader(fiber.HeaderETag))
		if etag == "" {
			return nil
		}

		updatedResource, ok := config.Resolve(c)
		if !ok || updatedResource.Key != resource.Key || updatedResource.Version == "" || updatedResource.Version != resource.Version {
			return nil
		}

		mu.Lock()
		entries[updatedResource.Key] = cachedETagEntry{
			Version:      updatedResource.Version,
			ETag:         etag,
			CacheControl: utils.CopyString(c.GetRespHeader(fiber.HeaderCacheControl)),
			LastModified: utils.CopyString(c.GetRespHeader(fiber.HeaderLastModified)),
		}
		mu.Unlock()

		return nil
	}
}

func restoreCachedETagHeaders(c fiber.Ctx, entry cachedETagEntry) {
	c.Set(fiber.HeaderETag, entry.ETag)
	if entry.CacheControl != "" {
		c.Set(fiber.HeaderCacheControl, entry.CacheControl)
	}
	if entry.LastModified != "" {
		c.Set(fiber.HeaderLastModified, entry.LastModified)
	}
}

// etagMatches mirrors the comparison behavior used by Fiber's ETag middleware
// closely enough for cache hits to preserve the same conditional request
// semantics.
func etagMatches(ifNoneMatch string, serverETag string) bool {
	if ifNoneMatch == "" || serverETag == "" {
		return false
	}

	if strings.TrimSpace(ifNoneMatch) == "*" {
		return true
	}

	if strings.HasPrefix(ifNoneMatch, "W/") {
		if strings.TrimPrefix(ifNoneMatch, "W/") == serverETag {
			return true
		}
		if strings.HasPrefix(serverETag, "W/") && strings.TrimPrefix(ifNoneMatch, "W/") == strings.TrimPrefix(serverETag, "W/") {
			return true
		}
		return false
	}

	return strings.Contains(ifNoneMatch, serverETag)
}
