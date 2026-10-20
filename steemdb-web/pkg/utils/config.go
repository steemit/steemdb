package utils

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Auth       AuthConfig       `mapstructure:"auth"`
	API        APIConfig        `mapstructure:"api"`
	WebSocket  WebSocketConfig  `mapstructure:"websocket"`
	Steem      SteemConfig      `mapstructure:"steem"`
	Log        LogConfig        `mapstructure:"log"`
	Metrics    MetricsConfig    `mapstructure:"metrics"`
	Monitoring MonitoringConfig `mapstructure:"monitoring"`
	Analytics  AnalyticsConfig  `mapstructure:"analytics"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	Host         string        `mapstructure:"host"`
	Mode         string        `mapstructure:"mode"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	MongoDB MongoDBConfig `mapstructure:"mongodb"`
}

// MongoDBConfig holds MongoDB configuration
type MongoDBConfig struct {
	URI      string        `mapstructure:"uri"`
	Database string        `mapstructure:"database"`
	PoolSize int           `mapstructure:"pool_size"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret     string        `mapstructure:"jwt_secret"`
	JWTExpiry     time.Duration `mapstructure:"jwt_expiry"`
	RefreshExpiry time.Duration `mapstructure:"refresh_expiry"`
}

// APIConfig holds API configuration
type APIConfig struct {
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	CORS      CORSConfig      `mapstructure:"cors"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	RequestsPerMinute int  `mapstructure:"requests_per_minute"`
	Burst             int  `mapstructure:"burst"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	Enabled          bool     `mapstructure:"enabled"`
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods"`
	AllowedHeaders   []string `mapstructure:"allowed_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
}

// WebSocketConfig holds WebSocket configuration
type WebSocketConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	Path            string        `mapstructure:"path"`
	ReadBufferSize  int           `mapstructure:"read_buffer_size"`
	WriteBufferSize int           `mapstructure:"write_buffer_size"`
	MaxConnections  int           `mapstructure:"max_connections"`
	PingPeriod      time.Duration `mapstructure:"ping_period"`
	PongWait        time.Duration `mapstructure:"pong_wait"`
	WriteWait       time.Duration `mapstructure:"write_wait"`
}

// SteemConfig holds Steem blockchain configuration
type SteemConfig struct {
	Nodes         []string      `mapstructure:"nodes"`
	Timeout       time.Duration `mapstructure:"timeout"`
	RetryAttempts int           `mapstructure:"retry_attempts"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	File       string `mapstructure:"file"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval"`
	ErrorThreshold      int           `mapstructure:"error_threshold"`
	RecoveryTimeout     time.Duration `mapstructure:"recovery_timeout"`
}

// AnalyticsConfig holds the optional third-party web analytics selection.
//
// The web service deliberately keeps no analytics snippet in the static HTML
// (the previous mechanism sed-replaced a placeholder in index.html on every
// container start). It only serves this config via GET /api/v1/meta, and
// the SPA loads the matching third-party script (Google Analytics gtag or
// self-hosted Umami) on the client. Adding a provider never requires
// touching the deployment pipeline.
type AnalyticsConfig struct {
	Provider       string `mapstructure:"provider"`         // "" (unset) | "none" | "ga" | "umami"
	GAID           string `mapstructure:"ga_id"`            // GA4 measurement ID (G-...)
	UmamiURL       string `mapstructure:"umami_url"`        // base URL of a self-hosted Umami
	UmamiWebsiteID string `mapstructure:"umami_website_id"` // Umami website UUID (data-website-id)
}

// LoadConfig loads configuration from file
func LoadConfig(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Set default values first
	setDefaults()

	// Enable environment variable override BEFORE reading config file
	// This ensures environment variables have higher priority
	viper.AutomaticEnv()

	// Map dots in config keys to underscores for AutomaticEnv lookups:
	// without the replacer, viper would look up e.g. "SERVER.MODE" for
	// server.mode, which never exists as an environment variable.
	// With the replacer, the override surface is every dotted key already
	// declared in setDefaults() or the config file (SERVER_MODE,
	// AUTH_JWT_SECRET, LOG_LEVEL, ...); an explicit BindEnv
	// below is only needed for keys that are declared nowhere, or to add
	// extra env aliases.
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Bind specific environment variables for nested configs
	// Must be called before ReadInConfig() to ensure proper override
	viper.BindEnv("server.mode", "SERVER_MODE")
	viper.BindEnv("server.port", "SERVER_PORT")
	viper.BindEnv("server.host", "SERVER_HOST")
	viper.BindEnv("database.mongodb.uri", "DATABASE_MONGODB_URI", "MONGODB_URI")

	// Analytics: keep the operator surface flat (GA_ID, UMAMI_URL,
	// UMAMI_WEBSITE_ID) instead of nested ANALYTICS_* names. The provider
	// key is bound WITHOUT a "none" default on purpose: an unset
	// ANALYTICS_PROVIDER ("" after unmarshal) must stay distinguishable
	// from an explicit "none", because Normalize() applies the legacy
	// GA_ID-only fallback (pre-/api/v1/meta behavior) only to the former.
	viper.BindEnv("analytics.provider")
	viper.BindEnv("analytics.ga_id", "GA_ID")
	viper.BindEnv("analytics.umami_url", "UMAMI_URL")
	viper.BindEnv("analytics.umami_website_id", "UMAMI_WEBSITE_ID")

	// Read config file (will be overridden by environment variables if set)
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Explicitly check and override with environment variables if they exist
	// This ensures environment variables always take precedence
	if envURI := os.Getenv("DATABASE_MONGODB_URI"); envURI != "" {
		viper.Set("database.mongodb.uri", envURI)
	} else if envURI := os.Getenv("MONGODB_URI"); envURI != "" {
		viper.Set("database.mongodb.uri", envURI)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// After unmarshal, explicitly override with environment variables if they exist
	// This ensures environment variables always take precedence over config file
	if envURI := os.Getenv("DATABASE_MONGODB_URI"); envURI != "" {
		config.Database.MongoDB.URI = envURI
	} else if envURI := os.Getenv("MONGODB_URI"); envURI != "" {
		config.Database.MongoDB.URI = envURI
	}

	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.host", "127.0.0.1")
	viper.SetDefault("server.mode", "development")
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.idle_timeout", "60s")

	// Database defaults
	viper.SetDefault("database.mongodb.uri", "mongodb://localhost:27017")
	viper.SetDefault("database.mongodb.database", "steemdb")
	viper.SetDefault("database.mongodb.pool_size", 100)
	viper.SetDefault("database.mongodb.timeout", "30s")

	// Auth defaults
	viper.SetDefault("auth.jwt_expiry", "24h")
	viper.SetDefault("auth.refresh_expiry", "168h")

	// API defaults
	viper.SetDefault("api.rate_limit.enabled", true)
	viper.SetDefault("api.rate_limit.requests_per_minute", 100)
	viper.SetDefault("api.rate_limit.burst", 20)

	// WebSocket defaults
	viper.SetDefault("websocket.enabled", true)
	viper.SetDefault("websocket.path", "/ws")
	viper.SetDefault("websocket.max_connections", 1000)

	// Steem defaults
	viper.SetDefault("steem.nodes", []string{"https://api.steemit.com"})
	viper.SetDefault("steem.timeout", "30s")
	viper.SetDefault("steem.retry_attempts", 3)

	// Log defaults
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")
	viper.SetDefault("log.max_size", 100)
	viper.SetDefault("log.max_backups", 5)
	viper.SetDefault("log.max_age", 30)

	// Metrics defaults
	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.port", 9090)
	viper.SetDefault("metrics.path", "/metrics")

	// No analytics defaults here: analytics.provider is registered via
	// BindEnv in LoadConfig (see the comment there). Tracking is opt-in
	// and fails closed in (*AnalyticsConfig).Normalize.
}

// gaIDPattern is the GA4 measurement ID format ("G-" + alphanumerics). It
// mirrors the guard the old inline index.html script used before loading
// gtag, so behavior is unchanged for any ID that worked before.
var gaIDPattern = regexp.MustCompile(`^G-[A-Z0-9]+$`)

// umamiWebsiteIDPattern is the Umami website ID format: a UUID, as issued
// per website by the Umami dashboard. The SPA sets it as the script tag's
// data-website-id attribute — Umami's tracker attributes events by that
// attribute, NOT by the requesting hostname.
var umamiWebsiteIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Normalize validates the raw analytics config and fails closed: an unknown
// provider, or a provider missing (or misformatted) credentials, degrades to
// "none" so the SPA never loads a broken half-configured snippet.
//
// Compatibility: before the /api/v1/meta endpoint existed, setting a valid
// GA_ID alone enabled Google Analytics (the container entrypoint injected
// it into index.html). An *unset* provider ("") keeps honoring that so
// existing operators keep tracking after upgrading; an explicit "none" is
// a hard off.
func (a *AnalyticsConfig) Normalize() {
	a.Provider = strings.ToLower(strings.TrimSpace(a.Provider))
	a.GAID = sanitizeID(a.GAID)
	a.UmamiURL = strings.TrimRight(strings.TrimSpace(a.UmamiURL), "/")
	a.UmamiWebsiteID = strings.ToLower(strings.TrimSpace(a.UmamiWebsiteID))

	switch a.Provider {
	case "ga":
		if !gaIDPattern.MatchString(a.GAID) {
			a.degrade()
			return
		}
		a.UmamiURL = ""
		a.UmamiWebsiteID = ""
	case "umami":
		if !isValidAnalyticsURL(a.UmamiURL) || !umamiWebsiteIDPattern.MatchString(a.UmamiWebsiteID) {
			a.degrade()
			return
		}
		a.GAID = ""
	case "": // provider unset: legacy GA_ID-only compatibility
		if gaIDPattern.MatchString(a.GAID) {
			a.Provider = "ga"
		} else {
			a.degrade()
		}
	default: // explicit "none" or anything unknown: tracking off
		a.degrade()
	}
}

// degrade disables tracking entirely, dropping every provider credential.
func (a *AnalyticsConfig) degrade() {
	a.Provider = "none"
	a.GAID = ""
	a.UmamiURL = ""
	a.UmamiWebsiteID = ""
}

// sanitizeID keeps only the characters GA measurement IDs may contain
// (alphanumerics and dashes), mirroring the `tr -cd 'A-Za-z0-9-'` the old
// container entrypoint applied. Defense in depth: the format check in
// Normalize is authoritative, this only scrubs hostiles from operator input.
func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isValidAnalyticsURL accepts only absolute http(s) URLs with a host: the
// SPA loads <url>/script.js from it, and anything else (relative paths,
// javascript: and other schemes) would be a client-side injection vector.
func isValidAnalyticsURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
