package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Mongo     MongoConfig     `yaml:"mongo"`
	RPC       RPCConfig       `yaml:"rpc"`
	ColdStart ColdStartConfig `yaml:"cold_start"`
	Batch     BatchConfig     `yaml:"batch"`
	Ingest    IngestConfig    `yaml:"ingest"`
	Processor ProcessorConfig `yaml:"processor"`
	Refresher RefresherConfig `yaml:"refresher"`
	LiveSync  LiveSyncConfig  `yaml:"live_sync"`
	Log       LogConfig       `yaml:"log"`
}

// LiveSyncConfig contains live_sync settings. Catch-up (gap larger than
// follow_threshold) fetches in parallel chunks; near the chain head the loop
// degrades to single-block following.
type LiveSyncConfig struct {
	// ChunkSize is the block count of one batch-fetch unit during catch-up.
	// Env: LIVE_SYNC_CHUNK_SIZE
	ChunkSize int `yaml:"chunk_size"`
	// FollowThreshold: gaps up to this size are handled by the follow loop.
	// Env: LIVE_SYNC_FOLLOW_THRESHOLD
	FollowThreshold int `yaml:"follow_threshold"`
	// RPCConcurrency bounds the SDK v2 worker pool (per range call).
	// Env: LIVE_SYNC_RPC_CONCURRENCY
	RPCConcurrency int `yaml:"rpc_concurrency"`
}

// RefresherConfig contains refresher-process settings (witness/stats/clients/
// funds tickers, optional full account rescan). Replaces legacy history.py +
// witnesses.py (PROCESSOR_PLAN.md Batch 7/9).
type RefresherConfig struct {
	Enabled       bool                `yaml:"enabled"`
	Witness       RefresherTickConfig `yaml:"witness"`
	Stats         RefresherTickConfig `yaml:"stats"`
	Clients       RefresherTickConfig `yaml:"clients"`
	Funds         RefresherTickConfig `yaml:"funds"`
	AccountRescan AccountRescanConfig `yaml:"account_rescan"`
}

// RefresherTickConfig is the common enabled/interval pair for a refresher ticker
type RefresherTickConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"`
}

// AccountRescanConfig contains full account rescan settings (default off —
// the dirty-based AccountRefresher covers active accounts)
type AccountRescanConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Interval     string `yaml:"interval"`
	RPCBatchSize int    `yaml:"rpc_batch_size"` // accounts per get_accounts call
}

// ProcessorConfig contains operation processor settings
type ProcessorConfig struct {
	Enabled      bool   `yaml:"enabled"`
	CatchUpSleep string `yaml:"catch_up_sleep"`
	StartHeight  uint32 `yaml:"start_height"`
	// WindowSize is the number of blocks fetched/dispatched per loop
	// iteration (window-level cursor commit). 0 = default (64).
	WindowSize int `yaml:"window_size"`
	// BufferLimit caps the per-collection write buffer size (enforced by the
	// inserter). 0 = default (5000).
	BufferLimit int `yaml:"buffer_limit"`
	// SkipErrorOpsAfterRetries is the escape hatch for poison operations:
	// when an op's handler has failed for this many consecutive window
	// attempts, the op is skipped (not dispatched) with a loud log and the
	// window may commit past it. 0 (default) never skips — a stalled cursor
	// is visible (repeated per-attempt error logs, frozen
	// status.processor_height), while silently skipped ops are not.
	// Env: PROCESSOR_SKIP_ERROR_OPS_AFTER_RETRIES
	SkipErrorOpsAfterRetries int                    `yaml:"skip_error_ops_after_retries"`
	AccountRefresher         AccountRefresherConfig `yaml:"account_refresher"`
	CommentRescanner         CommentRescannerConfig `yaml:"comment_rescanner"`
}

// AccountRefresherConfig contains account dirty-refresh settings
type AccountRefresherConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Interval       string `yaml:"interval"`
	BatchSize      int    `yaml:"batch_size"`
	RPCBatchSize   int    `yaml:"rpc_batch_size"`
	Workers        int    `yaml:"workers"`
	ColdStartPause bool   `yaml:"cold_start_pause"`
}

// CommentRescannerConfig contains comment rescan settings
type CommentRescannerConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Interval   string `yaml:"interval"`
	BatchSize  int    `yaml:"batch_size"`
	Workers    int    `yaml:"workers"`
	WindowDays int    `yaml:"window_days"`
	StaleHours int    `yaml:"stale_hours"`
}

// MongoConfig contains MongoDB connection settings
type MongoConfig struct {
	URI         string `yaml:"uri" env:"MONGO_URI"`
	Database    string `yaml:"database" env:"MONGO_DATABASE"`
	MinPoolSize int    `yaml:"min_pool_size" env:"MONGO_MIN_POOL_SIZE"`
	MaxPoolSize int    `yaml:"max_pool_size" env:"MONGO_MAX_POOL_SIZE"`
}

// RPCConfig contains Steem RPC node settings
type RPCConfig struct {
	Endpoint string `yaml:"endpoint" env:"RPC_ENDPOINT"`
	MaxRetry int    `yaml:"max_retry" env:"RPC_MAX_RETRY"`
	Timeout  string `yaml:"timeout" env:"RPC_TIMEOUT"`
}

// ColdStartConfig contains cold start phase settings
type ColdStartConfig struct {
	TargetHeight uint32 `yaml:"target_height" env:"COLD_START_TARGET_HEIGHT"`
	SafetyMargin uint32 `yaml:"safety_margin" env:"COLD_START_SAFETY_MARGIN"`
}

// BatchConfig contains batch processing settings
type BatchConfig struct {
	Size          int    `yaml:"size" env:"BATCH_SIZE"`
	FlushInterval string `yaml:"flush_interval" env:"BATCH_FLUSH_INTERVAL"`
}

// IngestConfig contains ingest service settings
type IngestConfig struct {
	// ListenAddr is the HTTP listen address for the unauthenticated
	// ingest endpoint. It defaults to loopback: the steemd ingest plugin
	// posts to http://localhost:8080/ingest/applied_ops, and anyone who
	// can reach this endpoint can write arbitrary operations into the
	// database. Bind a wildcard address (":8080" / "0.0.0.0:8080") only
	// on a trusted network (e.g. a docker network shared with steemd).
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
			URI:         "mongodb://localhost:27017",
			Database:    "steemdb",
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
			Size:          1000,
			FlushInterval: "1s",
		},
		Ingest: IngestConfig{
			// Loopback by default: the endpoint has no authentication, so
			// it must only be reachable by the local steemd ingest plugin.
			ListenAddr: "127.0.0.1:8080",
			QueueSize:  100000,
		},
		Processor: ProcessorConfig{
			Enabled:      true,
			CatchUpSleep: "1s",
			StartHeight:  0,
			// 0 = never skip poison ops (see SkipErrorOpsAfterRetries).
			SkipErrorOpsAfterRetries: 0,
			AccountRefresher: AccountRefresherConfig{
				Enabled:        true,
				Interval:       "30s",
				BatchSize:      500,
				RPCBatchSize:   100,
				Workers:        8,
				ColdStartPause: true,
			},
			CommentRescanner: CommentRescannerConfig{
				Enabled:    true,
				Interval:   "60s",
				BatchSize:  100,
				Workers:    5,
				WindowDays: 3,
				StaleHours: 6,
			},
		},
		Refresher: RefresherConfig{
			Enabled: true,
			Witness: RefresherTickConfig{Enabled: true, Interval: "30s"},
			Stats:   RefresherTickConfig{Enabled: true, Interval: "5m"},
			Clients: RefresherTickConfig{Enabled: true, Interval: "1h"},
			Funds:   RefresherTickConfig{Enabled: true, Interval: "1h"},
			AccountRescan: AccountRescanConfig{
				Enabled:      false,
				Interval:     "24h",
				RPCBatchSize: 100,
			},
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
	if v := os.Getenv("PROCESSOR_WINDOW_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Processor.WindowSize = n
		}
	}
	if v := os.Getenv("PROCESSOR_BUFFER_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Processor.BufferLimit = n
		}
	}
	if v := os.Getenv("PROCESSOR_SKIP_ERROR_OPS_AFTER_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.Processor.SkipErrorOpsAfterRetries = n
		}
	}
	if v := os.Getenv("LIVE_SYNC_CHUNK_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.LiveSync.ChunkSize = n
		}
	}
	if v := os.Getenv("LIVE_SYNC_FOLLOW_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.LiveSync.FollowThreshold = n
		}
	}
	if v := os.Getenv("LIVE_SYNC_RPC_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.LiveSync.RPCConcurrency = n
		}
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
	if c.Processor.SkipErrorOpsAfterRetries < 0 {
		return errors.New("processor.skip_error_ops_after_retries must be >= 0")
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

// ProcessorCatchUpSleep returns the catch-up sleep duration
func (c *Config) ProcessorCatchUpSleep() (time.Duration, error) {
	return time.ParseDuration(c.Processor.CatchUpSleep)
}

// AccountRefresherInterval returns the refresher interval duration
func (c *Config) AccountRefresherInterval() (time.Duration, error) {
	return time.ParseDuration(c.Processor.AccountRefresher.Interval)
}

// CommentRescannerInterval returns the comment rescanner interval duration
func (c *Config) CommentRescannerInterval() (time.Duration, error) {
	return time.ParseDuration(c.Processor.CommentRescanner.Interval)
}

// RefresherWitnessInterval returns the witness ticker interval duration
func (c *Config) RefresherWitnessInterval() (time.Duration, error) {
	return time.ParseDuration(c.Refresher.Witness.Interval)
}

// RefresherStatsInterval returns the stats ticker interval duration
func (c *Config) RefresherStatsInterval() (time.Duration, error) {
	return time.ParseDuration(c.Refresher.Stats.Interval)
}

// RefresherClientsInterval returns the clients ticker interval duration
func (c *Config) RefresherClientsInterval() (time.Duration, error) {
	return time.ParseDuration(c.Refresher.Clients.Interval)
}

// RefresherFundsInterval returns the funds ticker interval duration
func (c *Config) RefresherFundsInterval() (time.Duration, error) {
	return time.ParseDuration(c.Refresher.Funds.Interval)
}

// RefresherAccountRescanInterval returns the account rescan interval duration
func (c *Config) RefresherAccountRescanInterval() (time.Duration, error) {
	return time.ParseDuration(c.Refresher.AccountRescan.Interval)
}
