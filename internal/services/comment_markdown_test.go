package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCommentMarkdownSafeRendering(t *testing.T) {
	body := "**Bold** and *italic*\nnext line\n\n- one\n- two\n\n`code`\n\n> quoted\n\n[site](https://example.test)\n\n[@Chetan](mention:admin)"
	rendered, ids, err := RenderCommentMarkdown(body)
	require.NoError(t, err)
	for _, fragment := range []string{"<strong>Bold</strong>", "<em>italic</em>", "<br>", "<ul>", "<code>code</code>", "<blockquote>", `href="https://example.test"`, `class="comment-mention" data-user-id="admin">@Chetan</span>`} {
		require.Contains(t, rendered, fragment)
	}
	require.Equal(t, []string{"admin"}, ids)
	rendered, _, err = RenderCommentMarkdown(`[@Alex \[Team\] &amp; Co](mention:admin)`)
	require.NoError(t, err)
	require.Contains(t, rendered, `>@Alex [Team] &amp; Co</span>`)
	for _, source := range []string{
		`<script>alert(1)</script><img src=x onerror=alert(1)>`,
		`[click](javascript:alert%281%29)`,
		`[click](data:text/html,test)`,
		`![tracking](https://evil.test/pixel)`,
		`[@<img src=x onerror=alert(1)>](mention:admin)`,
		`[@evil](mention:bad%22onclick)`,
	} {
		rendered, _, err := RenderCommentMarkdown(source)
		require.NoError(t, err)
		require.NotContains(t, rendered, "<script")
		require.NotContains(t, rendered, "<img")
		require.NotContains(t, rendered, `href="javascript:`)
		require.NotContains(t, rendered, `href="data:`)
		require.NotContains(t, rendered, `href="mention:`)
	}
	_, ids, err = RenderCommentMarkdown("`[@Chetan](mention:admin)`\n\n```\n[@Chetan](mention:admin)\n```")
	require.NoError(t, err)
	require.Empty(t, ids, "code is literal, not a mention")
}

func TestCommentMarkdownSourcePreviewAndHistory(t *testing.T) {
	f, s := commentFixture(t)
	req := f.req("admin", "file", AccessRead)
	body := "    indented code\n\n**Ready**\n\n[@Admin](mention:admin)\n"
	preview, err := s.Preview(t.Context(), req, CommentInput{Body: body, Visibility: CommentShared})
	require.NoError(t, err)
	created, err := s.Create(t.Context(), req, CommentInput{Body: body, Visibility: CommentShared}, "markdown")
	require.NoError(t, err)
	require.Equal(t, body, created.Body)
	require.Equal(t, preview, *created.BodyHTML)
	updated, err := s.Update(t.Context(), req, created.ID, "*Edited*\n", 1, false)
	require.NoError(t, err)
	require.Equal(t, "*Edited*\n", updated.Body)
	require.Contains(t, *updated.BodyHTML, "<em>Edited</em>")
	revisions, err := s.History(t.Context(), req, created.ID, 0)
	require.NoError(t, err)
	require.Equal(t, body, revisions[1].Body)
	require.Equal(t, preview, *revisions[1].BodyHTML)
	_, err = s.Preview(t.Context(), f.req("member", "file", AccessRead), CommentInput{Body: body, Visibility: CommentTeam})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = s.Preview(t.Context(), req, CommentInput{Body: strings.Repeat("a", 4001), Visibility: CommentShared})
	require.ErrorIs(t, err, ErrCommentInput)
	_, err = s.Update(t.Context(), req, created.ID, "", 2, true)
	require.NoError(t, err)
	page, err := s.List(t.Context(), req, "")
	require.NoError(t, err)
	require.Nil(t, page.Items[0].BodyHTML)
}

func TestCommentMentionsRespectAudienceAndDirectory(t *testing.T) {
	f, s := commentFixture(t)
	require.NoError(t, f.db.Model(&models.User{}).Where("tenant_id = ?", "tenant").Update("name", "Person").Error)
	f.grant(t, "file", "recipient", models.AccessCapabilities{CanRead: true})
	staff := f.req("admin", "file", AccessRead)
	people, err := s.Mentions(t.Context(), staff, "Person", CommentShared)
	require.NoError(t, err)
	require.NotEmpty(t, people)
	initial, err := s.Mentions(t.Context(), staff, "", CommentShared)
	require.NoError(t, err)
	require.Equal(t, people, initial, "bare @ returns available people without broadening access")
	for _, person := range people {
		require.NotEmpty(t, person.Email)
		require.NotEqual(t, "outsider", person.ID)
	}
	people, err = s.Mentions(t.Context(), staff, "Person", CommentTeam)
	require.NoError(t, err)
	for _, person := range people {
		require.Contains(t, []string{"admin", "manager", "full"}, person.ID)
	}
	recipient := f.req("recipient", "file", AccessRead)
	people, err = s.Mentions(t.Context(), recipient, "Person", CommentShared)
	require.NoError(t, err)
	require.Len(t, people, 1, "item access does not expose the client directory")
	initial, err = s.Mentions(t.Context(), recipient, "", CommentShared)
	require.NoError(t, err)
	require.Equal(t, people, initial, "bare @ must not expose the client directory")
	require.Equal(t, "recipient", people[0].ID)
	require.Empty(t, people[0].Email)
	_, err = s.Create(t.Context(), staff, CommentInput{Body: "Hello", Visibility: CommentTeam}, "private-author")
	require.NoError(t, err)
	people, err = s.Mentions(t.Context(), recipient, "Person", CommentShared)
	require.NoError(t, err)
	require.Len(t, people, 1, "private authors must not leak")
	_, err = s.Create(t.Context(), staff, CommentInput{Body: "Hello", Visibility: CommentShared}, "visible-author")
	require.NoError(t, err)
	people, err = s.Mentions(t.Context(), recipient, "Person", CommentShared)
	require.NoError(t, err)
	require.Len(t, people, 2)
	_, err = s.Preview(t.Context(), recipient, CommentInput{Body: "[@Admin](mention:admin)", Visibility: CommentShared})
	require.NoError(t, err)
	for _, id := range []string{"outsider", "foreign", "unknown"} {
		_, err = s.Create(t.Context(), staff, CommentInput{Body: "[@Person](mention:" + id + ")", Visibility: CommentShared}, "forged-"+id)
		require.ErrorIs(t, err, ErrCommentMention)
	}
	_, err = s.Create(t.Context(), staff, CommentInput{Body: "[@Client](mention:member)", Visibility: CommentTeam}, "client-internal")
	require.ErrorIs(t, err, ErrCommentMention)
	require.NoError(t, f.db.Model(&models.ItemGrant{}).Where("user_id = ?", "recipient").Update("revoked_at", accessTestNow).Error)
	_, err = s.Preview(t.Context(), staff, CommentInput{Body: "[@Recipient](mention:recipient)", Visibility: CommentShared})
	require.ErrorIs(t, err, ErrCommentMention)
	_, err = s.Mentions(t.Context(), recipient, "Person", CommentShared)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestCommentMentionInitialSuggestionsAreBoundedAndTenantScoped(t *testing.T) {
	f, s := commentFixture(t)
	for i := range 12 {
		require.NoError(t, f.db.Create(&models.User{
			ID: fmt.Sprintf("mention-admin-%02d", i), TenantID: "tenant",
			Name: fmt.Sprintf("Admin %02d", i), Email: fmt.Sprintf("mention-%02d@example.test", i), Role: "admin",
		}).Error)
	}
	require.NoError(t, f.db.Create(&models.User{
		ID: "foreign-person", TenantID: "foreign", Name: "A Foreign person", Email: "foreign@example.test", Role: "admin",
	}).Error)
	people, err := s.Mentions(t.Context(), f.req("admin", "file", AccessRead), "", CommentTeam)
	require.NoError(t, err)
	require.Len(t, people, 8)
	for _, person := range people {
		require.NotEqual(t, "foreign-person", person.ID)
	}
}
