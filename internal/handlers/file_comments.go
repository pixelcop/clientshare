package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/services"
	"gorm.io/gorm"
)

type FileCommentsHandler struct{ service *services.FileCommentService }

func NewFileCommentsHandler(db *gorm.DB) *FileCommentsHandler {
	return &FileCommentsHandler{service: &services.FileCommentService{DB: db}}
}

func RegisterFileCommentRoutes(router fiber.Router, db *gorm.DB) {
	h := NewFileCommentsHandler(db)
	router.Get("/files/:id/comments", middleware.AuthRequired, h.List)
	router.Get("/files/:id/comments/count", middleware.AuthRequired, h.Count)
	router.Get("/files/:id/comments/mentions", middleware.AuthRequired, h.Mentions)
	router.Post("/files/:id/comments/preview", middleware.AuthRequired, h.Preview)
	router.Post("/files/:id/comments", middleware.AuthRequired, h.Create)
	router.Put("/files/:id/comments/:commentID", middleware.AuthRequired, h.Edit)
	router.Delete("/files/:id/comments/:commentID", middleware.AuthRequired, h.Remove)
	router.Get("/files/:id/comments/:commentID/history", middleware.AuthRequired, h.History)
}

func commentResponse(c fiber.Ctx, result any, err error, status int) error {
	var unavailable *services.UnavailableCommentMentions
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return c.Status(404).JSON(fiber.Map{"error": "Not found"})
	case errors.Is(err, services.ErrCommentInput):
		return c.Status(400).JSON(fiber.Map{"error": "Enter a comment of 1–4000 characters and choose a valid audience."})
	case errors.Is(err, services.ErrCommentConflict):
		return c.Status(409).JSON(fiber.Map{"error": "This comment changed. Refresh the conversation before saving again."})
	case errors.As(err, &unavailable):
		return c.Status(400).JSON(fiber.Map{"error": "A mentioned person is unavailable for this audience.", "unavailable_mention_ids": unavailable.IDs})
	case errors.Is(err, services.ErrCommentMention):
		return c.Status(400).JSON(fiber.Map{"error": "A mentioned person is unavailable for this audience. Choose people from the mention suggestions."})
	case err != nil:
		return c.Status(503).JSON(fiber.Map{"error": "Comments are temporarily unavailable. Please try again."})
	default:
		return c.Status(status).JSON(result)
	}
}

func (h *FileCommentsHandler) Mentions(c fiber.Ctx) error {
	people, err := h.service.Mentions(c.Context(), commentRequest(c), c.Query("search"), c.Query("visibility", services.CommentShared))
	return commentResponse(c, people, err, 200)
}

func (h *FileCommentsHandler) Preview(c fiber.Ctx) error {
	var input services.CommentInput
	if len(c.Body()) > 32*1024 || c.Bind().Body(&input) != nil {
		return commentResponse(c, nil, services.ErrCommentInput, 400)
	}
	rendered, err := h.service.Preview(c.Context(), commentRequest(c), input)
	return commentResponse(c, fiber.Map{"body_html": rendered}, err, 200)
}

func commentRequest(c fiber.Ctx) services.AccessRequest {
	return middleware.AccessRequest(c, "", c.Params("id"), services.AccessRead)
}

func (h *FileCommentsHandler) List(c fiber.Ctx) error {
	page, err := h.service.List(c.Context(), commentRequest(c), c.Query("before"))
	return commentResponse(c, page, err, 200)
}

func (h *FileCommentsHandler) Count(c fiber.Ctx) error {
	count, err := h.service.Count(c.Context(), commentRequest(c))
	return commentResponse(c, fiber.Map{"count": count}, err, 200)
}

func (h *FileCommentsHandler) Create(c fiber.Ctx) error {
	var input services.CommentInput
	if len(c.Body()) > 32*1024 || c.Bind().Body(&input) != nil {
		return commentResponse(c, nil, services.ErrCommentInput, 400)
	}
	comment, err := h.service.Create(c.Context(), commentRequest(c), input, c.Get("Idempotency-Key"))
	return commentResponse(c, comment, err, 201)
}

func (h *FileCommentsHandler) change(c fiber.Ctx, remove bool) error {
	var input struct {
		Body       string  `json:"body"`
		Version    int     `json:"version"`
		Visibility *string `json:"visibility"`
	}
	if len(c.Body()) > 32*1024 || c.Bind().Body(&input) != nil || input.Visibility != nil {
		return commentResponse(c, nil, services.ErrCommentInput, 400)
	}
	comment, err := h.service.Update(c.Context(), commentRequest(c), c.Params("commentID"), input.Body, input.Version, remove)
	return commentResponse(c, comment, err, 200)
}

func (h *FileCommentsHandler) Edit(c fiber.Ctx) error   { return h.change(c, false) }
func (h *FileCommentsHandler) Remove(c fiber.Ctx) error { return h.change(c, true) }

func (h *FileCommentsHandler) History(c fiber.Ctx) error {
	rows, err := h.service.History(c.Context(), commentRequest(c), c.Params("commentID"), fiber.Query[int](c, "before_version", 0))
	return commentResponse(c, rows, err, 200)
}
