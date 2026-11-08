package services

import (
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

func cancelInvitationMail(tx *gorm.DB, tenantID, invitationID string, now time.Time) error {
	return tx.Model(&models.QueuedEmail{}).Where("tenant_id = ? AND access_invitation_id = ? AND sent_at IS NULL AND canceled_at IS NULL", tenantID, invitationID).Update("canceled_at", now).Error
}

// RevokePendingRecipientInvitations is a transactional storage helper. Callers
// must already hold sharing authority for the supplied scope, or tenant-admin
// identity authority when ClientID is empty. It never matches CreatedBy.
func RevokePendingRecipientInvitations(tx *gorm.DB, req AccessRequest, userID, email string, now time.Time) error {
	q := tx.Where("tenant_id = ? AND accepted_at IS NULL AND revoked_at IS NULL AND (recipient_user_id = ? OR LOWER(email) = LOWER(?))", req.TenantID, userID, email)
	if req.ClientID != "" {
		q = q.Where("client_id = ?", req.ClientID)
	}
	if req.FileID != "" {
		q = q.Where("file_id = ?", req.FileID)
	}
	var invitations []models.AccessInvitation
	if err := q.Find(&invitations).Error; err != nil {
		return err
	}
	for _, invitation := range invitations {
		before := invitation
		if err := tx.Model(&invitation).Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		invitation.RevokedAt = &now
		if err := cancelInvitationMail(tx, req.TenantID, invitation.ID, now); err != nil {
			return err
		}
		scope := req
		scope.ClientID, scope.FileID = invitation.ClientID, ""
		if invitation.FileID != nil {
			scope.FileID = *invitation.FileID
		}
		if err := sharingAudit(tx, scope, "invitation.revoke", userID, before, invitation); err != nil {
			return err
		}
	}
	return nil
}
