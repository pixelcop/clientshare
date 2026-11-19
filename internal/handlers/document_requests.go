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
	management.Post("/:requestID/items/:itemID/review", h.Review)

	// The request credential is separate from secure-link sessions and portal
	// authentication. It is passed in a header, never in an API path or query.
	recipient := public.Group("/document-request")
	recipient.Get("/", h.PublicView)
	recipient.Post("/items/:itemID/files", h.Upload)
	recipient.Post("/items/:itemID/submit", h.Submit)
}

func documentRequestActor(c fiber.Ctx) services.AccessRequest {
	return middleware.AccessRequest(c, c.Params("clientID"), "", services.AccessContext)
}

func documentRequestToken(c fiber.Ctx) string { return c.Get("X-Document-Request") }

func documentRequestError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return c.Status(404).JSON(fiber.Map{"error": "Document request not found or no longer available"})
	case errors.Is(err, services.ErrDocumentRequestInput), errors.Is(err, services.ErrDocumentRequestFolder), errors.Is(err, services.ErrDocumentRequestRecipient):
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, services.ErrDocumentRequestConflict), errors.Is(err, services.ErrDocumentRequestFiles), errors.Is(err, services.ErrDocumentRequestFolderExists):
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
	service := *h.service
	if h.emailEnabled {
		service.NotificationBaseURL = normalizeAbsoluteURL(h.emailBaseURL)
		if resolved, ok := tenant.FromFiber(c); ok && normalizeAbsoluteURL(resolved.PublicBaseURL) != "" {
			service.NotificationBaseURL = normalizeAbsoluteURL(resolved.PublicBaseURL)
		}
	}
	view, err := service.Create(c.Context(), documentRequestActor(c), input)
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
