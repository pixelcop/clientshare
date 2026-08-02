package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddPasskeySupport(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}

	m := db.WithContext(ctx).Migrator()
	if !m.HasColumn(&models.User{}, "passkey_prompt_dismissed") {
		if err := m.AddColumn(&models.User{}, "PasskeyPromptDismissed"); err != nil {
			return fmt.Errorf("add users.passkey_prompt_dismissed: %w", err)
		}
	}
	if err := db.WithContext(ctx).AutoMigrate(&models.PasskeyCredential{}, &models.WebAuthnChallenge{}); err != nil {
		return fmt.Errorf("create passkey tables: %w", err)
	}
	return nil
}

func downAddPasskeySupport(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}

	m := db.WithContext(ctx).Migrator()
	if err := m.DropTable(&models.WebAuthnChallenge{}, &models.PasskeyCredential{}); err != nil {
		return fmt.Errorf("drop passkey tables: %w", err)
	}
	if m.HasColumn(&models.User{}, "passkey_prompt_dismissed") {
		if err := m.DropColumn(&models.User{}, "PasskeyPromptDismissed"); err != nil {
			return fmt.Errorf("drop users.passkey_prompt_dismissed: %w", err)
		}
	}
	return nil
}
