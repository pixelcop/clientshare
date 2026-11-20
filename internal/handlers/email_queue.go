package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type updateRequest struct {
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

type emailQueueHandler struct {
	EmailQueue emailpkg.EmailQueue
	db         *gorm.DB
}

func (h *emailQueueHandler) visiblePending(c fiber.Ctx, clientID string) ([]*models.QueuedEmail, error) {
	tenantID := tenantIDFromCtx(c)
	var actor models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantID, fiber.Locals[string](c, "user_id")).First(&actor).Error; err != nil {
		return nil, err
	}
	if actor.Role != "admin" && actor.Role != "manager" {
		return nil, fiber.ErrForbidden
	}
	allowed := map[string]bool{}
	if actor.Role != "admin" {
		q, err := (&services.AccessService{DB: h.db}).ClientScope(c.Context(), tenantID, actor.ID, services.AccessManageQueue)
		if err != nil {
			return nil, err
		}
		var ids []string
		if err := q.Pluck("clients.id", &ids).Error; err != nil {
			return nil, err
		}
		for _, id := range ids {
			allowed[id] = true
		}
	}
	visible := []*models.QueuedEmail{}
	for _, item := range h.EmailQueue.Pending(tenantID, clientID) {
		if item.TenantID != tenantID || item.CanceledAt != nil || item.SentAt != nil {
			continue
		}
		// Target-aware invitations are email-possession credentials. Even an
		// administrator who sent the invitation must not recover its setup or
		// acceptance URL through the generic queue inspection surface.
		if item.AccessInvitationID != nil || item.IdentityInviteID != nil || item.FileCommentID != nil || item.DocumentRequestID != nil {
			continue
		}
		// Account invitation/reset messages contain identity credentials, not
		// client sharing authority. Only admins can inspect those queue jobs.
		if actor.Role != "admin" && (item.ClientID == nil || !allowed[*item.ClientID] || (item.FileIDsCSV == "" && item.SecureLinkID == nil)) {
			continue
		}
		if item.SecureLinkID != nil {
			_, err := (&services.SharingService{DB: h.db}).LinkForDelivery(c.Context(), middleware.AccessRequest(c, "", "", services.AccessShare), *item.SecureLinkID)
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, services.ErrSharingCapabilities) {
				continue
			}
			if err != nil {
				return nil, err
			}
		}
		visible = append(visible, item)
	}
	return visible, nil
}

func (h *emailQueueHandler) list(c fiber.Ctx) error {
	if h.EmailQueue == nil {
		return c.Status(503).JSON(fiber.Map{"error": "email queue not available"})
	}
	clientID := strings.TrimSpace(fiber.Query[string](c, "client_id"))
	pending, err := h.visiblePending(c, clientID)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
	}
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
	items, err := h.visiblePending(c, "")
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
	}
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			break
		}
	}
	if !found {
		return c.Status(404).JSON(fiber.Map{"error": "queued email not found"})
	}

	if err := h.EmailQueue.SendNow(tenantIDFromCtx(c), id); err != nil {
		utils.Logger(c).Error("failed to send queued email", zap.String("id", id), zap.Error(err))
		status := 500
		if strings.Contains(err.Error(), "not found") {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "processed"})
}

func (h *emailQueueHandler) forceProcess(c fiber.Ctx) error {
	if h.EmailQueue == nil {
		return c.Status(503).JSON(fiber.Map{"error": "email queue not available"})
	}
	items, err := h.visiblePending(c, "")
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
	}
	for _, item := range items {
		if err := h.EmailQueue.SendNow(tenantIDFromCtx(c), item.ID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Could not process queued email"})
		}
	}
	return c.JSON(fiber.Map{"status": "processed"})
}

func RegisterEmailQueueRoutes(router fiber.Router, db *gorm.DB, emailQueue emailpkg.EmailQueue) {
	handler := &emailQueueHandler{EmailQueue: emailQueue, db: db}
	q := router.Group("/email/queue", middleware.RoleRequired("manager"))
	q.Get("", handler.list)
	q.Post("/:id/send", handler.sendNow)
	q.Post("/process", handler.forceProcess)
}

func isLikelyULID(id string) bool {
	return len(strings.TrimSpace(id)) == 26
}
