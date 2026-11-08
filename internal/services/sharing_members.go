package services

import (
	"context"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

type linkAuditState struct {
	ID         string     `json:"id"`
	FileID     *string    `json:"file_id,omitempty"`
	AccessType string     `json:"access_type"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func linkAudit(link models.SecureLink) linkAuditState {
	return linkAuditState{ID: link.ID, FileID: link.FileID, AccessType: link.AccessType, ExpiresAt: link.ExpiresAt, RevokedAt: link.RevokedAt}
}

func revokeClientLinks(tx *gorm.DB, req AccessRequest, now time.Time) (int, error) {
	var links []models.SecureLink
	if err := tx.Where("tenant_id = ? AND client_id = ? AND revoked_at IS NULL", req.TenantID, req.ClientID).Find(&links).Error; err != nil {
		return 0, err
	}
	for _, link := range links {
		before := linkAudit(link)
		if err := tx.Model(&link).Update("revoked_at", now).Error; err != nil {
			return 0, err
		}
		link.RevokedAt = &now
		if err := tx.Model(&models.QueuedEmail{}).Where("tenant_id = ? AND sent_at IS NULL AND canceled_at IS NULL AND (secure_link_id = ? OR recipient_link_id = ?)", req.TenantID, link.ID, link.ID).Update("canceled_at", now).Error; err != nil {
			return 0, err
		}
		linkReq := req
		if link.FileID != nil {
			linkReq.FileID = *link.FileID
		}
		if err := sharingAudit(tx, linkReq, "link.revoke", link.Email, before, linkAudit(link)); err != nil {
			return 0, err
		}
	}
	return len(links), nil
}

func (s *SharingService) RevokeClientLinks(ctx context.Context, req AccessRequest) (int, error) {
	if req.FileID != "" {
		return 0, ErrSharingInput
	}
	count := 0
	err := s.withAuthority(ctx, req, models.AccessCapabilities{}, func(tx *gorm.DB, _ *models.File) error {
		var err error
		count, err = revokeClientLinks(tx, req, s.now())
		return err
	})
	return count, err
}

// RemoveMember removes recipient access, never shares authored by that person.
// Bearer links survive unless the caller explicitly chooses client-wide revoke.
func (s *SharingService) RemoveMember(ctx context.Context, req AccessRequest, userID string, revokeLinks bool) error {
	if req.FileID != "" || userID == "" {
		return ErrSharingInput
	}
	return s.withAuthority(ctx, req, models.AccessCapabilities{}, func(tx *gorm.DB, _ *models.File) error {
		var member models.UserClient
		if err := tx.Where("tenant_id = ? AND client_id = ? AND user_id = ?", req.TenantID, req.ClientID, userID).First(&member).Error; err != nil {
			return err
		}
		var recipient, actor models.User
		if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, userID).First(&recipient).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND id = ?", req.TenantID, req.Actor.UserID).First(&actor).Error; err != nil {
			return err
		}
		if recipient.Role != "client" && recipient.Role != "customer" && actor.Role != "admin" {
			return gorm.ErrRecordNotFound
		}
		staffReq := req
		staffReq.Operation = AccessManageClient
		staff, err := (&AccessService{DB: tx}).Resolve(ctx, staffReq)
		if err != nil {
			return err
		}
		if !staff.Allowed && (member.AccessSource != "invitation" || member.InvitedBy == nil || *member.InvitedBy != req.Actor.UserID) {
			return gorm.ErrRecordNotFound
		}
		var grants []models.ItemGrant
		if err := tx.Where("tenant_id = ? AND client_id = ? AND user_id = ? AND revoked_at IS NULL", req.TenantID, req.ClientID, userID).Find(&grants).Error; err != nil {
			return err
		}
		for _, grant := range grants {
			before := grant
			if err := tx.Model(&grant).Updates(map[string]any{"revoked_at": s.now(), "updated_at": s.now(), "version": gorm.Expr("version + 1")}).Error; err != nil {
				return err
			}
			var current models.ItemGrant
			if err := tx.First(&current, "tenant_id = ? AND id = ?", req.TenantID, grant.ID).Error; err != nil {
				return err
			}
			grantReq := req
			grantReq.FileID = grant.FileID
			if err := sharingAudit(tx, grantReq, "grant.revoke", userID, before, current); err != nil {
				return err
			}
		}
		var invitations []models.AccessInvitation
		if err := tx.Where("tenant_id = ? AND client_id = ? AND accepted_at IS NULL AND revoked_at IS NULL AND (recipient_user_id = ? OR LOWER(email) = LOWER(?))", req.TenantID, req.ClientID, userID, recipient.Email).Find(&invitations).Error; err != nil {
			return err
		}
		for _, invitation := range invitations {
			before := invitation
			if err := tx.Model(&invitation).Update("revoked_at", s.now()).Error; err != nil {
				return err
			}
			inviteReq := req
			if invitation.FileID != nil {
				inviteReq.FileID = *invitation.FileID
			}
			if err := sharingAudit(tx, inviteReq, "invitation.revoke", userID, before, invitation); err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_id = ? AND client_id = ? AND user_id = ?", req.TenantID, req.ClientID, userID).Delete(&models.UserClient{}).Error; err != nil {
			return err
		}
		if err := sharingAudit(tx, req, "membership.remove", userID, member, nil); err != nil {
			return err
		}
		if revokeLinks {
			_, err = revokeClientLinks(tx, req, s.now())
			return err
		}
		return nil
	})
}
