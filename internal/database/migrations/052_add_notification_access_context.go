package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddNotificationAccessContext(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	for _, field := range []string{"RecipientUserID", "RecipientLinkID", "NotificationBaseURL", "NotificationProductName"} {
		if !m.HasColumn(&models.QueuedEmail{}, field) {
			if err := m.AddColumn(&models.QueuedEmail{}, field); err != nil {
				return err
			}
		}
	}
	for _, field := range []string{"RecipientUserID", "RecipientLinkID"} {
		if !m.HasIndex(&models.QueuedEmail{}, field) {
			if err := m.CreateIndex(&models.QueuedEmail{}, field); err != nil {
				return err
			}
		}
	}
	// Legacy upload batches cannot prove which identity authorized each name
	// in their rendered payload. Cancel rather than deliver stale merged data.
	return db.WithContext(ctx).Model(&models.QueuedEmail{}).Where("sent_at IS NULL AND canceled_at IS NULL AND file_ids_csv <> '' AND recipient_user_id IS NULL AND recipient_link_id IS NULL").Update("canceled_at", models.Now()).Error
}

func downAddNotificationAccessContext(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	for _, field := range []string{"RecipientUserID", "RecipientLinkID"} {
		if err := m.DropIndex(&models.QueuedEmail{}, field); err != nil {
			return err
		}
	}
	for _, field := range []string{"RecipientUserID", "RecipientLinkID", "NotificationBaseURL", "NotificationProductName"} {
		if err := m.DropColumn(&models.QueuedEmail{}, field); err != nil {
			return err
		}
	}
	return nil
}
