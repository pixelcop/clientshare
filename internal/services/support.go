package services

import (
	"context"
	"errors"
	"fmt"
	"github.com/pixelcop/clientshare/internal/models"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

var ErrSupportConflict = errors.New("retry key already used for another message")

type SupportService struct{ DB *gorm.DB }

func (s SupportService) Receive(ctx context.Context, row *models.SupportMessage) error {
	row.NextAttemptAt = time.Now()
	result := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var existing models.SupportMessage
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND requester_id = ? AND retry_key = ?", row.TenantID, row.RequesterID, row.RetryKey).First(&existing).Error; err != nil {
		return err
	}
	if existing.Message != row.Message {
		return ErrSupportConflict
	}
	*row = existing
	return nil
}

// Persisted leases recover interrupted attempts. SMTP is at-least-once.
func (s SupportService) Notify(sender emailpkg.EmailSender) error {
	var rows []models.SupportMessage
	now := time.Now()
	if err := s.DB.Where("notified_at IS NULL AND next_attempt_at <= ?", now).Order("created_at").Limit(20).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		lease := s.DB.Model(&models.SupportMessage{}).Where("id = ? AND notified_at IS NULL AND next_attempt_at <= ?", row.ID, now).Updates(map[string]any{"next_attempt_at": now.Add(10 * time.Minute), "attempts": gorm.Expr("attempts + 1")})
		if lease.Error != nil {
			return lease.Error
		}
		if lease.RowsAffected != 1 {
			continue
		}
		err := sender.Send(emailpkg.Email{TenantID: row.TenantID, To: row.Destination, ReplyTo: row.Email, Subject: "ClientShare support [" + row.ID + "]", Body: fmt.Sprintf("Request: %s\nProvider: %s\nTenant: %s\nRequester: %s (%s)\nReply to: %s <%s>\nSubmitted: %s\nScreen: %s\nVersion: %s\n\n%s", row.ID, row.Provider, row.TenantID, row.RequesterID, row.Role, row.Name, row.Email, row.CreatedAt.Format(time.RFC3339), row.Screen, row.Version, row.Message)})
		if err != nil {
			continue
		}
		if err := s.DB.Model(&models.SupportMessage{}).Where("id = ?", row.ID).Update("notified_at", time.Now()).Error; err != nil {
			return err
		}
	}
	return nil
}
