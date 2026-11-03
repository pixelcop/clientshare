package handlers

import (
	"errors"
	"fmt"
	"html"
	"net/mail"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/links"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type LinkHandler struct {
	db             *gorm.DB
	emailQueue     emailpkg.EmailQueue
	signingKey     string
	clientService  *services.ClientService
	baseURL        string
	tenantSettings *services.TenantSettingsService
}

func NewLinkHandler(db *gorm.DB, signingKey string, store storage.Storage, emailQueue emailpkg.EmailQueue, baseURL string, tenantSettings *services.TenantSettingsService) *LinkHandler {
	return &LinkHandler{
		db:             db,
		signingKey:     signingKey,
		clientService:  &services.ClientService{DB: db, Storage: store},
		emailQueue:     emailQueue,
		baseURL:        baseURL,
		tenantSettings: tenantSettings,
	}
}

// RegisterLinkRoutes sets up secure link endpoints
// Management endpoints (create/list/delete) are protected; public endpoints (by token) are not
func RegisterLinkRoutes(router fiber.Router, db *gorm.DB, signingKey string, store storage.Storage, emailQueue emailpkg.EmailQueue, baseURL string, tenantSettings *services.TenantSettingsService) {
	handler := NewLinkHandler(db, signingKey, store, emailQueue, baseURL, tenantSettings)

	// Secure link bootstrap route (sets JWT cookie on first hit)
	router.Get("/links/:token", handler.BootstrapLinkSession)
	router.Post("/links/:token", handler.BootstrapLinkSession)

	router.Post("/api/clients/:id/links", middleware.RoleRequired("admin"), handler.CreateSecureLink)
	router.Get("/api/clients/:id/links", middleware.RoleRequired("admin"), handler.ListSecureLinks)
	router.Get("/api/clients/:id/collection", middleware.RoleRequired("admin"), handler.CollectionStatus)
	router.Delete("/api/links/:id", middleware.RoleRequired("admin"), handler.DeleteSecureLink)
	router.Get("/api/link/:token", handler.ViewLink)
}

func (h *LinkHandler) tenantEmailSettings(c fiber.Ctx, tenantID string) (string, int) {
	if h.tenantSettings == nil {
		return services.DefaultSiteTitle, services.DefaultSecureLinkDefaultExpiryDays
	}
	settings, err := h.tenantSettings.Get(c.Context(), tenantID)
	if err != nil {
		utils.Logger(c).Warn("failed loading tenant settings for secure links", zap.Error(err), zap.String("tenant_id", tenantID))
		return services.DefaultSiteTitle, services.DefaultSecureLinkDefaultExpiryDays
	}
	return services.SiteTitleOrDefault(settings), services.SecureLinkExpiryOrDefault(settings)
}

func (h *LinkHandler) setLinkSessionCookie(c fiber.Ctx, link models.SecureLink) (string, error) {
	store := &auth.SessionStore{DB: h.db}
	previous := c.Cookies(auth.SessionCookie)
	identity, err := store.Validate(c.Context(), previous, auth.PortalAudience, link.TenantID)
	if err == nil && identity.LinkID == link.ID {
		return previous, nil
	}
	if err != nil && !errors.Is(err, auth.ErrSessionInvalid) {
		return "", err
	}
	if err == nil && c.Method() != "POST" {
		return "", fiber.NewError(409, "Opening this link requires signing out of the current portal session")
	}
	if err == nil && c.FormValue("session_id") != identity.SessionID {
		return "", fiber.NewError(409, "Your session changed. Confirm again to open this link.")
	}
	sessionToken, err := store.Create(c.Context(), link.TenantID, "", link.ID, auth.PortalAudience, "", previous)
	if err != nil {
		return "", err
	}
	auth.SetSessionCookie(c, sessionToken)
	return sessionToken, nil
}

// BootstrapLinkSession handles GET /links/:token
// Sets a secure-link JWT cookie and redirects to the SPA route.
func (h *LinkHandler) BootstrapLinkSession(c fiber.Ctx) error {
	// FIXME: NOT a json request. errors should be handled with HTML responses or redirects, not JSON

	token := c.Params("token")
	tenantID := tenantIDFromCtx(c)
	var link models.SecureLink
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND token = ?", tenantID, token).First(&link).Error; err != nil {
		utils.Logger(c).Warn("link not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "link not found"})
	}
	_, valid := links.ValidateSecureToken(token, h.signingKey)
	if !valid || links.IsExpired(link.ExpiresAt) {
		utils.Logger(c).Warn("link expired or invalid")
		return c.Status(403).JSON(fiber.Map{"error": "link expired or invalid"})
	}
	client, err := h.clientService.GetClient(c.Context(), tenantID, link.ClientID)
	if err != nil {
		utils.Logger(c).Warn("client not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "client not found"})
	}
	if _, err := h.setLinkSessionCookie(c, link); err != nil {
		var conflict *fiber.Error
		if errors.As(err, &conflict) && conflict.Code == 409 {
			identity, lookupErr := (&auth.SessionStore{DB: h.db}).Validate(c.Context(), c.Cookies(auth.SessionCookie), auth.PortalAudience, tenantID)
			if lookupErr != nil {
				return lookupErr
			}
			c.Set("Cache-Control", "no-store")
			c.Set("Referrer-Policy", "no-referrer")
			c.Type("html", "utf-8")
			return c.SendString(`<!doctype html><html><head><meta charset="utf-8"><title>Open shared link</title></head><body><main><h1>Open this link as a recipient?</h1><p>This will sign you out of this portal in all tabs and end any billing session associated with this sign-in.</p><form method="post" action="` + html.EscapeString(c.Path()) + `"><input type="hidden" name="session_id" value="` + html.EscapeString(identity.SessionID) + `"><button type="submit">Sign out and open link</button></form><p><a href="/">Keep my current session</a></p></main></body></html>`)
		}
		return err
	}

	redirectPath := fmt.Sprintf("/folder/%s", derefString(client.RootFolderID))
	page := fmt.Sprintf(
		"<!doctype html><html><head><meta charset=\"utf-8\"><meta http-equiv=\"cache-control\" content=\"no-store\"></head><body>"+
			"<script>"+
			"try{localStorage.setItem('link', %q);localStorage.setItem('clientshare-session-change',String(Date.now()))}catch(e){}"+
			"window.location.replace(%q);"+
			"</script></body></html>",
		link.Token,
		redirectPath,
	)
	c.Set("Cache-Control", "no-store")
	c.Type("html", "utf-8")
	return c.SendString(page)
}

// CreateSecureLink handles POST /api/clients/:id/links
func (h *LinkHandler) CreateSecureLink(c fiber.Ctx) error {
	id := c.Params("id")
	tenantID := tenantIDFromCtx(c)
	if id == "" {
		utils.Logger(c).Warn("invalid client id")
		return c.Status(400).JSON(fiber.Map{"error": "invalid client id"})
	}
	if _, err := h.clientService.GetClient(c.Context(), tenantID, id); err != nil {
		utils.Logger(c).Warn("client not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "client not found"})
	}
	type req struct {
		AccessType string `json:"access_type"`
		ExpiryDays int    `json:"expiry_days"`
		Email      string `json:"email"`
	}
	var body req
	if err := c.Bind().Body(&body); err != nil {
		utils.Logger(c).Warn("invalid request body", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	switch body.AccessType {
	case "read", "view", "write", "readwrite":
	default:
		return c.Status(400).JSON(fiber.Map{"error": "access_type must be read, write, or readwrite"})
	}
	siteTitle, defaultExpiryDays := h.tenantEmailSettings(c, tenantID)
	if body.ExpiryDays < 1 {
		body.ExpiryDays = defaultExpiryDays
	}
	if body.ExpiryDays < 1 || body.ExpiryDays > services.MaxSecureLinkDefaultExpiryDays {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("expiry_days must be between 1 and %d", services.MaxSecureLinkDefaultExpiryDays)})
	}
	emailAddress := strings.TrimSpace(body.Email)
	if emailAddress != "" {
		parsed, parseErr := mail.ParseAddress(emailAddress)
		if parseErr != nil {
			utils.Logger(c).Warn("invalid link email", zap.Error(parseErr))
			return c.Status(400).JSON(fiber.Map{"error": "invalid email"})
		}
		emailAddress = parsed.Address
	}
	token, err := links.GenerateSecureToken(h.signingKey)
	if err != nil {
		utils.Logger(c).Error("error generating secure token", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	createdBy := fiber.Locals[string](c, "user_id")
	expiry := time.Now().Add(time.Duration(body.ExpiryDays) * 24 * time.Hour)
	link := models.SecureLink{
		TenantID:   tenantID,
		ClientID:   id,
		Token:      token,
		Email:      emailAddress,
		ExpiresAt:  expiry,
		AccessType: body.AccessType,
		CreatedBy:  createdBy,
		CreatedAt:  time.Now(),
	}
	if err := h.db.WithContext(c.Context()).Create(&link).Error; err != nil {
		utils.Logger(c).Error("error creating secure link", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	link.LinkURL = buildSecureLinkURL(tenantBaseURLFromCtx(c, h.baseURL), link.Token)
	queued := false
	if emailAddress != "" {
		queued = h.sendSecureLinkEmail(c, link, siteTitle)
	}
	return c.Status(201).JSON(struct {
		models.SecureLink
		EmailQueued bool `json:"email_queued"`
	}{link, queued})
}

func (h *LinkHandler) sendSecureLinkEmail(c fiber.Ctx, link models.SecureLink, siteTitle string) bool {
	if h.emailQueue == nil {
		utils.Logger(c).Warn("email queue unavailable for secure link")
		return false
	}
	client, err := h.clientService.GetClient(c.Context(), link.TenantID, link.ClientID)
	if err != nil {
		utils.Logger(c).Error("error fetching client for secure link email", zap.Error(err))
		return false
	}
	linkURL := buildSecureLinkURL(tenantBaseURLFromCtx(c, h.baseURL), link.Token)
	if linkURL == "" {
		utils.Logger(c).Warn("secure link base url missing")
		return false
	}
	subject, body, htmlBody := emailpkg.RenderSecureLinkEmail(client.Name, link.AccessType, linkURL, siteTitle, link.ExpiresAt)
	if _, err := h.emailQueue.Queue(emailpkg.Email{TenantID: link.TenantID, To: link.Email, Subject: subject, Body: body, HTMLBody: htmlBody}, models.Now()); err != nil {
		utils.Logger(c).Error("failed to queue secure link email", zap.Error(err))
		return false
	}
	return true
}

func (h *LinkHandler) CollectionStatus(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	if _, err := h.clientService.GetClient(c.Context(), tenantID, c.Params("id")); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "client not found"})
	}
	since, err := time.Parse(time.RFC3339Nano, c.Query("since"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid start time"})
	}
	status, err := services.CollectionStatus(c.Context(), h.db, tenantID, c.Params("id"), since)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not check upload requests"})
	}
	return c.JSON(status)
}

func buildSecureLinkURL(baseURL, token string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" || strings.TrimSpace(token) == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/links/" + token
}

// ListSecureLinks handles GET /api/clients/:id/links
func (h *LinkHandler) ListSecureLinks(c fiber.Ctx) error {
	id := c.Params("id")
	tenantID := tenantIDFromCtx(c)
	if id == "" {
		utils.Logger(c).Warn("invalid client id")
		return c.Status(400).JSON(fiber.Map{"error": "invalid client id"})
	}
	var linksList []models.SecureLink
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND client_id = ?", tenantID, id).Find(&linksList).Error; err != nil {
		utils.Logger(c).Error("error listing secure links", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	baseURL := tenantBaseURLFromCtx(c, h.baseURL)
	for index := range linksList {
		linksList[index].LinkURL = buildSecureLinkURL(baseURL, linksList[index].Token)
	}
	return c.JSON(linksList)
}

// DeleteSecureLink handles DELETE /api/links/:id
func (h *LinkHandler) DeleteSecureLink(c fiber.Ctx) error {
	id := c.Params("id")
	tenantID := tenantIDFromCtx(c)
	if id == "" {
		utils.Logger(c).Warn("invalid link id")
		return c.Status(400).JSON(fiber.Map{"error": "invalid link id"})
	}
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.SecureLink{}).Error; err != nil {
		utils.Logger(c).Error("error deleting secure link", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}

// ViewLink handles GET /api/link/:token
func (h *LinkHandler) ViewLink(c fiber.Ctx) error {
	token := c.Params("token")
	tenantID := tenantIDFromCtx(c)
	var link models.SecureLink
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND token = ?", tenantID, token).First(&link).Error; err != nil {
		utils.Logger(c).Warn("link not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "link not found"})
	}
	_, valid := links.ValidateSecureToken(token, h.signingKey)
	if !valid || links.IsExpired(link.ExpiresAt) {
		utils.Logger(c).Warn("link expired or invalid")
		return c.Status(403).JSON(fiber.Map{"error": "link expired or invalid"})
	}
	var client models.Client
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantID, link.ClientID).First(&client).Error; err != nil {
		utils.Logger(c).Warn("client not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "client not found"})
	}
	if _, err := h.setLinkSessionCookie(c, link); err != nil {
		return err
	}
	logFileActivity(c.Context(), h.db, tenantID, link.ClientID, nil, nil, &link.ID, "view_link", "")
	return c.JSON(fiber.Map{"client": client, "link": link})
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
