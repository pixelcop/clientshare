package clientshare

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/tenant"
)

type docsSiteLink struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

type docsSiteConfig struct {
	SiteLink *docsSiteLink `json:"site_link"`
}

func (cfg *Config) docsSiteLink(publicURL string) *docsSiteLink {
	settings := cfg.Server.Docs.SiteLink
	if settings.Enabled != nil && !*settings.Enabled {
		return nil
	}
	url := strings.TrimSpace(settings.URL)
	label := strings.TrimSpace(settings.Label)
	if url == "" {
		if strings.EqualFold(strings.TrimSpace(cfg.Tenancy.Mode), tenant.ModeHosted) {
			return nil
		}
		url = strings.TrimSpace(publicURL)
		if url == "" {
			url = strings.TrimSpace(cfg.Server.BaseURL)
		}
		if url == "" || !validDocsLinkURL(url) {
			url = "/"
		}
		if label == "" {
			label = "Back to portal"
		}
	} else if label == "" {
		label = "Website"
	}
	return &docsSiteLink{URL: url, Label: label}
}

func registerDocsSiteConfig(app *fiber.App, cfg *Config, settings *services.TenantSettingsService) {
	app.Get("/docs/site-config.json", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		var publicURL string
		linkConfig := cfg.Server.Docs.SiteLink
		automatic := strings.TrimSpace(linkConfig.URL) == "" && (linkConfig.Enabled == nil || *linkConfig.Enabled)
		if automatic && !strings.EqualFold(strings.TrimSpace(cfg.Tenancy.Mode), tenant.ModeHosted) {
			// Single-tenant mode always uses the default tenant; no caller-supplied
			// host, referrer, or return URL is used to choose this destination.
			current, err := settings.Get(c.Context(), tenant.DefaultTenantID)
			if err != nil {
				return c.Status(fiber.StatusServiceUnavailable).JSON(docsSiteConfig{})
			}
			publicURL = current.PublicBaseURL
		}
		return c.JSON(docsSiteConfig{SiteLink: cfg.docsSiteLink(publicURL)})
	})
}
