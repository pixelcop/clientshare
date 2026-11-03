package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pixelcop/clientshare/pkg/configschema"
	"github.com/stretchr/testify/require"
)

func TestRunConfigVerifyReportsMissingOptionalFields(t *testing.T) {
	filename := writeCommandConfig(t, validServerConfig+"server:\n  port: 8080\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runConfigCommand([]string{"verify", "--config", filename}, &stdout, &stderr)
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "Status: valid")
	require.Contains(t, stdout.String(), "Missing optional fields")
	require.Contains(t, stdout.String(), `auth.password_reset_duration (default: "30m"; since: 2026-02-09)`)
	require.Contains(t, stdout.String(), "auth.rate_limit_enabled (default: true; since: 2026-04-17)")
	require.Contains(t, stdout.String(), "sentry_dsn (since: 2026-04-16)")
}

func TestRunConfigVerifyReturnsInvalidForUnknownField(t *testing.T) {
	filename := writeCommandConfig(t, validServerConfig+"unknown: true\n")
	var stdout bytes.Buffer

	err := runConfigCommand([]string{"verify", "--config", filename}, &stdout, &bytes.Buffer{})
	require.ErrorIs(t, err, configschema.ErrInvalid)
	require.Contains(t, stdout.String(), "Status: invalid")
	require.Contains(t, stdout.String(), "unknown")
}

func TestRunConfigVerifyRunsServerValidation(t *testing.T) {
	filename := writeCommandConfig(t, "auth:\n  jwt_secret: jwt-secret\n  invite_token_secret: invite-secret\n  hosted_passkey_origin: ftp://example.com\ndatabase:\n  path: ':memory:'\nsecure_links:\n  signing_key: signing-key\nstorage:\n  type: local\n  local:\n    root: /tmp/storage\n")
	var stdout bytes.Buffer

	err := runConfigCommand([]string{"verify", "--config", filename}, &stdout, &bytes.Buffer{})
	require.ErrorIs(t, err, configschema.ErrInvalid)
	require.Contains(t, stdout.String(), "Status: invalid")
	require.Contains(t, stdout.String(), "auth.hosted_passkey_origin must use http or https")
}

func TestRunConfigVerifyChecksDatabaseAndStorage(t *testing.T) {
	t.Run("database", func(t *testing.T) {
		filename := writeCommandConfig(t, validCoreConfig+"database:\n  driver: invalid\nstorage:\n  type: local\n  local:\n    root: /tmp/storage\n")
		var stdout bytes.Buffer

		err := runConfigCommand([]string{"verify", "--config", filename}, &stdout, &bytes.Buffer{})
		require.ErrorIs(t, err, configschema.ErrInvalid)
		require.Contains(t, stdout.String(), "unsupported database driver")
	})

	t.Run("storage", func(t *testing.T) {
		filename := writeCommandConfig(t, validCoreConfig+"database:\n  path: ':memory:'\nstorage:\n  type: invalid\n")
		var stdout bytes.Buffer

		err := runConfigCommand([]string{"verify", "--config", filename}, &stdout, &bytes.Buffer{})
		require.ErrorIs(t, err, configschema.ErrInvalid)
		require.Contains(t, stdout.String(), "value must be one of 'local', 's3'")
	})
}

func TestRunConfigSchemaWritesCurrentSchema(t *testing.T) {
	var stdout bytes.Buffer

	err := runConfigCommand([]string{"schema"}, &stdout, &bytes.Buffer{})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), `"$schema": "https://json-schema.org/draft/2020-12/schema"`)
	require.Contains(t, stdout.String(), `"vite_port"`)
	require.Contains(t, stdout.String(), `"default": 5193`)
	require.Contains(t, stdout.String(), `"x-since": "2026-08-01"`)
	require.NotContains(t, stdout.String(), `"BuildTime"`)
}

func TestRunConfigCommandRejectsUnknownCommand(t *testing.T) {
	err := runConfigCommand([]string{"unknown"}, &bytes.Buffer{}, &bytes.Buffer{})
	require.ErrorContains(t, err, "unknown command")
}

func runConfigCommand(args []string, stdout, stderr io.Writer) error {
	cmd := newRootCommand(context.Background(), stdout, stderr)
	cmd.SetArgs(append([]string{"config"}, args...))
	return cmd.Execute()
}

func TestCommandHelpDoesNotLoadConfig(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--help"}, {"help"}, {"serve", "--help"},
		{"config"}, {"config", "verify", "--help"}, {"config", "schema", "--help"},
		{"storage"}, {"storage", "--help"},
		{"storage", "inventory", "--help"}, {"storage", "migrate", "--help"},
		{"storage", "verify", "--help"}, {"storage", "view-status", "--help"},
		{"storage", "view-reconcile", "--help"}, {"storage", "view-clear", "--help"},
		{"storage", "orphan-cleanup", "--help"}, {"storage", "legacy-cleanup", "--help"},
	} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := newRootCommand(context.Background(), &stdout, &stderr)
			cmd.SetArgs(append([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml")}, args...))
			require.NoError(t, cmd.Execute())
			require.Contains(t, stdout.String(), "Usage:")
			require.Empty(t, stderr.String())
		})
	}
}

func TestCommandRejectsInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{"unknown"}, {"--unknown"}, {"serve", "extra"}, {"serve", "--unknown"},
		{"config", "unknown"}, {"config", "verify", "extra"}, {"config", "schema", "extra"},
		{"storage", "unknown"}, {"storage", "verify", "extra"},
		{"storage", "migrate", "--unknown"}, {"storage", "orphan-cleanup", "--grace", "invalid"},
	} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			cmd := newRootCommand(context.Background(), io.Discard, io.Discard)
			cmd.SetArgs(args)
			require.Error(t, cmd.Execute())
		})
	}
}

func TestServeUsesConfigFlag(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "missing.yaml")
	cmd := newRootCommand(context.Background(), io.Discard, io.Discard)
	cmd.SetArgs([]string{"serve", "--config", filename})
	require.ErrorContains(t, cmd.Execute(), filename)
}

func TestConfigFlagBeforeSubcommands(t *testing.T) {
	filename := writeCommandConfig(t, validServerConfig)
	var stdout bytes.Buffer
	cmd := newRootCommand(context.Background(), &stdout, io.Discard)
	cmd.SetArgs([]string{"--config", filename, "config", "verify"})
	require.NoError(t, cmd.Execute())
	require.Contains(t, stdout.String(), "Status: valid")
}

func writeCommandConfig(t *testing.T, content string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(filename, []byte(content), 0o600))
	return filename
}

const validCoreConfig = "auth:\n  jwt_secret: jwt-secret\n  invite_token_secret: invite-secret\nsecure_links:\n  signing_key: signing-key\n"
const validServerConfig = validCoreConfig + "database:\n  path: ':memory:'\nstorage:\n  type: local\n  local:\n    root: /tmp/storage\n"
