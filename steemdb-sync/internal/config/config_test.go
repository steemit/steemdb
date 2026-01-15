package config

import (
	"os"
	"testing"
)

func TestParseDatabaseFromURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "No database in URI",
			uri:      "mongodb://localhost:27017",
			expected: "",
		},
		{
			name:     "Database in URI",
			uri:      "mongodb://localhost:27017/steemdb",
			expected: "steemdb",
		},
		{
			name:     "Database in URI with auth",
			uri:      "mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin",
			expected: "steemdb_test",
		},
		{
			name:     "Database in URI with query params",
			uri:      "mongodb://user:pass@host:27017/mydb?authSource=admin&replicaSet=rs0",
			expected: "mydb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDatabaseFromURI(tt.uri)
			if result != tt.expected {
				t.Errorf("parseDatabaseFromURI(%q) = %q, want %q", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestLoadDatabaseResolution(t *testing.T) {
	// Test 1: database field set (should use it)
	t.Run("database field set", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test_config_*.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		configYAML := `mongo:
  uri: "mongodb://localhost:27017/other_db"
  database: "steemdb_test"
`
		if _, err := tmpFile.WriteString(configYAML); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
		tmpFile.Close()

		cfg, err := Load(tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		if cfg.Mongo.Database != "steemdb_test" {
			t.Errorf("Expected database 'steemdb_test', got '%s'", cfg.Mongo.Database)
		}
	})

	// Test 2: database field not set, parse from URI
	t.Run("database field not set, parse from URI", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test_config_*.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		configYAML := `mongo:
  uri: "mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin"
`
		if _, err := tmpFile.WriteString(configYAML); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
		tmpFile.Close()

		cfg, err := Load(tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		if cfg.Mongo.Database != "steemdb_test" {
			t.Errorf("Expected database 'steemdb_test' (from URI), got '%s'", cfg.Mongo.Database)
		}
	})

	// Test 3: neither set, use default
	t.Run("neither set, use default", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test_config_*.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		configYAML := `mongo:
  uri: "mongodb://localhost:27017"
`
		if _, err := tmpFile.WriteString(configYAML); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
		tmpFile.Close()

		cfg, err := Load(tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		if cfg.Mongo.Database != "steemdb" {
			t.Errorf("Expected database 'steemdb' (default), got '%s'", cfg.Mongo.Database)
		}
	})
}
