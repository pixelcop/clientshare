package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	router.Post("/api/links/:id/email", middleware.RoleRequired("admin"), handler.EmailSecureLink)
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
	if !valid || link.FileID != nil || link.RevokedAt != nil || links.IsExpired(link.ExpiresAt) {
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
		AccessType string     `json:"access_type"`
		ExpiryDays int        `json:"expiry_days"`
		Email      string     `json:"email"`
		ExpiresAt  *time.Time `json:"expires_at"`
	}
	var body req
	if err := c.Bind().Body(&body); err != nil {
		utils.Logger(c).Warn("invalid request body", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	creationKey, err := requestIdempotencyKey(c, "create-link:"+id, false)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
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
	if body.ExpiresAt != nil {
		expiry = body.ExpiresAt.UTC()
		if !expiry.After(time.Now()) || expiry.After(time.Now().Add(time.Duration(services.MaxSecureLinkDefaultExpiryDays)*24*time.Hour)) {
			return c.Status(400).JSON(fiber.Map{"error": "expires_at must be in the future and within the maximum link lifetime"})
		}
	}
	fingerprint, _ := json.Marshal(struct {
		AccessType string
		ExpiryDays int
		Email      string
		ExpiresAt  *time.Time
	}{body.AccessType, body.ExpiryDays, emailAddress, body.ExpiresAt})
	link := models.SecureLink{
		TenantID:     tenantID,
		ClientID:     id,
		Token:        token,
		Email:        emailAddress,
		ExpiresAt:    expiry,
		AccessType:   body.AccessType,
		CreatedBy:    createdBy,
		CreatedAt:    time.Now(),
		CreationKey:  creationKey,
		CreationHash: requestDigest(string(fingerprint)),
	}
	if err := services.PersistSecureLink(c.Context(), h.db, &link); err != nil {
		if errors.Is(err, services.ErrLinkRequestConflict) {
			return c.Status(409).JSON(fiber.Map{"error": err.Error()})
		}
		utils.Logger(c).Error("error creating secure link", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	link.LinkURL = buildSecureLinkURL(tenantBaseURLFromCtx(c, h.baseURL), link.Token)
	queued := false
	if emailAddress != "" {
		status, _ := h.queueSecureLinkEmail(c, link, emailAddress, requestDigest("create-link-email", link.ID), siteTitle)
		queued = status == "queued" || status == "sent"
	}
	return c.Status(201).JSON(struct {
		models.SecureLink
		EmailQueued bool `json:"email_queued"`
	}{link, queued})
}

func (h *LinkHandler) queueSecureLinkEmail(c fiber.Ctx, link models.SecureLink, recipient, key, siteTitle string) (string, error) {
	if link.FileID != nil || link.RevokedAt != nil || !link.ExpiresAt.After(time.Now()) {
		return "unavailable", errors.New("This link is expired or revoked")
	}
	if h.emailQueue == nil {
		return "unavailable", errors.New("Email is unavailable. Copy the existing link instead, or retry after email is configured.")
	}
	client, err := h.clientService.GetClient(c.Context(), link.TenantID, link.ClientID)
	if err != nil {
		utils.Logger(c).Error("error fetching client for secure link email", zap.Error(err))
		return "failed", errors.New("Could not load the link destination")
	}
	linkURL := buildSecureLinkURL(tenantBaseURLFromCtx(c, h.baseURL), link.Token)
	if linkURL == "" {
		utils.Logger(c).Warn("secure link base url missing")
		return "unavailable", errors.New("Secure link URL is unavailable")
	}
	subject, body, htmlBody := emailpkg.RenderSecureLinkEmail(client.Name, link.AccessType, linkURL, siteTitle, link.ExpiresAt)
	item, err := h.emailQueue.Queue(emailpkg.Email{TenantID: link.TenantID, To: recipient, Subject: subject, Body: body, HTMLBody: htmlBody, DeduplicationKey: key, SecureLinkID: &link.ID}, models.Now())
	if err != nil {
		utils.Logger(c).Error("failed to queue secure link email", zap.Error(err))
		return "failed", err
	}
	if item == nil {
		return "failed", errors.New("Email queue did not accept the message")
	}
	if item.CanceledAt != nil {
		return "unavailable", errors.New("This email request was canceled")
	}
	if item.SentAt != nil {
		return "sent", nil
	}
	return "queued", nil
}

// EmailSecureLink sends the already-created link. A retry key identifies one
// enqueue operation; changing recipient with that key is a conflict, not a new send.
func (h *LinkHandler) EmailSecureLink(c fiber.Ctx) error {
	key, err := requestIdempotencyKey(c, "email-link:"+c.Params("id"), true)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid email request"})
	}
	address, err := mail.ParseAddress(strings.TrimSpace(body.Email))
	if err != nil || strings.ContainsAny(body.Email, "\r\n") {
		return c.Status(400).JSON(fiber.Map{"error": "A valid recipient email is required"})
	}
	var link models.SecureLink
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantIDFromCtx(c), c.Params("id")).First(&link).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Link not found"})
	}
	if link.FileID != nil || link.RevokedAt != nil || !link.ExpiresAt.After(time.Now()) {
		return c.Status(410).JSON(fiber.Map{"status": "unavailable", "error": "This link is expired or revoked"})
	}
	siteTitle, _ := h.tenantEmailSettings(c, link.TenantID)
	status, queueErr := h.queueSecureLinkEmail(c, link, address.Address, *key, siteTitle)
	if queueErr != nil {
		code := 503
		message := "Could not queue the email. Retry sends this same link, not a new one."
		if errors.Is(queueErr, emailpkg.ErrIdempotencyConflict) {
			code, message = 409, queueErr.Error()
		}
		if status == "unavailable" {
			message = queueErr.Error()
		}
		return c.Status(code).JSON(fiber.Map{"status": status, "error": message})
	}
	return c.Status(202).JSON(fiber.Map{"status": status, "email_queued": true, "link_id": link.ID})
}

func requestDigest(parts ...string) string {
	value, _ := json.Marshal(parts)
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func requestIdempotencyKey(c fiber.Ctx, operation string, required bool) (*string, error) {
	key := c.Get("Idempotency-Key")
	if key == "" && !required {
		return nil, nil
	}
	if len(key) < 8 || len(key) > 128 {
		return nil, errors.New("Idempotency-Key must contain 8 to 128 printable characters")
	}
	for _, r := range key {
		if r < 33 || r > 126 {
			return nil, errors.New("Invalid Idempotency-Key")
		}
	}
	digest := requestDigest(tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), operation, key)
	return &digest, nil
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
	if _, err := h.clientService.GetClient(c.Context(), tenantID, id); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "client not found"})
	}
	var linksList []models.SecureLink
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND client_id = ?", tenantID, id).Order("created_at DESC, id DESC").Find(&linksList).Error; err != nil {
		utils.Logger(c).Error("error listing secure links", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	baseURL := tenantBaseURLFromCtx(c, h.baseURL)
	for index := range linksList {
		if linksList[index].FileID == nil && linksList[index].RevokedAt == nil && linksList[index].ExpiresAt.After(time.Now()) {
			linksList[index].LinkURL = buildSecureLinkURL(baseURL, linksList[index].Token)
		} else {
			linksList[index].Token = ""
		}
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
	if err := services.RevokeSecureLink(c.Context(), h.db, tenantID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "Link not found"})
		}
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
	if !valid || link.FileID != nil || link.RevokedAt != nil || links.IsExpired(link.ExpiresAt) {
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
