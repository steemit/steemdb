package utils

import (
	"fmt"
	"os"
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
	Cache      CacheConfig      `mapstructure:"cache"`
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
	Redis   RedisConfig   `mapstructure:"redis"`
}

// MongoDBConfig holds MongoDB configuration
type MongoDBConfig struct {
	URI      string        `mapstructure:"uri"`
	Database string        `mapstructure:"database"`
	PoolSize int           `mapstructure:"pool_size"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	URI          string        `mapstructure:"uri"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
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

// CacheConfig holds cache configuration
type CacheConfig struct {
	Enabled         bool            `mapstructure:"enabled"`
	DefaultExpiry   time.Duration   `mapstructure:"default_expiry"`
	CleanupInterval time.Duration   `mapstructure:"cleanup_interval"`
	Blocks          CacheItemConfig `mapstructure:"blocks"`
	Accounts        CacheItemConfig `mapstructure:"accounts"`
	Witnesses       CacheItemConfig `mapstructure:"witnesses"`
}

// CacheItemConfig holds cache item specific configuration
type CacheItemConfig struct {
	Expiry   time.Duration `mapstructure:"expiry"`
	MaxItems int           `mapstructure:"max_items"`
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

	// Bind specific environment variables for nested configs
	// Must be called before ReadInConfig() to ensure proper override
	viper.BindEnv("database.mongodb.uri", "DATABASE_MONGODB_URI", "MONGODB_URI")
	viper.BindEnv("database.redis.uri", "DATABASE_REDIS_URI", "REDIS_URI")

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

	if envURI := os.Getenv("DATABASE_REDIS_URI"); envURI != "" {
		viper.Set("database.redis.uri", envURI)
	} else if envURI := os.Getenv("REDIS_URI"); envURI != "" {
		viper.Set("database.redis.uri", envURI)
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

	if envURI := os.Getenv("DATABASE_REDIS_URI"); envURI != "" {
		config.Database.Redis.URI = envURI
	} else if envURI := os.Getenv("REDIS_URI"); envURI != "" {
		config.Database.Redis.URI = envURI
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

	viper.SetDefault("database.redis.uri", "redis://localhost:6379")
	viper.SetDefault("database.redis.password", "")
	viper.SetDefault("database.redis.db", 0)
	viper.SetDefault("database.redis.pool_size", 100)

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

	// Cache defaults
	viper.SetDefault("cache.enabled", true)
	viper.SetDefault("cache.default_expiry", "5m")
	viper.SetDefault("cache.cleanup_interval", "10m")

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
