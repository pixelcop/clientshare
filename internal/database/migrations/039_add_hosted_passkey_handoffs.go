package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddHostedPasskeyHandoffs(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	if !m.HasColumn(&models.PasskeyCredential{}, "RPID") {
		if err := m.AddColumn(&models.PasskeyCredential{}, "RPID"); err != nil {
			return fmt.Errorf("add passkey_credentials.rp_id: %w", err)
		}
	}
	if !m.HasColumn(&models.WebAuthnChallenge{}, "RPID") {
		if err := m.AddColumn(&models.WebAuthnChallenge{}, "RPID"); err != nil {
			return fmt.Errorf("add webauthn_challenges.rp_id: %w", err)
		}
	}
	if err := db.WithContext(ctx).AutoMigrate(&models.HostedLoginHandoff{}); err != nil {
		return fmt.Errorf("create hosted login handoffs: %w", err)
	}
	return nil
}

func downAddHostedPasskeyHandoffs(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	m := db.WithContext(ctx).Migrator()
	if err := m.DropTable(&models.HostedLoginHandoff{}); err != nil {
		return fmt.Errorf("drop hosted login handoffs: %w", err)
	}
	if m.HasColumn(&models.WebAuthnChallenge{}, "RPID") {
		if err := m.DropColumn(&models.WebAuthnChallenge{}, "RPID"); err != nil {
			return fmt.Errorf("drop webauthn_challenges.rp_id: %w", err)
		}
	}
	if m.HasColumn(&models.PasskeyCredential{}, "RPID") {
		if err := m.DropColumn(&models.PasskeyCredential{}, "RPID"); err != nil {
			return fmt.Errorf("drop passkey_credentials.rp_id: %w", err)
		}
	}
	return nil
}
