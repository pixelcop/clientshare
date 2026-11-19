package email

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/access"
	"github.com/pixelcop/clientshare/internal/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if email.DeduplicationKey == "" && email.ClientID != nil && email.HideRecipients {
		var existing models.QueuedEmail
		query := q.DB.Where("client_id = ? AND file_comment_id IS NULL AND sent_at IS NULL AND canceled_at IS NULL AND hide_recipients = ? AND reply_to = ?", *email.ClientID, true, email.ReplyTo)
		if email.FileIDsCSV != "" || email.RecipientUserID != nil || email.RecipientLinkID != nil {
			query = query.Where("recipient = ?", email.To)
		}
		if email.RecipientUserID != nil {
			query = query.Where("recipient_user_id = ? AND recipient_link_id IS NULL", *email.RecipientUserID)
		} else if email.RecipientLinkID != nil {
			query = query.Where("recipient_link_id = ? AND recipient_user_id IS NULL", *email.RecipientLinkID)
		} else {
			query = query.Where("recipient_user_id IS NULL AND recipient_link_id IS NULL")
		}
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
			query = q.DB.Where("id = ?", existing.ID)
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
		TenantID:                strings.TrimSpace(email.TenantID),
		Recipient:               email.To,
		ReplyTo:                 email.ReplyTo,
		HideRecipients:          email.HideRecipients,
		Subject:                 email.Subject,
		Body:                    email.Body,
		HTMLBody:                email.HTMLBody,
		FileIDsCSV:              email.FileIDsCSV,
		ClientID:                email.ClientID,
		EventType:               "generic",
		ScheduledFor:            *sendAt,
		SecureLinkID:            email.SecureLinkID,
		RecipientUserID:         email.RecipientUserID,
		RecipientLinkID:         email.RecipientLinkID,
		NotificationBaseURL:     email.NotificationBaseURL,
		NotificationProductName: email.NotificationProductName,
		AccessInvitationID:      email.AccessInvitationID,
		IdentityInviteID:        email.IdentityInviteID,
	}
	if email.DeduplicationKey != "" {
		key := email.DeduplicationKey
		item.DeduplicationKey = &key
		result := q.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "deduplication_key"}}, DoNothing: true}).Create(&item)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			item = models.QueuedEmail{}
			if err := q.DB.Where("tenant_id = ? AND deduplication_key = ?", email.TenantID, key).First(&item).Error; err != nil {
				return nil, err
			}
			if !strings.EqualFold(item.Recipient, email.To) || !sameLink(item.SecureLinkID, email.SecureLinkID) || !sameLink(item.AccessInvitationID, email.AccessInvitationID) || !sameLink(item.IdentityInviteID, email.IdentityInviteID) {
				return nil, ErrIdempotencyConflict
			}
		}
		// Durable acceptance only. The regular worker sends this record; retries
		// cannot launch another immediate goroutine or claim SMTP delivery.
		return &item, nil
	}
	if err := q.DB.Create(&item).Error; err != nil {
		return nil, err
	}
	if item.ScheduledFor.Before(time.Now().Add(time.Second)) && q.Sender != nil {
		go q.send(item)
	}
	return &item, nil
}

func (q *DBEmailQueue) Pending(tenantID, clientID string) []*models.QueuedEmail {
	if q.DB == nil {
		return []*models.QueuedEmail{}
	}
	var items []*models.QueuedEmail
	w := q.DB.Where("sent_at IS NULL AND canceled_at IS NULL")
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
	query := q.DB.Model(&models.QueuedEmail{}).Where("id = ? AND file_comment_id IS NULL AND sent_at IS NULL AND canceled_at IS NULL", id)
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
	// Worker snapshots are not authorization or cancellation snapshots.
	var current models.QueuedEmail
	if err := q.DB.Where("tenant_id = ? AND id = ? AND sent_at IS NULL AND canceled_at IS NULL", item.TenantID, item.ID).First(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	item = current
	if item.FileCommentID != nil {
		valid, err := q.revalidateCommentNotification(&item)
		if err != nil {
			return err
		}
		if !valid {
			return q.DB.Model(&models.QueuedEmail{}).Where("tenant_id = ? AND id = ? AND sent_at IS NULL", item.TenantID, item.ID).Update("canceled_at", q.now()).Error
		}
	}
	if item.AccessInvitationID != nil || item.IdentityInviteID != nil {
		valid, err := q.accessInvitationDeliveryValid(item)
		if err != nil {
			return err
		}
		if !valid {
			return q.DB.Model(&models.QueuedEmail{}).Where("tenant_id = ? AND id = ? AND sent_at IS NULL", item.TenantID, item.ID).Update("canceled_at", q.now()).Error
		}
	}
	if item.CanceledAt != nil {
		return nil
	}
	if item.FileIDsCSV != "" {
		valid, err := q.revalidateUploadNotification(&item)
		if err != nil {
			return err
		}
		if !valid {
			return q.DB.Model(&models.QueuedEmail{}).Where("id = ? AND sent_at IS NULL", item.ID).Update("canceled_at", q.now()).Error
		}
	}
	if item.SecureLinkID != nil {
		var link models.SecureLink
		err := q.DB.Where(`tenant_id = ? AND id = ? AND revoked_at IS NULL
			AND EXISTS (SELECT 1 FROM clients WHERE clients.tenant_id = secure_links.tenant_id AND clients.id = secure_links.client_id)`, item.TenantID, *item.SecureLinkID).First(&link).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Legacy SQLite timestamps can contain different offsets. Compare the
		// actual instants in Go, not lexicographically as wall-clock strings.
		valid := !errors.Is(err, gorm.ErrRecordNotFound) && link.ExpiresAt.After(q.now())
		if valid && link.FileID != nil {
			d, accessErr := (&access.AccessService{DB: q.DB}).Resolve(context.Background(), access.AccessRequest{TenantID: item.TenantID, ClientID: link.ClientID, FileID: *link.FileID, Actor: access.AccessActor{LinkID: link.ID}, Operation: access.AccessContext, Now: q.now()})
			if accessErr != nil {
				return accessErr
			}
			valid = d.Allowed
		}
		if !valid {
			return q.DB.Model(&models.QueuedEmail{}).Where("id = ? AND sent_at IS NULL", item.ID).Update("canceled_at", q.now()).Error
		}
	}
	if q.Sender == nil {
		return errors.New("email sender not configured")
	}
	if err := q.Sender.Send(ToEmail(&item)); err != nil {
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
	query := q.DB.Where("id = ? AND sent_at IS NULL AND canceled_at IS NULL", id)
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
	if err := q.DB.Where("tenant_id = ? AND sent_at IS NULL AND canceled_at IS NULL", strings.TrimSpace(tenantID)).Order("scheduled_for ASC").Find(&items).Error; err != nil {
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
	if err := q.DB.Where("sent_at IS NULL AND canceled_at IS NULL AND scheduled_for <= ?", now).Order("scheduled_for ASC").Find(&items).Error; err != nil {
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
	if err := q.DB.Where("sent_at IS NULL AND canceled_at IS NULL").Order("scheduled_for ASC").Find(&items).Error; err != nil {
		return err
	}
	return q.process(items, now)
}

func (q *DBEmailQueue) process(items []models.QueuedEmail, _ time.Time) error {
	for _, item := range items {
		if err := q.send(item); err != nil {
			zap.L().Warn("failed to send queued email", zap.String("id", item.ID), zap.Error(err))
			continue
		}
	}
	return nil
}

func sameLink(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (q *DBEmailQueue) now() time.Time {
	if q.Now != nil {
		return q.Now()
	}
	return time.Now()
}
