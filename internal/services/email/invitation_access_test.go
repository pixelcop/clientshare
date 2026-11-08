package email

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestInvitationDispatchRevalidatesIdentityIntentAndTarget(t *testing.T) {
	for _, change := range []string{"none", "creator-removed", "revoked", "accepted", "expired", "grant-expired", "deleted-ancestor", "deleted-client", "changed-email", "wrong-recipient", "foreign-tenant", "wrong-proof", "used-proof", "expired-proof", "password-enrolled", "passkey-enrolled"} {
		t.Run(change, func(t *testing.T) {
			db := notificationTestDB(t)
			require.NoError(t, db.AutoMigrate(&models.AccessInvitation{}, &models.InviteToken{}, &models.PasskeyCredential{}))
			now := time.Now().UTC()
			intent := models.AccessInvitation{TenantID: "tenant", ClientID: "client", FileID: notificationPtr("visible"), Email: "recipient@example.test", RecipientUserID: notificationPtr("recipient"), CreatedBy: "admin", TokenHash: "intent-hash", ExpiresAt: now.Add(time.Hour), AccessCapabilities: models.AccessCapabilities{CanRead: true}}
			require.NoError(t, db.Create(&intent).Error)
			proof := models.InviteToken{TenantID: "tenant", UserID: "recipient", Email: intent.Email, AccessInvitationID: &intent.ID, TokenHash: "identity-hash", ExpiresAt: now.Add(time.Hour), InvitedBy: "admin"}
			require.NoError(t, db.Create(&proof).Error)
			sender := &notificationRecorder{}
			queue := NewDBEmailQueue(db, sender, time.Hour)
			queue.Now = func() time.Time { return now }
			message := Email{TenantID: "tenant", To: intent.Email, Body: "email-only setup credential", AccessInvitationID: &intent.ID, IdentityInviteID: &proof.ID, DeduplicationKey: "invitation-mail"}
			item, err := queue.Queue(message, nil)
			require.NoError(t, err)
			replay, err := queue.Queue(message, nil)
			require.NoError(t, err)
			require.Equal(t, item.ID, replay.ID)
			message.IdentityInviteID = notificationPtr("another-proof")
			_, err = queue.Queue(message, nil)
			require.ErrorIs(t, err, ErrIdempotencyConflict)
			switch change {
			case "creator-removed":
				require.NoError(t, db.Delete(&models.User{}, "id = 'admin'").Error)
			case "revoked", "accepted":
				column := "revoked_at"
				if change == "accepted" {
					column = "accepted_at"
				}
				require.NoError(t, db.Model(&intent).Update(column, now).Error)
			case "expired":
				require.NoError(t, db.Model(&intent).Update("expires_at", now).Error)
			case "grant-expired":
				require.NoError(t, db.Model(&intent).Update("grant_expires_at", now).Error)
			case "deleted-ancestor":
				require.NoError(t, db.Model(&models.File{}).Where("id = 'folder'").Update("deleted_at", now).Error)
			case "deleted-client":
				require.NoError(t, db.Delete(&models.Client{}, "id = 'client'").Error)
			case "changed-email":
				require.NoError(t, db.Model(&models.User{}).Where("id = 'recipient'").Update("email", "changed@example.test").Error)
			case "wrong-recipient":
				require.NoError(t, db.Model(item).Update("recipient", "attacker@example.test").Error)
			case "foreign-tenant":
				require.NoError(t, db.Model(&intent).Update("tenant_id", "foreign").Error)
			case "wrong-proof":
				require.NoError(t, db.Model(&proof).Update("access_invitation_id", "another-intent").Error)
			case "used-proof":
				require.NoError(t, db.Model(&proof).Update("used_at", now).Error)
			case "expired-proof":
				require.NoError(t, db.Model(&proof).Update("expires_at", now).Error)
			case "password-enrolled":
				require.NoError(t, db.Model(&models.User{}).Where("id = 'recipient'").Update("password_hash", "enrolled").Error)
			case "passkey-enrolled":
				require.NoError(t, db.Create(&models.PasskeyCredential{TenantID: "tenant", UserID: "recipient", CredentialID: "credential", CredentialData: []byte("{}")}).Error)
			}
			require.NoError(t, queue.SendNow("tenant", item.ID))
			require.NoError(t, db.First(item, "id = ?", item.ID).Error)
			if change == "none" || change == "creator-removed" {
				require.Len(t, sender.sent, 1)
				require.NotNil(t, item.SentAt)
				require.Nil(t, item.CanceledAt)
			} else {
				require.Empty(t, sender.sent)
				require.Nil(t, item.SentAt)
				require.NotNil(t, item.CanceledAt)
			}
		})
	}
}
