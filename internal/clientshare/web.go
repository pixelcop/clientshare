package clientshare

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/services"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/pixelcop/clientshare/internal/static"
	"github.com/pixelcop/clientshare/pkg/web"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func NewWebApp(cfg *Config, db *gorm.DB, logger *zap.Logger, emailQueue emailpkg.EmailQueue, store storage.Storage) (*fiber.App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	maxBodyMB := cfg.Server.MaxBodyMB
	app := web.CreateApp(maxBodyMB, logger, cfg.DevMode)

	tenantResolver := middleware.NewTenantResolver(db, middleware.TenantResolverConfig{
		Mode:                 cfg.Tenancy.Mode,
		FallbackBaseURL:      cfg.Server.BaseURL,
		InternalTargetHeader: cfg.Tenancy.InternalTargetHeader,
		Next: func(c fiber.Ctx) bool {
			return c.Path() == "/api/healthz"
		},
	})
	app.Use("/api", tenantResolver)
	app.Use("/links", tenantResolver)

	insecure := fiber.New()
	app.Use("/api", insecure)

	// Health check endpoint
	insecure.Get("/healthz", func(c fiber.Ctx) error {
		return c.SendString("OK")
	})

	tenantSettingsService := services.NewTenantSettingsService(db)

	resetTTL := 30 * time.Minute
	if cfg.Auth.PasswordResetDuration != "" {
		if parsed, err := time.ParseDuration(cfg.Auth.PasswordResetDuration); err == nil {
			resetTTL = parsed
		} else {
			logger.Warn("invalid password reset duration", zap.Error(err))
		}
	}
	inviteTTL := 72 * time.Hour
	if cfg.Auth.InviteTokenDuration != "" {
		if parsed, err := time.ParseDuration(cfg.Auth.InviteTokenDuration); err == nil {
			inviteTTL = parsed
		} else {
			logger.Warn("invalid invite token duration", zap.Error(err))
		}
	}
	inviteTokenSecret := strings.TrimSpace(cfg.Auth.InviteTokenSecret)
	if inviteTokenSecret == "" {
		return nil, fmt.Errorf("auth.invite_token_secret is required")
	}
	authRateLimitEnabled := true
	if cfg.Auth.RateLimitEnabled != nil {
		authRateLimitEnabled = *cfg.Auth.RateLimitEnabled
	}
	if cfg.InternalAPI.Enabled && len(strings.TrimSpace(cfg.InternalAPI.Secret)) < 32 {
		return nil, fmt.Errorf("internal_api.secret must be at least 32 characters when internal_api.enabled is true")
	}
	jwtSecret := strings.TrimSpace(cfg.Auth.JWTSecret)
	if jwtSecret == "" {
		return nil, fmt.Errorf("auth.jwt_secret is required")
	}
	auth.SetJWTSecret(jwtSecret)
	hostedLoginSvc := services.NewHostedLoginService(db, cfg.Server.BaseURL, jwtSecret)
	authHandler := handlers.RegisterAuthRoutes(insecure, db, cfg.SecureLinks.SigningKey, emailQueue, cfg.Server.BaseURL, jwtSecret, inviteTokenSecret, resetTTL, inviteTTL, cfg.Auth.HostedPasskeyOrigin, hostedLoginSvc, tenantSettingsService, authRateLimitEnabled)
	app.Use("/.well-known", tenantResolver)
	app.Get("/.well-known/webauthn", authHandler.RelatedOriginsHandler)
	api := fiber.New()
	app.Use("/api", api)
	api.Use("/", middleware.AuthRequired)

	// User management endpoints (admin/manager)
	handlers.RegisterUserRoutes(api, db, emailQueue, cfg.Server.BaseURL, inviteTokenSecret, jwtSecret, inviteTTL, resetTTL, tenantSettingsService)

	// Client management endpoints (admin/manager)
	// You may want to restrict POST/PUT/DELETE to admin/manager only, GET to all roles
	// For now, protect all with AuthRequired (already applied via group)
	handlers.RegisterClientRoutes(api, db, store)
	handlers.RegisterFeedRoutes(api, db)

	// File endpoints (protected)
	handlers.RegisterFileRoutes(api, db, store, emailQueue, cfg.Server.BaseURL, tenantSettingsService)

	// Secure link management (admin/manager)
	handlers.RegisterLinkRoutes(app, db, cfg.SecureLinks.SigningKey, store, emailQueue, cfg.Server.BaseURL, tenantSettingsService)

	// Settings and branding (admin only)
	publicDir := "./public"
	handlers.RegisterSettingsRoutes(insecure, api, tenantSettingsService, publicDir, cfg.Server.BaseURL, cfg.Tenancy.Mode, cfg.Auth.HostedPasskeyOrigin)

	// Email queue controls (manager/admin)
	handlers.RegisterEmailQueueRoutes(api, emailQueue)

	// Internal control-plane API (disabled by default)
	if cfg.InternalAPI.Enabled {
		internalGroup := app.Group("/internal", middleware.InternalAuthRequired(cfg.InternalAPI.Secret))
		provisioningSvc := services.NewTenantProvisioningService(db, inviteTokenSecret, inviteTTL, emailQueue, cfg.Server.BaseURL)
		handlers.RegisterInternalTenantRoutes(internalGroup, provisioningSvc)
		handlers.RegisterInternalAuthRoutes(internalGroup, hostedLoginSvc, authHandler)
	}

	// setup static serving last due to catch-all routes
	webFS, err := static.GetEmbeddedWebFS(cfg.BuildTime)
	if err != nil {
		return nil, fmt.Errorf("load embedded frontend: %w", err)
	}
	vitePort := cfg.Server.VitePort
	if vitePort == 0 {
		vitePort = 5193
	}
	if err := web.SetupStaticFileServing(app, cfg.DevMode, vitePort, webFS); err != nil {
		return nil, fmt.Errorf("setup static file serving: %w", err)
	}

	return app, nil
}

func StartWebServer(cfg *Config, db *gorm.DB, logger *zap.Logger, emailQueue emailpkg.EmailQueue, store storage.Storage) error {
	app, err := NewWebApp(cfg, db, logger, emailQueue, store)
	if err != nil {
		return err
	}

	cleanupWorker := services.NewFileCleanupWorker(db, store, logger)
	cleanupWorker.Start()
	defer cleanupWorker.Stop()

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	mode := "production"
	if cfg.DevMode {
		mode = "development"
	}

	logFields := []zap.Field{
		zap.String("addr", addr),
		zap.String("mode", mode),
		zap.String("tenancy", cfg.Tenancy.Mode),
	}
	if cfg.CommitHash != "" && cfg.BuildTime != nil {
		logFields = append(logFields, zap.Timep("build_time", cfg.BuildTime), zap.String("commit_hash", cfg.CommitHash))
	}
	logger.Info("Starting ClientShare server", logFields...)

	return app.Listen(addr, fiber.ListenConfig{EnablePrintRoutes: cfg.DevMode, DisableStartupMessage: !cfg.DevMode})
}
