package services

import (
	"context"

	"github.com/pixelcop/clientshare/internal/access"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

type ClientMemberAccess struct {
	UserID       string                    `json:"user_id"`
	Name         string                    `json:"name"`
	Email        string                    `json:"email"`
	Role         string                    `json:"role"`
	Capabilities models.AccessCapabilities `json:"capabilities"`
	CanRemove    bool                      `json:"can_remove"`
}
type ClientAccess struct {
	ClientID     string                    `json:"client_id"`
	Capabilities models.AccessCapabilities `json:"capabilities"`
	CanManage    bool                      `json:"can_manage"`
	People       []ClientMemberAccess      `json:"people"`
	Invitations  []models.AccessInvitation `json:"invitations"`
}

// ClientAccess never turns an item grant into a roster permission. Non-staff
// inviters see only themselves and invitees whose access they actually own.
func (s *SharingService) ClientAccess(ctx context.Context, req AccessRequest) (*ClientAccess, error) {
	if req.FileID != "" {
		return nil, ErrSharingInput
	}
	view := &ClientAccess{ClientID: req.ClientID, People: []ClientMemberAccess{}, Invitations: []models.AccessInvitation{}}
	err := s.withAuthority(ctx, req, models.AccessCapabilities{}, func(tx *gorm.DB, _ *models.File) error {
		req.Operation, req.Now = AccessShare, s.now()
		d, err := (&AccessService{DB: tx}).Resolve(ctx, req)
		if err != nil {
			return err
		}
		if !d.Allowed {
			return gorm.ErrRecordNotFound
		}
		view.Capabilities, view.CanManage = d.Capabilities, d.CanManageClient
		var actor models.User
		if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, req.Actor.UserID).First(&actor).Error; err != nil {
			return err
		}
		var memberships []models.UserClient
		q := tx.Where("tenant_id = ? AND client_id = ?", req.TenantID, req.ClientID)
		if !d.CanManageClient {
			q = q.Where("(user_id = ? OR (access_source = 'invitation' AND invited_by = ?))", req.Actor.UserID, req.Actor.UserID)
		}
		if err := q.Order("created_at ASC, user_id ASC").Find(&memberships).Error; err != nil {
			return err
		}
		ids := make([]string, len(memberships))
		for i := range memberships {
			ids[i] = memberships[i].UserID
		}
		users := map[string]models.User{}
		for start := 0; start < len(ids); start += maxAccessItems {
			var batch []models.User
			if err := tx.Where("tenant_id = ? AND id IN ?", req.TenantID, ids[start:min(start+maxAccessItems, len(ids))]).Find(&batch).Error; err != nil {
				return err
			}
			for _, user := range batch {
				users[user.ID] = user
			}
		}
		for _, membership := range memberships {
			user, ok := users[membership.UserID]
			if !ok {
				continue
			}
			owned := membership.AccessSource == "invitation" && membership.InvitedBy != nil && *membership.InvitedBy == actor.ID
			canRemove := (d.CanManageClient || owned) && (actor.Role == "admin" || user.Role == "client" || user.Role == "customer")
			view.People = append(view.People, ClientMemberAccess{UserID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role, Capabilities: access.MembershipCapabilities(user.Role, membership), CanRemove: canRemove})
		}
		invitations := invitationTargetQuery(tx, req)
		if !d.CanManageClient {
			invitations = invitations.Where("created_by = ?", actor.ID)
		}
		return invitations.Order("created_at ASC, id ASC").Find(&view.Invitations).Error
	})
	return view, err
}
