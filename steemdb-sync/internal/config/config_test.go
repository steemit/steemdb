package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setEnvVarsForTest sets the given env vars for the duration of the test.
// An empty value unsets the variable, so a polluted parent environment
// cannot leak into the cases below.
func setEnvVarsForTest(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		old, had := os.LookupEnv(k)
		if v == "" {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, v)
		}
		t.Cleanup(func() {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

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

// TestLoadDatabaseResolutionFromEnvURI guards the env-URI database-name bug:
// loadFromEnv used to replace only the URI string, so the database name
// resolved during the YAML phase (e.g. steemdb_test from the image-baked
// config) stayed in effect and a sync service silently wrote the wrong
// database while connecting with the correct URI.
func TestLoadDatabaseResolutionFromEnvURI(t *testing.T) {
	// Mirrors the image-baked configs/config.yaml: no `database` field, the
	// URI carries steemdb_test — the YAML phase resolves Database to
	// "steemdb_test".
	configYAML := `mongo:
  uri: "mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin"
`

	tests := []struct {
		name        string
		envURI      string // "" = MONGO_URI unset
		envDatabase string // "" = MONGO_DATABASE unset
		expectedURI string
		expectedDB  string
	}{
		{
			name:        "env URI with database, MONGO_DATABASE unset: use env URI database",
			envURI:      "mongodb://mongo:27017/steemdb",
			envDatabase: "",
			expectedURI: "mongodb://mongo:27017/steemdb",
			expectedDB:  "steemdb",
		},
		{
			name:        "env URI with database, MONGO_DATABASE set: MONGO_DATABASE wins",
			envURI:      "mongodb://mongo:27017/steemdb",
			envDatabase: "explicit_db",
			expectedURI: "mongodb://mongo:27017/steemdb",
			expectedDB:  "explicit_db",
		},
		{
			name:        "env URI without database segment: keep YAML-derived value",
			envURI:      "mongodb://mongo:27017",
			envDatabase: "",
			expectedURI: "mongodb://mongo:27017",
			expectedDB:  "steemdb_test",
		},
		{
			name:        "no env MONGO_URI: YAML behavior unchanged",
			envURI:      "",
			envDatabase: "",
			expectedURI: "mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin",
			expectedDB:  "steemdb_test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnvVarsForTest(t, map[string]string{
				"MONGO_URI":      tt.envURI,
				"MONGO_DATABASE": tt.envDatabase,
			})

			tmpFile := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(tmpFile, []byte(configYAML), 0644); err != nil {
				t.Fatalf("Failed to write config: %v", err)
			}

			cfg, err := Load(tmpFile)
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			if cfg.Mongo.URI != tt.expectedURI {
				t.Errorf("URI = %q, want %q", cfg.Mongo.URI, tt.expectedURI)
			}
			if cfg.Mongo.Database != tt.expectedDB {
				t.Errorf("Database = %q, want %q", cfg.Mongo.Database, tt.expectedDB)
			}
		})
	}
}
