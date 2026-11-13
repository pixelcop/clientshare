package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/gorm"
)

type ReplayRequestHandler struct {
	db  *gorm.DB
	svc *services.ReplayRequestService
}

func NewReplayRequestHandler(db *gorm.DB) *ReplayRequestHandler {
	return &ReplayRequestHandler{db: db, svc: services.NewReplayRequestService(db)}
}

func (h *ReplayRequestHandler) RegisterPublic(router fiber.Router) { router.Get("/config", h.Config) }

func (h *ReplayRequestHandler) RegisterInternal(router fiber.Router) {
	router.Get("/replay-requests", h.List)
	router.Post("/replay-requests", h.Create)
	router.Delete("/replay-requests/:id", h.Revoke)
}

func (h *ReplayRequestHandler) Config(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store, private")
	policy := services.ReplayPolicy{}
	rules, err := h.svc.Active(c.Context())
	if err == nil && len(rules) > 0 {
		request := services.ReplayContext{IP: c.IP(), Hostname: c.Hostname(), TenantID: tenantctx.IDFromFiber(c)}
		// Resolve identity only when a rule can need it. Anonymous clients never
		// supply identity selectors, and invalid sessions remain anonymous.
		needsIdentity := false
		for _, rule := range rules {
			for _, target := range rule.Targets {
				needsIdentity = needsIdentity || target.UserID != "" || target.UserEmail != ""
			}
		}
		if needsIdentity && c.Cookies(auth.SessionCookie) != "" {
			identity, err := (&auth.SessionStore{DB: h.db}).Validate(c.Context(), c.Cookies(auth.SessionCookie), auth.PortalAudience, request.TenantID)
			if err == nil && identity.UserID != "" && identity.TenantID == request.TenantID {
				var user models.User
				if h.db.WithContext(c.Context()).Select("id", "email").Where("tenant_id = ? AND id = ?", request.TenantID, identity.UserID).First(&user).Error == nil {
					request.UserID, request.UserEmail = user.ID, user.Email
				}
			}
		}
		policy = services.MatchReplayRequests(rules, request, time.Now().UTC())
	}
	return c.JSON(fiber.Map{"debug": fiber.Map{"replay": policy}})
}

func (h *ReplayRequestHandler) List(c fiber.Ctx) error {
	rules, err := h.svc.List(c.Context())
	if err != nil {
		return replayRequestError(c, err)
	}
	return c.JSON(fiber.Map{"requests": rules})
}

func (h *ReplayRequestHandler) Create(c fiber.Ctx) error {
	var request models.ReplayRequest
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	rule, err := h.svc.Create(c.Context(), request)
	if err != nil {
		return replayRequestError(c, err)
	}
	return c.Status(201).JSON(rule)
}

func (h *ReplayRequestHandler) Revoke(c fiber.Ctx) error {
	if err := h.svc.Revoke(c.Context(), c.Params("id")); err != nil {
		return replayRequestError(c, err)
	}
	return c.SendStatus(204)
}

func replayRequestError(c fiber.Ctx, err error) error {
	if errors.Is(err, services.ErrReplayRequest) {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.Status(404).JSON(fiber.Map{"error": "replay request not found"})
	}
	return c.Status(503).JSON(fiber.Map{"error": "replay configuration unavailable"})
}
