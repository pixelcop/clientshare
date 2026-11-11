package handlers

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"gorm.io/gorm"
)

func roleResponse(fn fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := fn(c)
		if err == nil {
			return nil
		}
		status := 500
		switch {
		case errors.Is(err, services.ErrRoleInput), errors.Is(err, services.ErrSharingInput):
			status = 400
		case errors.Is(err, services.ErrRoleForbidden), errors.Is(err, services.ErrSharingCapabilities):
			status = 403
		case errors.Is(err, services.ErrRoleConflict), errors.Is(err, services.ErrSharingConflict):
			status = 409
		case errors.Is(err, gorm.ErrRecordNotFound):
			status = 404
		}
		message := err.Error()
		if strings.Contains(message, "UNIQUE constraint failed") || strings.Contains(message, "duplicate key value") {
			status = 409
			message = "A role or group with this name already exists"
		}
		if status == 500 {
			message = "Unable to update access settings"
		}
		return c.Status(status).JSON(fiber.Map{"error": message})
	}
}

func RegisterAccessRoleRoutes(router fiber.Router, db *gorm.DB) {
	s := &services.AccessRoleService{DB: db}
	assignmentMutation := func(c fiber.Ctx) error {
		var input struct {
			Version   uint64     `json:"version"`
			RoleID    string     `json:"role_id"`
			ExpiresAt *time.Time `json:"expires_at"`
		}
		if err := c.Bind().Body(&input); err != nil {
			return services.ErrRoleInput
		}
		if err := (&services.SharingService{DB: db}).UpdateRoleAssignment(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Params("id"), input.Version, input.RoleID, input.ExpiresAt, c.Method() == "DELETE"); err != nil {
			return err
		}
		return c.SendStatus(204)
	}
	router.Patch("/role-assignments/:id", middleware.AuthRequired, roleResponse(assignmentMutation))
	router.Delete("/role-assignments/:id", middleware.AuthRequired, roleResponse(assignmentMutation))
	router.Get("/access-roles", middleware.AuthRequired, roleResponse(func(c fiber.Ctx) error {
		roles, err := s.Roles(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Query("archived") == "true")
		if err != nil {
			return err
		}
		return c.JSON(roles)
	}))
	router.Get("/access-roles/:id/impact", middleware.AuthRequired, roleResponse(func(c fiber.Ctx) error {
		impact, err := s.Impact(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Params("id"))
		if err != nil {
			return err
		}
		return c.JSON(impact)
	}))
	router.Get("/access-roles/:id", middleware.AuthRequired, roleResponse(func(c fiber.Ctx) error {
		var role models.AccessRole
		if fiber.Locals[string](c, "user_id") == "" {
			return services.ErrRoleForbidden
		}
		if err := db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantIDFromCtx(c), c.Params("id")).First(&role).Error; err != nil {
			return err
		}
		return c.JSON(role)
	}))
	router.Put("/clients/:id/members/:userID/role", middleware.AuthRequired, roleResponse(func(c fiber.Ctx) error {
		var input struct {
			RoleID string `json:"role_id"`
		}
		if err := c.Bind().Body(&input); err != nil {
			return services.ErrRoleInput
		}
		req := middleware.AccessRequest(c, c.Params("id"), "", services.AccessShare)
		if err := (&services.SharingService{DB: db}).ChangeMemberRole(c.Context(), req, c.Params("userID"), input.RoleID); err != nil {
			return err
		}
		return c.SendStatus(204)
	}))
	type roleRequest struct {
		models.AccessRole
		ReplacementID      string `json:"replacement_id"`
		ReplacementVersion uint64 `json:"replacement_version"`
		ExpectedVersion    uint64 `json:"expected_version"`
		Impact             string `json:"impact"`
	}
	save := func(c fiber.Ctx) error {
		var input roleRequest
		if err := c.Bind().Body(&input); err != nil {
			return services.ErrRoleInput
		}
		role, err := s.SaveRole(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Params("id"), input.AccessRole, input.ExpectedVersion, input.Impact)
		if err != nil {
			return err
		}
		if c.Params("id") == "" {
			c.Status(201)
		}
		return c.JSON(role)
	}
	router.Post("/access-roles", middleware.AuthRequired, roleResponse(save))
	router.Put("/access-roles/:id", middleware.AuthRequired, roleResponse(save))
	router.Post("/access-roles/:id/archive", middleware.AuthRequired, roleResponse(func(c fiber.Ctx) error {
		var input roleRequest
		if err := c.Bind().Body(&input); err != nil {
			return services.ErrRoleInput
		}
		if err := s.ArchiveRole(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Params("id"), input.ExpectedVersion, input.Impact); err != nil {
			return err
		}
		return c.SendStatus(204)
	}))
	router.Delete("/access-roles/:id", middleware.AuthRequired, roleResponse(func(c fiber.Ctx) error {
		var input roleRequest
		if err := c.Bind().Body(&input); err != nil {
			return services.ErrRoleInput
		}
		if err := s.DeleteRole(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Params("id"), input.ReplacementID, input.ExpectedVersion, input.Impact, input.ReplacementVersion); err != nil {
			return err
		}
		return c.SendStatus(204)
	}))
	router.Get("/users/:id/access-settings", middleware.AuthRequired, roleResponse(func(c fiber.Ctx) error {
		var actor models.User
		if err := db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantIDFromCtx(c), fiber.Locals[string](c, "user_id")).First(&actor).Error; err != nil {
			return err
		}
		if actor.Role != "admin" {
			return services.ErrRoleForbidden
		}
		settings, err := services.UserAccessSettingsFor(c.Context(), db, tenantIDFromCtx(c), c.Params("id"))
		if err != nil {
			return err
		}
		return c.JSON(settings)
	}))
	router.Get("/access-groups", middleware.AuthRequired, roleResponse(func(c fiber.Ctx) error {
		groups, err := s.Groups(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"))
		if err != nil {
			return err
		}
		return c.JSON(groups)
	}))
	preview := func(c fiber.Ctx) error {
		var input services.GroupInput
		if err := c.Bind().Body(&input); err != nil {
			return services.ErrRoleInput
		}
		impact, err := s.PreviewGroup(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Params("id"), input)
		if err != nil {
			return err
		}
		return c.JSON(impact)
	}
	router.Post("/access-groups/preview", middleware.AuthRequired, roleResponse(preview))
	router.Post("/access-groups/:id/preview", middleware.AuthRequired, roleResponse(preview))
	saveGroup := func(c fiber.Ctx) error {
		var input services.GroupInput
		if err := c.Bind().Body(&input); err != nil {
			return services.ErrRoleInput
		}
		group, err := s.SaveGroup(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Params("id"), input)
		if err != nil {
			return err
		}
		if c.Params("id") == "" {
			c.Status(201)
		}
		return c.JSON(group)
	}
	router.Post("/access-groups", middleware.AuthRequired, roleResponse(saveGroup))
	router.Put("/access-groups/:id", middleware.AuthRequired, roleResponse(saveGroup))
	router.Delete("/access-groups/:id", middleware.AuthRequired, roleResponse(func(c fiber.Ctx) error {
		var input services.GroupInput
		if err := c.Bind().Body(&input); err != nil {
			return services.ErrRoleInput
		}
		if err := s.DeleteGroup(c.Context(), tenantIDFromCtx(c), fiber.Locals[string](c, "user_id"), c.Params("id"), input); err != nil {
			return err
		}
		return c.SendStatus(204)
	}))
}
