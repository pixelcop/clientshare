package services

import (
	"context"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

type GrantAccess struct {
	models.ItemGrant
	RecipientName     string `json:"recipient_name"`
	RecipientEmail    string `json:"recipient_email"`
	RecipientUserType string `json:"recipient_user_type"`
	CanManage         bool   `json:"can_manage"`
}

type ItemAccess struct {
	ClientID             string                    `json:"client_id"`
	AdditionalAccess     []AdditionalAccess        `json:"additional_access"`
	FileID               string                    `json:"file_id"`
	Capabilities         models.AccessCapabilities `json:"capabilities"`
	InheritsClientAccess bool                      `json:"inherits_client_access"`
	InheritsFolderAccess bool                      `json:"inherits_folder_access"`
	Grants               []GrantAccess             `json:"grants"`
	Invitations          []models.AccessInvitation `json:"invitations"`
	Links                []models.SecureLink       `json:"links"`
}

func linkCapabilities(mode string) (models.AccessCapabilities, bool) {
	switch mode {
	case "read", "view":
		return models.AccessCapabilities{CanRead: true}, true
	case "write":
		return models.AccessCapabilities{CanUpload: true}, true
	case "readwrite":
		return models.AccessCapabilities{CanRead: true, CanUpload: true}, true
	default:
		return models.AccessCapabilities{}, false
	}
}

// ItemAccess is an exact-scope management view, never a client roster. Inherited
// authority is explained without exposing the private ancestor's other people
// or bearer credentials. Link tokens above the actor's ceiling are withheld.
func (s *SharingService) ItemAccess(ctx context.Context, req AccessRequest) (*ItemAccess, error) {
	if req.FileID == "" {
		return nil, ErrSharingInput
	}
	view := &ItemAccess{ClientID: req.ClientID, FileID: req.FileID, Grants: []GrantAccess{}, Invitations: []models.AccessInvitation{}, Links: []models.SecureLink{}}
	err := s.withAuthority(ctx, req, models.AccessCapabilities{}, func(tx *gorm.DB, _ *models.File) error {
		req.Operation, req.Now = AccessShare, s.now()
		d, err := (&AccessService{DB: tx}).Resolve(ctx, req)
		if err != nil {
			return err
		}
		if !d.Allowed {
			return gorm.ErrRecordNotFound
		}
		view.Capabilities = d.Capabilities
		view.AdditionalAccess = []AdditionalAccess{}
		for _, source := range d.Sources {
			if source.Kind == "membership" || source.Kind == "staff" || ((source.Kind == "group" || source.Kind == "role_assignment") && source.FileID == "") {
				view.InheritsClientAccess = true
			}
			if (source.Kind == "grant" || source.Kind == "group" || source.Kind == "role_assignment") && source.FileID != "" && source.FileID != req.FileID {
				view.InheritsFolderAccess = true
			}
		}
		p := d.Capabilities.Permissions()
		if p.CanInviteExternal || p.CanManageAccess {
			view.AdditionalAccess, err = additionalAccess(ctx, tx, req)
			if err != nil {
				return err
			}
			grants := tx.Table("item_grants AS g").Select("g.*, recipient.name AS recipient_name, recipient.email AS recipient_email").
				Joins("JOIN users recipient ON recipient.tenant_id = g.tenant_id AND recipient.id = g.user_id").
				Where("g.tenant_id = ? AND g.client_id = ? AND g.file_id = ?", req.TenantID, req.ClientID, req.FileID)
			if d.Capabilities.Sharing != nil && !p.CanManageAccess {
				grants = grants.Where("g.created_by = ? OR g.user_id = ?", req.Actor.UserID, req.Actor.UserID)
			}
			if err := grants.Order("g.created_at ASC, g.id ASC").Scan(&view.Grants).Error; err != nil {
				return err
			}
			for i := range view.Grants {
				grant := &view.Grants[i]
				var user models.User
				if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, grant.UserID).First(&user).Error; err != nil {
					return err
				}
				profile, err := AccessProfileFor(tx, user)
				if err != nil {
					return err
				}
				grant.RecipientUserType = profile.UserType
				grant.CanManage = d.Capabilities.Permissions().CanManageAccess || grant.CreatedBy == req.Actor.UserID
			}
			invitations := invitationTargetQuery(tx, req)
			if d.Capabilities.Sharing != nil && !p.CanManageAccess {
				invitations = invitations.Where("created_by = ?", req.Actor.UserID)
			}
			if err := invitations.Order("created_at ASC, id ASC").Find(&view.Invitations).Error; err != nil {
				return err
			}
		}
		if p.CanCreateLinks {
			if err := tx.Where("tenant_id = ? AND client_id = ? AND file_id = ?", req.TenantID, req.ClientID, req.FileID).Order("created_at ASC, id ASC").Find(&view.Links).Error; err != nil {
				return err
			}
			for i := range view.Links {
				link := &view.Links[i]
				caps, valid := linkCapabilities(link.AccessType)
				if !valid || !d.Capabilities.Permissions().CanCreateLinks || !d.CanGrant(caps) || link.RevokedAt != nil || !link.ExpiresAt.After(s.now()) {
					link.Token = ""
				}
			}
		}
		return nil
	})
	return view, err
}
