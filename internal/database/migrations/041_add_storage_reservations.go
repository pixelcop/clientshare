package main

import (
	"context"
	"database/sql"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upAddStorageReservations(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).AutoMigrate(&models.StorageReservation{})
}

func downAddStorageReservations(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Migrator().DropTable(&models.StorageReservation{})
}
