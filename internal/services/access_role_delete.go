package services

import (
	"context"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *AccessRoleService) DeleteRole(ctx context.Context, tenantID, actorID, id, replacementID string, version uint64, fingerprint string, replacementVersion ...uint64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.administrator(tx, tenantID, actorID); err != nil {
			return err
		}
		var role models.AccessRole
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id).First(&role).Error; err != nil {
			return err
		}
		impact, err := roleImpact(tx, tenantID, id)
		if err != nil {
			return err
		}
		if version != role.Version || fingerprint != impact.Fingerprint {
			return ErrRoleConflict
		}
		inUse := impact.Memberships+impact.Grants+impact.Invitations+impact.GroupAssignments > 0
		if replacementID == id || (inUse && replacementID == "") {
			return ErrRoleInput
		}
		if replacementID != "" {
			var replacement models.AccessRole
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ? AND archived_at IS NULL", tenantID, replacementID).First(&replacement).Error; err != nil {
				return err
			}
			if len(replacementVersion) > 0 && replacement.Version != replacementVersion[0] {
				return ErrRoleConflict
			}
			check := func(userID string, fileID *string) error {
				scope := "workspace"
				if fileID != nil {
					var file models.File
					if err := tx.Where("tenant_id = ? AND id = ?", tenantID, *fileID).First(&file).Error; err != nil {
						return err
					}
					scope = file.Type
				}
				if _, valid := replacement.PermissionsAt(scope); !valid {
					return ErrRoleInput
				}
				if userID != "" {
					var user models.User
					if err := tx.Where("tenant_id = ? AND id = ?", tenantID, userID).First(&user).Error; err != nil {
						return err
					}
					p, err := AccessProfileFor(tx, user)
					if err != nil {
						return err
					}
					if !replacement.AppliesTo(p.UserType, scope) {
						return ErrRoleInput
					}
				}
				return nil
			}
			var memberships []models.UserClient
			if err := tx.Where("tenant_id = ? AND role_id = ?", tenantID, id).Find(&memberships).Error; err != nil {
				return err
			}
			for _, m := range memberships {
				if err := check(m.UserID, nil); err != nil {
					return err
				}
			}
			var grants []models.ItemGrant
			if err := tx.Where("tenant_id = ? AND role_id = ?", tenantID, id).Find(&grants).Error; err != nil {
				return err
			}
			for _, g := range grants {
				if err := check(g.UserID, &g.FileID); err != nil {
					return err
				}
			}
			var invitations []models.AccessInvitation
			if err := tx.Where("tenant_id = ? AND role_id = ?", tenantID, id).Find(&invitations).Error; err != nil {
				return err
			}
			for _, i := range invitations {
				uid := ""
				if i.RecipientUserID != nil {
					uid = *i.RecipientUserID
				}
				if err := check(uid, i.FileID); err != nil {
					return err
				}
				if uid == "" && replacement.Audience == "internal" {
					return ErrRoleInput
				}
			}
			var assignments []models.RoleAssignment
			if err := tx.Where("tenant_id = ? AND role_id = ?", tenantID, id).Find(&assignments).Error; err != nil {
				return err
			}
			for _, a := range assignments {
				if a.UserID != nil {
					if err := check(*a.UserID, a.FileID); err != nil {
						return err
					}
				}
				if a.GroupID != nil {
					var members []models.AccessGroupMember
					if err := tx.Where("tenant_id = ? AND group_id = ?", tenantID, *a.GroupID).Find(&members).Error; err != nil {
						return err
					}
					if err := check("", a.FileID); err != nil {
						return err
					}
					for _, m := range members {
						if err := check(m.UserID, a.FileID); err != nil {
							return err
						}
					}
				}
			}
			for _, model := range []any{&models.UserClient{}, &models.ItemGrant{}, &models.AccessInvitation{}, &models.RoleAssignment{}} {
				if err := tx.Model(model).Where("tenant_id = ? AND role_id = ?", tenantID, id).Update("role_id", replacementID).Error; err != nil {
					return err
				}
			}
			if err := refreshRoleSnapshots(tx, tenantID, replacementID, replacement.RolePermissions); err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.AccessRole{}).Error; err != nil {
			return err
		}
		return sharingAudit(tx, AccessRequest{TenantID: tenantID, Actor: AccessActor{UserID: actorID}}, "role.delete", id, role, map[string]string{"replacement_id": replacementID})
	})
}
