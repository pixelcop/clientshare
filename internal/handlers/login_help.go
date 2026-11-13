package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/mail"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/google/uuid"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"gorm.io/gorm"
)

// LoginHelpHandler accepts login issues without exposing account or booking details.
type LoginHelpHandler struct {
	db      *gorm.DB
	options HelpOptions
}

func NewLoginHelpHandler(db *gorm.DB, options HelpOptions) *LoginHelpHandler {
	return &LoginHelpHandler{db: db, options: options}
}

func RegisterLoginHelpRoutes(api fiber.Router, db *gorm.DB, options HelpOptions) {
	h := NewLoginHelpHandler(db, options)
	api.Get("/help/login", h.Capabilities)
	api.Post("/help/login/messages", limiter.New(limiter.Config{
		Max: 5, Expiration: time.Hour,
		KeyGenerator: func(c fiber.Ctx) string { return c.IP() },
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "Please wait before sending another message."})
		},
	}), h.Receive)
}

func (h *LoginHelpHandler) available() bool {
	return h.options.Hosted && h.options.Provider == "clientshare" && h.options.Recipient != ""
}

func (h *LoginHelpHandler) Capabilities(c fiber.Ctx) error {
	result := fiber.Map{"provider": "none", "guide_url": h.options.GuideURL}
	if h.available() {
		result["provider"] = "clientshare"
		result["support_label"] = "Contact ClientShare support"
	} else if h.options.Provider == "operator" {
		result["provider"] = "operator"
		result["support_label"] = h.options.OperatorLabel
		result["support_url"] = h.options.OperatorURL
	}
	return c.JSON(result)
}

func (h *LoginHelpHandler) Receive(c fiber.Ctx) error {
	fail := func(status int, message string) error { return c.Status(status).JSON(fiber.Map{"error": message}) }
	if !h.available() {
		return fail(403, "Support is not available for this portal.")
	}
	if c.Get("Sec-Fetch-Site") == "cross-site" || !strings.HasPrefix(strings.ToLower(c.Get("Content-Type")), "application/json") {
		return fail(415, "JSON required.")
	}
	if len(c.Body()) > 24000 {
		return fail(413, "Message too large.")
	}
	var input struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Email   string `json:"email"`
	}
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		return fail(400, "Invalid message fields.")
	}
	id, err := uuid.Parse(input.ID)
	input.Message = strings.TrimSpace(input.Message)
	input.Email = strings.TrimSpace(input.Email)
	address, emailErr := mail.ParseAddress(input.Email)
	if err != nil || id.Version() != 4 || len(input.ID) != 36 || len(input.Message) == 0 || len(input.Message) > 8000 || len(input.Email) > 254 || emailErr != nil || address.Address != input.Email {
		return fail(400, "Enter a valid reply email and a message of at most 8000 bytes.")
	}
	// This is a self-reported address, never an authenticated requester identity.
	row := models.SupportMessage{TenantID: tenantIDFromCtx(c), RequesterID: "login:" + strings.ToLower(input.Email), Role: "anonymous", Email: input.Email, RetryKey: id.String(), Provider: "clientshare", Destination: h.options.Recipient, Message: input.Message, Screen: "login", Version: h.options.Version}
	err = (services.SupportService{DB: h.db}).Receive(c.Context(), &row)
	if errors.Is(err, services.ErrSupportConflict) {
		return fail(409, "This request ID was already used.")
	}
	if err != nil {
		return fail(503, "Message could not be saved. Please retry.")
	}
	return c.Status(202).JSON(fiber.Map{"received": true, "id": row.ID})
}
