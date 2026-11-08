package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddAccessInvitations(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	m := db.Migrator()
	for _, change := range []struct {
		model any
		field string
	}{
		{&models.ItemGrant{}, "Version"}, {&models.InviteToken{}, "AccessInvitationID"},
	} {
		if !m.HasColumn(change.model, change.field) {
			if err := m.AddColumn(change.model, change.field); err != nil {
				return err
			}
		}
	}
	if !m.HasIndex(&models.InviteToken{}, "AccessInvitationID") {
		if err := m.CreateIndex(&models.InviteToken{}, "AccessInvitationID"); err != nil {
			return err
		}
	}
	if err := db.AutoMigrate(&models.AccessInvitation{}); err != nil {
		return err
	}
	return db.Exec(`CREATE UNIQUE INDEX idx_access_invitations_pending ON access_invitations (tenant_id, client_id, COALESCE(file_id, ''), email) WHERE revoked_at IS NULL AND accepted_at IS NULL`).Error
}

func downAddAccessInvitations(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	if err := m.DropTable(&models.AccessInvitation{}); err != nil {
		return err
	}
	if err := m.DropIndex(&models.InviteToken{}, "AccessInvitationID"); err != nil {
		return err
	}
	if err := m.DropColumn(&models.InviteToken{}, "AccessInvitationID"); err != nil {
		return err
	}
	return m.DropColumn(&models.ItemGrant{}, "Version")
}
