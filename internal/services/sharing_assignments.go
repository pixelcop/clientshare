package services

import (
	"context"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

type AdditionalAccess struct {
	models.RoleAssignment
	RecipientName     string `json:"recipient_name"`
	RecipientEmail    string `json:"recipient_email"`
	RecipientUserType string `json:"recipient_user_type"`
	GroupName         string `json:"group_name"`
	CanManage         bool   `json:"can_manage"`
}

func additionalAccess(ctx context.Context, tx *gorm.DB, req AccessRequest) ([]AdditionalAccess, error) {
	rows := []models.RoleAssignment{}
	q := invitationTargetQuery(tx, req).Where("source_kind NOT IN ('membership','grant')")
	{
		r := req
		r.Operation = AccessShare
		d, err := (&AccessService{DB: tx}).Resolve(ctx, r)
		if err != nil {
			return nil, err
		}
		limited := (req.FileID == "" && !d.CanManageClient && !(d.Capabilities.Sharing != nil && d.Capabilities.Permissions().CanManageAccess)) || (d.Capabilities.Sharing != nil && !d.Capabilities.Permissions().CanManageAccess)
		if limited {
			q = q.Where("user_id = ? OR group_id IN (SELECT group_id FROM access_group_members WHERE tenant_id = ? AND user_id = ?) OR (source_kind = 'invitation' AND source_id IN (SELECT id FROM access_invitations WHERE tenant_id = ? AND created_by = ?))", req.Actor.UserID, req.TenantID, req.Actor.UserID, req.TenantID, req.Actor.UserID)
		}
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := []AdditionalAccess{}
	for _, row := range rows {
		item := AdditionalAccess{RoleAssignment: row}
		if row.UserID != nil {
			var user models.User
			if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, *row.UserID).First(&user).Error; err != nil {
				return nil, err
			}
			p, err := AccessProfileFor(tx, user)
			if err != nil {
				return nil, err
			}
			item.RecipientName, item.RecipientEmail, item.RecipientUserType = user.Name, user.Email, p.UserType
			owner := ""
			if row.SourceKind == "invitation" && row.SourceID != nil {
				var invite models.AccessInvitation
				if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, *row.SourceID).First(&invite).Error; err != nil {
					return nil, err
				}
				owner = invite.CreatedBy
			}
			item.CanManage = requireOwnedSharing(ctx, tx, req, owner) == nil
		}
		if row.GroupID != nil {
			var group models.AccessGroup
			if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, *row.GroupID).First(&group).Error; err != nil {
				return nil, err
			}
			item.GroupName = group.Name
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *SharingService) UpdateRoleAssignment(ctx context.Context, tenantID, actorID, id string, version uint64, roleID string, expiry *time.Time, revoke bool) error {
	var assignment models.RoleAssignment
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ? AND user_id IS NOT NULL AND group_id IS NULL", tenantID, id).First(&assignment).Error; err != nil {
		return err
	}
	if version == 0 || (!revoke && (roleID == "" || (expiry != nil && !expiry.After(s.now())))) {
		return ErrSharingInput
	}
	req := AccessRequest{TenantID: tenantID, ClientID: assignment.ClientID, Actor: AccessActor{UserID: actorID}, Operation: AccessManageOwn}
	if assignment.FileID != nil {
		req.FileID = *assignment.FileID
	}
	caps := models.AccessCapabilities{}
	if !revoke {
		var err error
		caps, err = sharingRoleInput(ctx, s.DB, req, roleID, *assignment.UserID, "", &assignment.RoleID)
		if err != nil {
			return err
		}
		req.GrantRoleID = roleID
		req.PreserveArchivedRole = assignment.RoleID == roleID
	}
	return s.withAuthority(ctx, req, caps, func(tx *gorm.DB, _ *models.File) error {
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).First(&assignment).Error; err != nil {
			return err
		}
		if assignment.Version != version {
			return ErrSharingConflict
		}
		owner := ""
		if assignment.SourceKind == "invitation" && assignment.SourceID != nil {
			var invite models.AccessInvitation
			if err := tx.Where("tenant_id = ? AND id = ?", tenantID, *assignment.SourceID).First(&invite).Error; err != nil {
				return err
			}
			owner = invite.CreatedBy
		}
		if err := requireOwnedSharing(ctx, tx, req, owner); err != nil {
			return err
		}
		before := assignment
		updates := map[string]any{"version": version + 1, "updated_at": s.now()}
		if revoke {
			updates["revoked_at"] = s.now()
		} else {
			updates["role_id"] = roleID
			updates["expires_at"] = expiry
			updates["revoked_at"] = nil
		}
		updated := tx.Model(&models.RoleAssignment{}).Where("tenant_id = ? AND id = ? AND version = ?", tenantID, id, version).Updates(updates)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrSharingConflict
		}
		return sharingAudit(tx, req, "role_assignment.update", id, before, updates)
	})
}
