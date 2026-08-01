package clientshare

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/pkg/db"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

const mockConfig = `
server:
  host: "127.0.0.1"
  port: 1234
  vite_port: 4321
  base_url: "https://test.example.com"
tenancy:
	mode: "hosted"
	internal_target_header: "X-Test-Tenant"
database:
	driver: "sqlite"
  path: "/tmp/test.db"
storage:
  type: "local"
  local:
    root: "/tmp/storage"
  s3:
    bucket: "mybucket"
    region: "us-east-1"
		endpoint: "http://127.0.0.1:9000"
		path_style: true
    access_key: "AKIA..."
    secret_key: "SECRET..."
auth:
  jwt_secret: "jwtsecret"
  invite_token_secret: "invitesecret"
  session_duration: "12h"
  invite_token_duration: "72h"
secure_links:
  default_expiry_days: 30
  signing_key: "signkey"
email:
  enabled: true
  smtp:
    host: "smtp.example.com"
    port: 587
    username: "user"
    password: "pass"
    from: "noreply@example.com"
  batch_delay_minutes: 5
	user_invite_welcome_text: "Welcome to the 2025 tax year portal."
branding:
  site_title: "Test Portal"
  logo_path: "/logo.png"
  primary_color: "#123456"
admin:
  bootstrap_email: "admin@example.com"
  bootstrap_password: "secret"
`

func TestConfigUnmarshal(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	_, err = tmpfile.Write([]byte(strings.ReplaceAll(mockConfig, "\t", "  ")))
	require.NoError(t, err)
	tmpfile.Close()

	v := viper.New()
	v.SetConfigFile(tmpfile.Name())
	err = v.ReadInConfig()
	require.NoError(t, err)

	var cfg Config
	err = v.Unmarshal(&cfg)
	require.NoError(t, err)

	require.Equal(t, "127.0.0.1", cfg.Server.Host)
	require.Equal(t, 1234, cfg.Server.Port)
	require.Equal(t, 4321, cfg.Server.VitePort)
	require.Equal(t, "https://test.example.com", cfg.Server.BaseURL)
	require.Equal(t, "hosted", cfg.Tenancy.Mode)
	require.Equal(t, "X-Test-Tenant", cfg.Tenancy.InternalTargetHeader)
	require.Equal(t, db.DatabaseDriverSQLite, cfg.Database.Driver)
	require.Equal(t, "/tmp/test.db", cfg.Database.Path)
	require.Equal(t, "", cfg.Database.DSN)
	require.Equal(t, "local", cfg.Storage.Type)
	require.Equal(t, "/tmp/storage", cfg.Storage.Local.Root)
	require.Equal(t, "mybucket", cfg.Storage.S3.Bucket)
	require.Equal(t, "us-east-1", cfg.Storage.S3.Region)
	require.Equal(t, "http://127.0.0.1:9000", cfg.Storage.S3.Endpoint)
	require.True(t, cfg.Storage.S3.PathStyle)
	require.Equal(t, "AKIA...", cfg.Storage.S3.AccessKey)
	require.Equal(t, "SECRET...", cfg.Storage.S3.SecretKey)
	require.Equal(t, "jwtsecret", cfg.Auth.JWTSecret)
	require.Equal(t, "invitesecret", cfg.Auth.InviteTokenSecret)
	require.Equal(t, "12h", cfg.Auth.SessionDuration)
	require.Equal(t, "72h", cfg.Auth.InviteTokenDuration)
	require.Equal(t, 30, cfg.SecureLinks.DefaultExpiryDays)
	require.Equal(t, "signkey", cfg.SecureLinks.SigningKey)
	require.True(t, cfg.Email.Enabled)
	require.Equal(t, "smtp.example.com", cfg.Email.SMTP.Host)
	require.Equal(t, 587, cfg.Email.SMTP.Port)
	require.Equal(t, "user", cfg.Email.SMTP.Username)
	require.Equal(t, "pass", cfg.Email.SMTP.Password)
	require.Equal(t, "noreply@example.com", cfg.Email.SMTP.From)
	require.Equal(t, 5, cfg.Email.BatchDelayMinutes)
	require.Equal(t, "Welcome to the 2025 tax year portal.", cfg.Email.UserInviteWelcomeText)
	require.Equal(t, "Test Portal", cfg.Branding.SiteTitle)
	require.Equal(t, "/logo.png", cfg.Branding.LogoPath)
	require.Equal(t, "#123456", cfg.Branding.PrimaryColor)
	require.Equal(t, "admin@example.com", cfg.Admin.BootstrapEmail)
	require.Equal(t, "secret", cfg.Admin.BootstrapPassword)
}

func TestLoadConfigDefaultsVitePort(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	_, err = tmpfile.WriteString("server:\n  port: 8320\n")
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	cfg, err := LoadConfig(tmpfile.Name())
	require.NoError(t, err)
	require.Equal(t, 5193, cfg.Server.VitePort)
}

func TestApplyBuildInfo(t *testing.T) {
	originalBuildTime := BuildTime
	originalCommitHash := CommitHash
	t.Cleanup(func() {
		BuildTime = originalBuildTime
		CommitHash = originalCommitHash
	})

	BuildTime = "2026-03-11T00:00:00Z"
	CommitHash = "deadbeefcafe"

	var cfg Config
	err := ApplyBuildInfo(&cfg)
	require.NoError(t, err)

	require.Equal(t, time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC), *cfg.BuildTime)
	require.Equal(t, CommitHash, cfg.CommitHash)
}

func TestApplyBuildInfoRejectsInvalidBuildTime(t *testing.T) {
	originalBuildTime := BuildTime
	originalCommitHash := CommitHash
	t.Cleanup(func() {
		BuildTime = originalBuildTime
		CommitHash = originalCommitHash
	})

	BuildTime = "not-a-timestamp"
	CommitHash = "deadbeefcafe"

	err := ApplyBuildInfo(&Config{})
	require.Error(t, err)
}
