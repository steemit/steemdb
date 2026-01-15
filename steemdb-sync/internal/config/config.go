package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Mongo      MongoConfig      `yaml:"mongo"`
	RPC        RPCConfig        `yaml:"rpc"`
	ColdStart  ColdStartConfig  `yaml:"cold_start"`
	Batch      BatchConfig      `yaml:"batch"`
	Ingest     IngestConfig     `yaml:"ingest"`
	Log        LogConfig        `yaml:"log"`
}

// MongoConfig contains MongoDB connection settings
type MongoConfig struct {
	URI        string `yaml:"uri" env:"MONGO_URI"`
	Database   string `yaml:"database" env:"MONGO_DATABASE"`
	MinPoolSize int   `yaml:"min_pool_size" env:"MONGO_MIN_POOL_SIZE"`
	MaxPoolSize int   `yaml:"max_pool_size" env:"MONGO_MAX_POOL_SIZE"`
}

// RPCConfig contains Steem RPC node settings
type RPCConfig struct {
	Endpoint   string `yaml:"endpoint" env:"RPC_ENDPOINT"`
	MaxRetry   int    `yaml:"max_retry" env:"RPC_MAX_RETRY"`
	Timeout    string `yaml:"timeout" env:"RPC_TIMEOUT"`
}

// ColdStartConfig contains cold start phase settings
type ColdStartConfig struct {
	TargetHeight uint32 `yaml:"target_height" env:"COLD_START_TARGET_HEIGHT"`
	SafetyMargin uint32 `yaml:"safety_margin" env:"COLD_START_SAFETY_MARGIN"`
}

// BatchConfig contains batch processing settings
type BatchConfig struct {
	Size         int    `yaml:"size" env:"BATCH_SIZE"`
	FlushInterval string `yaml:"flush_interval" env:"BATCH_FLUSH_INTERVAL"`
}

// IngestConfig contains ingest service settings
type IngestConfig struct {
	ListenAddr string `yaml:"listen_addr" env:"INGEST_LISTEN_ADDR"`
	QueueSize  int    `yaml:"queue_size" env:"INGEST_QUEUE_SIZE"`
}

// LogConfig contains logging settings
type LogConfig struct {
	Level  string `yaml:"level" env:"LOG_LEVEL"`
	Format string `yaml:"format" env:"LOG_FORMAT"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	cfg := &Config{
		// Default values
		Mongo: MongoConfig{
			URI:        "mongodb://localhost:27017",
			Database:   "steemdb",
			MinPoolSize: 10,
			MaxPoolSize: 100,
		},
		RPC: RPCConfig{
			Endpoint: "https://api.steemit.com",
			MaxRetry: 3,
			Timeout:  "30s",
		},
		ColdStart: ColdStartConfig{
			TargetHeight: 0,
			SafetyMargin: 5,
		},
		Batch: BatchConfig{
			Size:         1000,
			FlushInterval: "1s",
		},
		Ingest: IngestConfig{
			ListenAddr: ":8080",
			QueueSize:  100000,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	// Store original URI to detect if it was changed by YAML
	originalURI := cfg.Mongo.URI

	// Load from YAML file if provided
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read config file: %s", configPath)
		}

		// Temporarily set Database to empty to detect if it's set in YAML
		originalDatabase := cfg.Mongo.Database
		cfg.Mongo.Database = ""

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, errors.Wrapf(err, "failed to parse config file: %s", configPath)
		}

		// If database was not set in YAML (still empty), restore original or parse from URI
		if cfg.Mongo.Database == "" {
			// Check if URI was changed (meaning YAML was loaded)
			if cfg.Mongo.URI != originalURI {
				// YAML was loaded but database not set, try to parse from URI
				if dbName := parseDatabaseFromURI(cfg.Mongo.URI); dbName != "" {
					cfg.Mongo.Database = dbName
				} else {
					// Use default
					cfg.Mongo.Database = originalDatabase
				}
			}
			// If URI wasn't changed, keep the default database
		}
	}

	// Override with environment variables
	loadFromEnv(cfg)

	// Resolve database name: use database field if set, otherwise parse from URI, otherwise use default
	if cfg.Mongo.Database == "" {
		if dbName := parseDatabaseFromURI(cfg.Mongo.URI); dbName != "" {
			cfg.Mongo.Database = dbName
		}
		// If still empty, use default value (already set in initial struct)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid configuration")
	}

	return cfg, nil
}

// parseDatabaseFromURI extracts database name from MongoDB URI
// Format: mongodb://[username:password@]host[:port][/database][?options]
func parseDatabaseFromURI(uri string) string {
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return ""
	}

	// Extract database name from path
	// Path format: /database or /database?options
	path := strings.TrimPrefix(parsedURL.Path, "/")
	if path == "" {
		return ""
	}

	// Remove query parameters if present
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}

	return path
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv(cfg *Config) {
	if v := os.Getenv("MONGO_URI"); v != "" {
		cfg.Mongo.URI = v
	}
	if v := os.Getenv("MONGO_DATABASE"); v != "" {
		cfg.Mongo.Database = v
	}
	if v := os.Getenv("RPC_ENDPOINT"); v != "" {
		cfg.RPC.Endpoint = v
	}
	if v := os.Getenv("COLD_START_TARGET_HEIGHT"); v != "" {
		var h uint32
		if _, err := fmt.Sscanf(v, "%d", &h); err == nil {
			cfg.ColdStart.TargetHeight = h
		}
	}
	if v := os.Getenv("INGEST_LISTEN_ADDR"); v != "" {
		cfg.Ingest.ListenAddr = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Mongo.URI == "" {
		return errors.New("mongo.uri is required")
	}
	
	// Ensure database is set: use database field if set, otherwise parse from URI, otherwise use default
	if c.Mongo.Database == "" {
		if dbName := parseDatabaseFromURI(c.Mongo.URI); dbName != "" {
			c.Mongo.Database = dbName
		} else {
			// Use default if still empty
			c.Mongo.Database = "steemdb"
		}
	}
	if c.RPC.Endpoint == "" {
		return errors.New("rpc.endpoint is required")
	}
	if c.Batch.Size <= 0 {
		return errors.New("batch.size must be > 0")
	}
	if c.Ingest.QueueSize <= 0 {
		return errors.New("ingest.queue_size must be > 0")
	}
	return nil
}

// BatchFlushInterval returns the batch flush interval as time.Duration
func (c *Config) BatchFlushInterval() (time.Duration, error) {
	return time.ParseDuration(c.Batch.FlushInterval)
}

// RPCTimeout returns the RPC timeout as time.Duration
func (c *Config) RPCTimeout() (time.Duration, error) {
	return time.ParseDuration(c.RPC.Timeout)
}
