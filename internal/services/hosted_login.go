package services

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/gorm"
)

var (
	ErrHostedLoginInvalidCredentials  = errors.New("invalid credentials")
	ErrHostedLoginAmbiguousEmail      = errors.New("hosted login email matches multiple tenants")
	ErrHostedLoginTenantInactive      = errors.New("tenant is not active")
	ErrHostedLoginRedirectUnavailable = errors.New("tenant redirect URL is unavailable")
)

type HostedLoginResult struct {
	Token       string `json:"token"`
	TenantID    string `json:"tenant_id"`
	TenantSlug  string `json:"tenant_slug"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	RedirectURL string `json:"redirect_url"`
}

type HostedLoginService struct {
	db              *gorm.DB
	fallbackBaseURL string
}

type hostedLoginDomainRow struct {
	Domain    string
	Kind      string
	IsPrimary bool
}

func NewHostedLoginService(db *gorm.DB, fallbackBaseURL string) *HostedLoginService {
	return &HostedLoginService{db: db, fallbackBaseURL: normalizeHostedLoginBaseURL(fallbackBaseURL)}
}

func (s *HostedLoginService) Login(email, password string) (*HostedLoginResult, error) {
	if s == nil || s.db == nil {
		return nil, gorm.ErrInvalidDB
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	password = strings.TrimSpace(password)
	if normalizedEmail == "" || password == "" {
		return nil, ErrHostedLoginInvalidCredentials
	}

	type userMatch struct {
		UserID            string
		Role              string
		TenantID          string
		TenantSlug        string
		PasswordHash      string
		PublicBaseURL     string
		EntitlementStatus string
	}

	var matches []userMatch
	if err := s.db.Table("users AS u").
		Select("u.id AS user_id, u.role, u.tenant_id, t.slug AS tenant_slug, u.password_hash, COALESCE(ts.public_base_url, '') AS public_base_url, COALESCE(te.status, '') AS entitlement_status").
		Joins("JOIN tenants AS t ON t.id = u.tenant_id").
		Joins("LEFT JOIN tenant_settings AS ts ON ts.tenant_id = u.tenant_id").
		Joins("LEFT JOIN tenant_entitlements AS te ON te.tenant_id = u.tenant_id").
		Where("LOWER(u.email) = ?", normalizedEmail).
		Order("u.created_at ASC").
		Find(&matches).Error; err != nil {
		return nil, fmt.Errorf("lookup hosted login user: %w", err)
	}

	if len(matches) == 0 {
		return nil, ErrHostedLoginInvalidCredentials
	}
	if len(matches) > 1 {
		return nil, ErrHostedLoginAmbiguousEmail
	}

	match := matches[0]
	if !auth.CheckPasswordHash(password, match.PasswordHash) {
		return nil, ErrHostedLoginInvalidCredentials
	}
	if status := strings.TrimSpace(match.EntitlementStatus); status != "" && status != tenant.StatusActive {
		return nil, ErrHostedLoginTenantInactive
	}

	redirectURL, err := s.resolveRedirectURL(match.TenantID, match.TenantSlug, match.PublicBaseURL)
	if err != nil {
		return nil, err
	}

	token, err := auth.GenerateJWT(match.UserID, match.Role, match.TenantID)
	if err != nil {
		return nil, fmt.Errorf("generate hosted login token: %w", err)
	}

	return &HostedLoginResult{
		Token:       token,
		TenantID:    match.TenantID,
		TenantSlug:  match.TenantSlug,
		UserID:      match.UserID,
		Role:        match.Role,
		RedirectURL: redirectURL,
	}, nil
}

func (s *HostedLoginService) resolveRedirectURL(tenantID, tenantSlug, publicBaseURL string) (string, error) {
	if normalized := normalizeHostedLoginBaseURL(publicBaseURL); normalized != "" {
		return normalized, nil
	}

	var domains []hostedLoginDomainRow
	if err := s.db.Table("tenant_domains").
		Select("domain, kind, is_primary").
		Where("tenant_id = ?", strings.TrimSpace(tenantID)).
		Order("is_primary DESC, created_at ASC").
		Find(&domains).Error; err != nil {
		return "", fmt.Errorf("load tenant domains: %w", err)
	}

	if domain := preferredHostedLoginDomain(domains); domain != "" {
		scheme := hostedLoginBaseScheme(s.fallbackBaseURL)
		return scheme + "://" + domain, nil
	}

	if fallback := buildHostedLoginURLFromFallback(s.fallbackBaseURL, tenantSlug); fallback != "" {
		return fallback, nil
	}

	return "", ErrHostedLoginRedirectUnavailable
}

func preferredHostedLoginDomain(domains []hostedLoginDomainRow) string {
	bestDomain := ""
	bestRank := 0
	for _, candidate := range domains {
		domain := strings.ToLower(strings.TrimSpace(candidate.Domain))
		if domain == "" {
			continue
		}
		rank := hostedLoginDomainRank(candidate.Kind, candidate.IsPrimary)
		if rank > bestRank {
			bestRank = rank
			bestDomain = domain
		}
	}
	return bestDomain
}

func hostedLoginDomainRank(kind string, isPrimary bool) int {
	rank := 1
	if isPrimary {
		rank += 10
	}
	switch strings.TrimSpace(kind) {
	case tenant.DomainKindPublicBaseURL:
		rank += 5
	case tenant.DomainKindBase:
		rank += 4
	case "custom":
		rank += 3
	case "subdomain":
		rank += 2
	}
	return rank
}

func normalizeHostedLoginBaseURL(value string) string {
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

func hostedLoginBaseScheme(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" {
		return "https"
	}
	return parsed.Scheme
}

func buildHostedLoginURLFromFallback(baseURL, tenantSlug string) string {
	baseURL = normalizeHostedLoginBaseURL(baseURL)
	tenantSlug = strings.TrimSpace(tenantSlug)
	if baseURL == "" || tenantSlug == "" {
		return ""
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	hostname := strings.TrimSpace(parsed.Hostname())
	if hostname == "" {
		return ""
	}
	host := tenantSlug + "." + hostname
	if port := strings.TrimSpace(parsed.Port()); port != "" {
		host = net.JoinHostPort(host, port)
	}
	parsed.Host = host
	return parsed.String()
}
