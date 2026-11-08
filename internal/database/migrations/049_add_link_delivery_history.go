package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddLinkDeliveryHistory(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	for _, change := range []struct {
		model           any
		fields, indexes []string
	}{
		{&models.SecureLink{}, []string{"RevokedAt", "CreationKey", "CreationHash"}, []string{"RevokedAt", "idx_secure_links_creation_key"}},
		{&models.QueuedEmail{}, []string{"DeduplicationKey", "SecureLinkID", "CanceledAt"}, []string{"idx_email_queue_deduplication_key", "SecureLinkID"}},
	} {
		for _, field := range change.fields {
			if !m.HasColumn(change.model, field) {
				if err := m.AddColumn(change.model, field); err != nil {
					return err
				}
			}
		}
		for _, index := range change.indexes {
			if !m.HasIndex(change.model, index) {
				if err := m.CreateIndex(change.model, index); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func downAddLinkDeliveryHistory(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	for _, change := range []struct {
		model           any
		fields, indexes []string
	}{
		{&models.QueuedEmail{}, []string{"DeduplicationKey", "SecureLinkID", "CanceledAt"}, []string{"idx_email_queue_deduplication_key", "SecureLinkID"}},
		{&models.SecureLink{}, []string{"RevokedAt", "CreationKey", "CreationHash"}, []string{"RevokedAt", "idx_secure_links_creation_key"}},
	} {
		for _, index := range change.indexes {
			if m.HasIndex(change.model, index) {
				if err := m.DropIndex(change.model, index); err != nil {
					return err
				}
			}
		}
		for _, field := range change.fields {
			if m.HasColumn(change.model, field) {
				if err := m.DropColumn(change.model, field); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
