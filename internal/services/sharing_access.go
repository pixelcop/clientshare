package services

import (
	"context"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

type GrantAccess struct {
	models.ItemGrant
	RecipientName  string `json:"recipient_name"`
	RecipientEmail string `json:"recipient_email"`
}

type ItemAccess struct {
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
	view := &ItemAccess{FileID: req.FileID, Grants: []GrantAccess{}, Invitations: []models.AccessInvitation{}, Links: []models.SecureLink{}}
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
		for _, source := range d.Sources {
			if source.Kind == "membership" || source.Kind == "staff" {
				view.InheritsClientAccess = true
			}
			if source.Kind == "grant" && source.FileID != req.FileID {
				view.InheritsFolderAccess = true
			}
		}
		if err := tx.Table("item_grants AS g").Select("g.*, recipient.name AS recipient_name, recipient.email AS recipient_email").
			Joins("JOIN users recipient ON recipient.tenant_id = g.tenant_id AND recipient.id = g.user_id").
			Where("g.tenant_id = ? AND g.client_id = ? AND g.file_id = ?", req.TenantID, req.ClientID, req.FileID).
			Order("g.created_at ASC, g.id ASC").Scan(&view.Grants).Error; err != nil {
			return err
		}
		if err := invitationTargetQuery(tx, req).Order("created_at ASC, id ASC").Find(&view.Invitations).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND client_id = ? AND file_id = ?", req.TenantID, req.ClientID, req.FileID).Order("created_at ASC, id ASC").Find(&view.Links).Error; err != nil {
			return err
		}
		for i := range view.Links {
			link := &view.Links[i]
			caps, valid := linkCapabilities(link.AccessType)
			if !valid || !d.CanGrant(caps) || link.RevokedAt != nil || !link.ExpiresAt.After(s.now()) {
				link.Token = ""
			}
		}
		return nil
	})
	return view, err
}
