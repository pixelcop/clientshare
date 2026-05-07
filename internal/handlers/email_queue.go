package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
)

type updateRequest struct {
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

type emailQueueHandler struct {
	EmailQueue emailpkg.EmailQueue
}

func (h *emailQueueHandler) list(c fiber.Ctx) error {
	if h.EmailQueue == nil {
		return c.Status(503).JSON(fiber.Map{"error": "email queue not available"})
	}
	tenantID := tenantIDFromCtx(c)
	clientID := strings.TrimSpace(fiber.Query[string](c, "client_id"))
	pending := h.EmailQueue.Pending(tenantID, clientID)
	return c.JSON(fiber.Map{"count": len(pending), "pending": pending})
}

func (h *emailQueueHandler) update(c fiber.Ctx) error {
	if h.EmailQueue == nil {
		return c.Status(503).JSON(fiber.Map{"error": "email queue not available"})
	}
	id := strings.TrimSpace(c.Params("id"))
	if !isLikelyULID(id) {
		return c.Status(400).JSON(fiber.Map{"error": "invalid email id"})
	}

	if len(c.Body()) > 0 {
		var req updateRequest
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		recipient := strings.TrimSpace(req.Recipient)
		subject := strings.TrimSpace(req.Subject)
		if recipient == "" || subject == "" {
			return c.Status(400).JSON(fiber.Map{"error": "recipient and subject are required"})
		}
		if err := h.EmailQueue.Update(tenantIDFromCtx(c), id, recipient, subject, req.Body); err != nil {
			status := 500
			if strings.Contains(err.Error(), "not found") {
				status = 404
			}
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
	}
	return c.JSON(fiber.Map{"status": "updated"})
}

func (h *emailQueueHandler) sendNow(c fiber.Ctx) error {
	if h.EmailQueue == nil {
		return c.Status(503).JSON(fiber.Map{"error": "email queue not available"})
	}
	id := strings.TrimSpace(c.Params("id"))
	if !isLikelyULID(id) {
		return c.Status(400).JSON(fiber.Map{"error": "invalid email id"})
	}

	if err := h.EmailQueue.SendNow(tenantIDFromCtx(c), id); err != nil {
		utils.Logger(c).Error("failed to send queued email", zap.String("id", id), zap.Error(err))
		status := 500
		if strings.Contains(err.Error(), "not found") {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "sent"})
}

func (h *emailQueueHandler) forceProcess(c fiber.Ctx) error {
	if h.EmailQueue == nil {
		return c.Status(503).JSON(fiber.Map{"error": "email queue not available"})
	}
	if err := h.EmailQueue.ProcessAll(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "processed"})
}

func RegisterEmailQueueRoutes(router fiber.Router, emailQueue emailpkg.EmailQueue) {
	handler := &emailQueueHandler{EmailQueue: emailQueue}
	q := router.Group("/email/queue", middleware.RoleRequired("manager"))
	q.Get("", handler.list)
	q.Post("/:id/send", handler.sendNow)
	q.Post("/process", handler.forceProcess)
}

func isLikelyULID(id string) bool {
	return len(strings.TrimSpace(id)) == 26
}
