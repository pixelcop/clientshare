package handlers

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/gorm"
)

type DocumentRequestsHandler struct {
	service      *services.DocumentRequestService
	emailBaseURL string
	emailEnabled bool
}

func NewDocumentRequestsHandler(db *gorm.DB, store storage.Storage, signingKey string) *DocumentRequestsHandler {
	return &DocumentRequestsHandler{service: &services.DocumentRequestService{DB: db, Store: store, SigningKey: signingKey}}
}

func RegisterDocumentRequestRoutes(public, staff fiber.Router, db *gorm.DB, store storage.Storage, signingKey, baseURL string, emailEnabled bool) {
	h := NewDocumentRequestsHandler(db, store, signingKey)
	h.emailBaseURL, h.emailEnabled = baseURL, emailEnabled
	management := staff.Group("/clients/:clientID/document-requests", middleware.AuthRequired)
	management.Get("/", h.List)
	management.Post("/", h.Create)
	management.Get("/:requestID", h.StaffView)
	management.Post("/:requestID/revoke", h.Revoke)
	management.Post("/:requestID/additions", h.AddItems)
	management.Post("/:requestID/items/:itemID/review", h.Review)
	staff.Get("/my-document-requests/:requestID/link", middleware.AuthRequired, h.AccountLink)
	staff.Get("/my-document-requests", middleware.AuthRequired, h.AccountRequests)

	// The request credential is separate from secure-link sessions and portal
	// authentication. It is passed in a header, never in an API path or query.
	recipient := public.Group("/document-request")
	recipient.Get("/", h.PublicView)
	recipient.Post("/items/:itemID/files", h.Upload)
	recipient.Post("/items/:itemID/submit", h.Submit)
	files := recipient.Group("/files/:fileID")
	files.Get("/", h.RecipientFile)
	files.Get("/view", h.recipientFileContent(true))
	files.Get("/download", h.recipientFileContent(false))
	files.Get("/comments", h.RecipientComments)
	files.Get("/comments/count", h.RecipientCommentCount)
	files.Post("/comments", h.RecipientCommentCreate)
}

func (h *DocumentRequestsHandler) RecipientFile(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	file, err := h.service.RecipientFile(c.Context(), tenantIDFromCtx(c), documentRequestToken(c), c.Params("fileID"))
	if err != nil {
		return documentRequestError(c, err)
	}
	return c.JSON(fiber.Map{"id": file.ID, "filename": file.Filename, "size": file.Size,
		"type": file.Type, "uploaded_at": file.UploadedAt, "capabilities": fiber.Map{"can_read": true}})
}

func (h *DocumentRequestsHandler) recipientFileContent(preview bool) fiber.Handler {
	files := NewFileHandler(h.service.DB, h.service.Store, nil, "", nil)
	return func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		file, err := h.service.RecipientFile(c.Context(), tenantIDFromCtx(c), documentRequestToken(c), c.Params("fileID"))
		if err != nil {
			return documentRequestError(c, err)
		}
		c.Locals("file", file)
		return files.sendFileContent(c, preview)
	}
}

func (h *DocumentRequestsHandler) requestComments(c fiber.Ctx) (*services.FileCommentService, services.AccessRequest) {
	c.Set("Cache-Control", "no-store")
	return h.service.RecipientComments(documentRequestToken(c), h.withEmail(c).NotificationBaseURL),
		services.AccessRequest{TenantID: tenantIDFromCtx(c), FileID: c.Params("fileID")}
}

func (h *DocumentRequestsHandler) RecipientComments(c fiber.Ctx) error {
	svc, req := h.requestComments(c)
	page, err := svc.ListAround(c.Context(), req, c.Query("before"), c.Query("focus"))
	return commentResponse(c, page, err, 200)
}

func (h *DocumentRequestsHandler) RecipientCommentCount(c fiber.Ctx) error {
	svc, req := h.requestComments(c)
	count, err := svc.Count(c.Context(), req)
	return commentResponse(c, fiber.Map{"count": count}, err, 200)
}

func (h *DocumentRequestsHandler) RecipientCommentCreate(c fiber.Ctx) error {
	var input services.CommentInput
	if len(c.Body()) > 32*1024 || c.Bind().Body(&input) != nil {
		return commentResponse(c, nil, services.ErrCommentInput, 400)
	}
	svc, req := h.requestComments(c)
	comment, err := svc.Create(c.Context(), req, input, c.Get("Idempotency-Key"))
	return commentResponse(c, comment, err, 201)
}

func documentRequestActor(c fiber.Ctx) services.AccessRequest {
	return middleware.AccessRequest(c, c.Params("clientID"), "", services.AccessContext)
}

func documentRequestToken(c fiber.Ctx) string { return c.Get("X-Document-Request") }

func (h *DocumentRequestsHandler) AccountLink(c fiber.Ctx) error {
	if fiber.Locals[string](c, "role") == "link" {
		return documentRequestError(c, gorm.ErrRecordNotFound)
	}
	token, err := h.service.AccountLink(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Params("requestID"))
	if err != nil {
		return documentRequestError(c, err)
	}
	return c.JSON(fiber.Map{"token": token})
}

func (h *DocumentRequestsHandler) AccountRequests(c fiber.Ctx) error {
	role := fiber.Locals[string](c, "role")
	if role != "client" && role != "customer" {
		return documentRequestError(c, gorm.ErrRecordNotFound)
	}
	view, err := h.service.AccountRequests(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"))
	if err != nil {
		return documentRequestError(c, err)
	}
	return c.JSON(view)
}

func documentRequestError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return c.Status(404).JSON(fiber.Map{"error": "Document request not found or no longer available"})
	case errors.Is(err, services.ErrDocumentRequestInput), errors.Is(err, services.ErrDocumentRequestFolder), errors.Is(err, services.ErrDocumentRequestRecipient):
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, services.ErrDocumentRequestConflict), errors.Is(err, services.ErrDocumentRequestFiles), errors.Is(err, services.ErrDocumentRequestFolderExists), errors.Is(err, services.ErrDocumentRequestItemLimit):
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	case respondQuotaError(c, err):
		return nil
	default:
		return c.Status(500).JSON(fiber.Map{"error": "Could not complete the request. Try again."})
	}
}

func (h *DocumentRequestsHandler) List(c fiber.Ctx) error {
	view, err := h.service.List(c.Context(), documentRequestActor(c))
	if err != nil {
		return documentRequestError(c, err)
	}
	return c.JSON(view)
}

func (h *DocumentRequestsHandler) Create(c fiber.Ctx) error {
	var input services.DocumentRequestInput
	if err := c.Bind().JSON(&input); err != nil {
		return documentRequestError(c, services.ErrDocumentRequestInput)
	}
	service := h.withEmail(c)
	view, err := service.Create(c.Context(), documentRequestActor(c), input)
	if err != nil {
		return documentRequestError(c, err)
	}
	return c.Status(201).JSON(view)
}

func (h *DocumentRequestsHandler) withEmail(c fiber.Ctx) services.DocumentRequestService {
	service := *h.service
	if h.emailEnabled {
		service.NotificationBaseURL = normalizeAbsoluteURL(h.emailBaseURL)
		if resolved, ok := tenant.FromFiber(c); ok && normalizeAbsoluteURL(resolved.PublicBaseURL) != "" {
			service.NotificationBaseURL = normalizeAbsoluteURL(resolved.PublicBaseURL)
		}
	}
	return service
}

func (h *DocumentRequestsHandler) AddItems(c fiber.Ctx) error {
	var input services.DocumentRequestAddition
	if err := c.Bind().JSON(&input); err != nil {
		return documentRequestError(c, services.ErrDocumentRequestInput)
	}
	service := h.withEmail(c)
	view, err := service.AddItems(c.Context(), documentRequestActor(c), c.Params("requestID"), input)
	if err != nil {
		return documentRequestError(c, err)
	}
	return c.Status(201).JSON(view)
}

func (h *DocumentRequestsHandler) StaffView(c fiber.Ctx) error {
	view, err := h.service.StaffView(c.Context(), documentRequestActor(c), c.Params("requestID"))
	if err != nil {
		return documentRequestError(c, err)
	}
	return c.JSON(view)
}

func (h *DocumentRequestsHandler) Review(c fiber.Ctx) error {
	var input struct {
		Round  int    `json:"round"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := c.Bind().JSON(&input); err != nil {
		return documentRequestError(c, services.ErrDocumentRequestInput)
	}
	if err := h.service.Review(c.Context(), documentRequestActor(c), c.Params("requestID"), c.Params("itemID"), input.Round, input.Status, input.Reason); err != nil {
		return documentRequestError(c, err)
	}
	return c.SendStatus(204)
}

func (h *DocumentRequestsHandler) Revoke(c fiber.Ctx) error {
	if err := h.service.Revoke(c.Context(), documentRequestActor(c), c.Params("requestID")); err != nil {
		return documentRequestError(c, err)
	}
	return c.SendStatus(204)
}

func (h *DocumentRequestsHandler) PublicView(c fiber.Ctx) error {
	view, err := h.service.PublicView(c.Context(), tenantIDFromCtx(c), documentRequestToken(c))
	if err != nil {
		return documentRequestError(c, err)
	}
	return c.JSON(view)
}

func (h *DocumentRequestsHandler) Upload(c fiber.Ctx) error {
	round, err := strconv.Atoi(c.FormValue("round"))
	if err != nil {
		return documentRequestError(c, services.ErrDocumentRequestInput)
	}
	header, err := c.FormFile("file")
	if err != nil {
		return documentRequestError(c, services.ErrDocumentRequestInput)
	}
	file, err := header.Open()
	if err != nil {
		return documentRequestError(c, err)
	}
	defer file.Close()
	if err := h.service.Upload(c.Context(), tenantIDFromCtx(c), documentRequestToken(c), c.Params("itemID"), round, header.Filename, file, header.Size); err != nil {
		return documentRequestError(c, err)
	}
	return c.SendStatus(201)
}

func (h *DocumentRequestsHandler) Submit(c fiber.Ctx) error {
	var input struct {
		Round int `json:"round"`
	}
	if err := c.Bind().JSON(&input); err != nil {
		return documentRequestError(c, services.ErrDocumentRequestInput)
	}
	if err := h.service.Submit(c.Context(), tenantIDFromCtx(c), documentRequestToken(c), c.Params("itemID"), input.Round); err != nil {
		return documentRequestError(c, err)
	}
	return c.SendStatus(204)
}
