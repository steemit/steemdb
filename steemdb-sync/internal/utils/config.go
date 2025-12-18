package utils

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Steem      SteemConfig      `mapstructure:"steem"`
	MongoDB    MongoDBConfig    `mapstructure:"mongodb"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Sync       SyncConfig       `mapstructure:"sync"`
	History    HistoryConfig    `mapstructure:"history"`
	Witnesses  WitnessesConfig  `mapstructure:"witnesses"`
	Log        LogConfig        `mapstructure:"log"`
	Metrics    MetricsConfig    `mapstructure:"metrics"`
	Monitoring MonitoringConfig `mapstructure:"monitoring"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
	Mode string `mapstructure:"mode"`
}

type SteemConfig struct {
	Nodes         []string      `mapstructure:"nodes"`
	Timeout       time.Duration `mapstructure:"timeout"`
	RetryAttempts int           `mapstructure:"retry_attempts"`
}

type MongoDBConfig struct {
	URI      string        `mapstructure:"uri"`
	Database string        `mapstructure:"database"`
	PoolSize int           `mapstructure:"pool_size"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

type RedisConfig struct {
	URI      string `mapstructure:"uri"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type SyncConfig struct {
	BatchSize        int           `mapstructure:"batch_size"`
	BlockBatchSize   int           `mapstructure:"block_batch_size"`
	AccountBatchSize int           `mapstructure:"account_batch_size"`
	Workers          int           `mapstructure:"workers"`
	QueueSize        int           `mapstructure:"queue_size"`
	BlockInterval    time.Duration `mapstructure:"block_interval"`
	StartBlock       int64         `mapstructure:"start_block"`
}

type HistoryConfig struct {
	BatchSize        int           `mapstructure:"batch_size"`
	Workers          int           `mapstructure:"workers"`
	Interval         time.Duration `mapstructure:"interval"`
	AccountScanLimit int           `mapstructure:"account_scan_limit"`
}

type WitnessesConfig struct {
	Workers       int           `mapstructure:"workers"`
	Interval      time.Duration `mapstructure:"interval"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	File       string `mapstructure:"file"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

type MonitoringConfig struct {
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval"`
	ErrorThreshold      int           `mapstructure:"error_threshold"`
	RecoveryTimeout     time.Duration `mapstructure:"recovery_timeout"`
}

// LoadConfig loads configuration from file
func LoadConfig(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Set default values
	setDefaults()

	// Enable environment variable override
	viper.AutomaticEnv()

	// Bind specific environment variables for nested configs
	viper.BindEnv("mongodb.uri", "DATABASE_MONGODB_URI", "MONGODB_URI")
	viper.BindEnv("redis.uri", "DATABASE_REDIS_URI", "REDIS_URI")
	viper.BindEnv("log.level", "LOG_LEVEL")

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Explicitly check and override with environment variables if they exist
	// This ensures environment variables always take precedence
	if envURI := os.Getenv("DATABASE_MONGODB_URI"); envURI != "" {
		viper.Set("mongodb.uri", envURI)
	} else if envURI := os.Getenv("MONGODB_URI"); envURI != "" {
		viper.Set("mongodb.uri", envURI)
	}

	if envURI := os.Getenv("DATABASE_REDIS_URI"); envURI != "" {
		viper.Set("redis.uri", envURI)
	} else if envURI := os.Getenv("REDIS_URI"); envURI != "" {
		viper.Set("redis.uri", envURI)
	}

	// Unmarshal config
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// After unmarshal, explicitly override with environment variables if they exist
	if envURI := os.Getenv("DATABASE_MONGODB_URI"); envURI != "" {
		config.MongoDB.URI = envURI
	} else if envURI := os.Getenv("MONGODB_URI"); envURI != "" {
		config.MongoDB.URI = envURI
	}

	if envURI := os.Getenv("DATABASE_REDIS_URI"); envURI != "" {
		config.Redis.URI = envURI
	} else if envURI := os.Getenv("REDIS_URI"); envURI != "" {
		config.Redis.URI = envURI
	}

	// Override log level from environment variable if set
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		config.Log.Level = logLevel
	}

	return &config, nil
}

func setDefaults() {
	viper.SetDefault("server.port", 9090)
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.mode", "development")

	viper.SetDefault("steem.nodes", []string{"https://api.steemit.com"})
	viper.SetDefault("steem.timeout", "30s")
	viper.SetDefault("steem.retry_attempts", 3)

	viper.SetDefault("mongodb.uri", "mongodb://localhost:27017")
	viper.SetDefault("mongodb.database", "steemdb")
	viper.SetDefault("mongodb.pool_size", 100)
	viper.SetDefault("mongodb.timeout", "30s")

	viper.SetDefault("redis.uri", "redis://localhost:6379")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.pool_size", 100)

	viper.SetDefault("sync.batch_size", 50)
	viper.SetDefault("sync.block_batch_size", 50)
	viper.SetDefault("sync.account_batch_size", 100)
	viper.SetDefault("sync.workers", 1) // Single goroutine
	viper.SetDefault("sync.queue_size", 1000)
	viper.SetDefault("sync.block_interval", "3s")
	viper.SetDefault("sync.start_block", 1)

	viper.SetDefault("history.batch_size", 50)
	viper.SetDefault("history.workers", 5)
	viper.SetDefault("history.interval", "6h")
	viper.SetDefault("history.account_scan_limit", 1000)

	viper.SetDefault("witnesses.workers", 2)
	viper.SetDefault("witnesses.interval", "1m")
	viper.SetDefault("witnesses.check_interval", "10s")

	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")
	viper.SetDefault("log.file", "/var/log/steemdb-sync.log")
	viper.SetDefault("log.max_size", 100)
	viper.SetDefault("log.max_backups", 5)
	viper.SetDefault("log.max_age", 30)

	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.port", 9091)
	viper.SetDefault("metrics.path", "/metrics")

	viper.SetDefault("monitoring.health_check_interval", "30s")
	viper.SetDefault("monitoring.error_threshold", 10)
	viper.SetDefault("monitoring.recovery_timeout", "5m")
}
