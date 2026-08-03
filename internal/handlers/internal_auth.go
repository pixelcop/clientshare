package handlers

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
)

type InternalAuthHandler struct {
	svc         *services.HostedLoginService
	passkeyAuth *AuthHandler
}

func NewInternalAuthHandler(svc *services.HostedLoginService, passkeyAuth ...*AuthHandler) *InternalAuthHandler {
	h := &InternalAuthHandler{svc: svc}
	if len(passkeyAuth) > 0 {
		h.passkeyAuth = passkeyAuth[0]
	}
	return h
}

func RegisterInternalAuthRoutes(router fiber.Router, svc *services.HostedLoginService, passkeyAuth ...*AuthHandler) {
	h := NewInternalAuthHandler(svc, passkeyAuth...)
	router.Post("/auth/hosted-login/passkey/options", h.HostedPasskeyOptions)
	router.Post("/auth/hosted-login/passkey/verify", h.HostedPasskeyVerify)
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
		return hostedLoginError(c, err)
	}

	return c.JSON(result)
}

func (h *InternalAuthHandler) HostedPasskeyOptions(c fiber.Ctx) error {
	if h.svc == nil || h.passkeyAuth == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "hosted passkey login is unavailable"})
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Email) == "" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "email is required"})
	}
	result, err := h.passkeyAuth.BeginHostedPasskeyLogin(c, h.svc, req.Email)
	if err != nil {
		return hostedPasskeyOptionsError(c, err)
	}
	return c.JSON(result)
}

func (h *InternalAuthHandler) HostedPasskeyVerify(c fiber.Ctx) error {
	if h.svc == nil || h.passkeyAuth == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "hosted passkey login is unavailable"})
	}
	var req struct {
		Email       string          `json:"email"`
		ChallengeID string          `json:"challenge_id"`
		Credential  json.RawMessage `json:"credential"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.ChallengeID) == "" || len(req.Credential) == 0 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "email, challenge_id, and credential are required"})
	}
	result, err := h.passkeyAuth.FinishHostedPasskeyLogin(c, h.svc, req.Email, req.ChallengeID, req.Credential)
	if err != nil {
		if isHostedLoginError(err) {
			return hostedLoginError(c, err)
		}
		if errors.Is(err, errHostedPasskeyVerificationFailed) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "passkey sign in failed"})
		}
		utils.Logger(c).Error("hosted passkey sign-in failed", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "passkey sign in is unavailable"})
	}
	return c.JSON(result)
}

func hostedLoginError(c fiber.Ctx, err error) error {
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

func isHostedLoginError(err error) bool {
	return errors.Is(err, services.ErrHostedLoginInvalidCredentials) ||
		errors.Is(err, services.ErrHostedLoginAmbiguousEmail) ||
		errors.Is(err, services.ErrHostedLoginTenantInactive) ||
		errors.Is(err, services.ErrHostedLoginRedirectUnavailable)
}

func hostedPasskeyOptionsError(c fiber.Ctx, err error) error {
	if isHostedLoginError(err) {
		return hostedLoginError(c, err)
	}
	utils.Logger(c).Error("failed beginning hosted passkey login", zap.Error(err))
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "passkey sign in is unavailable"})
}
