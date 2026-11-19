package email

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestCommentDispatchRevalidatesAndDoesNotSendCommentText(t *testing.T) {
	for _, change := range []string{"none", "revoked", "deleted", "changed-email", "removed-mention", "fallback", "team"} {
		t.Run(change, func(t *testing.T) {
			db := notificationTestDB(t)
			require.NoError(t, db.AutoMigrate(&models.FileComment{}, &models.FileCommentParticipant{}))
			now := time.Now().UTC()
			comment := models.FileComment{ID: "comment", TenantID: "tenant", FileID: "visible", AuthorID: "admin",
				Visibility: "shared", Version: 1, Body: "Private body", CreationKey: "new", CreatedAt: now}
			if change == "team" {
				comment.Visibility = "team"
			}
			mark := models.FileCommentParticipant{TenantID: "tenant", CommentID: comment.ID, UserID: "recipient"}
			require.NoError(t, db.Create(&comment).Error)
			require.NoError(t, db.Create(&mark).Error)
			require.NoError(t, db.Model(&models.User{}).Where("id = ?", "admin").Update("name", "Alex & Co").Error)
			item := models.QueuedEmail{TenantID: "tenant", Recipient: "recipient@example.test",
				ClientID: notificationPtr("client"), RecipientUserID: notificationPtr("recipient"), FileCommentID: &comment.ID,
				EventType: CommentMentionNotification, NotificationBaseURL: "https://portal.example.test", ScheduledFor: now}
			switch change {
			case "revoked":
				require.NoError(t, db.Model(&models.ItemGrant{}).Where("id = ?", "grant").Update("revoked_at", now).Error)
			case "deleted":
				require.NoError(t, db.Model(&comment).Update("deleted_at", now).Error)
			case "changed-email":
				require.NoError(t, db.Model(&models.User{}).Where("id = ?", "recipient").Update("email", "other@example.test").Error)
			case "fallback":
				item.EventType = CommentMentionAndReplyNotification
				require.NoError(t, db.Model(&models.File{}).Where("id = ?", "visible").Update("user_id", "recipient").Error)
				fallthrough
			case "removed-mention":
				require.NoError(t, db.Delete(&mark).Error)
			}
			require.NoError(t, db.Create(&item).Error)
			sender := &notificationRecorder{}
			queue := NewDBEmailQueue(db, sender, time.Hour)
			require.NoError(t, queue.SendNow("tenant", item.ID))
			if change != "none" && change != "fallback" {
				require.Empty(t, sender.sent)
				require.NoError(t, db.First(&item, "id = ?", item.ID).Error)
				require.NotNil(t, item.CanceledAt)
				return
			}
			require.Len(t, sender.sent, 1)
			message := sender.sent[0]
			require.Equal(t, "recipient@example.test", message.To)
			require.Equal(t, "Comments on visible.txt", message.Subject)
			link := "https://portal.example.test/files/visible#comment-comment"
			require.Contains(t, message.Body, link)
			require.Contains(t, message.HTMLBody, `href="`+link+`"`)
			require.Contains(t, message.HTMLBody, "Alex &amp; Co")
			require.NotContains(t, message.Body+message.HTMLBody, "Private body")
			if change == "fallback" {
				require.Contains(t, message.Body, "Alex & Co commented")
			} else {
				require.Contains(t, message.Body, "Alex & Co mentioned you")
			}
		})
	}
}

func TestCommentNotificationSubjectStaysStableWithinAnAudience(t *testing.T) {
	for _, mentioned := range []bool{false, true} {
		subject, _, _, err := renderCommentNotification("Alex", "Report\r\nDraft.pdf", "https://portal.example.test", false, mentioned)
		require.NoError(t, err)
		require.Equal(t, "Comments on Report Draft.pdf", subject)
		subject, _, _, err = renderCommentNotification("Alex", "Report.pdf", "https://portal.example.test", true, mentioned)
		require.NoError(t, err)
		require.Equal(t, "Team notes on Report.pdf", subject)
	}
}
