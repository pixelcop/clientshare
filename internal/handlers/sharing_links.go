package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
)

func (h *SharingHandler) ItemLinks(c fiber.Ctx) error {
	req, err := h.itemRequest(c)
	if err != nil {
		return err
	}
	return h.listLinks(c, req)
}
func (h *SharingHandler) listLinks(c fiber.Ctx, req services.AccessRequest) error {
	links, err := h.service.LinksAtScope(c.Context(), req)
	if err != nil {
		return sharingError(err)
	}
	for i := range links {
		if links[i].Token != "" {
			links[i].LinkURL = buildSecureLinkURL(tenantBaseURLFromCtx(c, h.baseURL), links[i].Token)
		}
	}
	return c.JSON(links)
}

func (h *SharingHandler) CreateItemLink(c fiber.Ctx) error {
	req, err := h.itemRequest(c)
	if err != nil {
		return err
	}
	return h.createLink(c, req)
}
func (h *SharingHandler) createLink(c fiber.Ctx, req services.AccessRequest) error {
	key, err := requestIdempotencyKey(c, "create-link:"+req.ClientID+":"+req.FileID, req.FileID != "")
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	var input services.LinkInput
	if err := c.Bind().Body(&input); err != nil {
		return fiber.ErrBadRequest
	}
	if input.ExpiryDays < 1 {
		input.ExpiryDays = services.DefaultSecureLinkDefaultExpiryDays
		settings, err := services.NewTenantSettingsService(h.service.DB).Get(c.Context(), req.TenantID)
		if err != nil {
			return sharingError(err)
		}
		input.ExpiryDays = services.SecureLinkExpiryOrDefault(settings)
	}
	requestKey := ""
	if key != nil {
		requestKey = *key
	}
	link, err := h.service.CreateLink(c.Context(), req, input, requestKey)
	if err != nil {
		return sharingError(err)
	}
	link.LinkURL = buildSecureLinkURL(tenantBaseURLFromCtx(c, h.baseURL), link.Token)
	return c.Status(201).JSON(struct {
		models.SecureLink
		EmailQueued bool `json:"email_queued"`
	}{*link, false})
}
