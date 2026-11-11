package email

import (
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestRenderUploadNotification_MultipleFileNames(t *testing.T) {
	files := []models.File{
		{ID: "report", Filename: "q1-report.pdf"},
		{ID: "invoice", Filename: "invoice.csv"},
		{ID: "notes", Filename: "notes.txt"},
	}

	subject, body, htmlBody := RenderUploadNotification("Acme Corp", files, "https://example.com", "ClientShare")

	// fmt.Println(body)
	// fmt.Println(htmlBody)

	wantSubject := "ClientShare: 3 new files uploaded"
	if subject != wantSubject {
		t.Fatalf("expected subject %q, got %q", wantSubject, subject)
	}

	assert.NotEmpty(t, body, "expected non-empty text body")
	assert.NotEmpty(t, htmlBody, "expected non-empty html body")

	assert.Contains(t, strings.ToLower(body), "3 new files uploaded", "expected text body to include multi-file summary")
	assert.Contains(t, strings.ToLower(htmlBody), "3 new files uploaded", "expected html body to include multi-file summary")

	for _, file := range files {
		assert.Contains(t, body, file.Filename, "expected text body to include filename %q", file.Filename)
		assert.Contains(t, htmlBody, file.Filename, "expected html body to include filename %q", file.Filename)
		assert.Contains(t, htmlBody, `href="https://example.com/files/`+file.ID+`"`)
	}
}

func TestGeneratedEmailTemplatesResolveBothVueAndGoValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		render func() (string, string, string)
		want   string
	}{
		{"password reset", func() (string, string, string) {
			return RenderPasswordReset("Alex", "https://example.test/reset", "Example Portal", time.Hour)
		}, "https://example.test/reset"},
		{"secure link", func() (string, string, string) {
			return RenderSecureLinkEmail("Example client", "Read and upload", "https://example.test/link", "Example Portal", models.Now().Add(time.Hour))
		}, "https://example.test/link"},
		{"invitation", func() (string, string, string) {
			return RenderUserInviteEmail("Alex", "https://example.test/invite", "Example Portal", "Welcome aboard", models.Now().Add(time.Hour))
		}, "https://example.test/invite"},
		{"upload", func() (string, string, string) {
			return RenderUploadNotification("Example client", []models.File{{ID: "document", Filename: "document.pdf"}}, "https://example.test", "Example Portal")
		}, "document.pdf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, text, html := tc.render()
			assert.Contains(t, html, "background-color: rgb(243,244,246)", "email theme utilities must still be inlined")
			for _, body := range []string{text, html} {
				assert.Contains(t, body, "Example Portal")
				assert.Contains(t, body, tc.want)
				assert.NotContains(t, body, "${")
				assert.NotContains(t, body, "{{")
			}
		})
	}
}

func TestRenderUserInviteEmail(t *testing.T) {
	welcomeText := "Welcome to your 2025 tax year portal. Upload documents securely in one place."
	subject, body, htmlBody := RenderUserInviteEmail("Alex", "https://app.example.com/accept-invite#token=abc", "ClientShare", welcomeText, models.Now().Add(72*time.Hour))

	assert.Equal(t, "ClientShare: Set up your account", subject)
	assert.NotEmpty(t, body)
	assert.NotEmpty(t, htmlBody)
	assert.Contains(t, strings.ToLower(body), "set your password")
	assert.Contains(t, body, welcomeText)
	assert.Contains(t, body, "https://app.example.com/accept-invite#token=abc")
	assert.Contains(t, htmlBody, welcomeText)
	assert.Contains(t, htmlBody, "https://app.example.com/accept-invite#token=abc")
}
