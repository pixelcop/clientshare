package handlers

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"gorm.io/gorm"
)

// RegisterSharingLifecycleRoutes is intentionally not called by NewWebApp yet.
// Keeping registration separate lets HTTP lifecycle/security tests exercise the
// real handlers without enabling creation before the enforcement release gate.
func RegisterSharingLifecycleRoutes(router fiber.Router, db *gorm.DB, secret, signingKey string, queue emailpkg.EmailQueue, baseURL string, identityTTL time.Duration) {
	h := &SharingHandler{service: &services.SharingService{DB: db, TokenSecret: secret, LinkSigningKey: signingKey}, emailQueue: queue, baseURL: baseURL, identityTTL: identityTTL}
	router.Post("/files/:id/grants", middleware.AuthRequired, h.CreateGrant)
	router.Patch("/files/:id/grants/:grantID", middleware.AuthRequired, h.UpdateGrant)
	router.Delete("/files/:id/grants/:grantID", middleware.AuthRequired, h.RevokeGrant)
	router.Post("/files/:id/invitations", middleware.AuthRequired, h.CreateItemInvitation)
	router.Post("/clients/:id/invitations", middleware.AuthRequired, h.CreateClientInvitation)
	router.Post("/invitations/:invitationID/email", middleware.AuthRequired, h.EmailInvitation)
	router.Delete("/invitations/:invitationID", middleware.AuthRequired, h.RevokeInvitation)
	router.Post("/invitations/accept", middleware.AuthRequired, h.AcceptInvitation)
	router.Delete("/clients/:id/members/:userID", middleware.AuthRequired, h.RemoveMember)
	router.Post("/files/:id/links", middleware.AuthRequired, h.CreateItemLink)
}

func sharingError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return fiber.ErrNotFound
	case errors.Is(err, services.ErrSharingConflict), errors.Is(err, services.ErrLinkRequestConflict), errors.Is(err, emailpkg.ErrIdempotencyConflict):
		return fiber.NewError(409, "Access changed; refresh before retrying")
	case errors.Is(err, services.ErrSharingCapabilities):
		return fiber.NewError(403, "Requested capabilities exceed your current sharing authority")
	case errors.Is(err, services.ErrSharingInput):
		return fiber.NewError(400, "Invalid sharing settings")
	default:
		return fiber.NewError(503, "Access validation is temporarily unavailable")
	}
}

func (h *SharingHandler) itemRequest(c fiber.Ctx) (services.AccessRequest, error) {
	req := middleware.AccessRequest(c, "", c.Params("id"), services.AccessShare)
	if req.Actor.UserID == "" || req.Actor.LinkID != "" {
		return req, fiber.ErrNotFound
	}
	var file models.File
	if err := h.service.DB.WithContext(c.Context()).Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", req.TenantID, req.FileID).First(&file).Error; err != nil {
		return req, sharingError(err)
	}
	req.ClientID = file.ClientID
	return req, nil
}

func (h *SharingHandler) ItemAccess(c fiber.Ctx) error {
	req, err := h.itemRequest(c)
	if err != nil {
		return err
	}
	view, err := h.service.ItemAccess(c.Context(), req)
	if err != nil {
		return sharingError(err)
	}
	for i := range view.Links {
		if view.Links[i].Token != "" {
			view.Links[i].LinkURL = buildSecureLinkURL(tenantBaseURLFromCtx(c, h.baseURL), view.Links[i].Token)
		}
	}
	return c.JSON(view)
}

func (h *SharingHandler) CreateGrant(c fiber.Ctx) error {
	req, err := h.itemRequest(c)
	if err != nil {
		return err
	}
	var input services.GrantInput
	if err := c.Bind().Body(&input); err != nil {
		return fiber.ErrBadRequest
	}
	grant, err := h.service.CreateGrant(c.Context(), req, input)
	if err != nil {
		return sharingError(err)
	}
	return c.Status(201).JSON(grant)
}

func (h *SharingHandler) UpdateGrant(c fiber.Ctx) error { return h.mutateGrant(c, false) }
func (h *SharingHandler) RevokeGrant(c fiber.Ctx) error { return h.mutateGrant(c, true) }
func (h *SharingHandler) mutateGrant(c fiber.Ctx, revoke bool) error {
	req, err := h.itemRequest(c)
	if err != nil {
		return err
	}
	var body struct {
		services.GrantInput
		Version uint64 `json:"version"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return fiber.ErrBadRequest
	}
	grant, err := h.service.UpdateGrant(c.Context(), req, c.Params("grantID"), body.Version, body.GrantInput, revoke)
	if err != nil {
		return sharingError(err)
	}
	return c.JSON(grant)
}

func (h *SharingHandler) CreateItemInvitation(c fiber.Ctx) error {
	req, err := h.itemRequest(c)
	if err != nil {
		return err
	}
	return h.createInvitation(c, req)
}
func (h *SharingHandler) CreateClientInvitation(c fiber.Ctx) error {
	return h.createInvitation(c, middleware.AccessRequest(c, c.Params("id"), "", services.AccessInviteClient))
}
func (h *SharingHandler) createInvitation(c fiber.Ctx, req services.AccessRequest) error {
	key, err := requestIdempotencyKey(c, "invite:"+req.ClientID+":"+req.FileID, true)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	var input services.InvitationInput
	if err := c.Bind().Body(&input); err != nil {
		return fiber.ErrBadRequest
	}
	invitation, err := h.service.CreateInvitation(c.Context(), req, input, *key)
	if err != nil {
		return sharingError(err)
	}
	// Creation and delivery are separate. Never return identity-setup URLs or
	// raw credentials to an inviter, even if outbound email is disabled.
	return c.Status(201).JSON(invitation)
}

func (h *SharingHandler) EmailInvitation(c fiber.Ctx) error {
	key, err := requestIdempotencyKey(c, "email-invitation:"+c.Params("invitationID"), true)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	req := middleware.AccessRequest(c, "", "", services.AccessShare)
	// Authorize even when delivery is unavailable; don't reveal identity state
	// or mint an account-setup credential merely to report disabled mail.
	if _, _, err := h.service.InvitationForDelivery(c.Context(), req, c.Params("invitationID")); err != nil {
		return sharingError(err)
	}
	if h.emailQueue == nil {
		return c.Status(503).JSON(fiber.Map{"status": "unavailable", "error": "Email is unavailable; retry this invitation after email is configured"})
	}
	delivery, err := h.service.PrepareInvitationDelivery(c.Context(), req, c.Params("invitationID"), h.identityTTL)
	if err != nil {
		return sharingError(err)
	}
	baseURL := strings.TrimRight(tenantBaseURLFromCtx(c, h.baseURL), "/")
	if baseURL == "" {
		return fiber.NewError(503, "Invitation URL is unavailable")
	}
	path := "/accept-access?token="
	if delivery.NeedsIdentity {
		path = "/accept-invite?token="
	}
	inviteURL := baseURL + path + url.QueryEscape(delivery.RawToken)
	subject, body, htmlBody := emailpkg.RenderUserInviteEmail(delivery.User.Name, inviteURL, services.DefaultSiteTitle, "You have been invited to shared content. Sign in with the invited email to accept.", delivery.ExpiresAt)
	item, err := h.emailQueue.Queue(emailpkg.Email{TenantID: req.TenantID, To: delivery.Invitation.Email, Subject: subject, Body: body, HTMLBody: htmlBody, ClientID: &delivery.Invitation.ClientID, AccessInvitationID: &delivery.Invitation.ID, IdentityInviteID: delivery.IdentityInviteID, DeduplicationKey: *key}, models.Now())
	if err != nil {
		return sharingError(err)
	}
	if item == nil {
		return fiber.NewError(503, "Email queue did not accept the invitation")
	}
	if item.CanceledAt != nil {
		return c.Status(410).JSON(fiber.Map{"status": "unavailable", "error": "This delivery request was canceled"})
	}
	status := "queued"
	if item.SentAt != nil {
		status = "sent"
	}
	return c.Status(202).JSON(fiber.Map{"status": status, "invitation_id": delivery.Invitation.ID})
}

func (h *SharingHandler) RevokeInvitation(c fiber.Ctx) error {
	err := h.service.RevokeInvitation(c.Context(), middleware.AccessRequest(c, "", "", services.AccessShare), c.Params("invitationID"))
	if err != nil {
		return sharingError(err)
	}
	return c.SendStatus(204)
}
func (h *SharingHandler) AcceptInvitation(c fiber.Ctx) error {
	req := middleware.AccessRequest(c, "", "", services.AccessContext)
	if req.Actor.UserID == "" || req.Actor.LinkID != "" {
		return fiber.ErrNotFound
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return fiber.ErrBadRequest
	}
	invitation, err := h.service.AcceptInvitation(c.Context(), req.TenantID, req.Actor.UserID, body.Token)
	if err != nil {
		return sharingError(err)
	}
	// Recipient results omit inviter/other-recipient provenance and secrets.
	return c.JSON(fiber.Map{"accepted": true, "file_id": invitation.FileID, "client_id": invitation.ClientID})
}
func (h *SharingHandler) RemoveMember(c fiber.Ctx) error {
	var body struct {
		RevokeLinks bool `json:"revoke_links"`
	}
	if len(c.Body()) > 0 {
		if err := c.Bind().Body(&body); err != nil {
			return fiber.ErrBadRequest
		}
	}
	err := h.service.RemoveMember(c.Context(), middleware.AccessRequest(c, c.Params("id"), "", services.AccessInviteClient), c.Params("userID"), body.RevokeLinks)
	if err != nil {
		return sharingError(err)
	}
	return c.SendStatus(204)
}
