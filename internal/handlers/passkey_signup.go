package handlers

import (
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
)

func (h *InternalAuthHandler) passkeySignup(c fiber.Ctx, action string) error {
	if h.passkeyAuth == nil || h.passkeyAuth.hostedPasskeyOrigin == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "passkey signup is unavailable"})
	}
	var req struct {
		Email       string          `json:"email"`
		Name        string          `json:"name"`
		TenantSlug  string          `json:"tenant_slug"`
		SignupToken string          `json:"signup_token"`
		Credential  json.RawMessage `json:"credential"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	svc := services.NewPasskeySignupService(h.passkeyAuth.db, h.passkeyAuth.hostedPasskeyOrigin)
	var result any = fiber.Map{"ok": true}
	var err error
	switch action {
	case "options":
		result, err = svc.Begin(c.Context(), req.Email, req.Name, req.TenantSlug)
	case "verify":
		err = svc.Verify(c.Context(), req.SignupToken, req.Credential)
	case "reserve":
		err = svc.Reserve(c.Context(), req.SignupToken, req.Email, req.TenantSlug)
	}
	if errors.Is(err, services.ErrPasskeySignupInvalid) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid or expired passkey signup"})
	}
	if err != nil {
		utils.Logger(c).Error("passkey signup failed", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "passkey signup is unavailable"})
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(result)
}
