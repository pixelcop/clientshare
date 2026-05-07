package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

func init() {
	goose.AddMigrationContext(upBackfillTenantDomainsFromPublicBaseURLs, downBackfillTenantDomainsFromPublicBaseURLs)
}

func upBackfillTenantDomainsFromPublicBaseURLs(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)

	var settings []models.TenantSettings
	if err := db.Select("tenant_id", "public_base_url").
		Where("TRIM(COALESCE(public_base_url, '')) <> ''").
		Order("tenant_id ASC").
		Find(&settings).Error; err != nil {
		return fmt.Errorf("query tenant settings public urls: %w", err)
	}

	seen := make(map[string]string)
	now := time.Now().UTC()
	for _, setting := range settings {
		tenantID := strings.TrimSpace(setting.TenantID)

		hostname, err := publicBaseURLHostname(setting.PublicBaseURL)
		if err != nil {
			return fmt.Errorf("tenant %s public_base_url: %w", tenantID, err)
		}
		if existingTenantID, ok := seen[hostname]; ok && existingTenantID != tenantID {
			return fmt.Errorf("duplicate tenant public url hostname %q for tenants %s and %s", hostname, existingTenantID, tenantID)
		}
		seen[hostname] = tenantID

		var existingDomain models.TenantDomain
		err = db.Where("LOWER(domain) = LOWER(?)", hostname).Take(&existingDomain).Error
		switch {
		case err == nil:
			if strings.TrimSpace(existingDomain.TenantID) != tenantID {
				return fmt.Errorf("tenant domain %q already belongs to tenant %s", hostname, strings.TrimSpace(existingDomain.TenantID))
			}
			if err := db.Model(&models.TenantDomain{}).
				Where("id = ? AND kind IN ?", existingDomain.ID, []string{tenant.DomainKindBase, tenant.DomainKindPublicBaseURL}).
				Updates(map[string]any{
					"kind":       tenant.DomainKindPublicBaseURL,
					"is_primary": true,
					"updated_at": now,
				}).Error; err != nil {
				return fmt.Errorf("update tenant domain for hostname %q: %w", hostname, err)
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := db.Create(&models.TenantDomain{
				ID:        ids.NewULID(),
				TenantID:  tenantID,
				Domain:    hostname,
				Kind:      tenant.DomainKindPublicBaseURL,
				IsPrimary: true,
				CreatedAt: now,
				UpdatedAt: now,
			}).Error; err != nil {
				return fmt.Errorf("insert tenant domain for hostname %q: %w", hostname, err)
			}
		default:
			return fmt.Errorf("lookup tenant domain for hostname %q: %w", hostname, err)
		}
	}

	return nil
}

func downBackfillTenantDomainsFromPublicBaseURLs(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}

	if err := db.WithContext(ctx).Where("kind = ?", tenant.DomainKindPublicBaseURL).Delete(&models.TenantDomain{}).Error; err != nil {
		return fmt.Errorf("delete public base url tenant domains: %w", err)
	}
	return nil
}

func publicBaseURLHostname(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid public base url %q", value)
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname())), nil
}
