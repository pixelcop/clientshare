package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/services"
	"gorm.io/gorm"
)

type RecipientsHandler struct{ service *services.RecipientService }

func NewRecipientsHandler(db *gorm.DB) *RecipientsHandler {
	return &RecipientsHandler{service: &services.RecipientService{DB: db}}
}

func RegisterRecipientRoutes(router fiber.Router, db *gorm.DB) {
	h := NewRecipientsHandler(db)
	router.Get("/recipients", middleware.AuthRequired, h.Search)
}

func (h *RecipientsHandler) Search(c fiber.Ctx) error {
	scope := c.Query("scope", "client")
	kind := c.Query("kind")
	if (scope != "client" && scope != "all") || len(c.Query("search")) > 200 ||
		(kind != "" && (kind != "workspace_client" || scope != "client")) {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid recipient search"})
	}
	req := middleware.AccessRequest(c, strings.TrimSpace(c.Query("client_id")), "", services.AccessContext)
	var contacts []services.RecipientContact
	var err error
	if kind == "workspace_client" {
		contacts, err = h.service.SearchWorkspaceClients(c.Context(), req, c.Query("search"))
	} else {
		contacts, err = h.service.Search(c.Context(), req, c.Query("search"), scope == "all")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.Status(404).JSON(fiber.Map{"error": "Not found"})
	}
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Recipient search is temporarily unavailable"})
	}
	return c.JSON(contacts)
}
