package main

import (
	"github.com/pixelcop/clientshare/internal/clientshare"
	"github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/pressly/goose/v3"
)

func main() {
	goMigrations := []*goose.Migration{
		migration.NewGoMigration(34, upCreateCurrentSchema, downCreateCurrentSchema),
		migration.NewGoMigration(35, upSeedDefaultTenant, downSeedDefaultTenant),
		migration.NewGoMigration(36, upBackfillTenantDomainsFromPublicBaseURLs, downBackfillTenantDomainsFromPublicBaseURLs),
		migration.NewGoMigration(37, upAddFilesDiskDeletedAt, downAddFilesDiskDeletedAt),
		migration.NewGoMigration(38, upAddPasskeySupport, downAddPasskeySupport),
		migration.NewGoMigration(39, upAddHostedPasskeyHandoffs, downAddHostedPasskeyHandoffs),
		migration.NewGoMigration(40, upAddPasskeySignups, downAddPasskeySignups),
	}

	migration.Run(goMigrations, func(path string) (db.DatabaseConfig, error) {
		cfg, err := clientshare.LoadConfig(path)
		if err != nil {
			return db.DatabaseConfig{}, err
		}
		return cfg.Database, nil
	})

}
