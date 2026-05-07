package email

import (
	"net/smtp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubEmailSender struct {
	mu    sync.Mutex
	count int
}

func (s *stubEmailSender) Send(email Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	return nil
}

func (s *stubEmailSender) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func TestSMTPSenderBuildMessage(t *testing.T) {
	sender := SMTPSender{From: "Clientshare <noreply@example.com>"}

	msg, err := sender.buildMessage(Email{
		To:       "first@example.com; second@example.com, first@example.com",
		Subject:  "Queued files are ready",
		Body:     "Plain text body",
		HTMLBody: "<p>HTML body</p>",
	})

	require.NoError(t, err)
	require.Equal(t, sender.From, msg.From)
	require.Equal(t, []string{"first@example.com", "second@example.com"}, msg.Bcc)
	require.Equal(t, "Queued files are ready", msg.Subject)
	require.Equal(t, []byte("Plain text body"), msg.Text)
	require.Equal(t, []byte("<p>HTML body</p>"), msg.HTML)
}

func TestSMTPSenderBuildMessageRequiresRecipients(t *testing.T) {
	sender := SMTPSender{From: "noreply@example.com"}

	_, err := sender.buildMessage(Email{To: "   "})

	require.EqualError(t, err, "no recipients")
}

func TestSMTPSenderBuildMessageRequiresValidFrom(t *testing.T) {
	sender := SMTPSender{From: "not an address"}

	_, err := sender.buildMessage(Email{To: "person@example.com"})

	require.ErrorContains(t, err, "invalid from address")
}

func TestSMTPSenderSMTPAuth(t *testing.T) {
	t.Run("returns nil without credentials", func(t *testing.T) {
		sender := SMTPSender{Host: "smtp.example.com"}
		require.Nil(t, sender.smtpAuth())
	})

	t.Run("returns plain auth with credentials", func(t *testing.T) {
		sender := SMTPSender{Host: "smtp.example.com", Username: "user", Password: "pass"}
		require.IsType(t, smtp.PlainAuth("", "user", "pass", "smtp.example.com"), sender.smtpAuth())
	})
}

func TestRateLimitedSenderPacesSends(t *testing.T) {
	inner := &stubEmailSender{}
	sender := NewRateLimitedSender(inner, 20)

	start := time.Now()
	for range 5 {
		require.NoError(t, sender.Send(Email{To: "person@example.com", Subject: "Subject", Body: "Body"}))
	}
	elapsed := time.Since(start)

	require.Equal(t, 5, inner.Count())
	require.GreaterOrEqual(t, elapsed, 150*time.Millisecond)
}
