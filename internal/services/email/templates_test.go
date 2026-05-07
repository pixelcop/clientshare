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
		{Filename: "q1-report.pdf"},
		{Filename: "invoice.csv"},
		{Filename: "notes.txt"},
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
