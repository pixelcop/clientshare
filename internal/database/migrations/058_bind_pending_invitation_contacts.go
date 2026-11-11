package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

// Pending sharing recipients belong in the user directory even before email
// delivery or acceptance. Creating an uncredentialed identity grants no access.
func upBindPendingInvitationContacts(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	now := time.Now().UTC()
	var invitations []models.AccessInvitation
	if err := db.Where("recipient_user_id IS NULL AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?", now).Find(&invitations).Error; err != nil {
		return err
	}
	for _, invitation := range invitations {
		address, err := mail.ParseAddress(strings.TrimSpace(invitation.Email))
		if err != nil {
			return fmt.Errorf("invalid recipient on pending invitation %s: %w", invitation.ID, err)
		}
		email := strings.ToLower(strings.TrimSpace(address.Address))
		var users []models.User
		if err := db.Where("tenant_id = ? AND LOWER(email) = ?", invitation.TenantID, email).Limit(2).Find(&users).Error; err != nil {
			return err
		}
		if len(users) > 1 {
			return fmt.Errorf("ambiguous tenant-local recipient on pending invitation %s", invitation.ID)
		}
		var user models.User
		if len(users) == 1 {
			user = users[0]
		} else {
			user = models.User{TenantID: invitation.TenantID, Email: email, Role: "client", CreatedAt: now, UpdatedAt: now}
			if err := db.Create(&user).Error; err != nil {
				return err
			}
			profile := models.AccessProfile{TenantID: user.TenantID, UserID: user.ID, UserType: "external", OrganizationRole: "none"}
			if err := db.Create(&profile).Error; err != nil {
				return err
			}
		}
		if err := db.Model(&models.AccessInvitation{}).Where("tenant_id = ? AND id = ?", invitation.TenantID, invitation.ID).Update("recipient_user_id", user.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

func downBindPendingInvitationContacts(context.Context, *sql.Tx) error {
	// Directory identities may since have been enrolled or promoted. Never delete
	// them or detach their invitations during a data-only migration rollback.
	return nil
}
