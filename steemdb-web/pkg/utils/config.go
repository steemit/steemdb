package utils

import (
	"fmt"
	"os"
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
}
