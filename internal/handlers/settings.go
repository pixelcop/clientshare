package handlers

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
)

type SettingsHandler struct {
	service             *services.TenantSettingsService
	publicDir           string
	fallbackBaseURL     string
	tenancyMode         string
	accountDashboardURL string
}

func NewSettingsHandler(service *services.TenantSettingsService, publicDir, fallbackBaseURL, tenancyMode, hostedPasskeyOrigin string) *SettingsHandler {
	accountDashboardURL := ""
	if origin := strings.TrimRight(strings.TrimSpace(hostedPasskeyOrigin), "/"); origin != "" {
		accountDashboardURL = origin + "/account"
	}

	return &SettingsHandler{
		service:             service,
		publicDir:           publicDir,
		fallbackBaseURL:     fallbackBaseURL,
		tenancyMode:         normalizeTenancyMode(tenancyMode),
		accountDashboardURL: accountDashboardURL,
	}
}

func RegisterSettingsRoutes(insecureAPI, api fiber.Router, service *services.TenantSettingsService, publicDir, fallbackBaseURL, tenancyMode, hostedPasskeyOrigin string) {
	handler := NewSettingsHandler(service, publicDir, fallbackBaseURL, tenancyMode, hostedPasskeyOrigin)

	insecureAPI.Get("/settings/branding", handler.GetBranding)
	api.Get("/settings/tenant", middleware.RoleRequired("admin"), handler.GetTenantSettings)
	api.Put("/settings/tenant", middleware.RoleRequired("admin"), handler.UpdateTenantSettings)
	api.Get("/settings/system", middleware.RoleRequired("admin"), handler.GetSystemSettings)
	api.Put("/settings/branding", middleware.RoleRequired("admin"), handler.UpdateBranding)
	api.Put("/settings/system", middleware.RoleRequired("admin"), handler.UpdateSystemSettings)
}

type brandingUpdateRequest struct {
	SiteTitle    string `json:"site_title"`
	PrimaryColor string `json:"primary_color"`
	RemoveLogo   bool   `json:"remove_logo"`
}

type tenantSettingsUpdateRequest struct {
	SiteTitle                   string `json:"site_title"`
	PrimaryColor                string `json:"primary_color"`
	RemoveLogo                  bool   `json:"remove_logo"`
	PublicBaseURL               string `json:"public_base_url"`
	InviteWelcomeText           string `json:"invite_welcome_text"`
	SecureLinkDefaultExpiryDays int    `json:"secure_link_default_expiry_days"`
}

type logLevelUpdateRequest struct {
	Level string `json:"level"`
}

type logLevelSettingsResponse struct {
	Level   string   `json:"level"`
	Options []string `json:"options"`
}

type systemSettingsResponse struct {
	TenancyMode string                    `json:"tenancy_mode"`
	LogLevel    *logLevelSettingsResponse `json:"log_level,omitempty"`
}

func (h *SettingsHandler) GetBranding(c fiber.Ctx) error {
	settings, err := h.service.Get(c.Context(), tenantIDFromCtx(c))
	if err != nil {
		utils.Logger(c).Error("failed loading tenant branding", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load branding settings"})
	}
	return c.JSON(brandingResponse(settings, effectiveTenantBaseURL(c, settings.PublicBaseURL, h.fallbackBaseURL), h.accountDashboardURL))
}

func (h *SettingsHandler) GetTenantSettings(c fiber.Ctx) error {
	settings, err := h.service.Get(c.Context(), tenantIDFromCtx(c))
	if err != nil {
		utils.Logger(c).Error("failed loading tenant settings", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load tenant settings"})
	}
	return c.JSON(tenantSettingsResponse(settings, effectiveTenantBaseURL(c, settings.PublicBaseURL, h.fallbackBaseURL)))
}

func (h *SettingsHandler) UpdateBranding(c fiber.Ctx) error {
	var req brandingUpdateRequest
	if err := bindBrandingUpdateRequest(c, &req); err != nil {
		utils.Logger(c).Debug("invalid request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	updated, err := h.updateTenantSettings(c, tenantSettingsUpdateRequest{
		SiteTitle:    req.SiteTitle,
		PrimaryColor: req.PrimaryColor,
		RemoveLogo:   req.RemoveLogo,
	}, true)
	if err != nil {
		return c.Status(err.Code).JSON(fiber.Map{"error": err.Message})
	}

	return c.JSON(brandingResponse(updated, effectiveTenantBaseURL(c, updated.PublicBaseURL, h.fallbackBaseURL), h.accountDashboardURL))
}

func (h *SettingsHandler) UpdateTenantSettings(c fiber.Ctx) error {
	var req tenantSettingsUpdateRequest
	if err := bindTenantSettingsUpdateRequest(c, &req); err != nil {
		utils.Logger(c).Debug("invalid request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	updated, err := h.updateTenantSettings(c, req, false)
	if err != nil {
		return c.Status(err.Code).JSON(fiber.Map{"error": err.Message})
	}

	return c.JSON(tenantSettingsResponse(updated, effectiveTenantBaseURL(c, updated.PublicBaseURL, h.fallbackBaseURL)))
}

func (h *SettingsHandler) updateTenantSettings(c fiber.Ctx, req tenantSettingsUpdateRequest, brandingOnly bool) (*models.TenantSettings, *fiber.Error) {
	tenantID := tenantIDFromCtx(c)
	currentSettings, err := h.service.Get(c.Context(), tenantID)
	if err != nil {
		utils.Logger(c).Error("failed loading current tenant settings", zap.Error(err))
		return nil, fiber.NewError(fiber.StatusInternalServerError, "failed to load tenant settings")
	}

	if req.PrimaryColor != "" {
		if !isValidHexColor(req.PrimaryColor) {
			return nil, fiber.NewError(fiber.StatusBadRequest, "invalid primary_color")
		}
	}
	if !brandingOnly && req.PublicBaseURL != "" && !isValidAbsoluteURL(req.PublicBaseURL) {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid public_base_url")
	}
	if !brandingOnly {
		if req.SecureLinkDefaultExpiryDays < 1 || req.SecureLinkDefaultExpiryDays > services.MaxSecureLinkDefaultExpiryDays {
			return nil, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("secure_link_default_expiry_days must be between 1 and %d", services.MaxSecureLinkDefaultExpiryDays))
		}
	}

	logoFile, logoErr := c.FormFile("logo")
	currentLogoPath := strings.TrimSpace(currentSettings.LogoPath)
	newLogoPath := currentLogoPath
	logoChanged := false

	if logoErr == nil && logoFile != nil {
		savedPath, err := h.saveLogoFile(c, logoFile)
		if err != nil {
			utils.Logger(c).Error("failed saving logo", zap.Error(err))
			return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		newLogoPath = savedPath
		logoChanged = true
	} else if req.RemoveLogo && currentLogoPath != "" {
		newLogoPath = ""
		logoChanged = true
	}

	update := services.TenantSettingsUpdate{
		SiteTitle:    &req.SiteTitle,
		LogoPath:     &newLogoPath,
		PrimaryColor: &req.PrimaryColor,
	}
	if !brandingOnly {
		update.PublicBaseURL = &req.PublicBaseURL
		update.InviteWelcomeText = &req.InviteWelcomeText
		update.SecureLinkDefaultExpiryDays = &req.SecureLinkDefaultExpiryDays
	}

	updatedSettings, updateErr := h.service.Update(c.Context(), tenantID, update)
	if updateErr != nil {
		if logoChanged && newLogoPath != "" && newLogoPath != currentLogoPath {
			h.removePublicFile(newLogoPath)
		}
		if errors.Is(updateErr, services.ErrTenantPublicBaseURLConflict) {
			return nil, fiber.NewError(fiber.StatusConflict, "public_base_url hostname already belongs to another tenant")
		}
		utils.Logger(c).Error("failed updating tenant settings", zap.Error(updateErr))
		return nil, fiber.NewError(fiber.StatusInternalServerError, "failed to update tenant settings")
	}

	if currentLogoPath != "" && currentLogoPath != updatedSettings.LogoPath {
		h.removePublicFile(currentLogoPath)
	}

	return updatedSettings, nil
}

func (h *SettingsHandler) GetSystemSettings(c fiber.Ctx) error {
	return c.JSON(h.systemSettingsResponse())
}

func (h *SettingsHandler) UpdateSystemSettings(c fiber.Ctx) error {
	if h.tenancyMode == tenantctx.ModeHosted {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "system log level cannot be updated in hosted mode"})
	}

	var req logLevelUpdateRequest
	if err := c.Bind().Body(&req); err != nil {
		utils.Logger(c).Debug("invalid request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if err := utils.SetLogLevel(req.Level); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid log level"})
	}

	return c.JSON(h.systemSettingsResponse())
}

func (h *SettingsHandler) saveLogoFile(c fiber.Ctx, fileHeader *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !isAllowedImageExt(ext) {
		return "", fmt.Errorf("unsupported logo file type")
	}

	brandingDir := filepath.Join(h.publicDir, "branding")
	if err := os.MkdirAll(brandingDir, 0755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	relativePath := filepath.Join("branding", filename)
	absolutePath := filepath.Join(h.publicDir, relativePath)

	if err := c.SaveFile(fileHeader, absolutePath); err != nil {
		return "", err
	}

	return filepath.ToSlash(relativePath), nil
}

func (h *SettingsHandler) removePublicFile(relativePath string) {
	safePath, err := safePublicPath(h.publicDir, relativePath)
	if err != nil {
		return
	}
	_ = os.Remove(safePath)
}

func safePublicPath(publicDir string, relativePath string) (string, error) {
	cleanRoot := filepath.Clean(publicDir)
	cleanPath := filepath.Clean(filepath.Join(cleanRoot, relativePath))
	if !strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator)) && cleanPath != cleanRoot {
		return "", errors.New("invalid path")
	}
	return cleanPath, nil
}

func isAllowedImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".svg":
		return true
	default:
		return false
	}
}

func parseFormBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "on", "yes":
		return true
	default:
		return false
	}
}

func isValidHexColor(value string) bool {
	hexColor := regexp.MustCompile(`^#([a-fA-F0-9]{3}|[a-fA-F0-9]{6})$`)
	return hexColor.MatchString(value)
}

func isValidAbsoluteURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed.Scheme != "" && parsed.Host != ""
}

func brandingResponse(settings *models.TenantSettings, effectivePublicBaseURL, accountDashboardURL string) fiber.Map {
	return fiber.Map{
		"site_title":                settings.SiteTitle,
		"logo_path":                 settings.LogoPath,
		"primary_color":             settings.PrimaryColor,
		"effective_public_base_url": effectivePublicBaseURL,
		"account_dashboard_url":     accountDashboardURL,
	}
}

func tenantSettingsResponse(settings *models.TenantSettings, effectivePublicBaseURL string) fiber.Map {
	return fiber.Map{
		"site_title":                      settings.SiteTitle,
		"logo_path":                       settings.LogoPath,
		"primary_color":                   settings.PrimaryColor,
		"public_base_url":                 settings.PublicBaseURL,
		"effective_public_base_url":       effectivePublicBaseURL,
		"invite_welcome_text":             settings.InviteWelcomeText,
		"secure_link_default_expiry_days": services.SecureLinkExpiryOrDefault(settings),
	}
}

func (h *SettingsHandler) systemSettingsResponse() systemSettingsResponse {
	response := systemSettingsResponse{TenancyMode: h.tenancyMode}
	if h.tenancyMode == tenantctx.ModeHosted {
		return response
	}

	response.LogLevel = &logLevelSettingsResponse{
		Level:   utils.CurrentLogLevel(),
		Options: utils.SupportedLogLevels(),
	}

	return response
}

func normalizeTenancyMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), tenantctx.ModeHosted) {
		return tenantctx.ModeHosted
	}

	return tenantctx.ModeSingle
}

func bindBrandingUpdateRequest(c fiber.Ctx, req *brandingUpdateRequest) error {
	contentType := c.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		return c.Bind().Body(req)
	}

	req.SiteTitle = strings.TrimSpace(c.FormValue("site_title"))
	req.PrimaryColor = strings.TrimSpace(c.FormValue("primary_color"))
	req.RemoveLogo = parseFormBool(c.FormValue("remove_logo"))
	return nil
}

func bindTenantSettingsUpdateRequest(c fiber.Ctx, req *tenantSettingsUpdateRequest) error {
	contentType := c.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		return c.Bind().Body(req)
	}

	req.SiteTitle = strings.TrimSpace(c.FormValue("site_title"))
	req.PrimaryColor = strings.TrimSpace(c.FormValue("primary_color"))
	req.RemoveLogo = parseFormBool(c.FormValue("remove_logo"))
	req.PublicBaseURL = strings.TrimSpace(c.FormValue("public_base_url"))
	req.InviteWelcomeText = strings.TrimSpace(c.FormValue("invite_welcome_text"))
	if raw := strings.TrimSpace(c.FormValue("secure_link_default_expiry_days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return err
		}
		req.SecureLinkDefaultExpiryDays = parsed
	}
	return nil
}
