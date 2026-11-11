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

// RegisterSharingLifecycleRoutes is registered only alongside exact-resource
// enforcement on files, client/user management, sessions, feeds and mail. Its
// handlers independently revalidate scope and ceilings on every operation.
func RegisterSharingLifecycleRoutes(router fiber.Router, db *gorm.DB, secret, signingKey string, queue emailpkg.EmailQueue, baseURL string, identityTTL time.Duration) {
	h := &SharingHandler{service: &services.SharingService{DB: db, TokenSecret: secret, LinkSigningKey: signingKey}, emailQueue: queue, baseURL: baseURL, identityTTL: identityTTL}
	router.Post("/files/:id/grants", middleware.AuthRequired, sharingResponse(h.CreateGrant))
	router.Patch("/files/:id/grants/:grantID", middleware.AuthRequired, sharingResponse(h.UpdateGrant))
	router.Delete("/files/:id/grants/:grantID", middleware.AuthRequired, sharingResponse(h.RevokeGrant))
	router.Post("/files/:id/invitations", middleware.AuthRequired, sharingResponse(h.CreateItemInvitation))
	router.Post("/clients/:id/invitations", middleware.AuthRequired, sharingResponse(h.CreateClientInvitation))
	router.Post("/invitations/:invitationID/email", middleware.AuthRequired, sharingResponse(h.EmailInvitation))
	router.Delete("/invitations/:invitationID", middleware.AuthRequired, sharingResponse(h.RevokeInvitation))
	router.Post("/invitations/accept", middleware.AuthRequired, sharingResponse(h.AcceptInvitation))
	router.Delete("/clients/:id/members/:userID", middleware.AuthRequired, sharingResponse(h.RemoveMember))
	router.Post("/files/:id/links", middleware.AuthRequired, sharingResponse(h.CreateItemLink))
	router.Delete("/clients/:id/links", middleware.AuthRequired, sharingResponse(h.RevokeClientLinks))
}

func sharingError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return fiber.ErrNotFound
	case errors.Is(err, services.ErrSharingConflict), errors.Is(err, services.ErrLinkRequestConflict), errors.Is(err, emailpkg.ErrIdempotencyConflict):
		return fiber.NewError(409, "Sharing settings changed. Refresh and try again.")
	case errors.Is(err, services.ErrSharingCapabilities):
		return fiber.NewError(403, "You can only share permissions you already have.")
	case errors.Is(err, services.ErrSharingInput):
		return fiber.NewError(400, "Invalid sharing settings")
	case errors.Is(err, services.ErrRoleInput):
		return fiber.NewError(400, "Choose an access role available to this person.")
	default:
		return fiber.NewError(503, "Sharing is temporarily unavailable. Please try again.")
	}
}

func sharingJSONError(c fiber.Ctx, err error) error {
	code, message := 503, "Sharing is temporarily unavailable. Please try again."
	var apiError *fiber.Error
	if errors.As(err, &apiError) {
		code, message = apiError.Code, apiError.Message
	}
	return c.Status(code).JSON(fiber.Map{"error": message})
}

func sharingResponse(handler fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		if err := handler(c); err != nil {
			return sharingJSONError(c, err)
		}
		return nil
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
		return c.Status(503).JSON(fiber.Map{"status": "unavailable", "error": "Email is unavailable. Ask your portal administrator to check email settings."})
	}
	delivery, err := h.service.PrepareInvitationDelivery(c.Context(), req, c.Params("invitationID"), h.identityTTL)
	if err != nil {
		return sharingError(err)
	}
	baseURL := strings.TrimRight(tenantBaseURLFromCtx(c, h.baseURL), "/")
	if baseURL == "" {
		return fiber.NewError(503, "The invitation link is unavailable. Ask your portal administrator for help.")
	}
	path := "/accept-access#token="
	if delivery.NeedsIdentity {
		path = "/accept-invite#token="
	}
	inviteURL := baseURL + path + url.QueryEscape(delivery.RawToken)
	subject, body, htmlBody := emailpkg.RenderUserInviteEmail(delivery.User.Name, inviteURL, services.DefaultSiteTitle, "You have been invited to shared files. Use this email address to accept the invitation.", delivery.ExpiresAt)
	item, err := h.emailQueue.Queue(emailpkg.Email{TenantID: req.TenantID, To: delivery.Invitation.Email, Subject: subject, Body: body, HTMLBody: htmlBody, ClientID: &delivery.Invitation.ClientID, AccessInvitationID: &delivery.Invitation.ID, IdentityInviteID: delivery.IdentityInviteID, DeduplicationKey: *key}, models.Now())
	if err != nil {
		code, status, message := 503, "failed", "The invitation email could not be sent. Please try again."
		if errors.Is(err, emailpkg.ErrAccessValidationUnavailable) {
			status, message = "unavailable", "Email is unavailable. Ask your portal administrator to check email settings."
		}
		if errors.Is(err, emailpkg.ErrIdempotencyConflict) {
			code = 409
		}
		return c.Status(code).JSON(fiber.Map{"status": status, "error": message})
	}
	if item == nil {
		return fiber.NewError(503, "The invitation email could not be sent. Please try again.")
	}
	if item.CanceledAt != nil {
		return c.Status(410).JSON(fiber.Map{"status": "unavailable", "error": "This invitation email was canceled."})
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
	redirect := "/shared-with-me"
	req.ClientID = invitation.ClientID
	if invitation.FileID != nil {
		req.FileID = *invitation.FileID
		if target, err := (&services.AccessService{DB: h.service.DB}).DescribeItem(c.Context(), req); err == nil {
			redirect = "/files/" + url.PathEscape(target.ID)
			if target.Type == "folder" {
				redirect = "/folder/" + url.PathEscape(target.ID)
			}
		}
	} else {
		decision, err := (&services.AccessService{DB: h.service.DB}).Resolve(c.Context(), req)
		if err != nil {
			return sharingError(err)
		}
		if decision.Allowed {
			client, err := (&services.ClientService{DB: h.service.DB}).GetClient(c.Context(), req.TenantID, req.ClientID)
			if err != nil {
				return sharingError(err)
			}
			if client.RootFolderID != nil {
				redirect = "/folder/" + url.PathEscape(*client.RootFolderID)
			}
		}
	}
	// Recipient results omit inviter/other-recipient provenance and secrets.
	return c.JSON(fiber.Map{"accepted": true, "file_id": invitation.FileID, "client_id": invitation.ClientID, "redirect_to": redirect})
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

func (h *SharingHandler) ClientAccess(c fiber.Ctx) error {
	view, err := h.service.ClientAccess(c.Context(), middleware.AccessRequest(c, c.Params("id"), "", services.AccessShare))
	if err != nil {
		return sharingError(err)
	}
	return c.JSON(view)
}

func (h *SharingHandler) RevokeClientLinks(c fiber.Ctx) error {
	count, err := h.service.RevokeClientLinks(c.Context(), middleware.AccessRequest(c, c.Params("id"), "", services.AccessShare))
	if err != nil {
		return sharingError(err)
	}
	return c.JSON(fiber.Map{"revoked": count})
}
