package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
)

type InternalAuthHandler struct {
	svc *services.HostedLoginService
}

func NewInternalAuthHandler(svc *services.HostedLoginService) *InternalAuthHandler {
	return &InternalAuthHandler{svc: svc}
}

func RegisterInternalAuthRoutes(router fiber.Router, svc *services.HostedLoginService) {
	h := NewInternalAuthHandler(svc)
	router.Post("/auth/hosted-login", h.HostedLogin)
}

func (h *InternalAuthHandler) HostedLogin(c fiber.Ctx) error {
	if h.svc == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "hosted login is unavailable"})
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "email and password are required"})
	}

	result, err := h.svc.Login(req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrHostedLoginInvalidCredentials):
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
		case errors.Is(err, services.ErrHostedLoginAmbiguousEmail):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email matches multiple workspaces"})
		case errors.Is(err, services.ErrHostedLoginTenantInactive):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "workspace is not active"})
		case errors.Is(err, services.ErrHostedLoginRedirectUnavailable):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "workspace redirect is unavailable"})
		default:
			utils.Logger(c).Error("hosted login failed", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
		}
	}

	return c.JSON(result)
}
