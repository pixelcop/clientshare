package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type FeedHandler struct {
	service *services.FeedService
}

func RegisterFeedRoutes(router fiber.Router, db *gorm.DB) {
	handler := &FeedHandler{service: services.NewFeedService(db)}
	router.Get("/feed", handler.ListFeed)
	router.Post("/feed/read-all", handler.MarkAllRead)
	router.Post("/feed/:id/read", handler.MarkRead)
}

func (h *FeedHandler) ListFeed(c fiber.Ctx) error {
	role := fiber.Locals[string](c, "role")
	userID := fiber.Locals[string](c, "user_id")
	tenantID := tenantIDFromCtx(c)
	if role != "admin" && role != "manager" && role != "client" && role != "customer" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	page, pageSize := getPageParams(c)
	state := normalizeFeedState(c.Query("state"))
	if state == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid feed state"})
	}

	result, err := h.service.ListFeedByState(c.Context(), tenantID, userID, role, state, page, pageSize)
	if err != nil {
		utils.Logger(c).Error("error listing feed", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *FeedHandler) MarkRead(c fiber.Ctx) error {
	role := fiber.Locals[string](c, "role")
	userID := fiber.Locals[string](c, "user_id")
	tenantID := tenantIDFromCtx(c)
	if role != "admin" && role != "manager" && role != "client" && role != "customer" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	eventID := strings.TrimSpace(c.Params("id"))
	if eventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "event id is required"})
	}

	if err := h.service.MarkRead(c.Context(), tenantID, userID, role, eventID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "feed event not found"})
		}
		utils.Logger(c).Error("error marking feed read", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *FeedHandler) MarkAllRead(c fiber.Ctx) error {
	role := fiber.Locals[string](c, "role")
	userID := fiber.Locals[string](c, "user_id")
	tenantID := tenantIDFromCtx(c)
	if role != "admin" && role != "manager" && role != "client" && role != "customer" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	if err := h.service.MarkAllRead(c.Context(), tenantID, userID, role); err != nil {
		utils.Logger(c).Error("error marking all feed items read", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "ok"})
}

func normalizeFeedState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "unread":
		return "unread"
	case "read":
		return "read"
	case "all":
		return "all"
	default:
		return ""
	}
}
