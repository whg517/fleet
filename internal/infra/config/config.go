package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	App      AppConfig       `mapstructure:"app"`
	Server   ServerConfig    `mapstructure:"server"`
	Database DatabaseConfig  `mapstructure:"database"`
	Redis    RedisConfig     `mapstructure:"redis"`
	Log      LogConfig       `mapstructure:"log"`
	Security SecurityConfig  `mapstructure:"security"`
	OIDC     OIDCConfig      `mapstructure:"oidc"`
	JWT      JWTConfig       `mapstructure:"jwt"`
}

// AppConfig holds application-level settings.
type AppConfig struct {
	// Environment controls runtime behavior. Valid values: "development", "production".
	// In production, stricter security checks are enforced (e.g. DB sslmode).
	Environment string `mapstructure:"environment"`
}

// SecurityConfig holds security-related settings.
type SecurityConfig struct {
	// DEK is a hex-encoded 32-byte (256-bit) data encryption key used for
	// encrypting secrets such as kubeconfigs. Inject via FLEET_SECURITY_DEK.
	DEK string `mapstructure:"dek"`
}

// OIDCConfig holds OIDC provider settings.
type OIDCConfig struct {
	Issuer       string   `mapstructure:"issuer"`
	ClientID     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
	RedirectURL  string   `mapstructure:"redirect_url"`
	Scopes       []string `mapstructure:"scopes"`
}

// JWTConfig holds JWT signing settings for session tokens.
type JWTConfig struct {
	Secret       string        `mapstructure:"secret"`
	AccessTTL    time.Duration `mapstructure:"access_ttl"`
	RefreshTTL   time.Duration `mapstructure:"refresh_ttl"`
	FrontendURL  string        `mapstructure:"frontend_url"`
	Issuer       string        `mapstructure:"issuer"`
	Audience     string        `mapstructure:"audience"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	AllowedOrigins  []string      `mapstructure:"allowed_origins"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	User         string `mapstructure:"user"`
	Password     string `mapstructure:"password"`
	DBName       string `mapstructure:"dbname"`
	SSLMode      string `mapstructure:"sslmode"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

// Validate checks the configuration for environment-specific security requirements.
func (c *Config) Validate() error {
	if c.App.Environment == "production" {
		if c.Database.SSLMode == "disable" || c.Database.SSLMode == "" {
			return fmt.Errorf(
				"database.sslmode must be 'require' or 'verify-full' in production (got %q)",
				c.Database.SSLMode,
			)
		}
	}
	return nil
}

// DSN returns the PostgreSQL connection string for the database driver.
func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// DSNRedacted returns a DSN with the password masked, safe for logging.
func (c DatabaseConfig) DSNRedacted() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=*** dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.DBName, c.SSLMode,
	)
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

// Load reads configuration from the given path and environment variables.
// Environment variables use the FLEET_ prefix and replace dots with underscores
// (e.g., FLEET_DATABASE_HOST overrides database.host).
func Load(path string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(path)
	v.SetEnvPrefix("FLEET")
	v.AutomaticEnv()
	// Support nested env overrides like FLEET_DATABASE_HOST → database.host
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Defaults
	v.SetDefault("app.environment", "development")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.allowed_origins", []string{"http://localhost:3000"})
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.encoding", "json")
	v.SetDefault("security.dek", "")
	v.SetDefault("oidc.scopes", []string{"openid", "profile", "email", "groups"})
	v.SetDefault("jwt.access_ttl", "30m")
	v.SetDefault("jwt.refresh_ttl", "8h")
	v.SetDefault("jwt.frontend_url", "http://localhost:3000")
	v.SetDefault("jwt.issuer", "fleet")
	v.SetDefault("jwt.audience", "fleet-api")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}
