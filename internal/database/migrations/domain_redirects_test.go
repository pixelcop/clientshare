package main

import (
	"context"
	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pressly/goose/v3"
	"testing"
)

func TestDomainRedirectMigration(t *testing.T) {
	db := openMigrationTestDB(t)
	p := newMigrationProviderForTest(t, db, goose.DialectSQLite3, dbpkg.DatabaseConfig{Driver: dbpkg.DatabaseDriverSQLite})
	ctx := context.Background()
	if _, err := p.UpTo(ctx, 43); err != nil {
		t.Fatal(err)
	}
	// The baseline uses current models; remove the field to simulate an existing installation.
	if _, err := db.Exec(`ALTER TABLE tenant_domains DROP COLUMN redirect_to_primary`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tenant_domains WHERE redirect_to_primary <> false`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("old domains should not redirect: %d %v", count, err)
	}
	if _, err := p.DownTo(ctx, 43); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT redirect_to_primary FROM tenant_domains`); err == nil {
		t.Fatal("column survived rollback")
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
}
