package tenant

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
)

const (
	ModeSingle = "single"
	ModeHosted = "hosted"

	DefaultInternalTargetHeader = "X-ClientShare-Tenant"

	localKey = "tenant"
)

type RequestContext struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	PublicBaseURL string `json:"public_base_url"`
	Resolution    string `json:"resolution"`
}

func (c RequestContext) NormalizedBaseURL(fallback string) string {
	if value := normalizeBaseURL(c.PublicBaseURL); value != "" {
		return value
	}
	return normalizeBaseURL(fallback)
}

func SetLocal(ctx fiber.Ctx, tenantCtx RequestContext) {
	ctx.Locals(localKey, tenantCtx)
	ctx.Locals("tenant_id", tenantCtx.ID)
	ctx.Locals("tenant_slug", tenantCtx.Slug)
	ctx.Locals("tenant_name", tenantCtx.Name)
	ctx.Locals("tenant_base_url", tenantCtx.PublicBaseURL)
	ctx.Locals("tenant_resolution", tenantCtx.Resolution)
	ctx.Locals("tenant_hostname", tenantCtx.Hostname)
}

func FromFiber(ctx fiber.Ctx) (RequestContext, bool) {
	value := ctx.Locals(localKey)
	if value == nil {
		return RequestContext{}, false
	}
	tenantCtx, ok := value.(RequestContext)
	return tenantCtx, ok
}

func IDFromFiber(ctx fiber.Ctx) string {
	tenantCtx, ok := FromFiber(ctx)
	if !ok {
		return ""
	}
	return strings.TrimSpace(tenantCtx.ID)
}

func BaseURLFromFiber(ctx fiber.Ctx, fallback string) string {
	tenantCtx, ok := FromFiber(ctx)
	if ok {
		if value := normalizeBaseURL(tenantCtx.PublicBaseURL); value != "" {
			return value
		}
	}

	if value := normalizeBaseURL(ctx.BaseURL()); value != "" {
		return value
	}

	hostname := strings.TrimSpace(ctx.Hostname())
	if hostname == "" {
		return normalizeBaseURL(fallback)
	}

	scheme := ctx.Scheme()
	return fmt.Sprintf("%s://%s", scheme, hostname)
}
