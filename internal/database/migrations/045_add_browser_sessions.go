package main

import (
	"context"
	"database/sql"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddBrowserSessions(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	if err := db.WithContext(ctx).AutoMigrate(&models.BrowserSession{}); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&models.HostedLoginHandoff{}, "SessionGroupID") {
		if err := db.Migrator().AddColumn(&models.HostedLoginHandoff{}, "SessionGroupID"); err != nil {
			return err
		}
	}
	if !db.Migrator().HasColumn(&models.WebAuthnChallenge{}, "BrowserSessionID") {
		return db.Migrator().AddColumn(&models.WebAuthnChallenge{}, "BrowserSessionID")
	}
	return nil
}

func downAddBrowserSessions(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	if err := db.Migrator().DropColumn(&models.WebAuthnChallenge{}, "BrowserSessionID"); err != nil {
		return err
	}
	if err := db.Migrator().DropColumn(&models.HostedLoginHandoff{}, "SessionGroupID"); err != nil {
		return err
	}
	return db.Migrator().DropTable(&models.BrowserSession{})
}
