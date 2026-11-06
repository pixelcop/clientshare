package handlers

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
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
	for _, action := range []string{"options", "verify", "reserve"} {
		router.Post("/auth/hosted-signup/passkey/"+action, func(c fiber.Ctx) error { return h.passkeySignup(c, action) })
	}
	router.Post("/auth/hosted-login/passkey/options", h.HostedPasskeyOptions)
	router.Post("/auth/hosted-login/passkey/verify", h.HostedPasskeyVerify)
	router.Post("/auth/hosted-login", h.HostedLogin)
	router.Post("/auth/hosted-email/request", h.HostedEmailRequest)
	router.Post("/auth/hosted-email/complete", h.HostedEmailComplete)
	router.Post("/auth/sessions/validate", h.ValidateAccountSession)
	router.Post("/auth/sessions/revoke", h.RevokeAccountSession)
}

func (h *InternalAuthHandler) HostedEmailRequest(c fiber.Ctx) error {
	if h.svc == nil {
		return c.SendStatus(503)
	}
	var req struct {
		Email       string `json:"email"`
		Purpose     string `json:"purpose"`
		Destination string `json:"destination"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.SendStatus(400)
	}
	result, err := h.svc.IssueEmailToken(c.Context(), req.Email, req.Purpose, req.Destination)
	if err != nil {
		return hostedLoginError(c, err)
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(result)
}

func (h *InternalAuthHandler) HostedEmailComplete(c fiber.Ctx) error {
	if h.svc == nil {
		return c.SendStatus(503)
	}
	var req struct {
		Token    string `json:"token"`
		Purpose  string `json:"purpose"`
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.SendStatus(400)
	}
	if req.Purpose == services.HostedEmailReset && (len(req.Password) < 12 || len(req.Password) > 72) {
		return c.Status(400).JSON(fiber.Map{"error": "Password must contain 12 to 72 bytes"})
	}
	result, destination, err := h.svc.CompleteEmailToken(c.Context(), req.Token, req.Purpose, req.Password)
	if err != nil {
		if errors.Is(err, services.ErrHostedEmailTokenInvalid) {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid or expired email link"})
		}
		return hostedLoginError(c, err)
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"login": result, "destination": destination})
}

func (h *InternalAuthHandler) ValidateAccountSession(c fiber.Ctx) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.SendStatus(400)
	}
	identity, err := auth.Store(c).Validate(c.Context(), req.Token, auth.AccountAudience, "")
	if err != nil {
		if errors.Is(err, auth.ErrSessionInvalid) {
			return c.SendStatus(401)
		}
		return c.SendStatus(503)
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(identity)
}

func (h *InternalAuthHandler) RevokeAccountSession(c fiber.Ctx) error {
	var req struct {
		Token string `json:"token"`
		Scope string `json:"scope"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.SendStatus(400)
	}
	store := auth.Store(c)
	if req.Scope == "all" {
		identity, err := store.Validate(c.Context(), req.Token, auth.AccountAudience, "")
		if err != nil && !errors.Is(err, auth.ErrSessionInvalid) {
			return c.SendStatus(503)
		}
		if err == nil {
			if err := store.RevokeUser(c.Context(), identity.TenantID, identity.UserID); err != nil {
				return c.SendStatus(503)
			}
		}
	}
	if err := store.Revoke(c.Context(), req.Token, auth.AccountAudience, req.Scope == "group"); err != nil {
		return c.SendStatus(503)
	}
	return c.SendStatus(204)
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
