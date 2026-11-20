package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func commentFixture(t *testing.T) (accessFixture, *FileCommentService) {
	t.Helper()
	f, _ := sharingFixture(t)
	require.NoError(t, f.db.AutoMigrate(&models.FileComment{}, &models.FileCommentRevision{}, &models.FileCommentParticipant{}, &models.FeedEvent{}))
	return f, &FileCommentService{DB: f.db}
}

func TestFileCommentsEnforceFileIdentityAndAudience(t *testing.T) {
	f, s := commentFixture(t)
	shared, err := s.Create(t.Context(), f.req("admin", "file", AccessRead), CommentInput{Body: "Shared question", Visibility: CommentShared}, "shared")
	require.NoError(t, err)
	internal, err := s.Create(t.Context(), f.req("admin", "file", AccessRead), CommentInput{Body: "Private team note", Visibility: CommentTeam}, "team")
	require.NoError(t, err)
	for _, actor := range []string{"member", "customer"} {
		page, err := s.List(t.Context(), f.req(actor, "file", AccessRead), "")
		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		require.EqualValues(t, 1, page.TotalCount)
		count, err := s.Count(t.Context(), f.req(actor, "file", AccessRead))
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
		require.Equal(t, shared.ID, page.Items[0].ID)
		require.False(t, page.CanInternal)
		require.False(t, page.Items[0].CanEdit)
		_, err = s.Create(t.Context(), f.req(actor, "file", AccessRead), CommentInput{Body: "Try internal", Visibility: CommentTeam}, "internal-"+actor)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		_, err = s.History(t.Context(), f.req(actor, "file", AccessRead), internal.ID, 0)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
	page, err := s.List(t.Context(), f.req("manager", "file", AccessRead), "")
	require.NoError(t, err)
	require.True(t, page.CanInternal)
	require.Len(t, page.Items, 2)
	require.EqualValues(t, 2, page.TotalCount)
	// A recipient can discuss only their exact shared file, never its siblings.
	f.grant(t, "file", "recipient", models.AccessCapabilities{CanRead: true})
	own, err := s.Create(t.Context(), f.req("recipient", "file", AccessRead), CommentInput{Body: "Received", Visibility: CommentShared}, "recipient")
	require.NoError(t, err)
	require.True(t, own.CanEdit)
	for _, req := range []AccessRequest{
		f.req("recipient", "sibling", AccessRead),
		f.req("admin", "folder", AccessRead),
		{TenantID: "foreign", FileID: "file", Actor: AccessActor{UserID: "admin"}},
		{TenantID: "tenant", FileID: "file", Actor: AccessActor{UserID: "admin", LinkID: "link"}},
		{TenantID: "tenant", FileID: "file"},
	} {
		_, err := s.List(t.Context(), req, "")
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		_, err = s.Create(t.Context(), req, CommentInput{Body: "No", Visibility: CommentShared}, "no")
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
	require.NoError(t, f.db.Model(&models.ItemGrant{}).Where("user_id = ?", "recipient").Update("expires_at", accessTestNow.Add(-time.Minute)).Error)
	_, err = s.Update(t.Context(), f.req("recipient", "file", AccessRead), own.ID, "After expiry", 1, false)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, f.db.Model(&models.File{}).Where("id = ?", "folder").Update("deleted_at", time.Now()).Error)
	_, err = s.List(t.Context(), f.req("admin", "file", AccessRead), "")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "comments must not outlive file/ancestry authority")
}

func TestFileCommentEditsRemovalHistoryAndRetryAreExplicit(t *testing.T) {
	f, s := commentFixture(t)
	req := f.req("member", "file", AccessRead)
	input := CommentInput{Body: "  Original <script>text</script>  ", Visibility: CommentShared}
	comment, err := s.Create(t.Context(), req, input, "retry-key")
	require.NoError(t, err)
	require.Equal(t, input.Body, comment.Body)
	again, err := s.Create(t.Context(), req, input, "retry-key")
	require.NoError(t, err)
	require.Equal(t, comment.ID, again.ID)
	_, err = s.Create(t.Context(), req, CommentInput{Body: "Different", Visibility: CommentShared}, "retry-key")
	require.ErrorIs(t, err, ErrCommentConflict)
	_, err = s.Update(t.Context(), f.req("admin", "file", AccessRead), comment.ID, "Admin overwrite", 1, false)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "even staff do not rewrite another person's words")
	comment, err = s.Update(t.Context(), req, comment.ID, "Updated", 1, false)
	require.NoError(t, err)
	require.Equal(t, 2, comment.Version)
	_, err = s.Update(t.Context(), req, comment.ID, "Stale", 1, false)
	require.ErrorIs(t, err, ErrCommentConflict)
	comment, err = s.Update(t.Context(), req, comment.ID, "", 2, true)
	require.NoError(t, err)
	require.Empty(t, comment.Body)
	require.NotNil(t, comment.DeletedAt)
	count, err := s.Count(t.Context(), req)
	require.NoError(t, err)
	require.Zero(t, count, "removed placeholders are not live comments")
	require.False(t, comment.CanEdit)
	require.True(t, comment.CanViewHistory)
	// An uncertain original POST must never recreate a removed/edited comment.
	again, err = s.Create(t.Context(), req, input, "retry-key")
	require.NoError(t, err)
	require.Equal(t, comment.ID, again.ID)
	require.NotNil(t, again.DeletedAt)
	history, err := s.History(t.Context(), req, comment.ID, 0)
	require.NoError(t, err)
	require.Len(t, history, 3)
	require.Equal(t, "removed", history[0].Action)
	require.Equal(t, "Updated", history[1].Body)
	require.Equal(t, input.Body, history[2].Body)
	_, err = s.History(t.Context(), f.req("admin", "file", AccessRead), comment.ID, 0)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	var events []models.AuditEvent
	require.NoError(t, f.db.Find(&events).Error)
	require.Len(t, events, 3)
	for _, event := range events {
		require.NotContains(t, event.Payload, "Original")
		require.NotContains(t, event.Payload, "Updated")
	}
}

func TestFileCommentValidationPaginationAndCurrentStaffRole(t *testing.T) {
	f, s := commentFixture(t)
	req := f.req("manager", "file", AccessRead)
	for i, input := range []CommentInput{
		{Body: "", Visibility: CommentShared},
		{Body: " \n\t ", Visibility: CommentShared},
		{Body: strings.Repeat("界", 4001), Visibility: CommentShared},
		{Body: "Text", Visibility: "public"},
	} {
		_, err := s.Create(t.Context(), req, input, fmt.Sprint(i))
		require.ErrorIs(t, err, ErrCommentInput)
	}
	_, err := s.Create(t.Context(), req, CommentInput{Body: "Text", Visibility: CommentShared}, "")
	require.ErrorIs(t, err, ErrCommentInput)
	for i := range 53 {
		visibility := CommentShared
		if i%2 == 0 {
			visibility = CommentTeam
		}
		_, err := s.Create(t.Context(), req, CommentInput{Body: fmt.Sprintf("Note %d", i), Visibility: visibility}, fmt.Sprint(i))
		require.NoError(t, err)
	}
	page, err := s.List(t.Context(), req, "")
	require.NoError(t, err)
	require.Len(t, page.Items, 50)
	require.EqualValues(t, 53, page.TotalCount, "the badge counts every authorized page")
	require.NotEmpty(t, page.NextCursor)
	older, err := s.List(t.Context(), req, page.NextCursor)
	require.NoError(t, err)
	require.Len(t, older.Items, 3)
	require.Empty(t, older.NextCursor)
	// Losing staff identity immediately removes internal notes/history access.
	internalID := page.Items[0].ID
	require.NoError(t, f.db.Model(&models.User{}).Where("id = ?", "manager").Update("role", "client").Error)
	page, err = s.List(t.Context(), req, "")
	require.NoError(t, err)
	require.Len(t, page.Items, 26)
	require.EqualValues(t, 26, page.TotalCount, "team notes must not leak through the count")
	require.False(t, page.CanInternal)
	require.Empty(t, page.NextCursor, "pagination cannot reveal hidden team notes")
	_, err = s.History(t.Context(), req, internalID, 0)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestFileCommentCreationAndAuditAreAtomic(t *testing.T) {
	f, s := commentFixture(t)
	require.NoError(t, f.db.Migrator().DropTable(&models.AuditEvent{}))
	_, err := s.Create(t.Context(), f.req("admin", "file", AccessRead), CommentInput{Body: "Must roll back", Visibility: CommentShared}, "atomic")
	require.Error(t, err)
	for _, model := range []any{&models.FileComment{}, &models.FileCommentRevision{}} {
		var count int64
		require.NoError(t, f.db.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}
