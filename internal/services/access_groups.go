package services

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GroupAssignmentInput struct {
	ClientID string  `json:"client_id"`
	FileID   *string `json:"file_id,omitempty"`
	RoleID   string  `json:"role_id"`
}
type GroupInput struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	MemberIDs   []string               `json:"member_ids"`
	Assignments []GroupAssignmentInput `json:"assignments"`
	Version     uint64                 `json:"version"`
	Impact      string                 `json:"impact"`
}
type GroupDetails struct {
	models.AccessGroup
	MemberIDs   []string                `json:"member_ids"`
	Assignments []models.RoleAssignment `json:"assignments"`
}
type GroupImpact struct {
	Members             int    `json:"members"`
	Assignments         int    `json:"assignments"`
	PreviousMembers     int    `json:"previous_members"`
	PreviousAssignments int    `json:"previous_assignments"`
	Fingerprint         string `json:"fingerprint"`
}

func groupDetails(tx *gorm.DB, tenantID, id string) (GroupDetails, error) {
	d := GroupDetails{MemberIDs: []string{}, Assignments: []models.RoleAssignment{}}
	if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).First(&d.AccessGroup).Error; err != nil {
		return d, err
	}
	if err := tx.Model(&models.AccessGroupMember{}).Where("tenant_id = ? AND group_id = ?", tenantID, id).Order("user_id ASC").Pluck("user_id", &d.MemberIDs).Error; err != nil {
		return d, err
	}
	err := tx.Where("tenant_id = ? AND group_id = ? AND revoked_at IS NULL", tenantID, id).Order("id ASC").Find(&d.Assignments).Error
	if err != nil {
		return d, err
	}
	for i := range d.Assignments {
		assignment := &d.Assignments[i]
		if assignment.FileID != nil {
			var file models.File
			err := tx.Select("filename,type").Where("tenant_id = ? AND client_id = ? AND id = ?", tenantID, assignment.ClientID, *assignment.FileID).First(&file).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return d, err
			}
			assignment.ScopeName = file.Filename
			assignment.ScopeType = file.Type
			if assignment.ScopeName == "" {
				assignment.ScopeName = "Shared item"
			}
		} else {
			assignment.ScopeType = "workspace"
			var client models.Client
			err := tx.Select("name").Where("tenant_id = ? AND id = ?", tenantID, assignment.ClientID).First(&client).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return d, err
			}
			assignment.ScopeName = client.Name
			if assignment.ScopeName == "" {
				assignment.ScopeName = "Client workspace"
			}
		}
	}
	return d, err
}

func (s *AccessRoleService) Groups(ctx context.Context, tenantID, actorID string) ([]GroupDetails, error) {
	if err := s.administrator(s.DB.WithContext(ctx), tenantID, actorID); err != nil {
		return nil, err
	}
	var groups []models.AccessGroup
	if err := s.DB.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("name ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	result := []GroupDetails{}
	for _, g := range groups {
		d, err := groupDetails(s.DB.WithContext(ctx), tenantID, g.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, nil
}

func validateGroupInput(ctx context.Context, tx *gorm.DB, tenantID, existingGroupID string, input *GroupInput) error {
	input.Name, input.Description = strings.TrimSpace(input.Name), strings.TrimSpace(input.Description)
	if input.Name == "" || len(input.Name) > 100 || len(input.Description) > 500 || len(input.MemberIDs) > 1000 || len(input.Assignments) > 1000 {
		return ErrRoleInput
	}
	seen := map[string]bool{}
	types := []string{}
	for _, id := range input.MemberIDs {
		if seen[id] {
			return ErrRoleInput
		}
		seen[id] = true
		var user models.User
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).First(&user).Error; err != nil {
			return err
		}
		p, err := AccessProfileFor(tx, user)
		if err != nil {
			return err
		}
		types = append(types, p.UserType)
	}
	scopes := map[string]bool{}
	for _, a := range input.Assignments {
		if a.ClientID == "" || a.RoleID == "" {
			return ErrRoleInput
		}
		var client models.Client
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, a.ClientID).First(&client).Error; err != nil {
			return err
		}
		scope, key := "workspace", a.ClientID
		if a.FileID != nil {
			file, err := (&AccessService{DB: tx}).ValidateTarget(ctx, tenantID, a.ClientID, *a.FileID)
			if err != nil {
				return err
			}
			scope = file.Type
			key += "/" + *a.FileID
		}
		if scopes[key] {
			return ErrRoleInput
		}
		scopes[key] = true
		var role models.AccessRole
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, a.RoleID).First(&role).Error; err != nil {
			return err
		}
		if role.ArchivedAt != nil {
			if existingGroupID == "" {
				return ErrRoleInput
			}
			q := tx.Model(&models.RoleAssignment{}).Where("tenant_id = ? AND group_id = ? AND role_id = ? AND client_id = ? AND revoked_at IS NULL", tenantID, existingGroupID, role.ID, a.ClientID)
			if a.FileID == nil {
				q = q.Where("file_id IS NULL")
			} else {
				q = q.Where("file_id = ?", *a.FileID)
			}
			var count int64
			if err := q.Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrRoleInput
			}
		}
		if _, ok := role.PermissionsAt(scope); !ok {
			return ErrRoleInput
		}
		for _, kind := range types {
			if !role.Supports(kind, scope) {
				return ErrRoleInput
			}
		}
	}
	sort.Strings(input.MemberIDs)
	return nil
}

func groupImpact(ctx context.Context, tx *gorm.DB, tenantID, id string, input GroupInput) (GroupImpact, error) {
	i := GroupImpact{Members: len(input.MemberIDs), Assignments: len(input.Assignments)}
	if err := validateGroupInput(ctx, tx, tenantID, id, &input); err != nil {
		return i, err
	}
	var before GroupDetails
	if id != "" {
		var err error
		before, err = groupDetails(tx, tenantID, id)
		if err != nil {
			return i, err
		}
		if before.Version != input.Version {
			return i, ErrRoleConflict
		}
	}
	i.PreviousMembers, i.PreviousAssignments = len(before.MemberIDs), len(before.Assignments)
	versions := map[string]uint64{}
	for _, a := range input.Assignments {
		var r models.AccessRole
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, a.RoleID).First(&r).Error; err != nil {
			return i, err
		}
		versions[r.ID] = r.Version
	}
	input.Impact = ""
	i.Fingerprint = sharingDigest(struct {
		Before GroupDetails
		After  GroupInput
		Roles  map[string]uint64
	}{before, input, versions})
	return i, nil
}

func (s *AccessRoleService) PreviewGroup(ctx context.Context, tenantID, actorID, id string, input GroupInput) (GroupImpact, error) {
	if err := s.administrator(s.DB.WithContext(ctx), tenantID, actorID); err != nil {
		return GroupImpact{}, err
	}
	return groupImpact(ctx, s.DB.WithContext(ctx), tenantID, id, input)
}

func (s *AccessRoleService) SaveGroup(ctx context.Context, tenantID, actorID, id string, input GroupInput) (*GroupDetails, error) {
	var result GroupDetails
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.administrator(tx, tenantID, actorID); err != nil {
			return err
		}
		if id != "" {
			var group models.AccessGroup
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id).First(&group).Error; err != nil {
				return err
			}
		}
		impact, err := groupImpact(ctx, tx, tenantID, id, input)
		if err != nil {
			return err
		}
		if input.Impact == "" || input.Impact != impact.Fingerprint {
			return ErrRoleConflict
		}
		var before GroupDetails
		if id == "" {
			g := models.AccessGroup{TenantID: tenantID, Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), Version: 1}
			if err := tx.Create(&g).Error; err != nil {
				return err
			}
			id = g.ID
		} else {
			before, err = groupDetails(tx, tenantID, id)
			if err != nil {
				return err
			}
			if err := tx.Model(&models.AccessGroup{}).Where("tenant_id = ? AND id = ? AND version = ?", tenantID, id, input.Version).Updates(map[string]any{"name": strings.TrimSpace(input.Name), "description": strings.TrimSpace(input.Description), "version": input.Version + 1, "updated_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_id = ? AND group_id = ?", tenantID, id).Delete(&models.AccessGroupMember{}).Error; err != nil {
			return err
		}
		for _, userID := range input.MemberIDs {
			if err := tx.Create(&models.AccessGroupMember{TenantID: tenantID, GroupID: id, UserID: userID}).Error; err != nil {
				return err
			}
		}
		// Replacing the group's grants is atomic. Direct/inherited access from
		// other sources is untouched; audit retains the previous assignments.
		if err := tx.Where("tenant_id = ? AND group_id = ?", tenantID, id).Delete(&models.RoleAssignment{}).Error; err != nil {
			return err
		}
		for _, a := range input.Assignments {
			if err := tx.Create(&models.RoleAssignment{TenantID: tenantID, GroupID: &id, ClientID: a.ClientID, FileID: a.FileID, RoleID: a.RoleID, SourceKind: "group", Version: 1}).Error; err != nil {
				return err
			}
		}
		result, err = groupDetails(tx, tenantID, id)
		if err != nil {
			return err
		}
		return sharingAudit(tx, AccessRequest{TenantID: tenantID, Actor: AccessActor{UserID: actorID}}, "group.save", id, before, result)
	})
	return &result, err
}

func (s *AccessRoleService) DeleteGroup(ctx context.Context, tenantID, actorID, id string, input GroupInput) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.administrator(tx, tenantID, actorID); err != nil {
			return err
		}
		before, err := groupDetails(tx, tenantID, id)
		if err != nil {
			return err
		}
		if before.Version != input.Version {
			return ErrRoleConflict
		}
		// Deletion uses the same preview with an empty resulting membership and
		// grant set, so people see the access that will be removed first.
		input.Name, input.MemberIDs, input.Assignments = before.Name, []string{}, []GroupAssignmentInput{}
		impact, err := groupImpact(ctx, tx, tenantID, id, input)
		if err != nil {
			return err
		}
		if input.Impact != impact.Fingerprint {
			return ErrRoleConflict
		}
		for _, model := range []any{&models.AccessGroupMember{}, &models.RoleAssignment{}} {
			if err := tx.Where("tenant_id = ? AND group_id = ?", tenantID, id).Delete(model).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.AccessGroup{}).Error; err != nil {
			return err
		}
		return sharingAudit(tx, AccessRequest{TenantID: tenantID, Actor: AccessActor{UserID: actorID}}, "group.delete", id, before, nil)
	})
}

// ValidateGroupMembership prevents changing someone's type or group membership
// into an audience-ineligible role through the user form.
func ValidateGroupMembership(tx *gorm.DB, tenantID, groupID, userType string) error {
	var assignments []models.RoleAssignment
	if err := tx.Where("tenant_id = ? AND group_id = ? AND revoked_at IS NULL", tenantID, groupID).Find(&assignments).Error; err != nil {
		return err
	}
	for _, a := range assignments {
		var role models.AccessRole
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, a.RoleID).First(&role).Error; err != nil {
			return err
		}
		if role.Audience != "both" && role.Audience != userType {
			return ErrRoleInput
		}
	}
	return nil
}
