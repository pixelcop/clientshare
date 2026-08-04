package clientshare

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/pkg/configschema"
	"github.com/pixelcop/clientshare/pkg/db"
	"github.com/spf13/viper"
)

const (
	StorageLocal                  = "local"
	StorageS3                     = "s3"
	DefaultVitePort               = 5193
	DefaultMaxBodyMB              = 32
	DefaultPasswordResetDuration  = "30m"
	DefaultInviteTokenDuration    = "72h"
	DefaultEmailBatchDelayMinutes = 15
)

var (
	BuildTime  = ""
	CommitHash = ""
)

type Config struct {
	BuildTime  *time.Time `mapstructure:"-"`
	CommitHash string     `mapstructure:"-"`
	DevMode    bool       `mapstructure:"dev_mode"`
	SentryDSN  string     `mapstructure:"sentry_dsn" jsonschema_extras:"x-since=2026-04-16"`
	Server     struct {
		Host      string `mapstructure:"host"`
		Port      int    `mapstructure:"port"`
		VitePort  int    `mapstructure:"vite_port" jsonschema_extras:"x-since=2026-08-01"`
		BaseURL   string `mapstructure:"base_url"`
		MaxBodyMB int    `mapstructure:"max_body_mb" jsonschema_extras:"x-since=2026-02-06"`
	} `mapstructure:"server"`
	Tenancy struct {
		Mode                 string `mapstructure:"mode"`
		InternalTargetHeader string `mapstructure:"internal_target_header" jsonschema_extras:"x-since=2026-03-17"`
	} `mapstructure:"tenancy"`
	Database db.DatabaseConfig `mapstructure:"database" jsonschema:"required" jsonschema_extras:"x-since=2026-01-17"`
	Storage  struct {
		Type  string `mapstructure:"type" jsonschema:"required,enum=local,enum=s3" jsonschema_extras:"x-since=2026-01-17"`
		Local struct {
			Root string `mapstructure:"root"`
		} `mapstructure:"local"`
		S3 struct {
			Bucket    string `mapstructure:"bucket" jsonschema_extras:"x-since=2026-03-18"`
			Region    string `mapstructure:"region" jsonschema_extras:"x-since=2026-03-18"`
			Endpoint  string `mapstructure:"endpoint" jsonschema_extras:"x-since=2026-03-18"`
			PathStyle bool   `mapstructure:"path_style" jsonschema_extras:"x-since=2026-03-18"`
			AccessKey string `mapstructure:"access_key" jsonschema_extras:"x-since=2026-03-18"`
			SecretKey string `mapstructure:"secret_key" jsonschema_extras:"x-since=2026-03-18"`
		} `mapstructure:"s3"`
	} `mapstructure:"storage" jsonschema:"required" jsonschema_extras:"x-since=2026-01-17"`
	Auth struct {
		JWTSecret             string `mapstructure:"jwt_secret" jsonschema:"required,minLength=1,pattern=.*\\S.*" jsonschema_extras:"x-since=2026-01-17"`
		InviteTokenSecret     string `mapstructure:"invite_token_secret" jsonschema:"required,minLength=1,pattern=.*\\S.*" jsonschema_extras:"x-since=2026-03-06"`
		SessionDuration       string `mapstructure:"session_duration"`
		PasswordResetDuration string `mapstructure:"password_reset_duration" jsonschema_extras:"x-since=2026-02-09"`
		InviteTokenDuration   string `mapstructure:"invite_token_duration"`
		RateLimitEnabled      *bool  `mapstructure:"rate_limit_enabled" jsonschema_extras:"x-since=2026-04-17"`
		HostedPasskeyOrigin   string `mapstructure:"hosted_passkey_origin" jsonschema_extras:"x-since=2026-08-03"`
	} `mapstructure:"auth" jsonschema:"required" jsonschema_extras:"x-since=2026-01-17"`
	SecureLinks struct {
		DefaultExpiryDays int    `mapstructure:"default_expiry_days"`
		SigningKey        string `mapstructure:"signing_key" jsonschema:"required,minLength=1" jsonschema_extras:"x-since=2026-01-16"`
	} `mapstructure:"secure_links" jsonschema:"required" jsonschema_extras:"x-since=2026-01-16"`
	Email struct {
		Enabled bool `mapstructure:"enabled"`
		SMTP    struct {
			Host     string `mapstructure:"host"`
			Port     int    `mapstructure:"port"`
			Username string `mapstructure:"username"`
			Password string `mapstructure:"password"`
			From     string `mapstructure:"from"`
		} `mapstructure:"smtp"`
		BatchDelayMinutes     int    `mapstructure:"batch_delay_minutes"`
		SendRate              int    `mapstructure:"send_rate"`
		UserInviteWelcomeText string `mapstructure:"user_invite_welcome_text" jsonschema_extras:"x-since=2026-03-08"`
	} `mapstructure:"email"`
	Branding struct {
		SiteTitle    string `mapstructure:"site_title"`
		LogoPath     string `mapstructure:"logo_path"`
		PrimaryColor string `mapstructure:"primary_color"`
	} `mapstructure:"branding"`
	Admin struct {
		BootstrapEmail    string `mapstructure:"bootstrap_email"`
		BootstrapPassword string `mapstructure:"bootstrap_password"`
	} `mapstructure:"admin"`
	InternalAPI struct {
		Enabled bool   `mapstructure:"enabled" jsonschema_extras:"x-since=2026-03-18"`
		Secret  string `mapstructure:"secret" jsonschema_extras:"x-since=2026-03-18"`
	} `mapstructure:"internal_api"`
}

// ConfigSchemaOptions describes defaults applied by the server when optional
// configuration fields are omitted.
func ConfigSchemaOptions() []configschema.Option {
	return []configschema.Option{
		configschema.WithFieldNameTag("mapstructure"),
		configschema.WithDefaults(map[string]any{
			"dev_mode":                     false,
			"server.vite_port":             DefaultVitePort,
			"server.max_body_mb":           DefaultMaxBodyMB,
			"storage.s3.path_style":        false,
			"auth.password_reset_duration": DefaultPasswordResetDuration,
			"auth.invite_token_duration":   DefaultInviteTokenDuration,
			"auth.rate_limit_enabled":      true,
			"email.enabled":                false,
			"email.batch_delay_minutes":    DefaultEmailBatchDelayMinutes,
			"internal_api.enabled":         false,
		}),
	}
}

func LoadConfig(configPath string) (*Config, error) {
	cfg, _, err := LoadConfigWithReport(configPath)
	return cfg, err
}

func LoadConfigWithReport(configPath string) (*Config, configschema.Report, error) {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, configschema.Report{Filename: configPath, SchemaName: "Config"}, fmt.Errorf("read config: %w", err)
	}
	report, err := configschema.ValidateYAML(configPath, configData, Config{}, ConfigSchemaOptions()...)
	if err != nil {
		return nil, report, err
	}
	if err := report.Err(); err != nil {
		return nil, report, fmt.Errorf("validate config file: %w", err)
	}

	v := viper.New()
	v.SetDefault("server.vite_port", DefaultVitePort)
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(configData)); err != nil {
		return nil, report, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, report, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := ApplyBuildInfo(&cfg); err != nil {
		return nil, report, fmt.Errorf("apply build info: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		report.Issues = append(report.Issues, configschema.ValidationIssue{Path: "$", Message: err.Error()})
		return &cfg, report, fmt.Errorf("validate config: %w", report.Err())
	}

	return &cfg, report, nil
}

func LoadServerConfigWithReport(configPath string) (*Config, configschema.Report, error) {
	cfg, report, err := LoadConfigWithReport(configPath)
	if err != nil && (!errors.Is(err, configschema.ErrInvalid) || cfg == nil) {
		return nil, report, err
	}

	if cfg.InternalAPI.Enabled {
		secret := strings.TrimSpace(cfg.InternalAPI.Secret)
		if !report.MarkRequiredIfBlank("internal_api.secret", secret) && len(secret) < 32 {
			report.Issues = append(report.Issues, configschema.ValidationIssue{
				Path:    "internal_api.secret",
				Message: "must be at least 32 characters when internal_api.enabled is true",
			})
		}
	}
	validateDatabaseReport(cfg, &report)
	validateStorageReport(cfg, &report)
	if err := report.Err(); err != nil {
		return nil, report, fmt.Errorf("validate server config: %w", err)
	}
	return cfg, report, nil
}

func validateDatabaseReport(cfg *Config, report *configschema.Report) {
	driver, err := cfg.Database.ResolvedDriver()
	if err != nil {
		report.Issues = append(report.Issues, configschema.ValidationIssue{Path: "database.driver", Message: err.Error()})
		return
	}

	switch driver {
	case db.DatabaseDriverSQLite:
		if strings.TrimSpace(cfg.Database.Path) == "" && strings.TrimSpace(cfg.Database.DSN) == "" {
			report.Issues = append(report.Issues, configschema.ValidationIssue{Path: "database", Message: "database.path or database.dsn is required for sqlite"})
			return
		}
	case db.DatabaseDriverPostgres, db.DatabaseDriverMySQL:
		if report.MarkRequiredIfBlank("database.dsn", cfg.Database.DSN) {
			return
		}
	}
	if _, err := cfg.Database.RuntimeDSN(); err != nil {
		report.Issues = append(report.Issues, configschema.ValidationIssue{Path: "database.dsn", Message: err.Error()})
	}
}

func validateStorageReport(cfg *Config, report *configschema.Report) {
	switch strings.TrimSpace(cfg.Storage.Type) {
	case StorageLocal:
		report.MarkRequiredIfBlank("storage.local.root", cfg.Storage.Local.Root)
	case StorageS3:
		report.MarkRequiredIfBlank("storage.s3.bucket", cfg.Storage.S3.Bucket)
		report.MarkRequiredIfBlank("storage.s3.region", cfg.Storage.S3.Region)
		accessKeySet := strings.TrimSpace(cfg.Storage.S3.AccessKey) != ""
		secretKeySet := strings.TrimSpace(cfg.Storage.S3.SecretKey) != ""
		if !accessKeySet && secretKeySet {
			report.MarkRequiredIfBlank("storage.s3.access_key", cfg.Storage.S3.AccessKey)
		}
		if accessKeySet && !secretKeySet {
			report.MarkRequiredIfBlank("storage.s3.secret_key", cfg.Storage.S3.SecretKey)
		}
	}
}

func (cfg *Config) Validate() error {
	origin := strings.TrimSpace(cfg.Auth.HostedPasskeyOrigin)
	if origin == "" {
		return nil
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("auth.hosted_passkey_origin must be an absolute origin")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("auth.hosted_passkey_origin must use http or https")
	}
	if parsed.Scheme == "http" && (!cfg.DevMode || !isLoopbackHost(parsed.Hostname())) {
		return fmt.Errorf("auth.hosted_passkey_origin must use https outside local development")
	}
	if parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("auth.hosted_passkey_origin must not include a path, query, or fragment")
	}
	if !cfg.InternalAPI.Enabled {
		return fmt.Errorf("auth.hosted_passkey_origin requires internal_api.enabled")
	}
	return nil
}

func isLoopbackHost(hostname string) bool {
	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if hostname == "localhost" || hostname == "::1" {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

func ApplyBuildInfo(cfg *Config) error {
	if BuildTime != "" {
		parsedBuildTime, err := time.Parse(time.RFC3339, BuildTime)
		if err != nil {
			return fmt.Errorf("parse build time: %w", err)
		}
		cfg.BuildTime = &parsedBuildTime
	}

	cfg.CommitHash = CommitHash

	return nil
}
