package main

import (
	"context"
	"database/sql"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/migration"
)

func upDocumentRequests(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).AutoMigrate(&models.DocumentRequest{}, &models.DocumentRequestItem{}, &models.DocumentRequestFile{}, &models.DocumentRequestReview{})
}

func downDocumentRequests(ctx context.Context, tx *sql.Tx) error {
	db, err := migration.MigrationTxGormDB(tx)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Migrator().DropTable(&models.DocumentRequestReview{}, &models.DocumentRequestFile{}, &models.DocumentRequestItem{}, &models.DocumentRequest{})
}
