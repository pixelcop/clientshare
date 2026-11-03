package clientshare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// StorageCommand is deliberately offline: no HTTP server, emails or workers.
func StorageCommand(ctx context.Context, args []string, out io.Writer) error {
	configPath := "config/config.yaml"
	cmd := NewStorageCommand(&configPath)
	cmd.PersistentFlags().StringVar(&configPath, "config", configPath, "Path to YAML configuration file")
	cmd.SetArgs(args)
	cmd.SetOut(out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd.ExecuteContext(ctx)
}

// NewStorageCommand builds offline storage maintenance commands; it never starts
// the HTTP server, email delivery or background workers.
func NewStorageCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage",
		Short: "Inspect and maintain stored objects offline",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	for _, operation := range []struct{ name, description string }{
		{"inventory", "Inventory legacy objects for migration"},
		{"migrate", "Migrate objects using a reviewed inventory"},
		{"shard-objects", "Convert flat immutable objects to hash buckets offline"},
		{"verify", "Verify retained objects"},
		{"view-status", "Show readable view status"},
		{"view-reconcile", "Reconcile readable views"},
		{"view-clear", "Clear previously managed readable views"},
		{"orphan-cleanup", "Clean up unreferenced immutable objects"},
		{"legacy-cleanup", "Clean up legacy objects after migration"},
	} {
		cmd.AddCommand(newStorageOperation(operation.name, operation.description, configPath))
	}
	return cmd
}

func newStorageOperation(operation, description string, config *string) *cobra.Command {
	cmd := &cobra.Command{Use: operation, Short: description, Args: cobra.NoArgs}
	flags := cmd.Flags()
	manifest := flags.String("manifest", "", "inventory JSON path (required for migrate)")
	overrides := flags.String("sources", "", "reviewed JSON map of tenant/file IDs to tenant-scoped source paths")
	missing := flags.String("acknowledge-missing", "", "reviewed JSON array of tenant/file IDs whose bytes were already missing")
	mode := flags.String("view-mode", "", "explicit migration mode: none, hardlink, symlink, export")
	apply := flags.Bool("apply", false, "apply changes; otherwise report only")
	tenant := flags.String("tenant", "", "filter view status/reconcile by tenant")
	client := flags.String("client", "", "filter view status/reconcile by client")
	grace := flags.Duration("grace", 7*24*time.Hour, "minimum orphan age (at least 24h)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		ctx, out := cmd.Context(), cmd.OutOrStdout()
		cfg, err := LoadConfig(*config)
		if err != nil {
			return err
		}
		dialector, err := cfg.Database.GormDialector()
		if err != nil {
			return err
		}
		db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			return err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		defer sqlDB.Close()
		if operation == "shard-objects" {
			if cfg.Storage.Type != StorageLocal {
				return errors.New("shard-objects supports local storage only")
			}
			if cfg.Storage.Local.ReadableView.Mode != "" && cfg.Storage.Local.ReadableView.Mode != services.ViewModeNone {
				return errors.New("clear readable views using the old version and configure mode none before sharding")
			}
			return services.ShardFlatObjects(ctx, db, cfg.Storage.Local.Root, *apply, out)
		}
		if operation == "orphan-cleanup" || operation == "legacy-cleanup" {
			if cfg.Storage.Type != StorageLocal {
				return errors.New("cleanup command supports local storage only")
			}
			if operation == "orphan-cleanup" {
				return services.CleanupOrphanObjects(ctx, db, cfg.Storage.Local.Root, *grace, *apply, out)
			}
			if *manifest == "" {
				return errors.New("legacy-cleanup requires original --manifest; retain a verified full backup")
			}
			b, err := os.ReadFile(*manifest)
			if err != nil {
				return err
			}
			var plan services.ObjectMigrationPlan
			if err := json.Unmarshal(b, &plan); err != nil {
				return err
			}
			return services.CleanupLegacyObjects(ctx, db, cfg.Storage.Local.Root, plan, *apply, out)
		}
		if operation == "inventory" || operation == "migrate" {
			if cfg.Storage.Type != StorageLocal {
				return errors.New("legacy migration supports local storage only")
			}
			if operation == "inventory" {
				var sources map[string]string
				if *overrides != "" {
					b, err := os.ReadFile(*overrides)
					if err != nil {
						return err
					}
					if err := json.Unmarshal(b, &sources); err != nil {
						return err
					}
				}
				var missingIDs []string
				if *missing != "" {
					b, err := os.ReadFile(*missing)
					if err != nil {
						return err
					}
					if err := json.Unmarshal(b, &missingIDs); err != nil {
						return err
					}
				}
				plan, err := services.InventoryObjects(ctx, db, cfg.Storage.Local.Root, sources, missingIDs...)
				if err != nil {
					return err
				}
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(plan); err != nil {
					return err
				}
				if len(plan.Problems) > 0 {
					return fmt.Errorf("inventory blocked by %d problems", len(plan.Problems))
				}
				return nil
			}
			if *manifest == "" || *mode == "" {
				return errors.New("migrate requires --manifest and explicit --view-mode (production: none)")
			}
			views, err := services.NewReadableViews(db, cfg.Storage.Local.Root, cfg.Storage.Local.ReadableView.Root, *mode)
			if err != nil {
				return err
			}
			if cfg.Storage.Local.ReadableView.Mode != "" && cfg.Storage.Local.ReadableView.Mode != *mode {
				return errors.New("migration view mode differs from config")
			}
			b, err := os.ReadFile(*manifest)
			if err != nil {
				return err
			}
			var plan services.ObjectMigrationPlan
			if err := json.Unmarshal(b, &plan); err != nil {
				return err
			}
			if !*apply {
				if err := services.VerifyMigrationPlan(ctx, cfg.Storage.Local.Root, plan); err != nil {
					return err
				}
				fmt.Fprintf(out, "Would verify/copy %d objects and cut over in one database transaction; %d unresolved problems. Legacy sources remain untouched.\n", len(plan.Entries), len(plan.Problems))
				if len(plan.Problems) > 0 {
					return errors.New("resolve inventory problems first")
				}
				return nil
			}
			lock, err := views.Lock()
			if err != nil {
				return err
			}
			defer lock.Close()
			if err := views.Probe(); err != nil {
				return err
			}
			if err := services.ApplyObjects(ctx, db, cfg.Storage.Local.Root, plan); err != nil {
				return err
			}
			missingCount := 0
			for _, e := range plan.Entries {
				if e.Missing {
					missingCount++
				}
			}
			fmt.Fprintf(out, "Verified objects and committed database cutover; %d explicitly acknowledged missing records. Set storage.layout: %s and readable_view.mode: %s\n", missingCount, storage.ImmutableLayout, *mode)
			return nil
		}
		if operation == "verify" {
			store, err := BuildStorage(ctx, cfg)
			if err != nil {
				return err
			}
			var files []models.File
			if err := db.Where("type <> 'folder' AND disk_deleted_at IS NULL").Find(&files).Error; err != nil {
				return err
			}
			missingCount := 0
			for _, f := range files {
				if f.StorageMissing {
					missingCount++
					continue
				}
				if f.StorageKey == nil {
					return fmt.Errorf("unmigrated file %s/%s", f.TenantID, f.ID)
				}
				info, err := store.Tenant(f.TenantID).Stat(ctx, *f.StorageKey)
				if err != nil {
					return err
				}
				if info.Size != f.Size || info.Checksum != f.Checksum {
					return fmt.Errorf("verification failed: %s/%s", f.TenantID, f.ID)
				}
			}
			fmt.Fprintf(out, "Verified %d retained objects; %d acknowledged missing records have no bytes.\n", len(files)-missingCount, missingCount)
			return nil
		}
		var states []models.ReadableView
		q := db.Order("tenant_id, client_id")
		if *tenant != "" {
			q = q.Where("tenant_id = ?", *tenant)
		}
		if *client != "" {
			q = q.Where("client_id = ?", *client)
		}
		if err := q.Find(&states).Error; err != nil {
			return err
		}
		if operation == "view-status" {
			return json.NewEncoder(out).Encode(states)
		}
		views, err := services.NewReadableViews(db, cfg.Storage.Local.Root, cfg.Storage.Local.ReadableView.Root, cfg.Storage.Local.ReadableView.Mode)
		if err != nil {
			return err
		}
		if *apply {
			lock, err := views.Lock()
			if err != nil {
				return err
			}
			defer lock.Close()
			if err := views.Probe(); err != nil {
				return err
			}
		}
		for _, state := range states {
			if operation == "view-clear" {
				var previous services.ViewManifest
				if state.Manifest == "" {
					continue
				}
				if err := json.Unmarshal([]byte(state.Manifest), &previous); err != nil {
					return err
				}
				if views.Mode != previous.Mode || (views.Mode != services.ViewModeNone && views.Root != previous.Root) {
					return errors.New("restore the manifest's old mode/root configuration before view-clear")
				}
				views.Clear = true
			}
			if err := views.Reconcile(ctx, state, !*apply, out); err != nil {
				return err
			}
			if operation == "view-clear" && *apply {
				if err := db.Model(&models.ReadableView{}).Where("tenant_id = ? AND client_id = ?", state.TenantID, state.ClientID).Updates(map[string]interface{}{"manifest": "", "applied_revision": 0}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}
	return cmd
}
