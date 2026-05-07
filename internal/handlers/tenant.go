package handlers

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
)

func tenantIDFromCtx(c fiber.Ctx) string {
	return tenantctx.IDFromFiber(c)
}

func tenantBaseURLFromCtx(c fiber.Ctx, fallback string) string {
	return tenantctx.BaseURLFromFiber(c, fallback)
}

func effectiveTenantBaseURL(c fiber.Ctx, configuredBaseURL, fallback string) string {
	if value := normalizeAbsoluteURL(configuredBaseURL); value != "" {
		return value
	}
	return tenantctx.BaseURLFromFiber(c, fallback)
}

func normalizeAbsoluteURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}
