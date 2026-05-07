package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/clientshare"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"github.com/pressly/goose/v3"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

func init() {
	goose.AddMigrationContext(upSeedDefaultTenant, downSeedDefaultTenant)
}

func upSeedDefaultTenant(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}

	cfg := loadTenantSeedConfig()
	now := time.Now().UTC()
	db = db.WithContext(ctx)

	var existingTenant models.Tenant
	err = db.Where("id = ? OR slug = ?", tenant.DefaultTenantID, tenant.DefaultTenantSlug).Take(&existingTenant).Error
	if err == nil {
		return nil // nothing to do, default tenant already exists
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup default tenant: %w", err)
	}

	if err := db.Create(&models.Tenant{
		ID:        tenant.DefaultTenantID,
		Slug:      tenant.DefaultTenantSlug,
		Name:      tenant.DefaultTenantName,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error; err != nil {
		return fmt.Errorf("seed default tenant: %w", err)
	}

	var settings models.TenantSettings
	err = db.Where("tenant_id = ?", tenant.DefaultTenantID).Take(&settings).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lookup default tenant settings: %w", err)
		}

		if err := db.Create(&models.TenantSettings{
			ID:                          ids.NewULID(),
			TenantID:                    tenant.DefaultTenantID,
			SiteTitle:                   cfg.SiteTitle,
			LogoPath:                    cfg.LogoPath,
			PrimaryColor:                cfg.PrimaryColor,
			PublicBaseURL:               cfg.PublicBaseURL,
			InviteWelcomeText:           cfg.InviteWelcomeText,
			SecureLinkDefaultExpiryDays: cfg.SecureLinkDefaultExpiryDays,
			CreatedAt:                   now,
			UpdatedAt:                   now,
		}).Error; err != nil {
			return fmt.Errorf("seed default tenant settings: %w", err)
		}
	}

	var entitlement models.TenantEntitlement
	err = db.Where("tenant_id = ?", tenant.DefaultTenantID).Take(&entitlement).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lookup default tenant entitlement: %w", err)
		}

		if err := db.Create(&models.TenantEntitlement{
			ID:              ids.NewULID(),
			TenantID:        tenant.DefaultTenantID,
			Status:          tenant.StatusActive,
			PlanCode:        tenant.DefaultPlanCode,
			MaxUsers:        0,
			MaxStorageBytes: 0,
			CreatedAt:       now,
			UpdatedAt:       now,
		}).Error; err != nil {
			return fmt.Errorf("seed default tenant entitlements: %w", err)
		}
	}

	return nil
}

func downSeedDefaultTenant(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}

	db = db.WithContext(ctx)

	if err := db.Where("tenant_id = ?", tenant.DefaultTenantID).Delete(&models.TenantEntitlement{}).Error; err != nil {
		return fmt.Errorf("delete tenant entitlements: %w", err)
	}
	if err := db.Where("tenant_id = ?", tenant.DefaultTenantID).Delete(&models.TenantSettings{}).Error; err != nil {
		return fmt.Errorf("delete tenant settings: %w", err)
	}
	if err := db.Where("tenant_id = ?", tenant.DefaultTenantID).Delete(&models.TenantDomain{}).Error; err != nil {
		return fmt.Errorf("delete tenant domains: %w", err)
	}
	if err := db.Where("id = ?", tenant.DefaultTenantID).Delete(&models.Tenant{}).Error; err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	return nil
}

type tenantSeedConfig struct {
	SiteTitle                   string
	LogoPath                    string
	PrimaryColor                string
	PublicBaseURL               string
	InviteWelcomeText           string
	SecureLinkDefaultExpiryDays int
}

func loadTenantSeedConfig() tenantSeedConfig {
	result := tenantSeedConfig{
		SecureLinkDefaultExpiryDays: 90,
	}

	v := viper.New()
	v.SetConfigFile("config/config.yaml")
	if err := v.ReadInConfig(); err != nil {
		return result
	}

	var cfg clientshare.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return result
	}

	result.SiteTitle = strings.TrimSpace(cfg.Branding.SiteTitle)
	result.LogoPath = strings.TrimSpace(cfg.Branding.LogoPath)
	result.PrimaryColor = strings.TrimSpace(cfg.Branding.PrimaryColor)
	result.PublicBaseURL = strings.TrimSpace(cfg.Server.BaseURL)
	result.InviteWelcomeText = strings.TrimSpace(cfg.Email.UserInviteWelcomeText)
	if cfg.SecureLinks.DefaultExpiryDays > 0 {
		result.SecureLinkDefaultExpiryDays = cfg.SecureLinks.DefaultExpiryDays
	}

	return result
}
