package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ClientHandler struct {
	service *services.ClientService
}

func NewClientHandler(db *gorm.DB, store storage.Storage) *ClientHandler {
	return &ClientHandler{
		service: &services.ClientService{DB: db, Storage: store},
	}
}

// RegisterClientRoutes sets up client endpoints
func RegisterClientRoutes(router fiber.Router, db *gorm.DB, store storage.Storage) {
	handler := NewClientHandler(db, store)
	router.Get("/clients", middleware.AuthRequired, handler.ListClients)
	router.Post("/clients", middleware.AuthRequired, middleware.RoleRequired("manager"), handler.CreateClient)
	router.Get("/clients/:id", middleware.AuthRequired, middleware.RoleRequired("manager"), handler.GetClient)
	router.Put("/clients/:id", middleware.AuthRequired, middleware.RoleRequired("manager"), handler.UpdateClient)
	router.Delete("/clients/:id", middleware.AuthRequired, middleware.RoleRequired("manager"), handler.DeleteClient)
}

// Handler methods to be implemented below...

// ListClients handles GET /clients
func (h *ClientHandler) ListClients(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	userID, _ := c.Locals("user_id").(string)
	tenantID := tenantIDFromCtx(c)
	if role != "admin" && role != "manager" && role != "client" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	search := strings.TrimSpace(c.Query("search"))
	if c.Query("page") != "" || c.Query("page_size") != "" || search != "" {
		page := fiber.Query[int](c, "page", 1)
		pageSize := fiber.Query[int](c, "page_size", 20)
		pagination, clients, err := h.service.ListClientsPaginated(c.Context(), tenantID, userID, role, search, page, pageSize)
		if err != nil {
			utils.Logger(c).Error("error listing clients", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(models.PaginatedResult[models.Client]{
			Pagination: *pagination,
			Items:      clients,
		})
	}
	clients, err := h.service.ListClients(c.Context(), tenantID, userID, role)
	if err != nil {
		utils.Logger(c).Error("error listing clients", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(clients)
}

// CreateClient handles POST /clients
func (h *ClientHandler) CreateClient(c fiber.Ctx) error {
	type req struct {
		Name string `json:"name"`
	}
	var body req
	if err := c.Bind().Body(&body); err != nil {
		utils.Logger(c).Debug("invalid request body", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	client, err := h.service.CreateClient(c.Context(), tenantIDFromCtx(c), body.Name)
	if err != nil {
		utils.Logger(c).Error("error creating client", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(client)
}

// GetClient handles GET /clients/:id
func (h *ClientHandler) GetClient(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		utils.Logger(c).Debug("invalid id")
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	client, err := h.service.GetClient(c.Context(), tenantIDFromCtx(c), id)
	if err != nil {
		utils.Logger(c).Debug("client not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "client not found"})
	}
	return c.JSON(client)
}

// UpdateClient handles PUT /clients/:id
func (h *ClientHandler) UpdateClient(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		utils.Logger(c).Debug("invalid id")
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	type req struct {
		Name string `json:"name"`
	}
	var body req
	if err := c.Bind().Body(&body); err != nil {
		utils.Logger(c).Debug("invalid request body", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	client, err := h.service.UpdateClient(c.Context(), tenantIDFromCtx(c), id, body.Name)
	if err != nil {
		utils.Logger(c).Error("error updating client", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(client)
}

// DeleteClient handles DELETE /clients/:id
func (h *ClientHandler) DeleteClient(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		utils.Logger(c).Debug("invalid id")
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if c.Query("confirm") != "true" {
		utils.Logger(c).Debug("confirmation required")
		return c.Status(400).JSON(fiber.Map{"error": "confirmation required"})
	}
	if err := h.service.DeleteClient(c.Context(), tenantIDFromCtx(c), id); err != nil {
		utils.Logger(c).Error("error deleting client", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
