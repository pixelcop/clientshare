package services

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"gorm.io/gorm"
)

// TenantProvisioningService handles lifecycle operations for tenants.
type TenantProvisioningService struct {
	db           *gorm.DB
	inviteSecret string
	inviteTTL    time.Duration
	emailQueue   emailpkg.EmailQueue
	baseURL      string
}

func NewTenantProvisioningService(
	db *gorm.DB,
	inviteSecret string,
	inviteTTL time.Duration,
	emailQueue emailpkg.EmailQueue,
	baseURL string,
) *TenantProvisioningService {
	return &TenantProvisioningService{
		db:           db,
		inviteSecret: inviteSecret,
		inviteTTL:    inviteTTL,
		emailQueue:   emailQueue,
		baseURL:      baseURL,
	}
}

// CreateTenantInput defines the fields for creating a new tenant.
type CreateTenantInput struct {
	Name            string
	Slug            string
	PlanCode        string
	MaxUsers        int64
	MaxStorageBytes int64
}

type TenantAvailabilityResult struct {
	Slug      string `json:"slug,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// CreateTenant creates a tenant with its associated settings and entitlements rows.
func (s *TenantProvisioningService) CreateTenant(input CreateTenantInput) (*models.Tenant, error) {
	slug := strings.ToLower(strings.TrimSpace(input.Slug))
	name := strings.TrimSpace(input.Name)
	if slug == "" {
		return nil, errors.New("slug is required")
	}
	if name == "" {
		return nil, errors.New("name is required")
	}

	planCode := input.PlanCode
	if planCode == "" {
		planCode = "self_hosted"
	}

	tenant := models.Tenant{Slug: slug, Name: name}
	settings := models.TenantSettings{}
	entitlement := models.TenantEntitlement{
		Status:          "active",
		PlanCode:        planCode,
		MaxUsers:        input.MaxUsers,
		MaxStorageBytes: input.MaxStorageBytes,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&tenant).Error; err != nil {
			return err
		}
		settings.TenantID = tenant.ID
		if err := tx.Create(&settings).Error; err != nil {
			return err
		}
		entitlement.TenantID = tenant.ID
		if err := tx.Create(&entitlement).Error; err != nil {
			return err
		}
		return s.logAuditEvent(tx, nil, "internal-api", "tenant.created", &tenant.ID, map[string]any{"slug": slug, "name": name})
	})
	if err != nil {
		return nil, err
	}
	return &tenant, nil
}

func (s *TenantProvisioningService) CheckAvailability(slug, domain string) (*TenantAvailabilityResult, error) {
	normalizedSlug := strings.ToLower(strings.TrimSpace(slug))
	normalizedDomain := strings.ToLower(strings.TrimSpace(domain))
	if normalizedSlug == "" && normalizedDomain == "" {
		return nil, errors.New("slug or domain is required")
	}

	result := &TenantAvailabilityResult{
		Slug:      normalizedSlug,
		Domain:    normalizedDomain,
		Available: true,
	}

	if normalizedSlug != "" {
		var slugCount int64
		if err := s.db.Model(&models.Tenant{}).Where("slug = ?", normalizedSlug).Count(&slugCount).Error; err != nil {
			return nil, err
		}
		if slugCount > 0 {
			result.Available = false
			result.Reason = "slug_taken"
			return result, nil
		}
	}

	if normalizedDomain != "" {
		var domainCount int64
		if err := s.db.Model(&models.TenantDomain{}).Where("domain = ?", normalizedDomain).Count(&domainCount).Error; err != nil {
			return nil, err
		}
		if domainCount > 0 {
			result.Available = false
			result.Reason = "domain_taken"
		}
	}

	return result, nil
}

// UpdateTenantInput defines updatable fields for a tenant.
type UpdateTenantInput struct {
	Name            *string
	Slug            *string
	PlanCode        *string
	MaxUsers        *int64
	MaxStorageBytes *int64
}

// UpdateTenant updates the tenant and/or its entitlement row.
func (s *TenantProvisioningService) UpdateTenant(id string, input UpdateTenantInput) (*models.Tenant, *models.TenantEntitlement, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil, errors.New("id is required")
	}

	var tenant models.Tenant
	if err := s.db.Where("id = ?", id).First(&tenant).Error; err != nil {
		return nil, nil, err
	}

	var entitlement models.TenantEntitlement
	if err := s.db.Where("tenant_id = ?", id).First(&entitlement).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		tenantUpdates := map[string]any{}
		if input.Name != nil {
			tenantUpdates["name"] = strings.TrimSpace(*input.Name)
		}
		if input.Slug != nil {
			tenantUpdates["slug"] = strings.ToLower(strings.TrimSpace(*input.Slug))
		}
		if len(tenantUpdates) > 0 {
			if err := tx.Model(&tenant).Updates(tenantUpdates).Error; err != nil {
				return err
			}
		}

		entitlementUpdates := map[string]any{}
		if input.PlanCode != nil {
			entitlementUpdates["plan_code"] = *input.PlanCode
		}
		if input.MaxUsers != nil {
			entitlementUpdates["max_users"] = *input.MaxUsers
		}
		if input.MaxStorageBytes != nil {
			entitlementUpdates["max_storage_bytes"] = *input.MaxStorageBytes
		}
		if len(entitlementUpdates) > 0 {
			if err := tx.Model(&entitlement).Where("tenant_id = ?", id).Updates(entitlementUpdates).Error; err != nil {
				return err
			}
		}
		return s.logAuditEvent(tx, &id, "internal-api", "tenant.updated", &id, map[string]any{"updates": input})
	})
	if err != nil {
		return nil, nil, err
	}

	// Reload after updates
	s.db.Where("id = ?", id).First(&tenant)
	s.db.Where("tenant_id = ?", id).First(&entitlement)
	return &tenant, &entitlement, nil
}

// CreateAdminUserInput defines fields for provisioning an admin user.
type CreateAdminUserInput struct {
	PasskeySignupToken string
	Email              string
	Name               string
	PasswordHash       string
}

// CreateAdminUser creates an admin with a password hash or a verified signup passkey.
func (s *TenantProvisioningService) CreateAdminUser(tenantID string, input CreateAdminUserInput) (*models.User, error) {
	tenantID = strings.TrimSpace(tenantID)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	name := strings.TrimSpace(input.Name)
	passwordHash := strings.TrimSpace(input.PasswordHash)
	if input.PasskeySignupToken != "" {
		if passwordHash != "" {
			return nil, errors.New("choose password or passkey")
		}
		var user *models.User
		err := s.db.Transaction(func(tx *gorm.DB) error {
			var err error
			user, err = createPasskeyAdmin(tx, tenantID, input)
			if err != nil {
				return err
			}
			return s.logAuditEvent(tx, &tenantID, "internal-api", "admin_user.created", &user.ID, map[string]any{"email": email})
		})
		return user, err
	}
	if tenantID == "" {
		return nil, errors.New("tenantID is required")
	}
	if email == "" {
		return nil, errors.New("email is required")
	}
	if passwordHash == "" {
		return nil, errors.New("passwordHash is required")
	}

	var existing models.User
	if err := s.db.Where("tenant_id = ? AND email = ?", tenantID, email).First(&existing).Error; err == nil {
		return nil, errors.New("user already exists")
	}

	now := time.Now()
	user := models.User{
		TenantID:     tenantID,
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
		Role:         "admin",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return s.logAuditEvent(tx, &tenantID, "internal-api", "admin_user.created", &user.ID, map[string]any{"email": email})
	})
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// RegisterDomainInput defines fields for registering a domain.
type RegisterDomainInput struct {
	Domain string
	Kind   string
}

// RegisterDomain adds a domain entry to the tenant.
func (s *TenantProvisioningService) RegisterDomain(tenantID string, input RegisterDomainInput) (*models.TenantDomain, error) {
	tenantID = strings.TrimSpace(tenantID)
	domain := strings.ToLower(strings.TrimSpace(input.Domain))
	kind := strings.TrimSpace(input.Kind)
	if tenantID == "" {
		return nil, errors.New("tenantID is required")
	}
	if domain == "" {
		return nil, errors.New("domain is required")
	}
	if kind == "" {
		kind = "custom"
	}

	var tenant models.Tenant
	if err := s.db.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil, err
	}

	td := models.TenantDomain{TenantID: tenantID, Domain: domain, Kind: kind}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&td).Error; err != nil {
			return err
		}
		return s.logAuditEvent(tx, &tenantID, "internal-api", "tenant.domain_registered", &td.ID, map[string]any{"domain": domain, "kind": kind})
	})
	if err != nil {
		return nil, err
	}
	return &td, nil
}

// SetTenantStatus updates tenant entitlement status (e.g. "active", "suspended").
func (s *TenantProvisioningService) SetTenantStatus(tenantID, status string) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return errors.New("tenantID is required")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.TenantEntitlement{}).Where("tenant_id = ?", tenantID).Update("status", status)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return s.logAuditEvent(tx, &tenantID, "internal-api", "tenant.status_changed", &tenantID, map[string]any{"status": status})
	})
}

func (s *TenantProvisioningService) logAuditEvent(tx *gorm.DB, tenantID *string, actor, action string, targetID *string, payload map[string]any) error {
	p, _ := json.Marshal(payload)
	event := models.AuditEvent{
		TenantID: tenantID,
		Actor:    actor,
		Action:   action,
		TargetID: targetID,
		Payload:  string(p),
	}
	return tx.Create(&event).Error
}
