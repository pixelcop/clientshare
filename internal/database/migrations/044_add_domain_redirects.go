package main

import (
	"context"
	"database/sql"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddDomainRedirects(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	if db.Migrator().HasColumn(&models.TenantDomain{}, "RedirectToPrimary") {
		return nil
	}
	return db.WithContext(ctx).Migrator().AddColumn(&models.TenantDomain{}, "RedirectToPrimary")
}

func downAddDomainRedirects(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Migrator().DropColumn(&models.TenantDomain{}, "RedirectToPrimary")
}
