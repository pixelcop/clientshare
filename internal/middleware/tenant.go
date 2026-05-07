package middleware

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	sharedmw "github.com/pixelcop/clientshare/pkg/web/middleware"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type TenantResolverConfig struct {
	Mode                 string
	FallbackBaseURL      string
	InternalTargetHeader string
	Next                 func(c fiber.Ctx) bool
}

func NewTenantResolver(db *gorm.DB, cfg TenantResolverConfig) fiber.Handler {
	resolver := tenantctx.NewResolver(db, cfg.Mode, cfg.FallbackBaseURL, cfg.InternalTargetHeader)

	return func(c fiber.Ctx) error {
		if cfg.Next != nil && cfg.Next(c) {
			return c.Next()
		}

		tenantCtx, err := resolver.Resolve(c)
		if err != nil {
			utilsLogger := sharedmw.LogWebRequest
			_ = utilsLogger
			switch {
			case errors.Is(err, tenantctx.ErrTenantTargetRequired):
				zap.L().Debug("missing tenant target", zap.String("path", c.Path()))
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant target is required"})
			case errors.Is(err, tenantctx.ErrTenantNotFound):
				zap.L().Debug("tenant not found", zap.String("path", c.Path()), zap.String("hostname", c.Hostname()))
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tenant not found"})
			default:
				zap.L().Error("tenant resolution failed", zap.Error(err), zap.String("path", c.Path()), zap.String("hostname", c.Hostname()))
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "tenant resolution failed"})
			}
		}

		tenantctx.SetLocal(c, tenantCtx)
		return c.Next()
	}
}
