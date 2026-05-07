package main

import (
	"context"

	"github.com/getsentry/sentry-go"
	"github.com/pixelcop/clientshare/internal/clientshare"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
)

func main() {
	utils.InitLogger()
	defer zap.L().Sync()

	cfg, err := clientshare.LoadConfig("config/config.yaml")
	if err != nil {
		zap.S().Fatalf("Error loading config: %v", err)
	}

	if cfg.SentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn: cfg.SentryDSN,
		}); err != nil {
			zap.L().Warn("Error setting up sentry", zap.Error(err))
		}
		zap.L().Info("Sentry initialized")
	} else {
		zap.L().Info("Sentry DSN not provided, skipping Sentry initialization")
	}

	if cfg.SecureLinks.SigningKey == "" {
		zap.L().Fatal("Secure link signing key must be set in config")
	}

	// Start clientshare
	ctx := context.Background()
	if err := clientshare.Start(ctx, cfg, zap.L()); err != nil {
		zap.L().Fatal("Startup failed", zap.Error(err))
	}
}
