package tenant

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

var (
	ErrTenantNotFound       = errors.New("tenant not found")
	ErrTenantTargetRequired = errors.New("tenant target is required")
)

type Resolver struct {
	DB                   *gorm.DB
	Mode                 string
	FallbackBaseURL      string
	InternalTargetHeader string
	singleTenantMu       sync.RWMutex
	singleTenant         *resolvedTenant
}

type resolvedTenant struct {
	ID            string
	Slug          string
	Name          string
	PublicBaseURL string
}

type tenantRow struct {
	ID   string
	Slug string
	Name string
}

func NewResolver(db *gorm.DB, mode, fallbackBaseURL, internalTargetHeader string) *Resolver {
	return &Resolver{
		DB:                   db,
		Mode:                 normalizeMode(mode),
		FallbackBaseURL:      normalizeBaseURL(fallbackBaseURL),
		InternalTargetHeader: normalizeInternalTargetHeader(internalTargetHeader),
	}
}

func (r *Resolver) Resolve(ctx fiber.Ctx) (RequestContext, error) {
	hostname := normalizeHostname(ctx.Hostname())

	if r.Mode == ModeSingle {
		resolved, err := r.singleResolvedTenant(ctx.Context())
		if err != nil {
			return RequestContext{}, err
		}
		return RequestContext{
			ID:            resolved.ID,
			Slug:          resolved.Slug,
			Name:          resolved.Name,
			Hostname:      hostname,
			PublicBaseURL: strings.TrimSpace(resolved.PublicBaseURL),
			Resolution:    "single",
		}, nil
	}

	if isInternalRoute(ctx.Path()) {
		target := strings.TrimSpace(ctx.Get(r.InternalTargetHeader))
		if target == "" {
			target = strings.TrimSpace(ctx.Query("tenant"))
		}
		if target == "" {
			return RequestContext{}, ErrTenantTargetRequired
		}

		resolved, err := r.lookupByIdentifier(ctx.Context(), target)
		if err != nil {
			return RequestContext{}, err
		}
		return RequestContext{
			ID:            resolved.ID,
			Slug:          resolved.Slug,
			Name:          resolved.Name,
			Hostname:      hostname,
			PublicBaseURL: strings.TrimSpace(resolved.PublicBaseURL),
			Resolution:    "internal_target",
		}, nil
	}

	if hostname == "" {
		return RequestContext{}, ErrTenantNotFound
	}

	if resolved, err := r.lookupByDomain(ctx.Context(), hostname); err == nil {
		return RequestContext{
			ID:            resolved.ID,
			Slug:          resolved.Slug,
			Name:          resolved.Name,
			Hostname:      hostname,
			PublicBaseURL: strings.TrimSpace(resolved.PublicBaseURL),
			Resolution:    "hostname",
		}, nil
	} else if !errors.Is(err, ErrTenantNotFound) {
		return RequestContext{}, err
	}

	baseHost := hostnameFromBaseURL(r.FallbackBaseURL)
	if slug, ok := subdomainSlug(hostname, baseHost); ok {
		resolved, err := r.lookupBySlug(ctx.Context(), slug)
		if err != nil {
			return RequestContext{}, err
		}
		return RequestContext{
			ID:            resolved.ID,
			Slug:          resolved.Slug,
			Name:          resolved.Name,
			Hostname:      hostname,
			PublicBaseURL: strings.TrimSpace(resolved.PublicBaseURL),
			Resolution:    "subdomain",
		}, nil
	}

	return RequestContext{}, ErrTenantNotFound
}

func (r *Resolver) singleResolvedTenant(ctx context.Context) (resolvedTenant, error) {
	if r == nil || r.DB == nil {
		return resolvedTenant{}, gorm.ErrInvalidDB
	}

	r.singleTenantMu.RLock()
	if r.singleTenant != nil {
		cached := *r.singleTenant
		r.singleTenantMu.RUnlock()
		return cached, nil
	}
	r.singleTenantMu.RUnlock()

	resolved, err := r.lookupDefaultTenant(ctx)
	if err != nil {
		return resolvedTenant{}, err
	}

	r.singleTenantMu.Lock()
	if r.singleTenant == nil {
		cached := resolved
		r.singleTenant = &cached
	}
	cached := *r.singleTenant
	r.singleTenantMu.Unlock()

	return cached, nil
}

func (r *Resolver) lookupDefaultTenant(ctx context.Context) (resolvedTenant, error) {
	var tenantModel tenantRow
	if err := r.DB.WithContext(ctx).Table("tenants").Select("id, slug, name").Where("id = ?", DefaultTenantID).First(&tenantModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resolvedTenant{}, ErrTenantNotFound
		}
		return resolvedTenant{}, err
	}

	return r.loadResolvedTenant(ctx, tenantModel)
}

func (r *Resolver) lookupByIdentifier(ctx context.Context, identifier string) (resolvedTenant, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return resolvedTenant{}, ErrTenantNotFound
	}

	var tenantModel tenantRow
	if err := r.DB.
		WithContext(ctx).
		Table("tenants").
		Select("id, slug, name").
		Where("id = ?", identifier).
		Or("LOWER(slug) = ?", strings.ToLower(identifier)).
		First(&tenantModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resolvedTenant{}, ErrTenantNotFound
		}
		return resolvedTenant{}, err
	}

	return r.loadResolvedTenant(ctx, tenantModel)
}

func (r *Resolver) lookupBySlug(ctx context.Context, slug string) (resolvedTenant, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		return resolvedTenant{}, ErrTenantNotFound
	}

	var tenantModel tenantRow
	if err := r.DB.WithContext(ctx).Table("tenants").Select("id, slug, name").Where("LOWER(slug) = ?", slug).First(&tenantModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resolvedTenant{}, ErrTenantNotFound
		}
		return resolvedTenant{}, err
	}

	return r.loadResolvedTenant(ctx, tenantModel)
}

func (r *Resolver) lookupByDomain(ctx context.Context, hostname string) (resolvedTenant, error) {
	resolved, err := r.lookupByTenantDomain(ctx, hostname)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, ErrTenantNotFound) {
		return resolvedTenant{}, err
	}

	return r.lookupByPublicBaseURLHostname(ctx, hostname)
}

func (r *Resolver) lookupByTenantDomain(ctx context.Context, hostname string) (resolvedTenant, error) {
	type domainRow struct {
		ID            string
		Slug          string
		Name          string
		PublicBaseURL string
	}

	var row domainRow
	err := r.DB.Table("tenant_domains AS td").
		WithContext(ctx).
		Select("t.id, t.slug, t.name, COALESCE(ts.public_base_url, '') AS public_base_url").
		Joins("JOIN tenants AS t ON t.id = td.tenant_id").
		Joins("LEFT JOIN tenant_settings AS ts ON ts.tenant_id = t.id").
		Where("LOWER(td.domain) = ?", hostname).
		First(&row).Error
	if err == nil {
		return resolvedTenant{ID: row.ID, Slug: row.Slug, Name: row.Name, PublicBaseURL: row.PublicBaseURL}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return resolvedTenant{}, err
	}

	return resolvedTenant{}, ErrTenantNotFound
}

func (r *Resolver) lookupByPublicBaseURLHostname(ctx context.Context, hostname string) (resolvedTenant, error) {
	type settingsRow struct {
		ID            string
		Slug          string
		Name          string
		PublicBaseURL string
	}
	rows := make([]settingsRow, 0)
	if err := r.DB.WithContext(ctx).Table("tenant_settings AS ts").
		Select("t.id, t.slug, t.name, COALESCE(ts.public_base_url, '') AS public_base_url").
		Joins("JOIN tenants AS t ON t.id = ts.tenant_id").
		Where("TRIM(COALESCE(ts.public_base_url, '')) <> ''").
		Find(&rows).Error; err != nil {
		return resolvedTenant{}, err
	}
	for _, candidate := range rows {
		if hostnameFromBaseURL(candidate.PublicBaseURL) != hostname {
			continue
		}
		return resolvedTenant{ID: candidate.ID, Slug: candidate.Slug, Name: candidate.Name, PublicBaseURL: candidate.PublicBaseURL}, nil
	}

	return resolvedTenant{}, ErrTenantNotFound
}

func (r *Resolver) loadResolvedTenant(ctx context.Context, tenantModel tenantRow) (resolvedTenant, error) {
	result := resolvedTenant{
		ID:   strings.TrimSpace(tenantModel.ID),
		Slug: strings.TrimSpace(tenantModel.Slug),
		Name: strings.TrimSpace(tenantModel.Name),
	}

	var settings struct {
		PublicBaseURL string
	}
	if err := r.DB.WithContext(ctx).Table("tenant_settings").Select("COALESCE(public_base_url, '') AS public_base_url").Where("tenant_id = ?", tenantModel.ID).First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, nil
		}
		return resolvedTenant{}, err
	}
	result.PublicBaseURL = strings.TrimSpace(settings.PublicBaseURL)
	return result, nil
}

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModeHosted:
		return ModeHosted
	default:
		return ModeSingle
	}
}

func normalizeInternalTargetHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultInternalTargetHeader
	}
	return value
}

func normalizeHostname(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return strings.TrimSpace(host)
	}
	if idx := strings.Index(value, ":"); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func normalizeBaseURL(value string) string {
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

func hostnameFromBaseURL(value string) string {
	value = normalizeBaseURL(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return normalizeHostname(parsed.Hostname())
}

func subdomainSlug(hostname, baseHost string) (string, bool) {
	hostname = normalizeHostname(hostname)
	baseHost = normalizeHostname(baseHost)
	if hostname == "" || baseHost == "" || hostname == baseHost {
		return "", false
	}
	suffix := "." + baseHost
	if !strings.HasSuffix(hostname, suffix) {
		return "", false
	}
	left := strings.TrimSuffix(hostname, suffix)
	left = strings.Trim(strings.TrimSpace(left), ".")
	if left == "" || strings.Contains(left, ".") {
		return "", false
	}
	return left, true
}

func isInternalRoute(path string) bool {
	path = strings.TrimSpace(path)
	return path == "/internal" || strings.HasPrefix(path, "/internal/")
}
