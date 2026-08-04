package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/gorm"
)

var (
	ErrHostedLoginInvalidCredentials  = errors.New("invalid credentials")
	ErrHostedLoginAmbiguousEmail      = errors.New("hosted login email matches multiple tenants")
	ErrHostedLoginTenantInactive      = errors.New("tenant is not active")
	ErrHostedLoginRedirectUnavailable = errors.New("tenant redirect URL is unavailable")
	ErrHostedLoginHandoffInvalid      = errors.New("invalid or expired hosted login handoff")
)

type HostedLoginResult struct {
	HandoffCode string `json:"handoff_code"`
	TenantID    string `json:"tenant_id"`
	TenantSlug  string `json:"tenant_slug"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	RedirectURL string `json:"redirect_url"`
}

type HostedLoginService struct {
	db              *gorm.DB
	fallbackBaseURL string
	handoffSecret   string
}

type hostedLoginUserMatch struct {
	UserID            string
	Role              string
	TenantID          string
	TenantSlug        string
	PasswordHash      string
	PublicBaseURL     string
	EntitlementStatus string
}

type hostedLoginDomainRow struct {
	Domain string
}

// HostedLoginIdentity identifies a user and their hosted tenant redirect target.
// It is only returned to trusted internal callers.
type HostedLoginIdentity struct {
	UserID      string
	Role        string
	TenantID    string
	TenantSlug  string
	RedirectURL string
}

func NewHostedLoginService(db *gorm.DB, fallbackBaseURL, handoffSecret string) *HostedLoginService {
	return &HostedLoginService{
		db:              db,
		fallbackBaseURL: normalizeHostedLoginBaseURL(fallbackBaseURL),
		handoffSecret:   strings.TrimSpace(handoffSecret),
	}
}

func (s *HostedLoginService) Resolve(email string) (*HostedLoginIdentity, error) {
	match, err := s.lookupUser(email)
	if err != nil {
		return nil, err
	}
	redirectURL, err := s.redirectURL(match)
	if err != nil {
		return nil, err
	}
	return &HostedLoginIdentity{
		UserID:      match.UserID,
		Role:        match.Role,
		TenantID:    match.TenantID,
		TenantSlug:  match.TenantSlug,
		RedirectURL: redirectURL,
	}, nil
}

func (s *HostedLoginService) Login(email, password string) (*HostedLoginResult, error) {
	if password == "" {
		return nil, ErrHostedLoginInvalidCredentials
	}

	match, err := s.lookupUser(email)
	if err != nil {
		return nil, err
	}
	if !auth.CheckPasswordHash(password, match.PasswordHash) {
		return nil, ErrHostedLoginInvalidCredentials
	}
	redirectURL, err := s.redirectURL(match)
	if err != nil {
		return nil, err
	}

	return s.IssueHandoff(HostedLoginIdentity{
		UserID:      match.UserID,
		Role:        match.Role,
		TenantID:    match.TenantID,
		TenantSlug:  match.TenantSlug,
		RedirectURL: redirectURL,
	})
}

func (s *HostedLoginService) IssueHandoff(identity HostedLoginIdentity) (*HostedLoginResult, error) {
	if s == nil || s.db == nil || s.handoffSecret == "" {
		return nil, errors.New("hosted login handoffs are unavailable")
	}
	code, err := generateHostedLoginHandoffCode()
	if err != nil {
		return nil, fmt.Errorf("generate hosted login handoff: %w", err)
	}
	now := time.Now()
	_ = s.db.Where("expires_at < ? OR used_at IS NOT NULL", now).Delete(&models.HostedLoginHandoff{}).Error
	handoff := models.HostedLoginHandoff{
		TenantID:  identity.TenantID,
		UserID:    identity.UserID,
		Role:      identity.Role,
		CodeHash:  hashHostedLoginHandoff(s.handoffSecret, code),
		ExpiresAt: now.Add(2 * time.Minute),
		CreatedAt: now,
	}
	if err := s.db.Create(&handoff).Error; err != nil {
		return nil, fmt.Errorf("store hosted login handoff: %w", err)
	}
	return &HostedLoginResult{
		HandoffCode: code,
		TenantID:    identity.TenantID,
		TenantSlug:  identity.TenantSlug,
		UserID:      identity.UserID,
		Role:        identity.Role,
		RedirectURL: identity.RedirectURL,
	}, nil
}

func (s *HostedLoginService) ConsumeHandoff(tenantID, code string) (*HostedLoginIdentity, error) {
	if s == nil || s.db == nil || s.handoffSecret == "" {
		return nil, ErrHostedLoginHandoffInvalid
	}
	tenantID = strings.TrimSpace(tenantID)
	code = strings.TrimSpace(code)
	if tenantID == "" || code == "" {
		return nil, ErrHostedLoginHandoffInvalid
	}
	now := time.Now()
	codeHash := hashHostedLoginHandoff(s.handoffSecret, code)
	var handoff models.HostedLoginHandoff
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND code_hash = ? AND used_at IS NULL AND expires_at > ?", tenantID, codeHash, now).First(&handoff).Error; err != nil {
			return err
		}
		result := tx.Model(&models.HostedLoginHandoff{}).
			Where("id = ? AND used_at IS NULL AND expires_at > ?", handoff.ID, now).
			Update("used_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrHostedLoginHandoffInvalid
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrHostedLoginHandoffInvalid) {
			return nil, ErrHostedLoginHandoffInvalid
		}
		return nil, fmt.Errorf("consume hosted login handoff: %w", err)
	}
	return &HostedLoginIdentity{
		UserID:   handoff.UserID,
		Role:     handoff.Role,
		TenantID: handoff.TenantID,
	}, nil
}

func generateHostedLoginHandoffCode() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashHostedLoginHandoff(secret, code string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *HostedLoginService) lookupUser(email string) (hostedLoginUserMatch, error) {
	if s == nil || s.db == nil {
		return hostedLoginUserMatch{}, gorm.ErrInvalidDB
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return hostedLoginUserMatch{}, ErrHostedLoginInvalidCredentials
	}

	var matches []hostedLoginUserMatch
	if err := s.db.Table("users AS u").
		Select("u.id AS user_id, u.role, u.tenant_id, t.slug AS tenant_slug, u.password_hash, COALESCE(ts.public_base_url, '') AS public_base_url, COALESCE(te.status, '') AS entitlement_status").
		Joins("JOIN tenants AS t ON t.id = u.tenant_id").
		Joins("LEFT JOIN tenant_settings AS ts ON ts.tenant_id = u.tenant_id").
		Joins("LEFT JOIN tenant_entitlements AS te ON te.tenant_id = u.tenant_id").
		Where("LOWER(u.email) = ?", normalizedEmail).
		Order("u.created_at ASC").
		Find(&matches).Error; err != nil {
		return hostedLoginUserMatch{}, fmt.Errorf("lookup hosted login user: %w", err)
	}

	if len(matches) == 0 {
		return hostedLoginUserMatch{}, ErrHostedLoginInvalidCredentials
	}
	if len(matches) > 1 {
		return hostedLoginUserMatch{}, ErrHostedLoginAmbiguousEmail
	}
	return matches[0], nil
}

func (s *HostedLoginService) redirectURL(match hostedLoginUserMatch) (string, error) {
	if status := strings.TrimSpace(match.EntitlementStatus); status != "" && status != tenant.StatusActive {
		return "", ErrHostedLoginTenantInactive
	}

	redirectURL, err := s.resolveRedirectURL(match.TenantID)
	if err != nil {
		return "", err
	}
	return redirectURL, nil
}

func (s *HostedLoginService) resolveRedirectURL(tenantID string) (string, error) {
	if s == nil || s.db == nil {
		return "", gorm.ErrInvalidDB
	}
	var domains []hostedLoginDomainRow
	if err := s.db.Table("tenant_domains").
		Select("domain").
		Where("tenant_id = ?", strings.TrimSpace(tenantID)).
		Order("is_primary DESC, created_at ASC").
		Find(&domains).Error; err != nil {
		return "", fmt.Errorf("load tenant login domains: %w", err)
	}
	for _, candidate := range domains {
		domain := normalizeHostedLoginDomain(candidate.Domain)
		if domain == "" {
			continue
		}
		return hostedLoginBaseScheme(s.fallbackBaseURL) + "://" + domain, nil
	}
	return "", ErrHostedLoginRedirectUnavailable
}

func normalizeHostedLoginDomain(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || strings.Contains(value, "://") || strings.ContainsAny(value, "/?#") {
		return ""
	}
	parsed, err := url.Parse("//" + value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" {
		return ""
	}
	return parsed.Host
}

func normalizeHostedLoginBaseURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHostname(parsed.Hostname())) {
		return ""
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}

func isLoopbackHostname(hostname string) bool {
	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if hostname == "localhost" || hostname == "::1" {
		return true
	}
	parsed := net.ParseIP(hostname)
	return parsed != nil && parsed.IsLoopback()
}

func hostedLoginBaseScheme(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" {
		return "https"
	}
	return parsed.Scheme
}
