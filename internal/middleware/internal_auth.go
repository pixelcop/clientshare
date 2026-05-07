package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// InternalAuthRequired returns middleware that validates a shared secret.
// Accept both "Authorization: Bearer <secret>" and "X-Internal-Token: <secret>".
func InternalAuthRequired(secret string) fiber.Handler {
	secretBytes := []byte(strings.TrimSpace(secret))
	return func(c fiber.Ctx) error {
		token := strings.TrimPrefix(strings.TrimSpace(c.Get("Authorization")), "Bearer ")
		if token == "" {
			token = strings.TrimSpace(c.Get("X-Internal-Token"))
		}
		if len(token) == 0 || subtle.ConstantTimeCompare([]byte(token), secretBytes) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}
}
