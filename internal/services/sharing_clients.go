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
	RoleID       *string                   `json:"role_id,omitempty"`
	UserType     string                    `json:"user_type"`
	Capabilities models.AccessCapabilities `json:"capabilities"`
	CanRemove    bool                      `json:"can_remove"`
}
type ClientAccess struct {
	AdditionalAccess []AdditionalAccess        `json:"additional_access"`
	ClientID         string                    `json:"client_id"`
	Capabilities     models.AccessCapabilities `json:"capabilities"`
	CanManage        bool                      `json:"can_manage"`
	People           []ClientMemberAccess      `json:"people"`
	Invitations      []models.AccessInvitation `json:"invitations"`
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
		canManageAccess := d.CanManageClient || (d.Capabilities.Sharing != nil && d.Capabilities.Permissions().CanManageAccess)
		view.Capabilities, view.CanManage = d.Capabilities, canManageAccess
		if !d.Capabilities.Permissions().CanInviteExternal && !d.Capabilities.Permissions().CanManageAccess {
			view.AdditionalAccess = []AdditionalAccess{}
			return nil
		}
		view.AdditionalAccess, err = additionalAccess(ctx, tx, req)
		if err != nil {
			return err
		}
		var actor models.User
		if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, req.Actor.UserID).First(&actor).Error; err != nil {
			return err
		}
		var memberships []models.UserClient
		q := tx.Where("tenant_id = ? AND client_id = ?", req.TenantID, req.ClientID)
		if !canManageAccess {
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
			canRemove := (canManageAccess || owned) && (actor.Role == "admin" || d.CanManageClient || user.Role == "client" || user.Role == "customer" || (canManageAccess && d.Capabilities.Sharing != nil && d.Capabilities.Permissions().CanManageAccess))
			caps, err := (&AccessService{DB: tx}).RoleCapabilities(ctx, req.TenantID, membership.RoleID, access.MembershipCapabilities(user.Role, membership), "workspace")
			if err != nil {
				return err
			}
			profile, err := AccessProfileFor(tx, user)
			if err != nil {
				return err
			}
			view.People = append(view.People, ClientMemberAccess{UserID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role, RoleID: membership.RoleID, UserType: profile.UserType, Capabilities: caps, CanRemove: canRemove})
		}
		invitations := invitationTargetQuery(tx, req)
		if !canManageAccess {
			invitations = invitations.Where("created_by = ?", actor.ID)
		}
		return invitations.Order("created_at ASC, id ASC").Find(&view.Invitations).Error
	})
	return view, err
}

func (s *SharingService) ChangeMemberRole(ctx context.Context, req AccessRequest, userID, roleID string, expectedRoleID ...string) error {
	if req.FileID != "" || roleID == "" || userID == "" {
		return ErrSharingInput
	}
	req.Operation = AccessManageOwn
	var previous models.UserClient
	if err := s.DB.WithContext(ctx).Where("tenant_id=? AND client_id=? AND user_id=?", req.TenantID, req.ClientID, userID).First(&previous).Error; err != nil {
		return err
	}
	caps, err := sharingRoleInput(ctx, s.DB, req, roleID, userID, "", previous.RoleID)
	if err != nil {
		return err
	}
	req.GrantRoleID = roleID
	req.PreserveArchivedRole = previous.RoleID != nil && *previous.RoleID == roleID
	return s.withAuthority(ctx, req, caps, func(tx *gorm.DB, _ *models.File) error {
		var member models.UserClient
		if err := tx.Where("tenant_id = ? AND client_id = ? AND user_id = ?", req.TenantID, req.ClientID, userID).First(&member).Error; err != nil {
			return err
		}
		current := ""
		if member.RoleID != nil {
			current = *member.RoleID
		}
		if len(expectedRoleID) > 0 && current != expectedRoleID[0] {
			return ErrSharingConflict
		}
		owned := member.AccessSource == "invitation" && member.InvitedBy != nil && *member.InvitedBy == req.Actor.UserID
		if !owned {
			req.Operation = AccessManageClient
			d, err := (&AccessService{DB: tx}).Resolve(ctx, req)
			if err != nil {
				return err
			}
			if !d.Allowed && !(d.Capabilities.Sharing != nil && d.Capabilities.Permissions().CanManageAccess) {
				return ErrSharingCapabilities
			}
		}
		before := member
		if err := tx.Model(&member).Updates(map[string]any{"role_id": roleID, "capabilities_set": true, "can_read": caps.CanRead, "can_upload": caps.CanUpload, "can_delete": caps.CanDelete, "can_share": caps.CanShare}).Error; err != nil {
			return err
		}
		return sharingAudit(tx, req, "membership.role.update", userID, before, map[string]string{"role_id": roleID})
	})
}
