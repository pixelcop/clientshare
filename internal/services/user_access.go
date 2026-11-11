package services

import (
	"context"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WorkspaceRoleInput struct {
	ClientID string `json:"client_id"`
	RoleID   string `json:"role_id"`
}

type UserAccessInput struct {
	UserType         string                `json:"user_type"`
	OrganizationRole string                `json:"organization_role"`
	AllWorkspaces    bool                  `json:"all_workspaces"`
	WorkspaceAccess  *[]WorkspaceRoleInput `json:"workspace_access"`
	GroupIDs         *[]string             `json:"group_ids"`
	CanManageUsers   *bool                 `json:"can_manage_users,omitempty"`
}

func (i UserAccessInput) CompatibilityRole() (string, error) {
	if i.UserType == "external" {
		if (i.OrganizationRole != "" && i.OrganizationRole != "none") || i.AllWorkspaces {
			return "", ErrRoleInput
		}
		return "client", nil
	}
	if i.UserType == "internal" {
		switch i.OrganizationRole {
		case "administrator":
			return "admin", nil
		case "member":
			return "manager", nil
		}
	}
	return "", ErrRoleInput
}

type UserAccessSettings struct {
	Profile    models.AccessProfile `json:"profile"`
	Workspaces []models.UserClient  `json:"workspaces"`
	GroupIDs   []string             `json:"group_ids"`
}

func UserAccessSettingsFor(ctx context.Context, db *gorm.DB, tenantID, userID string) (UserAccessSettings, error) {
	var user models.User
	settings := UserAccessSettings{Workspaces: []models.UserClient{}, GroupIDs: []string{}}
	if err := db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, userID).First(&user).Error; err != nil {
		return settings, err
	}
	var err error
	settings.Profile, err = AccessProfileFor(db.WithContext(ctx), user)
	if err != nil {
		return settings, err
	}
	if err := db.WithContext(ctx).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Order("client_id ASC").Find(&settings.Workspaces).Error; err != nil {
		return settings, err
	}
	if err := db.WithContext(ctx).Model(&models.AccessGroupMember{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Order("group_id ASC").Pluck("group_id", &settings.GroupIDs).Error; err != nil {
		return settings, err
	}
	return settings, nil
}

// ApplyUserAccess is called inside the identity-creation/edit transaction. It
// never infers document access from Internal, Member or account creation alone.
func ApplyUserAccess(ctx context.Context, tx *gorm.DB, tenantID, actorID string, user models.User, input UserAccessInput) error {
	compatibility, err := input.CompatibilityRole()
	if err != nil {
		return err
	}
	if compatibility != user.Role {
		return ErrRoleInput
	}
	var actor models.User
	if err := tx.Where("tenant_id = ? AND id = ?", tenantID, actorID).First(&actor).Error; err != nil {
		return err
	}
	if actor.Role != "admin" && (input.UserType != "external" || input.AllWorkspaces || input.GroupIDs != nil) {
		return ErrRoleForbidden
	}
	groupIDs := []string{}
	if input.GroupIDs != nil {
		groupIDs = *input.GroupIDs
	} else {
		if err := tx.Model(&models.AccessGroupMember{}).Where("tenant_id = ? AND user_id = ?", tenantID, user.ID).Pluck("group_id", &groupIDs).Error; err != nil {
			return err
		}
	}
	for _, groupID := range groupIDs {
		if err := ValidateGroupMembership(tx, tenantID, groupID, input.UserType); err != nil {
			return err
		}
	}
	if input.WorkspaceAccess == nil {
		var currentUser models.User
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, user.ID).First(&currentUser).Error; err != nil {
			return err
		}
		var memberships []models.UserClient
		if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, user.ID).Find(&memberships).Error; err != nil {
			return err
		}
		for _, member := range memberships {
			if member.RoleID != nil {
				var role models.AccessRole
				if err := tx.Where("tenant_id = ? AND id = ?", tenantID, *member.RoleID).First(&role).Error; err != nil {
					return err
				}
				if !role.Supports(input.UserType, "workspace") {
					return ErrRoleInput
				}
			} else if LegacyAccessProfile(currentUser).UserType != input.UserType && (!member.CapabilitiesSet || member.CanShare) {
				return ErrRoleInput
			}
		}
	}
	// Reclassification must not leave internal-only direct grants on an external
	// identity (or vice versa). Historical, revoked grants are audit only.
	var grants []models.ItemGrant
	if err := tx.Where("tenant_id = ? AND user_id = ? AND role_id IS NOT NULL AND revoked_at IS NULL", tenantID, user.ID).Find(&grants).Error; err != nil {
		return err
	}
	for _, grant := range grants {
		var role models.AccessRole
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, *grant.RoleID).First(&role).Error; err != nil {
			return err
		}
		if role.Audience != "both" && role.Audience != input.UserType {
			return ErrRoleInput
		}
	}
	profile := models.AccessProfile{TenantID: tenantID, UserID: user.ID, UserType: input.UserType, OrganizationRole: input.OrganizationRole, AllWorkspaces: input.AllWorkspaces}
	if input.UserType == "external" {
		profile.OrganizationRole = "none"
	}
	if input.OrganizationRole == "administrator" {
		profile.CanManageUsers, profile.AllWorkspaces = true, true
	}
	var previous models.AccessProfile
	if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, user.ID).First(&previous).Error; err == nil && input.UserType == "internal" && previous.UserType == "internal" && previous.OrganizationRole == "member" {
		// Preserve migrated managers' explicit legacy user-management entitlement.
		profile.CanManageUsers = profile.CanManageUsers || previous.CanManageUsers
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if input.CanManageUsers != nil {
		if actor.Role != "admin" {
			return ErrRoleForbidden
		}
		if *input.CanManageUsers && input.OrganizationRole != "administrator" && (!previous.CanManageUsers || previous.OrganizationRole != "member") {
			return ErrRoleInput
		}
		if input.UserType == "external" && *input.CanManageUsers {
			return ErrRoleInput
		}
		profile.CanManageUsers = *input.CanManageUsers || input.OrganizationRole == "administrator"
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"user_type", "organization_role", "can_manage_users", "all_workspaces"})}).Create(&profile).Error; err != nil {
		return err
	}
	if input.WorkspaceAccess != nil {
		seen := map[string]bool{}
		for _, assignment := range *input.WorkspaceAccess {
			if assignment.ClientID == "" || assignment.RoleID == "" || seen[assignment.ClientID] {
				return ErrRoleInput
			}
			seen[assignment.ClientID] = true
			var existingMember models.UserClient
			load := tx.Where("tenant_id = ? AND user_id = ? AND client_id = ?", tenantID, user.ID, assignment.ClientID).First(&existingMember)
			if load.Error != nil && load.Error != gorm.ErrRecordNotFound {
				return load.Error
			}
			preserveArchived := load.Error == nil && existingMember.RoleID != nil && *existingMember.RoleID == assignment.RoleID
			_, caps, err := AssignmentRole(tx, tenantID, assignment.RoleID, input.UserType, "workspace", preserveArchived)
			if err != nil {
				return err
			}
			d, err := (&AccessService{DB: tx}).Resolve(ctx, AccessRequest{TenantID: tenantID, ClientID: assignment.ClientID, Actor: AccessActor{UserID: actorID}, Operation: AccessInviteClient})
			if err != nil {
				return err
			}
			if !d.CanGrant(caps) {
				return ErrSharingCapabilities
			}
			member := models.UserClient{TenantID: tenantID, UserID: user.ID, ClientID: assignment.ClientID, RoleID: &assignment.RoleID, CapabilitiesSet: true, AccessCapabilities: caps, AccessSource: "independent"}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "client_id"}}, DoUpdates: clause.AssignmentColumns([]string{"role_id", "capabilities_set", "can_read", "can_upload", "can_delete", "can_share", "access_source"})}).Create(&member).Error; err != nil {
				return err
			}
		}
		ids := []string{}
		for id := range seen {
			ids = append(ids, id)
		}
		q := tx.Where("tenant_id = ? AND user_id = ?", tenantID, user.ID)
		if len(ids) > 0 {
			q = q.Where("client_id NOT IN ?", ids)
		}
		if err := q.Delete(&models.UserClient{}).Error; err != nil {
			return err
		}
	}
	if input.GroupIDs != nil {
		if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, user.ID).Delete(&models.AccessGroupMember{}).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, groupID := range *input.GroupIDs {
			if seen[groupID] {
				continue
			}
			seen[groupID] = true
			var group models.AccessGroup
			if err := tx.Where("tenant_id = ? AND id = ?", tenantID, groupID).First(&group).Error; err != nil {
				return err
			}
			if err := ValidateGroupMembership(tx, tenantID, groupID, input.UserType); err != nil {
				return err
			}
			if err := tx.Create(&models.AccessGroupMember{TenantID: tenantID, GroupID: groupID, UserID: user.ID}).Error; err != nil {
				return err
			}
		}
	}
	return sharingAudit(tx, AccessRequest{TenantID: tenantID, Actor: AccessActor{UserID: actorID}}, "user.access.update", user.ID, nil, input)
}
