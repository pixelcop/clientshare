package main

import (
	"context"
	"database/sql"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddInvitationDeliveryContext(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	for _, field := range []string{"AccessInvitationID", "IdentityInviteID"} {
		if !m.HasColumn(&models.QueuedEmail{}, field) {
			if err := m.AddColumn(&models.QueuedEmail{}, field); err != nil {
				return err
			}
		}
		if !m.HasIndex(&models.QueuedEmail{}, field) {
			if err := m.CreateIndex(&models.QueuedEmail{}, field); err != nil {
				return err
			}
		}
	}
	return nil
}

func downAddInvitationDeliveryContext(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	for _, field := range []string{"AccessInvitationID", "IdentityInviteID"} {
		if err := m.DropIndex(&models.QueuedEmail{}, field); err != nil {
			return err
		}
		if err := m.DropColumn(&models.QueuedEmail{}, field); err != nil {
			return err
		}
	}
	return nil
}
