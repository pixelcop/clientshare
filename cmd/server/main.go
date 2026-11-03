package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/getsentry/sentry-go"
	"github.com/pixelcop/clientshare/internal/clientshare"
	"github.com/pixelcop/clientshare/pkg/configschema"
	"github.com/pixelcop/clientshare/pkg/utils"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const defaultConfigPath = "config/config.yaml"

func main() {
	if err := newRootCommand(context.Background(), os.Stdout, os.Stderr).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand(ctx context.Context, stdout, stderr io.Writer) *cobra.Command {
	configPath := defaultConfigPath
	root := &cobra.Command{
		Use:           "clientshare",
		Short:         "ClientShare self-hosted client portal",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetContext(ctx)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath, "Path to YAML configuration file")
	root.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Start the ClientShare server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServer(cmd.Context(), configPath)
		},
	})
	root.AddCommand(newConfigCommand(&configPath))
	root.AddCommand(clientshare.NewStorageCommand(&configPath))
	return root
}

func runServer(ctx context.Context, configPath string) error {
	utils.InitLogger()
	defer zap.L().Sync()

	cfg, report, err := clientshare.LoadServerConfigWithReport(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logConfigReport(report)

	if cfg.SentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn: cfg.SentryDSN,
		}); err != nil {
			zap.L().Warn("Error setting up sentry", zap.Error(err))
		}
		zap.L().Info("Sentry initialized")
	} else {
		zap.L().Info("Sentry DSN not provided, skipping Sentry initialization")
	}

	// Start clientshare
	if err := clientshare.Start(ctx, cfg, zap.L()); err != nil {
		return fmt.Errorf("startup failed: %w", err)
	}
	return nil
}

func newConfigCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Verify configuration or print its JSON schema",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Validate the server configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, report, err := clientshare.LoadServerConfigWithReport(*configPath)
			if err != nil && !errors.Is(err, configschema.ErrInvalid) {
				return err
			}
			if writeErr := configschema.WriteReport(cmd.OutOrStdout(), report); writeErr != nil {
				return writeErr
			}
			return err
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "schema",
		Short: "Print the configuration JSON schema",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			schema, err := configschema.Generate(clientshare.Config{}, clientshare.ConfigSchemaOptions()...)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(schema)
			return err
		},
	})
	return cmd
}

func logConfigReport(report configschema.Report) {
	fields := []zap.Field{
		zap.String("path", report.Filename),
		zap.String("schema_version", report.SchemaVersion),
	}
	if len(report.MissingOptional) > 0 {
		fields = append(fields, zap.Strings("missing_optional_fields", report.MissingOptional))
		zap.L().Warn("Configuration verified with missing optional fields", fields...)
		return
	}
	zap.L().Info("Configuration verified", fields...)
}
