// devpreview binds public preview domains in the worktree's copied database.
package main

import (
	"fmt"
	"net/url"
	"os"

	"github.com/pixelcop/clientshare/internal/clientshare"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/db"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := clientshare.LoadConfig("config/config.yaml")
	if err != nil {
		return err
	}
	if !cfg.DevMode {
		return fmt.Errorf("preview binding requires dev_mode")
	}
	base, err := url.Parse(cfg.Server.BaseURL)
	if err != nil || base.Hostname() == "" {
		return fmt.Errorf("configured server.base_url must identify the preview tenant")
	}
	database, err := db.InitDB(cfg.Database, zap.NewNop())
	if err != nil {
		return err
	}
	return database.Transaction(func(tx *gorm.DB) error {
		var original models.TenantDomain
		if err := tx.Where("domain = ?", base.Hostname()).First(&original).Error; err != nil {
			return fmt.Errorf("resolve original preview tenant: %w", err)
		}
		for _, key := range []string{"SERVER_BASE_URL", "DEVCRAFT_PUBLIC_URL"} {
			value := os.Getenv(key)
			if value == "" {
				continue
			}
			preview, err := url.Parse(value)
			if err != nil || preview.Scheme != "https" || preview.Hostname() == "" {
				return fmt.Errorf("%s must be a public HTTPS preview URL", key)
			}
			row := models.TenantDomain{TenantID: original.TenantID, Domain: preview.Hostname(), Kind: "preview"}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			var existing models.TenantDomain
			if err := tx.Where("domain = ?", row.Domain).First(&existing).Error; err != nil {
				return err
			}
			if existing.TenantID != original.TenantID {
				return fmt.Errorf("preview domain belongs to another tenant")
			}
		}
		return tx.Model(&models.TenantSettings{}).Where("tenant_id = ?", original.TenantID).Update("public_base_url", os.Getenv("SERVER_BASE_URL")).Error
	})
}
