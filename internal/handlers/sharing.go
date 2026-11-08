package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/services"
	"gorm.io/gorm"
)

// RegisterSharingReadRoutes deliberately registers no grant/invitation/link
// creation endpoints. Creation remains unavailable while M4 enforcement and
// lifecycle coverage are incomplete; a hidden Share button is not that gate.
func RegisterSharingReadRoutes(router fiber.Router, db *gorm.DB) {
	handler := &SharingHandler{service: &services.SharingService{DB: db}}
	router.Get("/shared-with-me", middleware.AuthRequired, handler.SharedWithMe)
}

type SharingHandler struct{ service *services.SharingService }

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
