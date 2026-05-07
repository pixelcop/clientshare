package services

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/pixelcop/clientshare/internal/models"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/gorm"
)

const (
	DefaultSiteTitle                   = "ClientShare"
	DefaultSecureLinkDefaultExpiryDays = 90
	MaxSecureLinkDefaultExpiryDays     = 120
)

var ErrTenantPublicBaseURLConflict = errors.New("tenant public base url conflicts with another tenant domain")

type TenantSettingsService struct {
	DB *gorm.DB
}

type TenantSettingsUpdate struct {
	SiteTitle                   *string
	LogoPath                    *string
	PrimaryColor                *string
	PublicBaseURL               *string
	InviteWelcomeText           *string
	SecureLinkDefaultExpiryDays *int
}

func NewTenantSettingsService(db *gorm.DB) *TenantSettingsService {
	return &TenantSettingsService{DB: db}
}

func (s *TenantSettingsService) Get(ctx context.Context, tenantID string) (*models.TenantSettings, error) {
	if s == nil || s.DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	validatedTenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}

	settings := &models.TenantSettings{}
	err = s.DB.WithContext(ctx).Where("tenant_id = ?", validatedTenantID).Attrs(models.TenantSettings{
		TenantID:                    validatedTenantID,
		SecureLinkDefaultExpiryDays: DefaultSecureLinkDefaultExpiryDays,
	}).FirstOrCreate(settings).Error
	if err != nil {
		return nil, err
	}

	if settings.SecureLinkDefaultExpiryDays < 1 {
		settings.SecureLinkDefaultExpiryDays = DefaultSecureLinkDefaultExpiryDays
	}
	settings.PublicBaseURL = normalizeTenantPublicBaseURL(settings.PublicBaseURL)

	return settings, nil
}

func (s *TenantSettingsService) Update(ctx context.Context, tenantID string, update TenantSettingsUpdate) (*models.TenantSettings, error) {
	validatedTenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}

	settings, err := s.Get(ctx, validatedTenantID)
	if err != nil {
		return nil, err
	}

	if update.SiteTitle != nil {
		settings.SiteTitle = strings.TrimSpace(*update.SiteTitle)
	}
	if update.LogoPath != nil {
		settings.LogoPath = strings.TrimSpace(*update.LogoPath)
	}
	if update.PrimaryColor != nil {
		settings.PrimaryColor = strings.TrimSpace(*update.PrimaryColor)
	}
	if update.PublicBaseURL != nil {
		settings.PublicBaseURL = normalizeTenantPublicBaseURL(*update.PublicBaseURL)
	}
	if update.InviteWelcomeText != nil {
		settings.InviteWelcomeText = strings.TrimSpace(*update.InviteWelcomeText)
	}
	if update.SecureLinkDefaultExpiryDays != nil {
		settings.SecureLinkDefaultExpiryDays = *update.SecureLinkDefaultExpiryDays
	}

	if err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(settings).Error; err != nil {
			return err
		}
		return syncTenantDomainFromPublicBaseURL(tx, validatedTenantID, settings.PublicBaseURL)
	}); err != nil {
		return nil, err
	}

	return settings, nil
}

func SiteTitleOrDefault(settings *models.TenantSettings) string {
	if settings == nil {
		return DefaultSiteTitle
	}
	if value := strings.TrimSpace(settings.SiteTitle); value != "" {
		return value
	}
	return DefaultSiteTitle
}

func SecureLinkExpiryOrDefault(settings *models.TenantSettings) int {
	if settings == nil || settings.SecureLinkDefaultExpiryDays < 1 {
		return DefaultSecureLinkDefaultExpiryDays
	}
	return settings.SecureLinkDefaultExpiryDays
}

func normalizeTenantPublicBaseURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}

func syncTenantDomainFromPublicBaseURL(tx *gorm.DB, tenantID, publicBaseURL string) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	validatedTenantID, err := requireTenantID(tenantID)
	if err != nil {
		return err
	}

	derivedKinds := []string{tenantctx.DomainKindBase, tenantctx.DomainKindPublicBaseURL}
	hostname := tenantPublicBaseURLHostname(publicBaseURL)
	if hostname == "" {
		return tx.Where("tenant_id = ? AND kind IN ?", validatedTenantID, derivedKinds).Delete(&models.TenantDomain{}).Error
	}

	var matching models.TenantDomain
	err = tx.Where("LOWER(domain) = ?", hostname).First(&matching).Error
	switch {
	case err == nil:
		if strings.TrimSpace(matching.TenantID) != validatedTenantID {
			return ErrTenantPublicBaseURLConflict
		}
		if !isSettingsDerivedTenantDomainKind(matching.Kind) {
			return tx.Where("tenant_id = ? AND kind IN ?", validatedTenantID, derivedKinds).Delete(&models.TenantDomain{}).Error
		}
		if err := tx.Model(&models.TenantDomain{}).
			Where("id = ?", matching.ID).
			Updates(map[string]any{"domain": hostname, "kind": tenantctx.DomainKindPublicBaseURL, "is_primary": true}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND kind IN ? AND id <> ?", validatedTenantID, derivedKinds, matching.ID).Delete(&models.TenantDomain{}).Error
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return err
	}

	var existingDerived models.TenantDomain
	err = tx.Where("tenant_id = ? AND kind IN ?", validatedTenantID, derivedKinds).Order("created_at ASC").First(&existingDerived).Error
	switch {
	case err == nil:
		if err := tx.Model(&models.TenantDomain{}).
			Where("id = ?", existingDerived.ID).
			Updates(map[string]any{"domain": hostname, "kind": tenantctx.DomainKindPublicBaseURL, "is_primary": true}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND kind IN ? AND id <> ?", validatedTenantID, derivedKinds, existingDerived.ID).Delete(&models.TenantDomain{}).Error
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return err
	}

	return tx.Create(&models.TenantDomain{
		TenantID:  validatedTenantID,
		Domain:    hostname,
		Kind:      tenantctx.DomainKindPublicBaseURL,
		IsPrimary: true,
	}).Error
}

func tenantPublicBaseURLHostname(value string) string {
	value = normalizeTenantPublicBaseURL(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func isSettingsDerivedTenantDomainKind(kind string) bool {
	kind = strings.TrimSpace(kind)
	return kind == tenantctx.DomainKindBase || kind == tenantctx.DomainKindPublicBaseURL
}
