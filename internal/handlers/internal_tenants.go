package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type InternalTenantsHandler struct {
	svc *services.TenantProvisioningService
}

func NewInternalTenantsHandler(svc *services.TenantProvisioningService) *InternalTenantsHandler {
	return &InternalTenantsHandler{svc: svc}
}

// RegisterInternalTenantRoutes mounts the internal control-plane endpoints.
// The caller is responsible for applying InternalAuthRequired middleware to the group.
func RegisterInternalTenantRoutes(router fiber.Router, svc *services.TenantProvisioningService) {
	h := NewInternalTenantsHandler(svc)
	router.Get("/tenants/availability", h.GetAvailability)
	router.Post("/tenants", h.CreateTenant)
	router.Patch("/tenants/:id", h.UpdateTenant)
	router.Post("/tenants/:id/admin-users", h.CreateAdminUser)
	router.Post("/tenants/:id/domains", h.RegisterDomain)
	router.Post("/tenants/:id/suspend", h.SuspendTenant)
	router.Post("/tenants/:id/resume", h.ResumeTenant)
}

// GET /internal/tenants/availability?slug=acme&domain=acme.example.com
func (h *InternalTenantsHandler) GetAvailability(c fiber.Ctx) error {
	result, err := h.svc.CheckAvailability(c.Query("slug"), c.Query("domain"))
	if err != nil {
		if err.Error() == "slug or domain is required" {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
		}
		utils.Logger(c).Error("check tenant availability failed", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}

	return c.JSON(result)
}

// POST /internal/tenants
func (h *InternalTenantsHandler) CreateTenant(c fiber.Ctx) error {
	var req struct {
		Name            string `json:"name"`
		Slug            string `json:"slug"`
		PlanCode        string `json:"plan_code"`
		MaxUsers        int64  `json:"max_users"`
		MaxStorageBytes int64  `json:"max_storage_bytes"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "name and slug are required"})
	}

	tenant, err := h.svc.CreateTenant(services.CreateTenantInput{
		Name:            req.Name,
		Slug:            req.Slug,
		PlanCode:        req.PlanCode,
		MaxUsers:        req.MaxUsers,
		MaxStorageBytes: req.MaxStorageBytes,
	})
	if err != nil {
		if isUniqueConstraintErr(err) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "a tenant with that slug already exists"})
		}
		utils.Logger(c).Error("create tenant failed", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	return c.Status(fiber.StatusCreated).JSON(tenant)
}

// PATCH /internal/tenants/:id
func (h *InternalTenantsHandler) UpdateTenant(c fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id is required"})
	}

	var req struct {
		Name            *string `json:"name"`
		Slug            *string `json:"slug"`
		PlanCode        *string `json:"plan_code"`
		MaxUsers        *int64  `json:"max_users"`
		MaxStorageBytes *int64  `json:"max_storage_bytes"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	tenant, entitlement, err := h.svc.UpdateTenant(id, services.UpdateTenantInput{
		Name:            req.Name,
		Slug:            req.Slug,
		PlanCode:        req.PlanCode,
		MaxUsers:        req.MaxUsers,
		MaxStorageBytes: req.MaxStorageBytes,
	})
	if err != nil {
		if isNotFoundErr(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tenant not found"})
		}
		if isUniqueConstraintErr(err) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "slug already in use"})
		}
		utils.Logger(c).Error("update tenant failed", zap.Error(err), zap.String("id", id))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	return c.JSON(fiber.Map{"id": tenant.ID, "slug": tenant.Slug, "name": tenant.Name, "entitlements": entitlement})
}

// POST /internal/tenants/:id/admin-users
func (h *InternalTenantsHandler) CreateAdminUser(c fiber.Ctx) error {
	tenantID := strings.TrimSpace(c.Params("id"))
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id is required"})
	}

	var req struct {
		Email        string `json:"email"`
		Name         string `json:"name"`
		PasswordHash string `json:"password_hash"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Email) == "" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "email is required"})
	}
	if strings.TrimSpace(req.PasswordHash) == "" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "password_hash is required"})
	}

	user, err := h.svc.CreateAdminUser(tenantID, services.CreateAdminUserInput{
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: req.PasswordHash,
	})
	if err != nil {
		if isNotFoundErr(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tenant not found"})
		}
		if err.Error() == "user already exists" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "user already exists"})
		}
		utils.Logger(c).Error("create admin user failed", zap.Error(err), zap.String("tenant_id", tenantID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": user.ID, "email": user.Email, "name": user.Name})
}

// POST /internal/tenants/:id/domains
func (h *InternalTenantsHandler) RegisterDomain(c fiber.Ctx) error {
	tenantID := strings.TrimSpace(c.Params("id"))
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id is required"})
	}

	var req struct {
		Domain string `json:"domain"`
		Kind   string `json:"kind"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Domain) == "" {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "domain is required"})
	}

	td, err := h.svc.RegisterDomain(tenantID, services.RegisterDomainInput{
		Domain: req.Domain,
		Kind:   req.Kind,
	})
	if err != nil {
		if isNotFoundErr(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tenant not found"})
		}
		if isUniqueConstraintErr(err) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "domain already registered"})
		}
		utils.Logger(c).Error("register domain failed", zap.Error(err), zap.String("tenant_id", tenantID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	return c.Status(fiber.StatusCreated).JSON(td)
}

// POST /internal/tenants/:id/suspend
func (h *InternalTenantsHandler) SuspendTenant(c fiber.Ctx) error {
	return h.setStatus(c, "suspended")
}

// POST /internal/tenants/:id/resume
func (h *InternalTenantsHandler) ResumeTenant(c fiber.Ctx) error {
	return h.setStatus(c, "active")
}

func (h *InternalTenantsHandler) setStatus(c fiber.Ctx, status string) error {
	tenantID := strings.TrimSpace(c.Params("id"))
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "id is required"})
	}
	if err := h.svc.SetTenantStatus(tenantID, status); err != nil {
		if isNotFoundErr(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tenant not found"})
		}
		utils.Logger(c).Error("set tenant status failed", zap.Error(err), zap.String("tenant_id", tenantID), zap.String("status", status))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	return c.JSON(fiber.Map{"status": status})
}

func isNotFoundErr(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}
