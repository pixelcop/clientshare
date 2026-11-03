package middleware

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/db"
	"github.com/pixelcop/clientshare/internal/models"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
)

func expectsJSON(c fiber.Ctx) bool {
	return strings.HasPrefix(c.Path(), "/api") ||
		strings.Contains(c.Get("Accept"), "application/json") ||
		c.Get("X-Requested-With") == "XMLHttpRequest"
}

// AuthRequired validates a revocable portal session and current permissions.
func AuthRequired(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	tokenStr := c.Cookies(auth.SessionCookie)
	expectsJSON := expectsJSON(c)
	if tokenStr == "" {
		utils.Logger(c).Debug("Missing token")
		if expectsJSON {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		return c.Redirect().To("/401")
	}
	store := auth.Store(c)
	if store == nil {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	identity, err := store.Validate(c.Context(), tokenStr, auth.PortalAudience, tenantctx.IDFromFiber(c))
	if err != nil {
		if !errors.Is(err, auth.ErrSessionInvalid) {
			return c.Status(503).JSON(fiber.Map{"error": "Session validation is temporarily unavailable"})
		}
		utils.Logger(c).Debug("Invalid token", zap.Error(err))
		if expectsJSON {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		return c.Redirect().To("/401")
	}
	requestTenantID := strings.TrimSpace(tenantctx.IDFromFiber(c))
	tokenTenantID := identity.TenantID
	if requestTenantID == "" || tokenTenantID == "" || requestTenantID != tokenTenantID {
		utils.Logger(c).Debug("Tenant mismatch", zap.String("request_tenant_id", requestTenantID), zap.String("token_tenant_id", tokenTenantID))
		if expectsJSON {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		return c.Redirect().To("/401")
	}

	c.Locals("session_id", identity.SessionID)
	c.Locals("role", identity.Role)
	c.Locals("user_id", identity.UserID)
	c.Locals("auth_tenant_id", tokenTenantID)
	if identity.Role == "link" {
		c.Locals("client_id", identity.ClientID)
	} else {
		c.Locals("client_id", "")
	}
	c.Locals("link_id", identity.LinkID)
	c.Locals("access_type", identity.AccessType)
	return c.Next()
}

func toString(val interface{}) string {
	if val == nil {
		return ""
	}
	if v, ok := val.(string); ok {
		return v
	}
	return ""
}

// RoleRequired middleware checks for required role.
// Admin is always allowed.
func RoleRequired(role string) fiber.Handler {
	return func(c fiber.Ctx) error {
		userRole, ok := c.Locals("role").(string)
		expectsJSON := expectsJSON(c)
		if !ok || !(userRole == role || userRole == "admin") {
			utils.Logger(c).Debug("Forbidden", zap.String("required_role", role), zap.String("user_role", userRole))
			if expectsJSON {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
			}
			return c.Redirect().To("/403")
		}
		return c.Next()
	}
}

func UserCanAccessFile(c fiber.Ctx) error {
	// file routes have an :id param for the folder or file ID
	fileID := c.Params("id")
	userID, _ := c.Locals("user_id").(string)
	clientID, _ := c.Locals("client_id").(string)
	tenantID := strings.TrimSpace(tenantctx.IDFromFiber(c))

	// Deletion workflows maintain subtree state transactionally. Check the file
	// and its client in one query rather than walking ancestors on every request.
	file, err := db.G[*models.File]().Where(`tenant_id = ? AND id = ? AND deleted_at IS NULL AND disk_deleted_at IS NULL
		AND EXISTS (SELECT 1 FROM clients WHERE clients.tenant_id = files.tenant_id AND clients.id = files.client_id)`, tenantID, fileID).First(c.Context())
	if err != nil {
		utils.Logger(c).Debug("file not found", zap.Error(err))
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.ErrNotFound.Error()})
	}
	c.Locals("file", file)

	role, _ := c.Locals("role").(string)
	if role == "admin" || role == "manager" {
		// admins and managers have access to all files, so we can proceed without further checks
		return c.Next()
	}

	if role == "link" {
		if clientID == "" || clientID != file.ClientID {
			utils.Logger(c).Debug("forbidden link access", zap.String("token_client_id", clientID), zap.String("file_client_id", file.ClientID))
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": fiber.ErrNotFound.Error()})
		}
		return c.Next()
	}

	_, err = db.G[*models.UserClient]().Where("tenant_id = ? AND user_id = ? AND client_id = ?", tenantID, userID, file.ClientID).First(c.Context())
	if err != nil {
		utils.Logger(c).Debug("user has no access to this client", zap.Error(err), zap.String("user_id", userID), zap.String("client_id", file.ClientID))
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": fiber.ErrNotFound.Error()})
	}
	return c.Next()
}
