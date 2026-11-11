package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	RegisterAccessRoleRoutes(router, db)
	h := NewUsersHandler(db, emailQueue, baseURL, tokenSecret, resetTokenSecret, inviteTTL, resetTokenTTL, tenantSettings)
	router.Get("/users", middleware.RoleRequired("manager"), h.ListUsersHandler)
	router.Post("/users", middleware.RoleRequired("manager"), h.CreateUserHandler)
	router.Post("/users/resend-invites", middleware.RoleRequired("manager"), h.ResendAllInvitesHandler)
	router.Put("/users/:id", middleware.RoleRequired("admin"), h.UpdateUserHandler)
	router.Delete("/users/:id", middleware.RoleRequired("admin"), h.DeleteUserHandler)
	router.Post("/users/:id/generate-access-link", middleware.RoleRequired("admin"), h.GenerateAccessLinkHandler)
	router.Post("/users/:id/resend-invite", middleware.RoleRequired("manager"), h.ResendInviteHandler)
	router.Post("/users/:id/assign-client", middleware.RoleRequired("admin"), h.AssignManagerToClientHandler)
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

func (h *UsersHandler) canManageCurrentUsers(c fiber.Ctx) bool {
	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantIDFromCtx(c), fiber.Locals[string](c, "user_id")).First(&user).Error; err != nil {
		return false
	}
	if user.Role != "admin" && user.Role != "manager" {
		return false
	}
	profile, err := services.AccessProfileFor(h.db.WithContext(c.Context()), user)
	return err == nil && (profile.OrganizationRole == "administrator" || profile.CanManageUsers)
}

// GET /api/users - list all users (admin/manager only)
func (h *UsersHandler) ListUsersHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) || !h.canManageCurrentUsers(c) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	search := strings.TrimSpace(c.Query("search"))
	rawRoleFilter := strings.TrimSpace(c.Query("role"))
	roleFilter := normalizeUserRole(rawRoleFilter)
	if rawRoleFilter == "internal" || rawRoleFilter == "external" {
		roleFilter = rawRoleFilter
	}
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
		result, err := h.listUsersPaginated(c.Context(), tenantID, fiber.Locals[string](c, "user_id"), search, roleFilter, inviteStatusFilter, clientIDFilter, page, pageSize)
		if err != nil {
			utils.Logger(c).Error("error listing users", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if err := h.redactUserClients(c, result.Items); err != nil {
			return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
		}
		return c.JSON(result)
	}
	var users []models.User
	q, err := h.userListQuery(c.Context(), tenantID, fiber.Locals[string](c, "user_id"), "", "", "", "")
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
	}
	if err := q.Find(&users).Error; err != nil {
		utils.Logger(c).Error("error listing users", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.attachClientIDs(c.Context(), tenantID, users); err != nil {
		utils.Logger(c).Error("error loading user client assignments", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	h.attachInviteStatus(users)
	if err := h.redactUserClients(c, users); err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
	}
	return c.JSON(users)
}

func (h *UsersHandler) listUsersPaginated(ctx context.Context, tenantID, actorID, search, roleFilter, inviteStatusFilter, clientIDFilter string, page, pageSize int) (*models.PaginatedResult[models.User], error) {
	page = normalizeUserListPage(page)
	pageSize = normalizeUserListPageSize(pageSize)

	q, err := h.userListQuery(ctx, tenantID, actorID, search, roleFilter, inviteStatusFilter, clientIDFilter)
	if err != nil {
		return nil, err
	}

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

func (h *UsersHandler) userListQuery(ctx context.Context, tenantID, actorID, search, roleFilter, inviteStatusFilter, clientIDFilter string) (*gorm.DB, error) {
	q := h.db.WithContext(ctx).Model(&models.User{}).Where("users.tenant_id = ?", tenantID)
	var actor models.User
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, actorID).First(&actor).Error; err != nil {
		return nil, err
	}
	var clientScope *gorm.DB
	pendingRecipients := func() *gorm.DB {
		return h.db.WithContext(ctx).Table("access_invitations").Select("recipient_user_id").
			Where("tenant_id = ? AND recipient_user_id IS NOT NULL AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?", tenantID, models.Now())
	}
	namedRecipients := func(table string) *gorm.DB {
		return h.db.WithContext(ctx).Table(table).Select("user_id").
			Where("tenant_id = ? AND user_id IS NOT NULL AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", tenantID, models.Now())
	}
	if actor.Role != "admin" {
		if actor.Role != "manager" {
			return q.Where("1 = 0"), nil
		}
		var err error
		clientScope, err = (&services.AccessService{DB: h.db}).ClientScope(ctx, tenantID, actorID, services.AccessManageClient)
		if err != nil {
			return nil, err
		}
		q = q.Where("users.role IN ?", []string{"client", "customer"}).Where(`(EXISTS (SELECT 1 FROM user_clients roster_uc WHERE roster_uc.tenant_id = users.tenant_id AND roster_uc.user_id = users.id AND roster_uc.client_id IN (?)) OR users.id IN (?) OR users.id IN (?) OR users.id IN (?))`,
			clientScope.Select("clients.id"),
			pendingRecipients().Where("client_id IN (?)", clientScope.Select("clients.id")),
			namedRecipients("item_grants").Where("client_id IN (?)", clientScope.Select("clients.id")),
			namedRecipients("role_assignments").Where("client_id IN (?)", clientScope.Select("clients.id")))
	}

	search = strings.TrimSpace(search)
	if search != "" {
		like := "%" + search + "%"
		matchingUserIDs := h.db.WithContext(ctx).Table("users").
			Distinct().
			Select("users.id")
		if clientScope != nil {
			// Restrict client-name matches, not email/name matches for pending contacts.
			matchingUserIDs = matchingUserIDs.Joins("LEFT JOIN user_clients uc ON uc.tenant_id = users.tenant_id AND uc.user_id = users.id AND uc.client_id IN (?)", clientScope.Select("clients.id"))
		} else {
			matchingUserIDs = matchingUserIDs.Joins("LEFT JOIN user_clients uc ON uc.tenant_id = users.tenant_id AND uc.user_id = users.id")
		}
		matchingUserIDs = matchingUserIDs.
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
		switch roleFilter {
		case "internal":
			q = q.Where("users.role IN ?", []string{"admin", "manager"})
		case "external", "client":
			q = q.Where("users.role IN ?", []string{"client", "customer"})
		default:
			q = q.Where("users.role = ?", roleFilter)
		}
	}
	switch inviteStatusFilter {
	case "accepted":
		q = q.Where(userHasSignInCredentialSQL)
	case "pending":
		q = q.Where("NOT " + userHasSignInCredentialSQL)
	}
	if clientIDFilter != "" {
		assignedUserIDs := h.db.WithContext(ctx).Table("user_clients").Select("user_id").Where("tenant_id = ? AND client_id = ?", tenantID, clientIDFilter)
		pendingUserIDs := pendingRecipients().Where("client_id = ?", clientIDFilter)
		grantUserIDs := namedRecipients("item_grants").Where("client_id = ?", clientIDFilter)
		assignmentUserIDs := namedRecipients("role_assignments").Where("client_id = ?", clientIDFilter)
		if clientScope != nil {
			assignedUserIDs = assignedUserIDs.Where("client_id IN (?)", clientScope.Select("clients.id"))
			pendingUserIDs = pendingUserIDs.Where("client_id IN (?)", clientScope.Select("clients.id"))
			grantUserIDs = grantUserIDs.Where("client_id IN (?)", clientScope.Select("clients.id"))
			assignmentUserIDs = assignmentUserIDs.Where("client_id IN (?)", clientScope.Select("clients.id"))
		}
		q = q.Where("(users.id IN (?) OR users.id IN (?) OR users.id IN (?) OR users.id IN (?))", assignedUserIDs, pendingUserIDs, grantUserIDs, assignmentUserIDs)
	}

	return q, nil
}

func (h *UsersHandler) currentAdmin(c fiber.Ctx) bool {
	var count int64
	err := h.db.WithContext(c.Context()).Model(&models.User{}).Where("tenant_id = ? AND id = ? AND role = 'admin'", tenantIDFromCtx(c), fiber.Locals[string](c, "user_id")).Count(&count).Error
	return err == nil && count == 1
}

func (h *UsersHandler) redactUserClients(c fiber.Ctx, users []models.User) error {
	if h.currentAdmin(c) {
		return nil
	}
	q, err := (&services.AccessService{DB: h.db}).ClientScope(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), services.AccessManageClient)
	if err != nil {
		return err
	}
	var ids []string
	if err := q.Pluck("clients.id", &ids).Error; err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, id := range ids {
		allowed[id] = true
	}
	for i := range users {
		visible := []string{}
		for _, id := range users[i].ClientIDs {
			if allowed[id] {
				visible = append(visible, id)
			}
		}
		users[i].ClientIDs = visible
	}
	return nil
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

const userHasSignInCredentialSQL = "(TRIM(COALESCE(users.password_hash, '')) <> '' OR EXISTS (SELECT 1 FROM passkey_credentials p WHERE p.tenant_id = users.tenant_id AND p.user_id = users.id))"

func (h *UsersHandler) userInviteAccepted(user models.User) bool {
	if strings.TrimSpace(user.PasswordHash) != "" {
		return true
	}
	var count int64
	err := h.db.Model(&models.PasskeyCredential{}).Where("tenant_id = ? AND user_id = ?", user.TenantID, user.ID).Count(&count).Error
	// Do not allow an invite reset when credential lookup is unavailable.
	return err != nil || count > 0
}

func (h *UsersHandler) attachInviteStatus(users []models.User) {
	for i := range users {
		users[i].InviteAccepted = h.userInviteAccepted(users[i])
		if profile, err := services.AccessProfileFor(h.db, users[i]); err == nil {
			users[i].AccessProfile = &profile
		}
	}
}

// POST /api/users - create user
func (h *UsersHandler) CreateUserHandler(c fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) || !h.canManageCurrentUsers(c) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	var req struct {
		services.UserAccessInput
		Email             string                     `json:"email"`
		Role              string                     `json:"role"`
		Name              string                     `json:"name"`
		ClientIDs         []string                   `json:"client_ids"`
		Capabilities      *models.AccessCapabilities `json:"capabilities"`
		ManagerFullAccess bool                       `json:"manager_full_access"`
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
	if req.UserType != "" {
		var err error
		resolvedRole, err = req.UserAccessInput.CompatibilityRole()
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		req.ManagerFullAccess = req.AllWorkspaces
	}
	if resolvedRole == "" {
		utils.Logger(c).Debug("invalid role", zap.String("role", req.Role))
		return c.Status(400).JSON(fiber.Map{"error": "invalid role"})
	}
	if !h.currentAdmin(c) && (resolvedRole != "client" || req.ManagerFullAccess) {
		return c.Status(403).JSON(fiber.Map{"error": "Only administrators may manage staff roles or full access"})
	}
	normalizedClientIDs := normalizeClientIDs(req.ClientIDs)
	if req.UserType != "" && req.WorkspaceAccess != nil {
		ids := []string{}
		for _, a := range *req.WorkspaceAccess {
			ids = append(ids, a.ClientID)
		}
		normalizedClientIDs = normalizeClientIDs(ids)
	}
	if resolvedRole == "client" && len(normalizedClientIDs) == 0 && req.UserType == "" {
		utils.Logger(c).Debug("client role requires client id")
		return c.Status(400).JSON(fiber.Map{"error": "client_ids is required for client users"})
	}
	now := time.Now()
	userName := strings.TrimSpace(req.Name)
	user := models.User{
		TenantID:          tenantID,
		Email:             strings.TrimSpace(req.Email),
		PasswordHash:      "",
		Role:              resolvedRole,
		Name:              userName,
		ClientIDs:         normalizedClientIDs,
		CreatedAt:         now,
		UpdatedAt:         now,
		ManagerFullAccess: resolvedRole == "manager" && req.ManagerFullAccess,
	}
	inviteRecord := models.InviteToken{}
	inviteRawToken := ""
	createdBy := strings.TrimSpace(fiber.Locals[string](c, "user_id"))
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		if err := services.LockTenantAccess(tx, tenantID); err != nil {
			return err
		}
		var actor models.User
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, createdBy).First(&actor).Error; err != nil {
			return err
		}
		if profile, err := services.AccessProfileFor(tx, actor); err != nil {
			return err
		} else if actor.Role != "admin" && (!profile.CanManageUsers || (resolvedRole != "client" && resolvedRole != "customer")) {
			return services.ErrRoleForbidden
		}
		caps := models.AccessCapabilities{CanRead: true, CanUpload: true, CanShare: true}
		if req.Capabilities != nil {
			if req.UserType == "" && req.Capabilities.Sharing != nil {
				return services.ErrRoleInput
			}
			caps = *req.Capabilities
		}
		if req.UserType == "" {
			for _, clientID := range normalizedClientIDs {
				d, err := (&services.AccessService{DB: tx}).Resolve(c.Context(), middleware.AccessRequest(c, clientID, "", services.AccessInviteClient))
				if err != nil {
					return err
				}
				if !d.Allowed || !d.CanGrant(caps) {
					return fiber.ErrNotFound
				}
			}
		}
		if err := services.CheckStaffQuota(tx, &user); err != nil {
			return err
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if req.UserType != "" {
			if err := services.ApplyUserAccess(c.Context(), tx, tenantID, createdBy, user, req.UserAccessInput); err != nil {
				return err
			}
		} else if err := h.replaceClientAssignments(tx, tenantID, user.ID, createdBy, normalizedClientIDs); err != nil {
			return err
		}
		if resolvedRole == "client" && req.UserType == "" {
			if err := tx.Model(&models.UserClient{}).Where("tenant_id = ? AND user_id = ?", tenantID, user.ID).Updates(map[string]any{
				"capabilities_set": true, "can_read": caps.CanRead, "can_upload": caps.CanUpload, "can_delete": caps.CanDelete, "can_share": caps.CanShare,
				"access_source": "invitation", "invited_by": createdBy,
			}).Error; err != nil {
				return err
			}
		}
		var err error
		inviteRecord, inviteRawToken, err = h.issueInviteForUser(tx, tenantID, user, createdBy, now)
		return err
	}); err != nil {
		utils.Logger(c).Error("error creating invited user", zap.Error(err))
		if accessErr, ok := err.(*fiber.Error); ok {
			return c.Status(accessErr.Code).JSON(fiber.Map{"error": accessErr.Message})
		}
		if errors.Is(err, services.ErrRoleInput) || errors.Is(err, services.ErrSharingCapabilities) {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if errors.Is(err, services.ErrRoleForbidden) {
			return c.Status(403).JSON(fiber.Map{"error": err.Error()})
		}
		if respondQuotaError(c, err) {
			return nil
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	user.ClientIDs = normalizedClientIDs
	user.InviteAccepted = h.userInviteAccepted(user)
	h.sendUserInviteEmail(c, user, inviteRawToken, inviteRecord.ExpiresAt)
	return c.Status(201).JSON(user)
}

// POST /api/users/:id/generate-access-link - issue a fresh invite or password reset link without sending email
func (h *UsersHandler) GenerateAccessLinkHandler(c fiber.Ctx) error {
	if !h.currentAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Only administrators may issue account access links"})
	}
	role, _ := c.Locals("role").(string)
	tenantID := tenantIDFromCtx(c)
	if !canManageUsers(role) || !h.canManageCurrentUsers(c) {
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
	if !canManageUsers(role) || !h.canManageCurrentUsers(c) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	id := c.Params("id")
	if strings.TrimSpace(id) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	var user models.User
	q, err := h.userListQuery(c.Context(), tenantID, fiber.Locals[string](c, "user_id"), "", "", "", "")
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
	}
	if err := q.Where("users.id = ?", id).First(&user).Error; err != nil {
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
	if !canManageUsers(role) || !h.canManageCurrentUsers(c) {
		utils.Logger(c).Debug("forbidden", zap.String("role", role))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	users := make([]models.User, 0)
	q, err := h.userListQuery(c.Context(), tenantID, fiber.Locals[string](c, "user_id"), "", "", "pending", "")
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
	}
	if err := q.Order("users.created_at DESC").Find(&users).Error; err != nil {
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
	if err := h.redactUserClients(c, users); err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
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
	if !h.currentAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Only administrators may change tenant-wide identity or staff settings"})
	}
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
		services.UserAccessInput
		Email             *string   `json:"email"`
		Password          *string   `json:"password"`
		Role              *string   `json:"role"`
		Name              *string   `json:"name"`
		ClientIDs         *[]string `json:"client_ids"`
		ManagerFullAccess *bool     `json:"manager_full_access"`
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
	if req.UserType != "" {
		var err error
		resolvedRole, err = req.UserAccessInput.CompatibilityRole()
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if id == fiber.Locals[string](c, "user_id") && resolvedRole != "admin" {
			return c.Status(400).JSON(fiber.Map{"error": "You cannot remove your own administrator access"})
		}
		user.Role = resolvedRole
		req.Role = nil
		req.ManagerFullAccess = &req.AllWorkspaces
	}
	if req.Role != nil {
		resolvedRole = normalizeUserRole(*req.Role)
		if resolvedRole == "" {
			utils.Logger(c).Debug("invalid role", zap.String("role", *req.Role))
			return c.Status(400).JSON(fiber.Map{"error": "invalid role"})
		}
		user.Role = resolvedRole
	}
	if id == fiber.Locals[string](c, "user_id") && resolvedRole != "admin" {
		return c.Status(400).JSON(fiber.Map{"error": "You cannot remove your own administrator access"})
	}
	if resolvedRole != "manager" {
		user.ManagerFullAccess = false
	} else if req.ManagerFullAccess != nil {
		user.ManagerFullAccess = *req.ManagerFullAccess
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
	// Item-only and removed client members legitimately have zero memberships.
	user.UpdatedAt = time.Now()
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		if err := services.RequireOrganizationAdministrator(tx, tenantID, fiber.Locals[string](c, "user_id")); err != nil {
			return err
		}
		if err := services.CheckStaffQuota(tx, &user); err != nil {
			return err
		}
		if req.UserType != "" {
			if err := services.ApplyUserAccess(c.Context(), tx, tenantID, fiber.Locals[string](c, "user_id"), user, req.UserAccessInput); err != nil {
				return err
			}
		}
		if req.UserType == "" && (req.Role != nil || req.ManagerFullAccess != nil) {
			profile, err := services.AccessProfileFor(tx, user)
			if err != nil {
				return err
			}
			if req.Role != nil {
				profile = services.LegacyAccessProfile(user)
			}
			profile.AllWorkspaces = user.Role == "admin" || (user.Role == "manager" && user.ManagerFullAccess)
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"user_type", "organization_role", "can_manage_users", "all_workspaces"})}).Create(&profile).Error; err != nil {
				return err
			}
		}
		if req.ClientIDs != nil && req.UserType == "" {
			if err := h.replaceClientAssignments(tx, tenantID, user.ID, fiber.Locals[string](c, "user_id"), normalizedClientIDs); err != nil {
				return err
			}
			user.ClientIDs = normalizedClientIDs
		} else {
			ids := []string{}
			if err := tx.Model(&models.UserClient{}).Where("tenant_id = ? AND user_id = ?", tenantID, user.ID).Order("client_id ASC").Pluck("client_id", &ids).Error; err != nil {
				return err
			}
			user.ClientIDs = ids
		}
		return tx.Save(&user).Error
	}); err != nil {
		utils.Logger(c).Error("error saving user", zap.Error(err))
		if errors.Is(err, services.ErrRoleInput) || errors.Is(err, services.ErrSharingCapabilities) {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if errors.Is(err, services.ErrRoleForbidden) {
			return c.Status(403).JSON(fiber.Map{"error": err.Error()})
		}
		if respondQuotaError(c, err) {
			return nil
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	user.InviteAccepted = h.userInviteAccepted(user)
	return c.JSON(user)
}

// DELETE /api/users/:id - delete user
func (h *UsersHandler) DeleteUserHandler(c fiber.Ctx) error {
	if !h.currentAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Only administrators may delete tenant-wide identities"})
	}
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
	if err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		var grants []models.ItemGrant
		if err := services.RequireOrganizationAdministrator(tx, tenantID, fiber.Locals[string](c, "user_id")); err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, id).Find(&grants).Error; err != nil {
			return err
		}
		// Only recipient access is removed. Onward grants/links created by this
		// identity survive; creator IDs are audit provenance, not dependencies.
		var recipient models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id).First(&recipient).Error; err != nil {
			return err
		}
		req := services.AccessRequest{TenantID: tenantID, Actor: services.AccessActor{UserID: fiber.Locals[string](c, "user_id")}}
		if err := services.RevokePendingRecipientInvitations(tx, req, id, recipient.Email, time.Now().UTC()); err != nil {
			return err
		}
		for _, model := range []any{&models.ItemGrant{}, &models.UserClient{}, &models.InviteToken{}, &models.RoleAssignment{}, &models.AccessGroupMember{}, &models.AccessProfile{}} {
			if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, id).Delete(model).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.User{}).Error; err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{"recipient_id": id, "removed_grants": grants})
		if err != nil {
			return err
		}
		return tx.Create(&models.AuditEvent{TenantID: &tenantID, Actor: fiber.Locals[string](c, "user_id"), Action: "user.delete", TargetID: &id, Payload: string(payload)}).Error
	}); err != nil {
		utils.Logger(c).Error("error deleting user", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}

// POST /api/users/:id/assign-client - assign manager to client
func (h *UsersHandler) AssignManagerToClientHandler(c fiber.Ctx) error {
	if !h.currentAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Only administrators may assign staff"})
	}
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
		if err := services.RequireOrganizationAdministrator(tx, tenantID, fiber.Locals[string](c, "user_id")); err != nil {
			return err
		}
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

func (h *UsersHandler) replaceClientAssignments(tx *gorm.DB, tenantID, userID, actorID string, clientIDs []string) error {
	var existing []models.UserClient
	if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Find(&existing).Error; err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, clientID := range clientIDs {
		wanted[clientID] = true
	}
	for _, membership := range existing {
		if wanted[membership.ClientID] {
			continue // never reset existing capabilities or invitation provenance
		}
		req := services.AccessRequest{TenantID: tenantID, ClientID: membership.ClientID, Actor: services.AccessActor{UserID: actorID}}
		if err := (&services.SharingService{DB: tx}).RemoveMember(tx.Statement.Context, req, userID, false); err != nil {
			return err
		}
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
	var user models.User
	if err := tx.Where("tenant_id = ? AND id = ?", rel.TenantID, rel.UserID).First(&user).Error; err != nil {
		return fiber.NewError(404, "Not Found")
	}
	var client models.Client
	if err := tx.Where("tenant_id = ? AND id = ?", rel.TenantID, rel.ClientID).First(&client).Error; err != nil {
		return fiber.NewError(404, "Not Found")
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rel).Error
}
