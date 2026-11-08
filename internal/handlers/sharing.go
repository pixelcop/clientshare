package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/services"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"gorm.io/gorm"
	"time"
)

// RegisterSharingReadRoutes deliberately registers no grant/invitation/link
// creation endpoints. Creation remains unavailable while M4 enforcement and
// lifecycle coverage are incomplete; a hidden Share button is not that gate.
func RegisterSharingReadRoutes(router fiber.Router, db *gorm.DB, baseURL string) {
	handler := &SharingHandler{service: &services.SharingService{DB: db}, baseURL: baseURL}
	router.Get("/shared-with-me", middleware.AuthRequired, sharingResponse(handler.SharedWithMe))
	router.Get("/files/:id/access", middleware.AuthRequired, sharingResponse(handler.ItemAccess))
	router.Get("/files/:id/links", middleware.AuthRequired, sharingResponse(handler.ItemLinks))
	router.Get("/clients/:id/access", middleware.AuthRequired, sharingResponse(handler.ClientAccess))
}

type SharingHandler struct {
	service     *services.SharingService
	emailQueue  emailpkg.EmailQueue
	baseURL     string
	identityTTL time.Duration
}

func (h *SharingHandler) SharedWithMe(c fiber.Ctx) error {
	req := middleware.AccessRequest(c, "", "", services.AccessContext)
	if req.Actor.UserID == "" || req.Actor.LinkID != "" {
		return fiber.ErrNotFound
	}
	page, pageSize := getPageParams(c)
	result, err := h.service.SharedWithMe(c.Context(), req, page, pageSize)
	if err != nil {
		return fiber.NewError(503, "Access validation is temporarily unavailable")
	}
	return c.JSON(result)
}
