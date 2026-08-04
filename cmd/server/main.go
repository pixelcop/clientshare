package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/pixelcop/clientshare/internal/clientshare"
	"github.com/pixelcop/clientshare/pkg/configschema"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
)

const defaultConfigPath = "config/config.yaml"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "config" {
		if err := runConfigCommand(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			if !errors.Is(err, configschema.ErrInvalid) {
				fmt.Fprintln(os.Stderr, err)
			}
			os.Exit(1)
		}
		return
	}

	utils.InitLogger()
	defer zap.L().Sync()

	cfg, report, err := clientshare.LoadServerConfigWithReport(defaultConfigPath)
	if err != nil {
		zap.S().Fatalf("Error loading config: %v", err)
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
	ctx := context.Background()
	if err := clientshare.Start(ctx, cfg, zap.L()); err != nil {
		zap.L().Fatal("Startup failed", zap.Error(err))
	}
}

func runConfigCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: clientshare config <verify|schema>")
	}

	switch args[0] {
	case "verify":
		flags := flag.NewFlagSet("clientshare config verify", flag.ContinueOnError)
		flags.SetOutput(stderr)
		configPath := flags.String("config", defaultConfigPath, "path to the YAML configuration file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments for config verify: %s", strings.Join(flags.Args(), " "))
		}
		_, report, err := clientshare.LoadServerConfigWithReport(*configPath)
		if err != nil && !errors.Is(err, configschema.ErrInvalid) {
			return err
		}
		if writeErr := configschema.WriteReport(stdout, report); writeErr != nil {
			return writeErr
		}
		if err != nil {
			return err
		}
		return nil
	case "schema":
		flags := flag.NewFlagSet("clientshare config schema", flag.ContinueOnError)
		flags.SetOutput(stderr)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments for config schema: %s", strings.Join(flags.Args(), " "))
		}
		schema, err := configschema.Generate(clientshare.Config{}, clientshare.ConfigSchemaOptions()...)
		if err != nil {
			return err
		}
		_, err = stdout.Write(schema)
		return err
	default:
		return fmt.Errorf("unknown config command %q; expected verify or schema", args[0])
	}
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
