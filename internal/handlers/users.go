package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UsersHandler struct {
	db               *gorm.DB
	emailQueue       emailpkg.EmailQueue
	baseURL          string
	tokenSecret      string
	resetTokenSecret string
	inviteTTL        time.Duration
	resetTokenTTL    time.Duration
	tenantSettings   *services.TenantSettingsService
}

func NewUsersHandler(db *gorm.DB, emailQueue emailpkg.EmailQueue, baseURL, tokenSecret, resetTokenSecret string, inviteTTL, resetTokenTTL time.Duration, tenantSettings *services.TenantSettingsService) *UsersHandler {
	return &UsersHandler{db: db, emailQueue: emailQueue, baseURL: baseURL, tokenSecret: tokenSecret, resetTokenSecret: resetTokenSecret, inviteTTL: inviteTTL, resetTokenTTL: resetTokenTTL, tenantSettings: tenantSettings}
}

func RegisterUserRoutes(router fiber.Router, db *gorm.DB, emailQueue emailpkg.EmailQueue, baseURL, tokenSecret, resetTokenSecret string, inviteTTL, resetTokenTTL time.Duration, tenantSettings *services.TenantSettingsService) {
	h := NewUsersHandler(db, emailQueue, baseURL, tokenSecret, resetTokenSecret, inviteTTL, resetTokenTTL, tenantSettings)
	router.Get("/users", middleware.RoleRequired("manager"), h.ListUsersHandler)
	router.Post("/users", middleware.RoleRequired("manager"), h.CreateUserHandler)
	router.Post("/users/resend-invites", middleware.RoleRequired("manager"), h.ResendAllInvitesHandler)
	router.Put("/users/:id", middleware.RoleRequired("manager"), h.UpdateUserHandler)
	router.Delete("/users/:id", middleware.RoleRequired("manager"), h.DeleteUserHandler)
	router.Post("/users/:id/generate-access-link", middleware.RoleRequired("manager"), h.GenerateAccessLinkHandler)
	router.Post("/users/:id/resend-invite", middleware.RoleRequired("manager"), h.ResendInviteHandler)
	router.Post("/users/:id/assign-client", middleware.RoleRequired("manager"), h.AssignManagerToClientHandler)
}

func (h *UsersHandler) inviteEmailSettings(c fiber.Ctx, tenantID string) (string, string) {
	if h.tenantSettings == nil {
		return services.DefaultSiteTitle, ""
	}
	settings, err := h.tenantSettings.Get(c.Context(), tenantID)
	if err != nil {
		utils.Logger(c).Warn("failed loading tenant settings for invite email", zap.Error(err), zap.String("tenant_id", tenantID))
		return services.DefaultSiteTitle, ""
	}
	return services.SiteTitleOrDefault(settings), strings.TrimSpace(settings.InviteWelcomeText)
}

func canManageUsers(role string) bool {
	return role == "admin" || role == "manager"
}

// GET /api/users - list all users (admin/manager only)
func (h *UsersHandler) ListUsersHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	search := strings.TrimSpace(c.Query("search"))
	rawRoleFilter := strings.TrimSpace(c.Query("role"))
	roleFilter := normalizeUserRole(rawRoleFilter)
	if rawRoleFilter != "" && roleFilter == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid role filter"})
	}
	rawInviteStatusFilter := strings.TrimSpace(c.Query("invite_status"))
	inviteStatusFilter := normalizeInviteStatusFilter(rawInviteStatusFilter)
	if rawInviteStatusFilter != "" && inviteStatusFilter == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid invite_status filter"})
	}
	clientIDFilter := strings.TrimSpace(c.Query("client_id"))
	if c.Query("page") != "" || c.Query("page_size") != "" || search != "" || roleFilter != "" || inviteStatusFilter != "" || clientIDFilter != "" {
		page := fiber.Query[int](c, "page", 1)
		pageSize := fiber.Query[int](c, "page_size", 20)
		result, err := h.listUsersPaginated(c.Context(), tenantID, search, roleFilter, inviteStatusFilter, clientIDFilter, page, pageSize)
		if err != nil {
			utils.Logger(c).Error("error listing users", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	}
	var users []models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ?", tenantID).Find(&users).Error; err != nil {
		utils.Logger(c).Error("error listing users", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.attachClientIDs(c.Context(), tenantID, users); err != nil {
		utils.Logger(c).Error("error loading user client assignments", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	h.attachInviteStatus(users)
	return c.JSON(users)
}

func (h *UsersHandler) listUsersPaginated(ctx context.Context, tenantID, search, roleFilter, inviteStatusFilter, clientIDFilter string, page, pageSize int) (*models.PaginatedResult[models.User], error) {
	page = normalizeUserListPage(page)
	pageSize = normalizeUserListPageSize(pageSize)

	q := h.userListQuery(ctx, tenantID, search, roleFilter, inviteStatusFilter, clientIDFilter)

	var totalItems int64
	if err := q.Count(&totalItems).Error; err != nil {
		return nil, err
	}

	totalPages := 0
	if totalItems > 0 {
		totalPages = int(math.Ceil(float64(totalItems) / float64(pageSize)))
		if page > totalPages {
			page = totalPages
		}
	}

	users := make([]models.User, 0)
	offset := (page - 1) * pageSize
	if err := q.Order("users.created_at DESC").Limit(pageSize).Offset(offset).Find(&users).Error; err != nil {
		return nil, err
	}
	if err := h.attachClientIDs(ctx, tenantID, users); err != nil {
		return nil, err
	}
	h.attachInviteStatus(users)

	return &models.PaginatedResult[models.User]{
		Pagination: models.PaginationResponse{
			TotalItems:   totalItems,
			TotalPages:   totalPages,
			Page:         page,
			PageSize:     pageSize,
			HasMore:      page < totalPages,
			ItemsPerPage: len(users),
		},
		Items: users,
	}, nil
}

func (h *UsersHandler) userListQuery(ctx context.Context, tenantID, search, roleFilter, inviteStatusFilter, clientIDFilter string) *gorm.DB {
	q := h.db.WithContext(ctx).Model(&models.User{}).Where("users.tenant_id = ?", tenantID)

	search = strings.TrimSpace(search)
	if search != "" {
		like := "%" + search + "%"
		matchingUserIDs := h.db.WithContext(ctx).Table("users").
			Distinct().
			Select("users.id").
			Joins("LEFT JOIN user_clients uc ON uc.tenant_id = users.tenant_id AND uc.user_id = users.id").
			Joins("LEFT JOIN clients c ON c.tenant_id = uc.tenant_id AND c.id = uc.client_id").
			Where("users.tenant_id = ?", tenantID).
			Where(
				"users.name LIKE ? OR users.email LIKE ? OR users.role LIKE ? OR c.name LIKE ?",
				like,
				like,
				like,
				like,
			)
		q = q.Where("id IN (?)", matchingUserIDs)
	}
	if roleFilter != "" {
		q = q.Where("users.role = ?", roleFilter)
	}
	switch inviteStatusFilter {
	case "accepted":
		q = q.Where("TRIM(COALESCE(users.password_hash, '')) <> ''")
	case "pending":
		q = q.Where("TRIM(COALESCE(users.password_hash, '')) = ''")
	}
	if clientIDFilter != "" {
		assignedUserIDs := h.db.WithContext(ctx).Table("user_clients").Select("user_id").Where("tenant_id = ? AND client_id = ?", tenantID, clientIDFilter)
		q = q.Where("users.id IN (?)", assignedUserIDs)
	}

	return q
}

func normalizeUserListPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizeUserListPageSize(pageSize int) int {
	if pageSize < 1 {
		return 20
	}
	return pageSize
}

func (h *UsersHandler) resolveInviteTTL() time.Duration {
	if h.inviteTTL > 0 {
		return h.inviteTTL
	}
	return 72 * time.Hour
}

func (h *UsersHandler) resolveResetTokenTTL() time.Duration {
	if h.resetTokenTTL > 0 {
		return h.resetTokenTTL
	}
	return 30 * time.Minute
}

func (h *UsersHandler) issueInviteForUser(tx *gorm.DB, tenantID string, user models.User, createdBy string, now time.Time) (models.InviteToken, string, error) {
	if strings.TrimSpace(h.tokenSecret) == "" {
		return models.InviteToken{}, "", fiber.NewError(fiber.StatusInternalServerError, "invite flow not configured")
	}

	rawToken, err := generateInviteToken()
	if err != nil {
		return models.InviteToken{}, "", err
	}

	inviteRecord := models.InviteToken{
		TenantID:  tenantID,
		UserID:    user.ID,
		Email:     user.Email,
		TokenHash: hashInviteToken(h.tokenSecret, rawToken),
		ExpiresAt: now.Add(h.resolveInviteTTL()),
		InvitedBy: strings.TrimSpace(createdBy),
		CreatedAt: now,
	}
	if err := tx.Create(&inviteRecord).Error; err != nil {
		return models.InviteToken{}, "", err
	}

	return inviteRecord, rawToken, nil
}

func (h *UsersHandler) issuePasswordResetForUser(tx *gorm.DB, tenantID string, user models.User, now time.Time) (models.PasswordResetToken, string, error) {
	if strings.TrimSpace(h.resetTokenSecret) == "" {
		return models.PasswordResetToken{}, "", fiber.NewError(fiber.StatusInternalServerError, "password reset flow not configured")
	}

	rawToken, err := generateResetToken()
	if err != nil {
		return models.PasswordResetToken{}, "", err
	}

	record := models.PasswordResetToken{
		TenantID:  tenantID,
		UserID:    user.ID,
		TokenHash: hashResetToken(h.resetTokenSecret, rawToken),
		ExpiresAt: now.Add(h.resolveResetTokenTTL()),
		CreatedAt: now,
	}
	if err := tx.Create(&record).Error; err != nil {
		return models.PasswordResetToken{}, "", err
	}

	return record, rawToken, nil
}

func (h *UsersHandler) userInviteAccepted(user models.User) bool {
	return strings.TrimSpace(user.PasswordHash) != ""
}

func (h *UsersHandler) attachInviteStatus(users []models.User) {
	for i := range users {
		users[i].InviteAccepted = h.userInviteAccepted(users[i])
	}
}

// POST /api/users - create user
func (h *UsersHandler) CreateUserHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	var req struct {
		Email     string   `json:"email"`
		Role      string   `json:"role"`
		Name      string   `json:"name"`
		ClientIDs []string `json:"client_ids"`
	}
	if err := c.Bind().Body(&req); err != nil {
		utils.Logger(c).Debug("invalid request body", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	var existing models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND email = ?", tenantID, req.Email).First(&existing).Error; err == nil {
		utils.Logger(c).Debug("user already exists", zap.String("email", req.Email))
		return c.Status(409).JSON(fiber.Map{"error": "user already exists"})
	}
	resolvedRole := normalizeUserRole(req.Role)
	if resolvedRole == "" {
		utils.Logger(c).Debug("invalid role", zap.String("role", req.Role))
		return c.Status(400).JSON(fiber.Map{"error": "invalid role"})
	}
	normalizedClientIDs := normalizeClientIDs(req.ClientIDs)
	if resolvedRole == "client" && len(normalizedClientIDs) == 0 {
		utils.Logger(c).Debug("client role requires client id")
		return c.Status(400).JSON(fiber.Map{"error": "client_ids is required for client users"})
	}
	now := time.Now()
	userName := strings.TrimSpace(req.Name)
	user := models.User{
		TenantID:     tenantID,
		Email:        strings.TrimSpace(req.Email),
		PasswordHash: "",
		Role:         resolvedRole,
		Name:         userName,
		ClientIDs:    normalizedClientIDs,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	inviteRecord := models.InviteToken{}
	inviteRawToken := ""
	createdBy := strings.TrimSpace(fiber.Locals[string](c, "user_id"))
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := h.replaceClientAssignments(tx, tenantID, user.ID, normalizedClientIDs); err != nil {
			return err
		}
		var err error
		inviteRecord, inviteRawToken, err = h.issueInviteForUser(tx, tenantID, user, createdBy, now)
		return err
	}); err != nil {
		utils.Logger(c).Error("error creating invited user", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	user.ClientIDs = normalizedClientIDs
	user.InviteAccepted = h.userInviteAccepted(user)
	h.sendUserInviteEmail(c, user, inviteRawToken, inviteRecord.ExpiresAt)
	return c.Status(201).JSON(user)
}

// POST /api/users/:id/generate-access-link - issue a fresh invite or password reset link without sending email
func (h *UsersHandler) GenerateAccessLinkHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantID, id).First(&user).Error; err != nil {
		utils.Logger(c).Debug("user not found", zap.Error(err))
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	baseURL := tenantBaseURLFromCtx(c, h.baseURL)
	now := time.Now()
	if h.userInviteAccepted(user) {
		resetRecord := models.PasswordResetToken{}
		resetRawToken := ""
		if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
			var err error
			resetRecord, resetRawToken, err = h.issuePasswordResetForUser(tx, tenantID, user, now)
			return err
		}); err != nil {
			utils.Logger(c).Error("error generating password reset link", zap.Error(err), zap.String("user_id", user.ID))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		resetLink := buildResetLink(baseURL, resetRawToken)
		if resetLink == "" {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "password reset link is unavailable"})
		}

		return c.JSON(fiber.Map{
			"kind":       "password_reset",
			"link":       resetLink,
			"expires_at": resetRecord.ExpiresAt,
		})
	}

	createdBy := strings.TrimSpace(fiber.Locals[string](c, "user_id"))
	inviteRecord := models.InviteToken{}
	inviteRawToken := ""
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		inviteRecord, inviteRawToken, err = h.issueInviteForUser(tx, tenantID, user, createdBy, now)
		return err
	}); err != nil {
		utils.Logger(c).Error("error generating invite link", zap.Error(err), zap.String("user_id", user.ID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	inviteLink := buildInviteAcceptURL(baseURL, inviteRawToken)
	if inviteLink == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "invite link is unavailable"})
	}

	return c.JSON(fiber.Map{
		"kind":       "invite",
		"link":       inviteLink,
		"expires_at": inviteRecord.ExpiresAt,
	})
}

// POST /api/users/:id/resend-invite - reissue a user invite email
func (h *UsersHandler) ResendInviteHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	id := c.Params("id")
	if strings.TrimSpace(id) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantID, id).First(&user).Error; err != nil {
		utils.Logger(c).Debug("user not found", zap.Error(err))
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	if h.userInviteAccepted(user) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user has already accepted the invite"})
	}

	clientIDs, err := h.listClientIDsForUser(c.Context(), tenantID, user.ID)
	if err != nil {
		utils.Logger(c).Error("error loading user client assignments", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	user.ClientIDs = clientIDs

	now := time.Now()
	createdBy := strings.TrimSpace(fiber.Locals[string](c, "user_id"))
	inviteRecord := models.InviteToken{}
	inviteRawToken := ""
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		inviteRecord, inviteRawToken, err = h.issueInviteForUser(tx, tenantID, user, createdBy, now)
		return err
	}); err != nil {
		utils.Logger(c).Error("error resending invite", zap.Error(err), zap.String("user_id", user.ID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	user.InviteAccepted = false
	h.sendUserInviteEmail(c, user, inviteRawToken, inviteRecord.ExpiresAt)
	return c.JSON(fiber.Map{"message": "invite resent"})
}

// POST /api/users/resend-invites - reissue invite emails for all pending users
func (h *UsersHandler) ResendAllInvitesHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	users := make([]models.User, 0)
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND COALESCE(password_hash, '') = ''", tenantID).Order("created_at DESC").Find(&users).Error; err != nil {
		utils.Logger(c).Error("error loading pending invite users", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if len(users) == 0 {
		return c.JSON(fiber.Map{"resent": 0})
	}
	if err := h.attachClientIDs(c.Context(), tenantID, users); err != nil {
		utils.Logger(c).Error("error loading pending invite client assignments", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	type inviteSend struct {
		user      models.User
		rawToken  string
		expiresAt time.Time
	}
	invites := make([]inviteSend, 0, len(users))
	now := time.Now()
	createdBy := strings.TrimSpace(fiber.Locals[string](c, "user_id"))
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		for _, user := range users {
			inviteRecord, inviteRawToken, err := h.issueInviteForUser(tx, tenantID, user, createdBy, now)
			if err != nil {
				return err
			}
			invites = append(invites, inviteSend{user: user, rawToken: inviteRawToken, expiresAt: inviteRecord.ExpiresAt})
		}
		return nil
	}); err != nil {
		utils.Logger(c).Error("error resending all invites", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	for _, invite := range invites {
		h.sendUserInviteEmail(c, invite.user, invite.rawToken, invite.expiresAt)
	}

	return c.JSON(fiber.Map{"resent": len(invites)})
}

func (h *UsersHandler) sendUserInviteEmail(c fiber.Ctx, user models.User, rawToken string, expiresAt time.Time) {
	if h.emailQueue == nil {
		utils.Logger(c).Warn("email queue unavailable for user invite email")
		return
	}
	if strings.TrimSpace(user.Email) == "" {
		utils.Logger(c).Warn("new user email missing, skipping invite email")
		return
	}

	inviteURL := buildInviteAcceptURL(tenantBaseURLFromCtx(c, h.baseURL), rawToken)
	siteTitle, inviteWelcomeText := h.inviteEmailSettings(c, user.TenantID)
	subject, body, htmlBody := emailpkg.RenderUserInviteEmail(user.Name, inviteURL, siteTitle, inviteWelcomeText, expiresAt)
	if _, err := h.emailQueue.Queue(emailpkg.Email{TenantID: user.TenantID, To: user.Email, Subject: subject, Body: body, HTMLBody: htmlBody, ClientID: firstClientIDPtr(user.ClientIDs)}, models.Now()); err != nil {
		utils.Logger(c).Error("failed to queue user invite email", zap.Error(err), zap.String("email", user.Email))
	}
}

func buildInviteAcceptURL(baseURL, token string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" || strings.TrimSpace(token) == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/accept-invite#token=" + token
}

func normalizeUserRole(role string) string {
	resolved := strings.ToLower(strings.TrimSpace(role))
	switch resolved {
	case "admin", "manager", "client":
		return resolved
	case "customer":
		return "client"
	default:
		return ""
	}
}

func normalizeInviteStatusFilter(status string) string {
	resolved := strings.ToLower(strings.TrimSpace(status))
	switch resolved {
	case "", "accepted", "pending":
		return resolved
	default:
		return ""
	}
}

func generateInviteToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashInviteToken(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

// PUT /api/users/:id - update user
func (h *UsersHandler) UpdateUserHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	id := c.Params("id")
	if id == "" {
		utils.Logger(c).Debug("invalid id")
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Email     *string   `json:"email"`
		Password  *string   `json:"password"`
		Role      *string   `json:"role"`
		Name      *string   `json:"name"`
		ClientIDs *[]string `json:"client_ids"`
	}
	if err := c.Bind().Body(&req); err != nil {
		utils.Logger(c).Debug("invalid request body", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantID, id).First(&user).Error; err != nil {
		utils.Logger(c).Debug("user not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	resolvedRole := user.Role
	if req.Role != nil {
		resolvedRole = normalizeUserRole(*req.Role)
		if resolvedRole == "" {
			utils.Logger(c).Debug("invalid role", zap.String("role", *req.Role))
			return c.Status(400).JSON(fiber.Map{"error": "invalid role"})
		}
		user.Role = resolvedRole
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			utils.Logger(c).Error("failed to hash password", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
		}
		user.PasswordHash = string(hash)
	}
	if req.Name != nil {
		user.Name = *req.Name
	}
	normalizedClientIDs := []string(nil)
	if req.ClientIDs != nil {
		normalizedClientIDs = normalizeClientIDs(*req.ClientIDs)
	}
	if resolvedRole == "client" {
		if req.ClientIDs != nil {
			if len(normalizedClientIDs) == 0 {
				return c.Status(400).JSON(fiber.Map{"error": "client_ids is required for client users"})
			}
		} else {
			existingClientIDs, err := h.listClientIDsForUser(c.Context(), tenantID, user.ID)
			if err != nil {
				utils.Logger(c).Error("error loading existing client assignments", zap.Error(err))
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			if len(existingClientIDs) == 0 {
				return c.Status(400).JSON(fiber.Map{"error": "client_ids is required for client users"})
			}
		}
	}
	user.UpdatedAt = time.Now()
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		if req.ClientIDs != nil {
			if err := h.replaceClientAssignments(tx, tenantID, user.ID, normalizedClientIDs); err != nil {
				return err
			}
			user.ClientIDs = normalizedClientIDs
			return nil
		}
		existingClientIDs, err := h.listClientIDsForUser(c.Context(), tenantID, user.ID)
		if err != nil {
			return err
		}
		user.ClientIDs = existingClientIDs
		return nil
	}); err != nil {
		utils.Logger(c).Error("error saving user", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	user.InviteAccepted = h.userInviteAccepted(user)
	return c.JSON(user)
}

// DELETE /api/users/:id - delete user
func (h *UsersHandler) DeleteUserHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	id := c.Params("id")
	if id == "" {
		utils.Logger(c).Debug("invalid id")
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.User{}).Error; err != nil {
		utils.Logger(c).Error("error deleting user", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}

// POST /api/users/:id/assign-client - assign manager to client
func (h *UsersHandler) AssignManagerToClientHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	userID := c.Params("id")
	if userID == "" {
		utils.Logger(c).Debug("invalid user id")
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	var req struct {
		ClientID string `json:"client_id"`
	}
	if err := c.Bind().Body(&req); err != nil {
		utils.Logger(c).Debug("invalid request body", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantID, userID).First(&user).Error; err != nil {
		utils.Logger(c).Debug("user not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if user.Role != "manager" {
		utils.Logger(c).Debug("user is not a manager", zap.String("role", user.Role))
		return c.Status(400).JSON(fiber.Map{"error": "user is not a manager"})
	}
	if strings.TrimSpace(req.ClientID) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "client_id is required"})
	}
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		if err := h.addClientAssignment(tx, tenantID, user.ID, req.ClientID); err != nil {
			return err
		}
		user.UpdatedAt = time.Now()
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		clientIDs := make([]string, 0)
		if err := tx.Model(&models.UserClient{}).
			Where("tenant_id = ? AND user_id = ?", tenantID, user.ID).
			Order("client_id ASC").
			Pluck("client_id", &clientIDs).Error; err != nil {
			return err
		}
		user.ClientIDs = clientIDs
		return nil
	}); err != nil {
		utils.Logger(c).Error("error saving user", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	user.InviteAccepted = h.userInviteAccepted(user)
	return c.JSON(user)
}

func normalizeClientIDs(clientIDs []string) []string {
	seen := make(map[string]struct{}, len(clientIDs))
	filtered := make([]string, 0, len(clientIDs))
	for _, raw := range clientIDs {
		clientID := strings.TrimSpace(raw)
		if clientID == "" {
			continue
		}
		if _, ok := seen[clientID]; ok {
			continue
		}
		seen[clientID] = struct{}{}
		filtered = append(filtered, clientID)
	}
	sort.Strings(filtered)
	return filtered
}

func firstClientIDPtr(clientIDs []string) *string {
	if len(clientIDs) == 0 {
		return nil
	}
	first := clientIDs[0]
	return &first
}

func (h *UsersHandler) attachClientIDs(ctx context.Context, tenantID string, users []models.User) error {
	if len(users) == 0 {
		return nil
	}
	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	clientIDsByUser, err := h.clientIDsByUserIDs(ctx, tenantID, userIDs)
	if err != nil {
		return err
	}
	for i := range users {
		users[i].ClientIDs = clientIDsByUser[users[i].ID]
	}
	return nil
}

func (h *UsersHandler) clientIDsByUserIDs(ctx context.Context, tenantID string, userIDs []string) (map[string][]string, error) {
	type row struct {
		UserID   string
		ClientID string
	}
	rows := make([]row, 0)
	if err := h.db.WithContext(ctx).Table("user_clients").
		Select("user_id, client_id").
		Where("tenant_id = ? AND user_id IN ?", tenantID, userIDs).
		Order("client_id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(userIDs))
	for _, id := range userIDs {
		result[id] = []string{}
	}
	for _, row := range rows {
		result[row.UserID] = append(result[row.UserID], row.ClientID)
	}
	return result, nil
}

func (h *UsersHandler) listClientIDsForUser(ctx context.Context, tenantID, userID string) ([]string, error) {
	result, err := h.clientIDsByUserIDs(ctx, tenantID, []string{userID})
	if err != nil {
		return nil, err
	}
	return result[userID], nil
}

func (h *UsersHandler) replaceClientAssignments(tx *gorm.DB, tenantID, userID string, clientIDs []string) error {
	if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Delete(&models.UserClient{}).Error; err != nil {
		return err
	}
	for _, clientID := range clientIDs {
		if err := h.addClientAssignment(tx, tenantID, userID, clientID); err != nil {
			return err
		}
	}
	return nil
}

func (h *UsersHandler) addClientAssignment(tx *gorm.DB, tenantID, userID, clientID string) error {
	rel := models.UserClient{TenantID: strings.TrimSpace(tenantID), UserID: strings.TrimSpace(userID), ClientID: strings.TrimSpace(clientID), CreatedAt: time.Now()}
	if rel.UserID == "" || rel.ClientID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "user_id and client_id are required")
	}
	if rel.TenantID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "tenant_id is required")
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rel).Error
}
