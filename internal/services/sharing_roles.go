package services

import (
	"context"
	"strings"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

func sharingRoleInput(ctx context.Context, db *gorm.DB, req AccessRequest, roleID, userID, email string, existing ...*string) (models.AccessCapabilities, error) {
	userType := "external"
	var users []models.User
	q := db.WithContext(ctx).Where("tenant_id = ?", req.TenantID)
	if userID != "" {
		q = q.Where("id = ?", userID)
	} else {
		q = q.Where("LOWER(email) = ?", strings.ToLower(strings.TrimSpace(email)))
	}
	if err := q.Limit(2).Find(&users).Error; err != nil {
		return models.AccessCapabilities{}, err
	}
	if userID != "" && len(users) != 1 {
		return models.AccessCapabilities{}, gorm.ErrRecordNotFound
	}
	if len(users) > 1 {
		return models.AccessCapabilities{}, ErrSharingConflict
	}
	if len(users) == 1 {
		p, err := AccessProfileFor(db, users[0])
		if err != nil {
			return models.AccessCapabilities{}, err
		}
		userType = p.UserType
	}
	scope := "workspace"
	if req.FileID != "" {
		file, err := (&AccessService{DB: db}).ValidateTarget(ctx, req.TenantID, req.ClientID, req.FileID)
		if err != nil {
			return models.AccessCapabilities{}, err
		}
		scope = file.Type
	}
	preserve := len(existing) > 0 && existing[0] != nil && *existing[0] == roleID
	_, caps, err := AssignmentRole(db, req.TenantID, roleID, userType, scope, preserve)
	if err != nil {
		return caps, err
	}
	actorReq := req
	actorReq.Operation = AccessShare
	d, err := (&AccessService{DB: db}).Resolve(ctx, actorReq)
	if err != nil {
		return caps, err
	}
	p := d.Capabilities.Permissions()
	if !d.Allowed || (!p.CanInviteExternal && !p.CanManageAccess) || (userType == "internal" && d.Capabilities.Sharing != nil && !p.CanManageAccess) || !d.CanGrant(caps) {
		return caps, ErrSharingCapabilities
	}
	if len(users) == 1 && len(existing) == 0 {
		if err := requireInvitationRecipient(ctx, db, req, users[0]); err != nil {
			return caps, err
		}
	}
	return caps, nil
}

func roleIDPointer(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

func requireInvitationRecipient(ctx context.Context, tx *gorm.DB, req AccessRequest, recipient models.User) error {
	req.Operation = AccessInvite
	if recipient.Role == "admin" || recipient.Role == "manager" {
		req.Operation = AccessManageAccess
	}
	d, err := (&AccessService{DB: tx}).Resolve(ctx, req)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return ErrSharingCapabilities
	}
	if recipient.Role == "admin" || recipient.Role == "manager" {
		if d.Capabilities.Sharing != nil && !d.Capabilities.Permissions().CanManageAccess {
			return ErrSharingCapabilities
		}
		if req.FileID == "" && d.Capabilities.Sharing == nil {
			var actor models.User
			if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, req.Actor.UserID).First(&actor).Error; err != nil {
				return err
			}
			if actor.Role != "admin" && actor.Role != "manager" {
				return ErrSharingCapabilities
			}
		}
	}
	return nil
}

func requireOwnedSharing(ctx context.Context, tx *gorm.DB, req AccessRequest, createdBy string) error {
	if createdBy == req.Actor.UserID {
		return nil
	}
	req.Operation = AccessManageAccess
	d, err := (&AccessService{DB: tx}).Resolve(ctx, req)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return ErrSharingCapabilities
	}
	return nil
}
