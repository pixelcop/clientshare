package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/email"
	"github.com/stretchr/testify/require"
)

func TestCommentActivityIsPrivateAndIndependentOfEmail(t *testing.T) {
	f, service := commentFixture(t)
	admin := f.req("admin", "file", AccessRead)
	shared, err := service.Create(t.Context(), admin, CommentInput{Body: "[@Member](mention:member) Shared text", Visibility: CommentShared}, "shared-feed")
	require.NoError(t, err)
	note, err := service.Create(t.Context(), admin, CommentInput{Body: "[@Member](mention:member) [@Manager](mention:manager) Private note", Visibility: CommentTeam}, "team-feed")
	require.NoError(t, err)
	feed := NewFeedService(f.db)
	member, err := feed.ListFeed(t.Context(), "tenant", "member", "client", 1, 10)
	require.NoError(t, err)
	require.Len(t, member.Items, 1)
	require.Equal(t, shared.ID, member.Items[0].SourceID)
	require.Equal(t, models.FeedEventTypeFileComment, member.Items[0].EventType)
	manager, err := feed.ListFeed(t.Context(), "tenant", "manager", "manager", 1, 10)
	require.NoError(t, err)
	require.Len(t, manager.Items, 1)
	require.Equal(t, note.ID, manager.Items[0].SourceID)
	require.Equal(t, models.FeedEventTypeTeamNote, manager.Items[0].EventType)
	require.NoError(t, f.db.Model(&models.FileComment{}).Where("id = ?", shared.ID).Update("deleted_at", time.Now()).Error)
	member, err = feed.ListFeed(t.Context(), "tenant", "member", "client", 1, 10)
	require.NoError(t, err)
	require.Empty(t, member.Items)
}

func TestCommentNotificationRecipientsFollowParticipationAndExactFileAccess(t *testing.T) {
	f, service := commentFixture(t)
	service.NotificationBaseURL = "https://portal.example.test"
	require.NoError(t, f.db.Model(&models.File{}).Where("id = ?", "file").Update("user_id", "member").Error)
	f.grant(t, "folder", "recipient", models.AccessCapabilities{CanRead: true})
	post := func(actor, body, visibility, key string) *models.FileComment {
		t.Helper()
		comment, err := service.Create(t.Context(), f.req(actor, "file", AccessRead), CommentInput{Body: body, Visibility: visibility}, key)
		require.NoError(t, err)
		return comment
	}
	deliveries := func(comment *models.FileComment) map[string]string {
		t.Helper()
		var rows []models.QueuedEmail
		require.NoError(t, f.db.Where("file_comment_id = ?", comment.ID).Find(&rows).Error)
		result := map[string]string{}
		for _, row := range rows {
			require.NotContains(t, result, *row.RecipientUserID)
			result[*row.RecipientUserID] = row.EventType
			require.Empty(t, row.Body)
		}
		return result
	}

	shared := post("admin", "[@Recipient](mention:recipient) [@Member](mention:member)", CommentShared, "shared")
	require.Equal(t, shared.ID, post("admin", shared.Body, CommentShared, "shared").ID)
	require.Equal(t, map[string]string{
		"member":    email.CommentMentionAndReplyNotification,
		"recipient": email.CommentMentionNotification,
	}, deliveries(shared))
	require.Equal(t, map[string]string{"admin": email.CommentNotification, "recipient": email.CommentNotification},
		deliveries(post("member", "Reply", CommentShared, "shared-reply")))

	require.NoError(t, f.db.Create(&models.User{ID: "staff-without-file", TenantID: "tenant", Role: "manager", Email: "staff@example.test"}).Error)
	f.grant(t, "sibling", "staff-without-file", models.AccessCapabilities{CanRead: true})
	note := post("admin", "[@Member](mention:member) [@Staff](mention:staff-without-file) [@Manager](mention:manager)", CommentTeam, "team")
	require.Equal(t, map[string]string{"manager": email.CommentMentionNotification}, deliveries(note))
	f.grant(t, "file", "staff-without-file", models.AccessCapabilities{CanRead: true})
	require.Equal(t, map[string]string{"manager": email.CommentNotification},
		deliveries(post("admin", "Same team note thread", CommentTeam, "team-reply")))
	require.Equal(t, map[string]string{"admin": email.CommentNotification, "recipient": email.CommentNotification, "member": email.CommentNotification},
		deliveries(post("customer", "No private participant leaks", CommentShared, "public-reply")))
}

func TestCommentFocusWindowRespectsAudienceAndDeletion(t *testing.T) {
	f, service := commentFixture(t)
	admin := f.req("admin", "file", AccessRead)
	member := f.req("member", "file", AccessRead)
	older, err := service.Create(t.Context(), admin, CommentInput{Body: "Old", Visibility: CommentShared}, "old")
	require.NoError(t, err)
	private, err := service.Create(t.Context(), admin, CommentInput{Body: "Private", Visibility: CommentTeam}, "private")
	require.NoError(t, err)
	for i := range 54 {
		// Fixed IDs isolate paging behavior from the ULID generator clock.
		require.NoError(t, f.db.Create(&models.FileComment{
			ID: fmt.Sprintf("Z%025d", i), TenantID: "tenant", FileID: "file", AuthorID: "admin",
			Body: "Newer", Visibility: CommentShared, Version: 1, CreationKey: fmt.Sprintf("new-%d", i),
		}).Error)
	}
	page, err := service.List(t.Context(), member, "")
	require.NoError(t, err)
	for _, comment := range page.Items {
		require.NotEqual(t, older.ID, comment.ID)
	}
	page, err = service.ListAround(t.Context(), member, "", older.ID)
	require.NoError(t, err)
	require.False(t, page.FocusUnavailable)
	require.True(t, page.FocusHasNewer)
	require.Equal(t, older.ID, page.Items[len(page.Items)-1].ID)
	require.Len(t, page.Items, 16)
	page, err = service.ListAround(t.Context(), member, "", private.ID)
	require.NoError(t, err)
	require.True(t, page.FocusUnavailable)
	_, err = service.Update(t.Context(), admin, older.ID, "", 1, true)
	require.NoError(t, err)
	page, err = service.ListAround(t.Context(), member, "", older.ID)
	require.NoError(t, err)
	require.True(t, page.FocusUnavailable)
}
