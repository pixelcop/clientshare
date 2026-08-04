package configschema

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type testConfig struct {
	Server struct {
		Host string `mapstructure:"host" jsonschema:"required" jsonschema_extras:"x-since=2026-06-30"`
		Port int    `mapstructure:"port"`
	} `mapstructure:"server" jsonschema:"required"`
	Enabled bool `mapstructure:"enabled" jsonschema_extras:"x-since=2026-07-04"`
}

func TestGenerateUsesConfiguredFieldTagAndExplicitRequiredFields(t *testing.T) {
	schema, err := Generate(testConfig{}, WithFieldNameTag("mapstructure"))
	require.NoError(t, err)

	text := string(schema)
	require.Contains(t, text, `"server"`)
	require.Contains(t, text, `"host"`)
	require.Contains(t, text, `"required"`)
	require.Contains(t, text, `"additionalProperties": false`)
	require.Contains(t, text, `"maximum": 9223372036854775807`)
	require.Contains(t, text, `"x-since": "2026-07-04"`)
}

func TestValidateYAMLFileReportsMissingFields(t *testing.T) {
	filename := writeTestConfig(t, "server: {}\n")

	report, err := ValidateYAMLFile(filename, testConfig{}, WithFieldNameTag("mapstructure"))
	require.NoError(t, err)
	require.False(t, report.Valid())
	require.Equal(t, []string{"server.host"}, report.MissingRequired)
	require.Equal(t, []string{"enabled", "server.port"}, report.MissingOptional)
	require.Equal(t, map[string]string{"server.host": "2026-06-30", "enabled": "2026-07-04"}, report.MissingFieldSince)
	require.NotEmpty(t, report.SchemaVersion)
	require.ErrorIs(t, report.Err(), ErrInvalid)

	var output bytes.Buffer
	require.NoError(t, WriteReport(&output, report))
	require.Contains(t, output.String(), "server.host (since: 2026-06-30)")
}

func TestValidateYAMLFileReportsUnknownAndInvalidFields(t *testing.T) {
	filename := writeTestConfig(t, "server:\n  host: localhost\n  port: wrong\nunknown: true\n")

	report, err := ValidateYAMLFile(filename, testConfig{}, WithFieldNameTag("mapstructure"))
	require.NoError(t, err)
	require.False(t, report.Valid())
	require.Empty(t, report.MissingRequired)
	require.Len(t, report.Issues, 2)
	require.Contains(t, report.Issues[0].Message+report.Issues[1].Message, "unknown")
	require.Contains(t, report.Issues[0].Message+report.Issues[1].Message, "integer")
}

func TestValidateYAMLFileRejectsIntegerOverflow(t *testing.T) {
	filename := writeTestConfig(t, "server:\n  host: localhost\n  port: 9223372036854775808\n")

	report, err := ValidateYAMLFile(filename, testConfig{}, WithFieldNameTag("mapstructure"))
	require.NoError(t, err)
	require.False(t, report.Valid())
	require.Len(t, report.Issues, 1)
	require.Equal(t, "server.port", report.Issues[0].Path)
	require.Contains(t, report.Issues[0].Message, "maximum")
}

func TestValidateYAMLFileAllowsMissingOptionalFields(t *testing.T) {
	filename := writeTestConfig(t, "server:\n  host: localhost\n")
	options := []Option{
		WithFieldNameTag("mapstructure"),
		WithDefaults(map[string]any{"enabled": false, "server.port": 8080}),
	}

	report, err := ValidateYAMLFile(filename, testConfig{}, options...)
	require.NoError(t, err)
	require.True(t, report.Valid())
	require.Equal(t, []string{"enabled", "server.port"}, report.MissingOptional)
	require.Equal(t, map[string]any{"enabled": false, "server.port": float64(8080)}, report.MissingOptionalDefaults)
	require.Equal(t, map[string]string{"enabled": "2026-07-04"}, report.MissingFieldSince)
	require.NoError(t, report.Err())

	var output bytes.Buffer
	require.NoError(t, WriteReport(&output, report))
	require.Contains(t, output.String(), "Status: valid")
	require.Contains(t, output.String(), "Missing optional fields (2)")
	require.Contains(t, output.String(), "enabled (default: false; since: 2026-07-04)")
	require.Contains(t, output.String(), "server.port (default: 8080)")
}

func TestGenerateRejectsUnknownDefaultPath(t *testing.T) {
	_, err := Generate(testConfig{}, WithFieldNameTag("mapstructure"), WithDefaults(map[string]any{"unknown": true}))
	require.ErrorContains(t, err, "property does not exist")
}

func TestGeneratePreservesIntegerBoundsWhenAddingDefaults(t *testing.T) {
	schema, err := Generate(testConfig{}, WithFieldNameTag("mapstructure"), WithDefaults(map[string]any{"enabled": false}))
	require.NoError(t, err)
	require.Contains(t, string(schema), `"maximum": 9223372036854775807`)
}

func TestValidateYAMLFileRejectsMultipleDocuments(t *testing.T) {
	filename := writeTestConfig(t, "server:\n  host: localhost\n---\nserver:\n  host: example.com\n")

	_, err := ValidateYAMLFile(filename, testConfig{}, WithFieldNameTag("mapstructure"))
	require.ErrorContains(t, err, "multiple YAML documents")
}

func TestValidateYAMLAcceptsEmptyDocument(t *testing.T) {
	report, err := ValidateYAML("config.yaml", []byte("# no values yet\n"), testConfig{}, WithFieldNameTag("mapstructure"))
	require.NoError(t, err)
	require.False(t, report.Valid())
	require.Equal(t, []string{"server"}, report.MissingRequired)
	require.Equal(t, []string{"enabled"}, report.MissingOptional)
}

func TestReportMarkRequired(t *testing.T) {
	report := Report{
		MissingOptional: []string{"internal_api", "storage.s3.bucket", "storage.s3.region"},
		MissingOptionalDefaults: map[string]any{
			"internal_api":      false,
			"storage.s3.bucket": "bucket",
		},
		MissingFieldSince: map[string]string{
			"internal_api":      "2026-03-18",
			"storage.s3.bucket": "2026-03-18",
		},
	}

	report.MarkRequired("storage.s3.bucket")
	report.MarkRequired("internal_api.secret")
	report.MarkRequired("storage.s3.bucket")

	require.Equal(t, []string{"internal_api.secret", "storage.s3.bucket"}, report.MissingRequired)
	require.Equal(t, []string{"storage.s3.region"}, report.MissingOptional)
	require.Empty(t, report.MissingOptionalDefaults)
	require.Equal(t, map[string]string{"storage.s3.bucket": "2026-03-18"}, report.MissingFieldSince)
}

func TestReportMarkRequiredIfBlank(t *testing.T) {
	report := Report{MissingOptional: []string{"database.dsn", "storage.local.root"}}

	require.True(t, report.MarkRequiredIfBlank("database.dsn", "  "))
	require.False(t, report.MarkRequiredIfBlank("storage.local.root", "/srv/files"))

	require.Equal(t, []string{"database.dsn"}, report.MissingRequired)
	require.Equal(t, []string{"storage.local.root"}, report.MissingOptional)
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(filename, []byte(strings.TrimSpace(content)+"\n"), 0o600))
	return filename
}
