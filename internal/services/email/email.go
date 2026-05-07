package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	jwemail "github.com/jordan-wright/email"
	"github.com/pixelcop/clientshare/internal/models"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type Email struct {
	TenantID   string
	To         string
	Subject    string
	Body       string
	HTMLBody   string
	ClientID   *string
	FileIDsCSV string
}

type EmailSender interface {
	Send(email Email) error
}

type RateLimitedSender struct {
	Sender  EmailSender
	Limiter *rate.Limiter
}

func NewRateLimitedSender(sender EmailSender, sendsPerSecond int) *RateLimitedSender {
	return &RateLimitedSender{
		Sender:  sender,
		Limiter: rate.NewLimiter(rate.Limit(sendsPerSecond), 1),
	}
}

func (s *RateLimitedSender) Send(email Email) error {
	if s.Sender == nil {
		return errors.New("email sender not configured")
	}
	if s.Limiter != nil {
		if err := s.Limiter.Wait(context.Background()); err != nil {
			return err
		}
	}
	return s.Sender.Send(email)
}

type SMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

func (s *SMTPSender) Send(email Email) error {
	msg, err := s.buildMessage(email)
	if err != nil {
		return err
	}
	if err := msg.SendWithStartTLS(s.smtpAddress(), s.smtpAuth(), &tls.Config{ServerName: s.Host}); err != nil {
		return err
	}

	zap.L().Info("Email sent", zap.String("to", email.To), zap.String("subject", email.Subject), zap.Stringp("client_id", email.ClientID))
	zap.L().Info("Email tenant", zap.String("tenant_id", strings.TrimSpace(email.TenantID)))

	return nil
}

func (s *SMTPSender) buildMessage(email Email) (*jwemail.Email, error) {
	recipients := splitRecipients(email.To)
	if len(recipients) == 0 {
		return nil, fmt.Errorf("no recipients")
	}
	if _, err := mail.ParseAddress(s.From); err != nil {
		return nil, fmt.Errorf("invalid from address: %w", err)
	}

	msg := jwemail.NewEmail()
	msg.From = s.From
	msg.Bcc = recipients
	msg.Subject = email.Subject
	msg.Text = []byte(email.Body)
	if strings.TrimSpace(email.HTMLBody) != "" {
		msg.HTML = []byte(email.HTMLBody)
	}
	return msg, nil
}

func (s *SMTPSender) smtpAddress() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func (s *SMTPSender) smtpAuth() smtp.Auth {
	if strings.TrimSpace(s.Username) == "" && strings.TrimSpace(s.Password) == "" {
		return nil
	}
	return smtp.PlainAuth("", s.Username, s.Password, s.Host)
}

type EmailQueue interface {
	Pending(tenantID, clientID string) []*models.QueuedEmail

	// Queue adds an email to the queue to be sent at the specified time.
	// If sendAt is nil, the email will sent after the default delay.
	Queue(email Email, sendAt *time.Time) (*models.QueuedEmail, error)
	Update(tenantID, id, recipient, subject, body string) error
	SendNow(tenantID, id string) error
	ProcessTenant(tenantID string) error

	// ProcessBatch sends emails that are scheduled to be sent up to the current time.
	ProcessBatch() error

	// ProcessAll sends all pending emails regardless of their scheduled time.
	ProcessAll() error
}

func splitRecipients(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	unique := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		email := strings.TrimSpace(part)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		unique = append(unique, email)
	}
	return unique
}

func mergeRecipients(existing, incoming string) string {
	combined := append(splitRecipients(existing), splitRecipients(incoming)...)
	if len(combined) == 0 {
		return ""
	}
	unique := make([]string, 0, len(combined))
	seen := make(map[string]struct{}, len(combined))
	for _, recipient := range combined {
		if recipient == "" {
			continue
		}
		if _, ok := seen[recipient]; ok {
			continue
		}
		seen[recipient] = struct{}{}
		unique = append(unique, recipient)
	}
	return strings.Join(unique, ", ")
}

func mergeBodies(existing, incoming string) string {
	existing = strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	if existing == "" {
		return incoming
	}
	if incoming == "" {
		return existing
	}
	if strings.Contains(existing, incoming) {
		return existing
	}
	return existing + "\n\n" + incoming
}

func mergeHTMLBodies(existing, incoming string) string {
	existing = strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	if incoming == "" {
		return existing
	}
	return incoming
}

func mergeCSVFileIDs(existing, incoming string) string {
	combined := append(splitRecipients(existing), splitRecipients(incoming)...)
	if len(combined) == 0 {
		return ""
	}
	return strings.Join(combined, ",")
}
