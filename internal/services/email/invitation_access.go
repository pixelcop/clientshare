package email

import (
	"context"
	"errors"
	"strings"

	"github.com/pixelcop/clientshare/internal/access"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

func (q *DBEmailQueue) accessInvitationDeliveryValid(item models.QueuedEmail) (bool, error) {
	if item.AccessInvitationID == nil || len(splitRecipients(item.Recipient)) != 1 {
		return false, nil
	}
	var invitation models.AccessInvitation
	if err := q.DB.Where("tenant_id = ? AND id = ? AND revoked_at IS NULL AND accepted_at IS NULL", item.TenantID, *item.AccessInvitationID).First(&invitation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if !invitation.ExpiresAt.After(q.now()) || !strings.EqualFold(strings.TrimSpace(item.Recipient), invitation.Email) {
		return false, nil
	}
	if invitation.GrantExpiresAt != nil && !invitation.GrantExpiresAt.After(q.now()) {
		return false, nil
	}
	var client models.Client
	if err := q.DB.Where("tenant_id = ? AND id = ?", item.TenantID, invitation.ClientID).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if invitation.FileID != nil {
		if _, err := (&access.AccessService{DB: q.DB}).ValidateTarget(context.Background(), item.TenantID, invitation.ClientID, *invitation.FileID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
	}
	if invitation.RecipientUserID == nil {
		return false, nil
	}
	var user models.User
	if err := q.DB.Where("tenant_id = ? AND id = ?", item.TenantID, *invitation.RecipientUserID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if !strings.EqualFold(strings.TrimSpace(user.Email), invitation.Email) {
		return false, nil
	}
	if user.Role != "admin" && user.Role != "manager" && user.Role != "client" && user.Role != "customer" {
		return false, nil
	}
	if invitation.FileID == nil && user.Role != "client" && user.Role != "customer" {
		return false, nil
	}
	if item.IdentityInviteID != nil {
		var proof models.InviteToken
		if err := q.DB.Where("tenant_id = ? AND id = ? AND user_id = ? AND access_invitation_id = ? AND used_at IS NULL", item.TenantID, *item.IdentityInviteID, user.ID, invitation.ID).First(&proof).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		if !proof.ExpiresAt.After(q.now()) || user.PasswordHash != "" || !strings.EqualFold(proof.Email, invitation.Email) {
			return false, nil
		}
		var credentials int64
		if err := q.DB.Model(&models.PasskeyCredential{}).Where("tenant_id = ? AND user_id = ?", item.TenantID, user.ID).Count(&credentials).Error; err != nil {
			return false, err
		}
		if credentials != 0 {
			return false, nil
		}
	}
	// The original inviter's loss of access does not revoke onward intent.
	return true, nil
}
