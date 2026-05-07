package email

import (
	"errors"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type DBEmailQueue struct {
	defaultBatchDelay time.Duration
	DB                *gorm.DB
	Sender            EmailSender
	Now               func() time.Time
}

func NewDBEmailQueue(db *gorm.DB, sender EmailSender, defaultBatchDelay time.Duration) *DBEmailQueue {
	return &DBEmailQueue{DB: db, Sender: sender, Now: time.Now, defaultBatchDelay: defaultBatchDelay}
}

func (q *DBEmailQueue) Queue(email Email, sendAt *time.Time) (*models.QueuedEmail, error) {
	if q.DB == nil {
		return nil, errors.New("email queue database not configured")
	}

	if sendAt == nil {
		t := q.now().Add(q.defaultBatchDelay)
		sendAt = &t
	}

	// look for existing queued email for the same client
	if email.ClientID != nil {
		var existing models.QueuedEmail
		query := q.DB.Where("client_id = ? AND sent_at IS NULL", *email.ClientID)
		if strings.TrimSpace(email.TenantID) != "" {
			query = query.Where("tenant_id = ?", strings.TrimSpace(email.TenantID))
		}
		err := query.Order("scheduled_for ASC").First(&existing).Error
		if err == nil {
			updates := map[string]interface{}{
				"recipient":     mergeRecipients(existing.Recipient, email.To),
				"subject":       email.Subject,
				"body":          mergeBodies(existing.Body, email.Body),
				"html_body":     mergeHTMLBodies(existing.HTMLBody, email.HTMLBody),
				"file_ids_csv":  mergeCSVFileIDs(existing.FileIDsCSV, email.FileIDsCSV),
				"scheduled_for": *sendAt,
			}
			err = q.DB.Model(&models.QueuedEmail{}).Where("id = ?", existing.ID).Updates(updates).Error
			if err != nil {
				return nil, err
			}
			query = q.DB.Where("client_id = ? AND sent_at IS NULL", *email.ClientID)
			if strings.TrimSpace(email.TenantID) != "" {
				query = query.Where("tenant_id = ?", strings.TrimSpace(email.TenantID))
			}
			err = query.Order("scheduled_for ASC").First(&existing).Error
			if err != nil {
				return nil, err
			}
			if existing.ScheduledFor.Before(time.Now().Add(time.Second)) {
				go q.send(existing) // send immediately
			}
			return &existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	// new queue item
	item := models.QueuedEmail{
		TenantID:     strings.TrimSpace(email.TenantID),
		Recipient:    email.To,
		Subject:      email.Subject,
		Body:         email.Body,
		HTMLBody:     email.HTMLBody,
		FileIDsCSV:   email.FileIDsCSV,
		ClientID:     email.ClientID,
		EventType:    "generic",
		ScheduledFor: *sendAt,
	}
	if item.ScheduledFor.Before(time.Now().Add(time.Second)) {
		go q.send(item) // send immediately
		item.SentAt = models.Now()
	}
	return &item, q.DB.Create(&item).Error
}

func (q *DBEmailQueue) Pending(tenantID, clientID string) []*models.QueuedEmail {
	if q.DB == nil {
		return []*models.QueuedEmail{}
	}
	var items []*models.QueuedEmail
	w := q.DB.Where("sent_at IS NULL")
	if strings.TrimSpace(tenantID) != "" {
		w = w.Where("tenant_id = ?", strings.TrimSpace(tenantID))
	}
	if clientID != "" {
		w = w.Where("client_id = ?", clientID)
	}
	if err := w.Order("scheduled_for ASC").Find(&items).Error; err != nil {
		zap.L().Warn("failed to list pending email queue items", zap.Error(err))
		return []*models.QueuedEmail{}
	}
	return items
}

func (q *DBEmailQueue) Update(tenantID, id, recipient, subject, body string) error {
	if q.DB == nil {
		return errors.New("email queue database not configured")
	}
	updates := map[string]interface{}{
		"recipient": recipient,
		"subject":   subject,
		"body":      body,
	}
	query := q.DB.Model(&models.QueuedEmail{}).Where("id = ? AND sent_at IS NULL", id)
	if strings.TrimSpace(tenantID) != "" {
		query = query.Where("tenant_id = ?", strings.TrimSpace(tenantID))
	}
	res := query.Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("queued email not found")
	}
	return nil
}

func (q *DBEmailQueue) send(item models.QueuedEmail) error {
	if err := q.Sender.Send(Email{TenantID: item.TenantID, To: item.Recipient, Subject: item.Subject, Body: item.Body, HTMLBody: item.HTMLBody, ClientID: item.ClientID}); err != nil {
		// TODO: store err w/ item so we dont' retry? or retry max 3 times or something?
		return err
	}
	sentAt := q.now()
	return q.DB.Model(&models.QueuedEmail{}).
		Where("id = ? AND sent_at IS NULL", item.ID).
		Update("sent_at", sentAt).Error
}

func (q *DBEmailQueue) SendNow(tenantID, id string) error {
	if q.DB == nil {
		return errors.New("email queue database not configured")
	}
	if q.Sender == nil {
		return errors.New("email sender not configured")
	}
	var item models.QueuedEmail
	query := q.DB.Where("id = ? AND sent_at IS NULL", id)
	if strings.TrimSpace(tenantID) != "" {
		query = query.Where("tenant_id = ?", strings.TrimSpace(tenantID))
	}
	if err := query.First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("queued email not found")
		}
		return err
	}
	return q.send(item)
}

func (q *DBEmailQueue) ProcessTenant(tenantID string) error {
	if q.DB == nil {
		return errors.New("email queue database not configured")
	}
	if q.Sender == nil {
		return errors.New("email sender not configured")
	}
	now := q.now()
	var items []models.QueuedEmail
	if err := q.DB.Where("tenant_id = ? AND sent_at IS NULL", strings.TrimSpace(tenantID)).Order("scheduled_for ASC").Find(&items).Error; err != nil {
		return err
	}
	return q.process(items, now)
}

func (q *DBEmailQueue) ProcessBatch() error {
	if q.DB == nil {
		return errors.New("email queue database not configured")
	}
	if q.Sender == nil {
		return errors.New("email sender not configured")
	}
	now := q.now()
	var items []models.QueuedEmail
	if err := q.DB.Where("sent_at IS NULL AND scheduled_for <= ?", now).Order("scheduled_for ASC").Find(&items).Error; err != nil {
		return err
	}
	return q.process(items, now)
}

func (q *DBEmailQueue) ProcessAll() error {
	if q.DB == nil {
		return errors.New("email queue database not configured")
	}
	if q.Sender == nil {
		return errors.New("email sender not configured")
	}
	now := q.now()
	var items []models.QueuedEmail
	if err := q.DB.Where("sent_at IS NULL").Order("scheduled_for ASC").Find(&items).Error; err != nil {
		return err
	}
	return q.process(items, now)
}

func (q *DBEmailQueue) process(items []models.QueuedEmail, sentAt time.Time) error {
	for _, item := range items {
		if err := q.Sender.Send(Email{To: item.Recipient, Subject: item.Subject, Body: item.Body, HTMLBody: item.HTMLBody, ClientID: item.ClientID}); err != nil {
			zap.L().Warn("failed to send queued email", zap.String("id", item.ID), zap.Error(err))
			continue
		}
		if err := q.DB.Model(&models.QueuedEmail{}).
			Where("id = ? AND sent_at IS NULL", item.ID).
			Update("sent_at", sentAt).Error; err != nil {
			return err
		}
	}
	return nil
}

func (q *DBEmailQueue) now() time.Time {
	if q.Now != nil {
		return q.Now()
	}
	return time.Now()
}
