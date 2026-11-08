package middleware

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/gorm"
)

func AccessRequest(c fiber.Ctx, clientID, fileID string, operation services.AccessOperation) services.AccessRequest {
	return services.AccessRequest{
		TenantID: tenantctx.IDFromFiber(c), ClientID: clientID, FileID: fileID,
		Actor:     services.AccessActor{UserID: fiber.Locals[string](c, "user_id"), LinkID: fiber.Locals[string](c, "link_id"), SessionID: fiber.Locals[string](c, "session_id")},
		Operation: operation,
	}
}

// FileAccess replaces role/membership bypasses with an operation on the actual
// route target. Inaccessible and missing resources have the same response.
func FileAccess(db *gorm.DB, operation services.AccessOperation) fiber.Handler {
	return func(c fiber.Ctx) error {
		var file models.File
		err := db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantctx.IDFromFiber(c), c.Params("id")).First(&file).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "Not Found"})
		}
		if err != nil {
			return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
		}
		decision, err := (&services.AccessService{DB: db}).Resolve(c.Context(), AccessRequest(c, file.ClientID, file.ID, operation))
		if err != nil {
			return c.Status(503).JSON(fiber.Map{"error": "Access validation is temporarily unavailable"})
		}
		if !decision.Allowed {
			return c.Status(404).JSON(fiber.Map{"error": "Not Found"})
		}
		c.Locals("file", &file)
		c.Locals("file_access", decision)
		return c.Next()
	}
}
