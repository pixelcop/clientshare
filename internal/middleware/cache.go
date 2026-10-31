package middleware

import "github.com/gofiber/fiber/v3"

// PrivateNoStore prevents browsers and shared caches from storing sensitive
// responses, including authentication failures and downloads.
func PrivateNoStore(c fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	err := c.Next()
	// File-serving handlers may set their own cache policy. Sensitive routes must
	// remain uncacheable even when a downstream handler supplies a public policy.
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	return err
}
