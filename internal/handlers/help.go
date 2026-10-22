package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/google/uuid"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"gorm.io/gorm"
	"io"
	"strings"
	"time"
)

type HelpOptions struct {
	Hosted                                                                         bool
	Provider, GuideURL, OperatorURL, OperatorLabel, Recipient, BookingURL, Version string
}

func RegisterHelpRoutes(api fiber.Router, db *gorm.DB, opt HelpOptions) {
	requester := func(c fiber.Ctx) (*models.User, error) {
		var user models.User
		err := db.Where("tenant_id = ? AND id = ?", tenantIDFromCtx(c), c.Locals("user_id")).First(&user).Error
		return &user, err
	}
	allowed := func(u *models.User) bool {
		return opt.Hosted && opt.Provider == "clientshare" && opt.Recipient != "" && (u.Role == "admin" || u.Role == "manager")
	}
	api.Get("/help", func(c fiber.Ctx) error {
		result := fiber.Map{"guide_url": opt.GuideURL, "provider": "none"}
		if opt.Provider == "operator" {
			result["provider"] = "operator"
			result["support_url"] = opt.OperatorURL
			result["support_label"] = opt.OperatorLabel
		}
		if u, err := requester(c); err == nil && allowed(u) {
			result["provider"] = "clientshare"
			result["support_label"] = "Contact ClientShare support"
			result["reply_email"] = u.Email
			result["requester_name"] = u.Name
			result["booking_url"] = opt.BookingURL
		}
		return c.JSON(result)
	})
	api.Post("/help/messages", middleware.RoleRequired("manager"), limiter.New(limiter.Config{Max: 10, Expiration: time.Hour, KeyGenerator: func(c fiber.Ctx) string { return tenantIDFromCtx(c) + ":" + c.Locals("user_id").(string) }, LimitReached: func(c fiber.Ctx) error {
		return c.Status(429).JSON(fiber.Map{"error": "Please wait before sending another message."})
	}}), func(c fiber.Ctx) error {
		fail := func(status int, message string) error { return c.Status(status).JSON(fiber.Map{"error": message}) }
		u, err := requester(c)
		if err != nil || !allowed(u) {
			return fail(403, "Support is not available for this account.")
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
			Screen  string `json:"screen"`
		}
		decoder := json.NewDecoder(bytes.NewReader(c.Body()))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			return fail(400, "Invalid message fields.")
		}
		id, e := uuid.Parse(input.ID)
		input.Message = strings.TrimSpace(input.Message)
		if e != nil || id.Version() != 4 || len(input.ID) != 36 || len(input.Message) == 0 || len(input.Message) > 8000 {
			return fail(400, "Enter a message of at most 8000 bytes.")
		}
		screen := "portal"
		switch input.Screen {
		case "clients", "files", "activity", "users", "settings":
			screen = input.Screen
		}
		row := models.SupportMessage{TenantID: u.TenantID, RequesterID: u.ID, Role: u.Role, Name: u.Name, Email: u.Email, RetryKey: id.String(), Provider: "clientshare", Destination: opt.Recipient, Message: input.Message, Screen: screen, Version: opt.Version}
		err = (services.SupportService{DB: db}).Receive(c.Context(), &row)
		if errors.Is(err, services.ErrSupportConflict) {
			return fail(409, "This request ID was already used.")
		}
		if err != nil {
			return fail(503, "Message could not be saved. Please retry.")
		}
		return c.Status(202).JSON(fiber.Map{"received": true, "id": row.ID})
	})
}
