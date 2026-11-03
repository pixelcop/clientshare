package middleware

import (
	"github.com/gofiber/fiber/v3"
	"net/url"
	"strings"
)

// BrowserOriginRequired blocks cross-origin browser mutations, including login
// CSRF and requests from same-site sibling tenant domains. Non-browser clients
// without Origin/Fetch-Metadata remain supported; browsers send Origin on POST.
func BrowserOriginRequired(c fiber.Ctx) error {
	if c.Method() == "GET" || c.Method() == "HEAD" || c.Method() == "OPTIONS" {
		return c.Next()
	}
	origin := c.Get("Origin")
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != c.Scheme() || !strings.EqualFold(u.Host, c.Host()) || u.User != nil {
			return c.Status(403).JSON(fiber.Map{"error": "Cross-origin request rejected"})
		}
	} else if site := c.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return c.Status(403).JSON(fiber.Map{"error": "Cross-origin request rejected"})
	}
	return c.Next()
}
