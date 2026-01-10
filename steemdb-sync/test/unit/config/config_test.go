package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/steemit/steemdb-sync/internal/config"
)

// TestLoadConfig tests loading configuration from YAML file
func TestLoadConfig(t *testing.T) {
	// Get the project root directory (steemdb-sync)
	// From test/unit/config, we need to go up 3 levels
	wd, err := os.Getwd()
	require.NoError(t, err)
	
	// Navigate to project root
	projectRoot := filepath.Join(wd, "..", "..", "..")
	configPath := filepath.Join(projectRoot, "configs", "config.yaml")
	
	// Check if config file exists, if not skip this test
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Config file not found, skipping test")
	}
	
	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	
	// Verify basic fields are loaded
	assert.NotEmpty(t, cfg.Mongo.URI)
	assert.NotEmpty(t, cfg.Mongo.Database)
	assert.NotEmpty(t, cfg.RPC.Endpoint)
	assert.Greater(t, cfg.Batch.Size, 0)
	assert.Greater(t, cfg.Ingest.QueueSize, 0)
}

// TestLoadConfigWithDefaults tests loading with default values when file doesn't exist
func TestLoadConfigWithDefaults(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	
	// Verify default values
	assert.Equal(t, "mongodb://localhost:27017", cfg.Mongo.URI)
	assert.Equal(t, "steemdb", cfg.Mongo.Database)
	assert.Equal(t, "https://api.steemit.com", cfg.RPC.Endpoint)
	assert.Equal(t, 1000, cfg.Batch.Size)
	assert.Equal(t, ":8080", cfg.Ingest.ListenAddr)
	assert.Equal(t, 100000, cfg.Ingest.QueueSize)
}

// TestEnvOverride tests environment variable override
func TestEnvOverride(t *testing.T) {
	// Set environment variables
	os.Setenv("MONGO_URI", "mongodb://test:27017/testdb")
	os.Setenv("MONGO_DATABASE", "testdb")
	os.Setenv("RPC_ENDPOINT", "https://test.steemit.com")
	defer func() {
		os.Unsetenv("MONGO_URI")
		os.Unsetenv("MONGO_DATABASE")
		os.Unsetenv("RPC_ENDPOINT")
	}()
	
	cfg, err := config.Load("")
	require.NoError(t, err)
	
	// Verify environment variables override defaults
	assert.Equal(t, "mongodb://test:27017/testdb", cfg.Mongo.URI)
	assert.Equal(t, "testdb", cfg.Mongo.Database)
	assert.Equal(t, "https://test.steemit.com", cfg.RPC.Endpoint)
}

// TestInvalidConfigFile tests handling of invalid config file
func TestInvalidConfigFile(t *testing.T) {
	// Create a temporary invalid YAML file
	tmpFile := filepath.Join(t.TempDir(), "invalid.yaml")
	err := os.WriteFile(tmpFile, []byte("invalid: yaml: content: [unclosed"), 0644)
	require.NoError(t, err)
	
	cfg, err := config.Load(tmpFile)
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

// TestMissingConfigFile tests handling of missing config file
func TestMissingConfigFile(t *testing.T) {
	cfg, err := config.Load("/nonexistent/config.yaml")
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

// TestConfigValidation tests configuration validation
func TestConfigValidation(t *testing.T) {
	// Test with valid config
	cfg, err := config.Load("")
	require.NoError(t, err)
	err = cfg.Validate()
	assert.NoError(t, err)
}

// TestBatchFlushInterval tests parsing batch flush interval
func TestBatchFlushInterval(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	
	// Set a valid interval
	cfg.Batch.FlushInterval = "5s"
	duration, err := cfg.BatchFlushInterval()
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, duration)
	
	// Test invalid interval
	cfg.Batch.FlushInterval = "invalid"
	_, err = cfg.BatchFlushInterval()
	assert.Error(t, err)
}

// TestRPCTimeout tests parsing RPC timeout
func TestRPCTimeout(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	
	// Set a valid timeout
	cfg.RPC.Timeout = "30s"
	duration, err := cfg.RPCTimeout()
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, duration)
	
	// Test invalid timeout
	cfg.RPC.Timeout = "invalid"
	_, err = cfg.RPCTimeout()
	assert.Error(t, err)
}

// TestConfigDefaults tests all default values are set correctly
func TestConfigDefaults(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	
	// Verify all default values
	assert.Equal(t, "mongodb://localhost:27017", cfg.Mongo.URI)
	assert.Equal(t, "steemdb", cfg.Mongo.Database)
	assert.Equal(t, 10, cfg.Mongo.MinPoolSize)
	assert.Equal(t, 100, cfg.Mongo.MaxPoolSize)
	
	assert.Equal(t, "https://api.steemit.com", cfg.RPC.Endpoint)
	assert.Equal(t, 3, cfg.RPC.MaxRetry)
	assert.Equal(t, "30s", cfg.RPC.Timeout)
	
	assert.Equal(t, uint32(0), cfg.ColdStart.TargetHeight)
	assert.Equal(t, uint32(5), cfg.ColdStart.SafetyMargin)
	
	assert.Equal(t, 1000, cfg.Batch.Size)
	assert.Equal(t, "1s", cfg.Batch.FlushInterval)
	
	assert.Equal(t, ":8080", cfg.Ingest.ListenAddr)
	assert.Equal(t, 100000, cfg.Ingest.QueueSize)
	
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "text", cfg.Log.Format)
}

// TestColdStartTargetHeightEnv tests COLD_START_TARGET_HEIGHT environment variable
func TestColdStartTargetHeightEnv(t *testing.T) {
	os.Setenv("COLD_START_TARGET_HEIGHT", "1000000")
	defer os.Unsetenv("COLD_START_TARGET_HEIGHT")
	
	cfg, err := config.Load("")
	require.NoError(t, err)
	assert.Equal(t, uint32(1000000), cfg.ColdStart.TargetHeight)
}

// TestIngestListenAddrEnv tests INGEST_LISTEN_ADDR environment variable
func TestIngestListenAddrEnv(t *testing.T) {
	os.Setenv("INGEST_LISTEN_ADDR", ":9090")
	defer os.Unsetenv("INGEST_LISTEN_ADDR")
	
	cfg, err := config.Load("")
	require.NoError(t, err)
	assert.Equal(t, ":9090", cfg.Ingest.ListenAddr)
}

// TestLogLevelEnv tests LOG_LEVEL environment variable
func TestLogLevelEnv(t *testing.T) {
	os.Setenv("LOG_LEVEL", "debug")
	defer os.Unsetenv("LOG_LEVEL")
	
	cfg, err := config.Load("")
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg.Log.Level)
}
