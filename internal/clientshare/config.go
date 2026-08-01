package clientshare

import (
	"fmt"
	"time"

	"github.com/pixelcop/clientshare/pkg/db"
	"github.com/spf13/viper"
)

const StorageLocal = "local"
const StorageS3 = "s3"

var (
	BuildTime  = ""
	CommitHash = ""
)

type Config struct {
	BuildTime  *time.Time `mapstructure:"-"`
	CommitHash string     `mapstructure:"-"`
	DevMode    bool       `mapstructure:"dev_mode"`
	SentryDSN  string     `mapstructure:"sentry_dsn"`
	Server     struct {
		Host      string `mapstructure:"host"`
		Port      int    `mapstructure:"port"`
		VitePort  int    `mapstructure:"vite_port"`
		BaseURL   string `mapstructure:"base_url"`
		MaxBodyMB int    `mapstructure:"max_body_mb"`
	} `mapstructure:"server"`
	Tenancy struct {
		Mode                 string `mapstructure:"mode"`
		InternalTargetHeader string `mapstructure:"internal_target_header"`
	} `mapstructure:"tenancy"`
	Database db.DatabaseConfig `mapstructure:"database"`
	Storage  struct {
		Type  string `mapstructure:"type"`
		Local struct {
			Root string `mapstructure:"root"`
		} `mapstructure:"local"`
		S3 struct {
			Bucket    string `mapstructure:"bucket"`
			Region    string `mapstructure:"region"`
			Endpoint  string `mapstructure:"endpoint"`
			PathStyle bool   `mapstructure:"path_style"`
			AccessKey string `mapstructure:"access_key"`
			SecretKey string `mapstructure:"secret_key"`
		} `mapstructure:"s3"`
	} `mapstructure:"storage"`
	Auth struct {
		JWTSecret             string `mapstructure:"jwt_secret"`
		InviteTokenSecret     string `mapstructure:"invite_token_secret"`
		SessionDuration       string `mapstructure:"session_duration"`
		PasswordResetDuration string `mapstructure:"password_reset_duration"`
		InviteTokenDuration   string `mapstructure:"invite_token_duration"`
		RateLimitEnabled      *bool  `mapstructure:"rate_limit_enabled"`
	} `mapstructure:"auth"`
	SecureLinks struct {
		DefaultExpiryDays int    `mapstructure:"default_expiry_days"`
		SigningKey        string `mapstructure:"signing_key"`
	} `mapstructure:"secure_links"`
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
		UserInviteWelcomeText string `mapstructure:"user_invite_welcome_text"`
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
		Enabled bool   `mapstructure:"enabled"`
		Secret  string `mapstructure:"secret"`
	} `mapstructure:"internal_api"`
}

func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()
	v.SetDefault("server.vite_port", 5193)
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := ApplyBuildInfo(&cfg); err != nil {
		return nil, fmt.Errorf("apply build info: %w", err)
	}

	return &cfg, nil
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
