package email

import (
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func notificationPtr[T any](v T) *T { return &v }

type notificationRecorder struct{ sent []Email }

func (s *notificationRecorder) Send(e Email) error { s.sent = append(s.sent, e); return nil }

func notificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/notifications.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.Client{}, &models.File{}, &models.User{}, &models.UserClient{}, &models.ItemGrant{}, &models.SecureLink{}, &models.QueuedEmail{}, &models.AccessRole{}, &models.RoleAssignment{}, &models.AccessGroupMember{}))
	for _, row := range []any{
		&models.Client{ID: "client", TenantID: "tenant", Name: "Private Client Name", FolderPath: "private-client", RootFolderID: notificationPtr("root")},
		&models.File{ID: "root", TenantID: "tenant", ClientID: "client", Type: "folder", Filename: "private-root", Path: "private-root"},
		&models.File{ID: "folder", TenantID: "tenant", ClientID: "client", Type: "folder", Filename: "Shared", FolderID: notificationPtr("root")},
		&models.File{ID: "visible", TenantID: "tenant", ClientID: "client", Type: "file", Filename: "visible.txt", FolderID: notificationPtr("folder")},
		&models.File{ID: "private", TenantID: "tenant", ClientID: "client", Type: "file", Filename: "secret.txt", FolderID: notificationPtr("root")},
		&models.User{ID: "recipient", TenantID: "tenant", Email: "recipient@example.test", Role: "client"},
		&models.User{ID: "admin", TenantID: "tenant", Email: "admin@example.test", Role: "admin"},
		&models.User{ID: "assigned", TenantID: "tenant", Email: "assigned@example.test", Role: "manager"},
		&models.User{ID: "unassigned", TenantID: "tenant", Email: "unassigned@example.test", Role: "manager"},
		&models.User{ID: "full", TenantID: "tenant", Email: "full@example.test", Role: "manager", ManagerFullAccess: true},
		&models.UserClient{TenantID: "tenant", ClientID: "client", UserID: "assigned"},
		&models.ItemGrant{ID: "grant", TenantID: "tenant", ClientID: "client", UserID: "recipient", FileID: "folder", CreatedBy: "removed-creator", AccessCapabilities: models.AccessCapabilities{CanRead: true}},
	} {
		require.NoError(t, db.Create(row).Error)
	}
	return db
}

func TestNotificationDispatchRebuildsOnlyCurrentlyAuthorizedContent(t *testing.T) {
	for _, change := range []string{"none", "revoked", "expired", "deleted-ancestor", "email-changed"} {
		t.Run(change, func(t *testing.T) {
			db := notificationTestDB(t)
			now := time.Now().UTC()
			message, valid, err := BuildUploadNotification(t.Context(), db, "tenant", "client", "recipient@example.test", notificationPtr("recipient"), nil, []string{"visible", "private"}, "https://portal.example.test", "ClientShare", now)
			require.NoError(t, err)
			require.True(t, valid)
			require.Equal(t, "visible", message.FileIDsCSV)
			require.NotContains(t, message.Body, "Private Client Name")
			require.NotContains(t, message.Body, "secret.txt")
			sender := &notificationRecorder{}
			queue := NewDBEmailQueue(db, sender, 24*time.Hour)
			queue.Now = func() time.Time { return now }
			item, err := queue.Queue(message, nil)
			require.NoError(t, err)
			// Stored rendered bodies/IDs are not a trusted authorization snapshot.
			require.NoError(t, db.Model(item).Updates(map[string]any{"body": "secret.txt", "html_body": "secret.txt", "file_ids_csv": "visible,private"}).Error)
			switch change {
			case "revoked":
				require.NoError(t, db.Model(&models.ItemGrant{}).Where("id = 'grant'").Update("revoked_at", now).Error)
			case "expired":
				require.NoError(t, db.Model(&models.ItemGrant{}).Where("id = 'grant'").Update("expires_at", now.Add(-time.Second)).Error)
			case "deleted-ancestor":
				require.NoError(t, db.Model(&models.File{}).Where("id = 'folder'").Update("deleted_at", now).Error)
			case "email-changed":
				require.NoError(t, db.Model(&models.User{}).Where("id = 'recipient'").Update("email", "changed@example.test").Error)
			}
			require.NoError(t, queue.SendNow("tenant", item.ID))
			require.NoError(t, db.First(item, "id = ?", item.ID).Error)
			if change == "none" {
				require.Len(t, sender.sent, 1)
				require.Contains(t, sender.sent[0].Body, "visible.txt")
				require.NotContains(t, sender.sent[0].Body, "secret.txt")
				require.NotContains(t, sender.sent[0].HTMLBody, "secret.txt")
				require.Equal(t, "visible", item.FileIDsCSV)
				require.NotNil(t, item.SentAt)
			} else {
				require.Empty(t, sender.sent)
				require.NotNil(t, item.CanceledAt)
				require.Nil(t, item.SentAt)
			}
		})
	}
}

func TestNotificationLinkScopeAndAutomaticSubscriptionBoundaries(t *testing.T) {
	db := notificationTestDB(t)
	now := time.Now().UTC()
	for _, mode := range []string{"read", "write"} {
		link := models.SecureLink{ID: mode, TenantID: "tenant", ClientID: "client", FileID: notificationPtr("folder"), Token: mode, Email: mode + "@example.test", AccessType: mode, ExpiresAt: now.Add(time.Hour).In(time.FixedZone("EST", -5*3600))}
		require.NoError(t, db.Create(&link).Error)
		message, valid, err := BuildUploadNotification(t.Context(), db, "tenant", "client", link.Email, nil, &link.ID, []string{"visible", "private"}, "https://portal.example.test", "ClientShare", now)
		require.NoError(t, err)
		require.Equal(t, mode == "read", valid)
		if valid {
			require.Equal(t, "visible", message.FileIDsCSV)
			require.NotContains(t, message.Body, "secret.txt")
		}
	}
	recipients, err := UploadNotificationRecipients(t.Context(), db, "tenant", "client", "client", now)
	require.NoError(t, err)
	addresses := []string{}
	for _, recipient := range recipients {
		addresses = append(addresses, recipient.Email)
	}
	require.ElementsMatch(t, []string{"admin@example.test", "assigned@example.test", "full@example.test"}, addresses)
	recipients, err = UploadNotificationRecipients(t.Context(), db, "tenant", "client", "admin", now)
	require.NoError(t, err)
	for _, recipient := range recipients {
		require.False(t, strings.EqualFold(recipient.Email, "recipient@example.test"), "direct grants do not subscribe to client-wide uploads")
		require.False(t, strings.EqualFold(recipient.Email, "write@example.test"), "upload-only links do not receive names")
	}
}
