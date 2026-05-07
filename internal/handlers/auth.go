package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/links"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db                *gorm.DB
	signingKey        string
	resetTokenSecret  string
	inviteTokenSecret string
	resetTokenTTL     time.Duration
	inviteTokenTTL    time.Duration
	resetBaseURL      string
	tenantSettings    *services.TenantSettingsService
	emailQueue        emailpkg.EmailQueue
	feedService       *services.FeedService
}

func RegisterAuthRoutes(insecureApi fiber.Router, db *gorm.DB, signingKey string, emailQueue emailpkg.EmailQueue, resetBaseURL string, resetTokenSecret, inviteTokenSecret string, resetTokenTTL, inviteTokenTTL time.Duration, tenantSettings *services.TenantSettingsService, rateLimitEnabled bool) {
	h := &AuthHandler{
		db:                db,
		signingKey:        signingKey,
		resetTokenSecret:  resetTokenSecret,
		inviteTokenSecret: inviteTokenSecret,
		resetTokenTTL:     resetTokenTTL,
		inviteTokenTTL:    inviteTokenTTL,
		resetBaseURL:      resetBaseURL,
		tenantSettings:    tenantSettings,
		emailQueue:        emailQueue,
		feedService:       services.NewFeedService(db),
	}

	var l fiber.Handler
	if rateLimitEnabled {
		l = limiter.New()
	} else {
		// no-op middleware (only used in tests)
		l = func(c fiber.Ctx) error {
			return c.Next()
		}
	}

	insecureApi.Post("/auth/login", l, h.LoginHandler)
	insecureApi.Post("/auth/logout", h.LogoutHandler)
	insecureApi.Post("/auth/register", l, h.RegisterHandler)
	insecureApi.Post("/auth/forgot-password", l, h.ForgotPasswordHandler)
	insecureApi.Post("/auth/reset-password", l, h.ResetPasswordHandler)
	insecureApi.Post("/auth/accept-invite", h.AcceptInviteHandler)

	// secure with auth
	insecureApi.Get("/auth/me", middleware.AuthRequired, h.MeHandler)
}

func (h *AuthHandler) siteTitleForTenant(c fiber.Ctx, tenantID string) string {
	if h.tenantSettings == nil {
		return services.DefaultSiteTitle
	}
	settings, err := h.tenantSettings.Get(c.Context(), tenantID)
	if err != nil {
		utils.Logger(c).Warn("failed loading tenant settings for auth email", zap.Error(err), zap.String("tenant_id", tenantID))
		return services.DefaultSiteTitle
	}
	return services.SiteTitleOrDefault(settings)
}

// ForgotPasswordHandler handles /api/auth/forgot-password (requests a password reset email)
func (h *AuthHandler) ForgotPasswordHandler(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	if strings.TrimSpace(req.Email) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email is required"})
	}

	response := fiber.Map{"message": "If an account exists with this email, a reset link has been sent."}
	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND email = ?", tenantID, req.Email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(response)
		}
		utils.Logger(c).Error("failed to look up user", zap.Error(err))
		return c.JSON(response)
	}
	if h.resetTokenSecret == "" {
		utils.Logger(c).Warn("password reset secret missing")
		return c.JSON(response)
	}
	if h.resetTokenTTL <= 0 {
		h.resetTokenTTL = 30 * time.Minute
	}

	resetToken, err := generateResetToken()
	if err != nil {
		utils.Logger(c).Error("failed to generate reset token", zap.Error(err))
		return c.JSON(response)
	}
	resetHash := hashResetToken(h.resetTokenSecret, resetToken)
	now := time.Now()
	expiresAt := now.Add(h.resetTokenTTL)

	if err := h.db.WithContext(c.Context()).Model(&models.PasswordResetToken{}).
		Where("tenant_id = ? AND user_id = ? AND used_at IS NULL", tenantID, user.ID).
		Update("used_at", now).Error; err != nil {
		utils.Logger(c).Warn("failed to invalidate existing reset tokens", zap.Error(err))
	}

	record := models.PasswordResetToken{
		TenantID:  tenantID,
		UserID:    user.ID,
		TokenHash: resetHash,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
	if err := h.db.WithContext(c.Context()).Create(&record).Error; err != nil {
		utils.Logger(c).Error("failed to store reset token", zap.Error(err))
		return c.JSON(response)
	}

	resetLink := buildResetLink(tenantBaseURLFromCtx(c, h.resetBaseURL), resetToken)
	if resetLink == "" {
		utils.Logger(c).Warn("password reset base url missing")
		return c.JSON(response)
	}
	name := user.Name
	if strings.TrimSpace(name) == "" {
		name = "there"
	}
	subject, body, htmlBody := emailpkg.RenderPasswordReset(name, resetLink, h.siteTitleForTenant(c, tenantID), h.resetTokenTTL)
	if h.emailQueue == nil {
		utils.Logger(c).Warn("email queue unavailable for password reset")
		return c.JSON(response)
	}
	if _, err := h.emailQueue.Queue(emailpkg.Email{TenantID: user.TenantID, To: user.Email, Subject: subject, Body: body, HTMLBody: htmlBody}, models.Now()); err != nil {
		utils.Logger(c).Error("failed to queue reset email", zap.Error(err))
		return c.JSON(response)
	}
	return c.JSON(response)
}

// ResetPasswordHandler handles /api/auth/reset-password (resets password using token)
func (h *AuthHandler) ResetPasswordHandler(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	if strings.TrimSpace(req.Token) == "" || strings.TrimSpace(req.Password) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Token and password are required"})
	}
	if h.resetTokenSecret == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Password reset is not configured"})
	}

	resetHash := hashResetToken(h.resetTokenSecret, req.Token)
	var record models.PasswordResetToken
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND token_hash = ?", tenantID, resetHash).First(&record).Error; err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired token"})
	}
	if !hmac.Equal([]byte(record.TokenHash), []byte(resetHash)) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired token"})
	}
	now := time.Now()
	if record.UsedAt != nil || now.After(record.ExpiresAt) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired token"})
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	return h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, record.UserID).First(&user).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired token"})
		}
		updates := map[string]interface{}{
			"password_hash": hash,
			"updated_at":    now,
		}
		if err := tx.Model(&models.User{}).
			Where("tenant_id = ? AND id = ?", tenantID, user.ID).
			Updates(updates).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update password"})
		}
		if err := tx.Model(&models.PasswordResetToken{}).
			Where("tenant_id = ? AND id = ? AND used_at IS NULL", tenantID, record.ID).
			Update("used_at", now).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update token"})
		}
		return c.JSON(fiber.Map{"message": "Password reset successfully"})
	})
}

// AcceptInviteHandler handles /api/auth/accept-invite (sets initial password via invite token)
func (h *AuthHandler) AcceptInviteHandler(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	if strings.TrimSpace(req.Token) == "" || strings.TrimSpace(req.Password) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Token and password are required"})
	}
	if h.inviteTokenSecret == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invite flow is not configured"})
	}

	inviteHash := hashResetToken(h.inviteTokenSecret, req.Token)
	var invite models.InviteToken
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND token_hash = ?", tenantID, inviteHash).First(&invite).Error; err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired invite"})
	}
	if !hmac.Equal([]byte(invite.TokenHash), []byte(inviteHash)) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired invite"})
	}
	now := time.Now()
	if invite.UsedAt != nil || now.After(invite.ExpiresAt) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired invite"})
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	var authenticatedUser models.User
	err = h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, invite.UserID).First(&authenticatedUser).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired invite"})
		}

		if err := tx.Model(&models.User{}).
			Where("tenant_id = ? AND id = ?", tenantID, authenticatedUser.ID).
			Updates(map[string]interface{}{"password_hash": hash, "updated_at": now}).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update password"})
		}

		if err := tx.Model(&models.InviteToken{}).
			Where("tenant_id = ? AND id = ? AND used_at IS NULL", tenantID, invite.ID).
			Update("used_at", now).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update invite"})
		}

		authenticatedUser.PasswordHash = hash
		authenticatedUser.UpdatedAt = now
		return nil
	})
	if err != nil {
		return err
	}
	if err := h.feedService.RecordInviteAccepted(c.Context(), tenantID, invite.ID, authenticatedUser.ID, now); err != nil {
		utils.Logger(c).Warn("failed to record invite accepted feed event", zap.Error(err))
	}

	token, err := auth.GenerateJWT(authenticatedUser.ID, authenticatedUser.Role, authenticatedUser.TenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate token"})
	}
	setAuthSessionCookie(c, token)

	return c.JSON(fiber.Map{"user": authenticatedUser})
}

func generateResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashResetToken(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func buildResetLink(baseURL, token string) string {
	if strings.TrimSpace(baseURL) == "" {
		return ""
	}
	baseURL = strings.TrimRight(baseURL, "/")
	fragment := url.Values{}
	fragment.Set("token", token)
	return baseURL + "/reset-password#" + fragment.Encode()
}

func setAuthSessionCookie(c fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     "jwt",
		Value:    token,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
		Path:     "/",
		Expires:  time.Now().Add(auth.AuthExpiry),
	})
}

func clearAuthSessionCookie(c fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     "jwt",
		Value:    "",
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

// LoginHandler handles /api/auth/login
func (h *AuthHandler) LoginHandler(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND email = ?", tenantID, req.Email).First(&user).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}
	if !auth.CheckPasswordHash(req.Password, user.PasswordHash) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}
	token, err := auth.GenerateJWT(user.ID, user.Role, user.TenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate token"})
	}
	setAuthSessionCookie(c, token)
	return c.JSON(fiber.Map{"authenticated": true})
}

// LogoutHandler handles /api/auth/logout
func (h *AuthHandler) LogoutHandler(c fiber.Ctx) error {
	clearAuthSessionCookie(c)
	return c.SendStatus(fiber.StatusOK)
}

// MeHandler handles /api/auth/me
func (h *AuthHandler) MeHandler(c fiber.Ctx) error {
	if c.Cookies("jwt") == "" {
		authorization := strings.TrimSpace(c.Get("Authorization"))
		if authorization != "" {
			setAuthSessionCookie(c, strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")))
		}
	}

	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if role == "link" {
		clientID, _ := c.Locals("client_id").(string)
		linkID, _ := c.Locals("link_id").(string)
		return c.JSON(fiber.Map{
			"id":        "",
			"email":     "",
			"role":      "link",
			"name":      "Secure Link",
			"client_id": clientID,
			"link_id":   linkID,
		})
	}
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantID, userID).First(&user).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}
	var clientIDs []string
	if err := h.db.WithContext(c.Context()).Model(&models.UserClient{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Order("client_id ASC").Pluck("client_id", &clientIDs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load client assignments"})
	}
	return c.JSON(fiber.Map{
		"id":         user.ID,
		"email":      user.Email,
		"role":       user.Role,
		"name":       user.Name,
		"client_ids": clientIDs,
	})
}

// RegisterHandler handles /api/auth/register via secure link
func (h *AuthHandler) RegisterHandler(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	var req struct {
		Token    string `json:"token"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	// Validate token
	var link models.SecureLink
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND token = ?", tenantID, req.Token).First(&link).Error; err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Invalid or expired link"})
	}
	_, valid := links.ValidateSecureToken(req.Token, h.signingKey)
	if !valid || links.IsExpired(link.ExpiresAt) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Invalid or expired link"})
	}
	// Check if user already exists
	var existing models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND email = ?", tenantID, req.Email).First(&existing).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "User already exists"})
	}
	// Create user
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash password"})
	}
	user := models.User{
		TenantID:     tenantID,
		Email:        req.Email,
		PasswordHash: hash,
		Role:         "client",
		Name:         req.Name,
	}
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		rel := models.UserClient{TenantID: tenantID, UserID: user.ID, ClientID: link.ClientID, CreatedAt: time.Now()}
		if err := tx.Create(&rel).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, link.ID).Delete(&models.SecureLink{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create user"})
	}
	user.ClientIDs = []string{link.ClientID}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"user": user})
}
