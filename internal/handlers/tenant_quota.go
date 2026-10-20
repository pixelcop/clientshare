package handlers

import (
	"errors"
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/services"
)

func respondQuotaError(c fiber.Ctx, err error) bool {
	var quota *services.QuotaError
	if !errors.As(err, &quota) {
		return false
	}
	_ = c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": quota.Message, "code": quota.Code})
	return true
}

func (h *InternalTenantsHandler) GetUsage(c fiber.Ctx) error {
	u, err := h.svc.Usage(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Unable to load tenant usage"})
	}
	return c.JSON(u)
}

func (h *SettingsHandler) GetUsage(c fiber.Ctx) error {
	u, err := h.service.Usage(c.Context(), tenantIDFromCtx(c))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Unable to load tenant usage"})
	}
	return c.JSON(u)
}
