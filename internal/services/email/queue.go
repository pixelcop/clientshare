package email

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
)

type InMemoryEmailQueue struct {
	defaultBatchDelay time.Duration
	mu                sync.Mutex
	Emails            []models.QueuedEmail
	Sender            EmailSender
}

func NewInMemoryEmailQueue(sender EmailSender, defaultBatchDelay time.Duration) *InMemoryEmailQueue {
	return &InMemoryEmailQueue{Sender: sender, defaultBatchDelay: defaultBatchDelay}
}

func (q *InMemoryEmailQueue) Queue(email Email, sendAt *time.Time) (*models.QueuedEmail, error) {
	if sendAt == nil {
		t := time.Now().Add(q.defaultBatchDelay)
		sendAt = &t
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	// look for existing queued email for the same client
	if email.ClientID != nil {
		for i := range q.Emails {
			existing := &q.Emails[i]
			if existing.Sent() {
				continue
			}
			if existing.ClientID != nil && *existing.ClientID == *email.ClientID {
				existing.Recipient = mergeRecipients(existing.Recipient, email.To)
				existing.Subject = email.Subject
				existing.Body = mergeBodies(existing.Body, email.Body)
				existing.HTMLBody = mergeHTMLBodies(existing.HTMLBody, email.HTMLBody)
				existing.FileIDsCSV = mergeCSVFileIDs(existing.FileIDsCSV, email.FileIDsCSV)
				existing.ScheduledFor = *sendAt
				if existing.ScheduledFor.Before(time.Now().Add(time.Second)) {
					go q.SendNow(existing.TenantID, existing.ID)
				}
				return existing, nil
			}
		}
	}

	// new item
	q.Emails = append(q.Emails, models.QueuedEmail{
		TenantID:     strings.TrimSpace(email.TenantID),
		ID:           ids.NewULID(),
		Recipient:    email.To,
		Subject:      email.Subject,
		Body:         email.Body,
		HTMLBody:     email.HTMLBody,
		FileIDsCSV:   email.FileIDsCSV,
		ClientID:     email.ClientID,
		ScheduledFor: *sendAt,
		CreatedAt:    time.Now(),
	})
	if sendAt.Before(time.Now().Add(time.Second)) {
		go q.SendNow(q.Emails[len(q.Emails)-1].TenantID, q.Emails[len(q.Emails)-1].ID)
	}
	return &q.Emails[len(q.Emails)-1], nil
}

func (q *InMemoryEmailQueue) Pending(tenantID, clientID string) []*models.QueuedEmail {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := make([]*models.QueuedEmail, 0, len(q.Emails))
	for _, email := range q.Emails {
		if tenantID != "" && strings.TrimSpace(email.TenantID) != strings.TrimSpace(tenantID) {
			continue
		}
		if !email.Sent() && (clientID == "" || (email.ClientID != nil && *email.ClientID == clientID)) {
			items = append(items, &email)
		}
	}
	return items
}

func (q *InMemoryEmailQueue) Update(tenantID, id, recipient, subject, body string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.Emails {
		item := &q.Emails[i]
		if item.ID == id && !item.Sent() && (tenantID == "" || strings.TrimSpace(item.TenantID) == strings.TrimSpace(tenantID)) {
			item.Recipient = recipient
			item.Subject = subject
			item.Body = body
			return nil
		}
	}
	return errors.New("queued email not found")
}

func ToEmail(e *models.QueuedEmail) Email {
	return Email{
		TenantID: e.TenantID,
		To:       e.Recipient,
		Subject:  e.Subject,
		Body:     e.Body,
		HTMLBody: e.HTMLBody,
		ClientID: e.ClientID,
	}
}

func (q *InMemoryEmailQueue) SendNow(tenantID, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.Sender == nil {
		return errors.New("email sender not configured")
	}
	now := time.Now()
	for i := range q.Emails {
		item := &q.Emails[i]
		if item.ID == id && !item.Sent() && (tenantID == "" || strings.TrimSpace(item.TenantID) == strings.TrimSpace(tenantID)) {
			if err := q.Sender.Send(ToEmail(item)); err != nil {
				return err
			}
			item.SentAt = &now
			return nil
		}
	}
	return errors.New("queued email not found")
}

func (q *InMemoryEmailQueue) ProcessTenant(tenantID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.Sender == nil {
		return errors.New("email sender not configured")
	}
	now := time.Now()
	for i := range q.Emails {
		e := &q.Emails[i]
		if strings.TrimSpace(e.TenantID) != strings.TrimSpace(tenantID) || e.Sent() {
			continue
		}
		if err := q.Sender.Send(ToEmail(e)); err == nil {
			e.SentAt = &now
		}
	}
	return nil
}

func (q *InMemoryEmailQueue) ProcessBatch() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	for i := range q.Emails {
		e := &q.Emails[i]
		if !e.Sent() && !now.Before(e.ScheduledFor) {
			if err := q.Sender.Send(ToEmail(e)); err == nil {
				e.SentAt = &now
			}
		}
	}
	return nil
}

func (q *InMemoryEmailQueue) ProcessAll() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.Emails {
		e := &q.Emails[i]
		if !e.Sent() {
			if err := q.Sender.Send(ToEmail(e)); err == nil {
				e.SentAt = models.Now()
			}
		}
	}
	return nil
}
