package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"gorm.io/gorm"
)

func (h *AuthHandler) createUserSession(c fiber.Ctx, tenantID, userID, groupID string) (string, error) {
	var raw string
	err := h.db.WithContext(c.Context()).Transaction(func(tx *gorm.DB) error {
		store := &auth.SessionStore{DB: tx}
		var err error
		raw, err = store.Create(c.Context(), tenantID, userID, "", auth.PortalAudience, groupID, c.Cookies(auth.SessionCookie))
		if err != nil {
			return err
		}
		if _, err = store.Validate(c.Context(), raw, auth.PortalAudience, tenantID); err != nil {
			return fiber.NewError(401, "Account is unavailable")
		}
		return nil
	})
	return raw, err
}

func revokeUserSessions(tx *gorm.DB, c fiber.Ctx, tenantID, userID string) error {
	return (&auth.SessionStore{DB: tx}).RevokeUser(c.Context(), tenantID, userID)
}

func toSessionID(c fiber.Ctx) string { id, _ := c.Locals("session_id").(string); return id }
