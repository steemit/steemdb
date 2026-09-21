package utils

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestConfig writes a minimal config file with the given server mode
// and returns its path.
func writeTestConfig(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "server:\n  mode: " + mode + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

// TestLoadConfigServerModeFromFile verifies the mode is read from the
// config file when no environment override is set.
func TestLoadConfigServerModeFromFile(t *testing.T) {
	os.Unsetenv("SERVER_MODE")

	cfg, err := LoadConfig(writeTestConfig(t, "development"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Server.Mode != "development" {
		t.Errorf("Server.Mode = %q, want %q", cfg.Server.Mode, "development")
	}
}

// TestLoadConfigServerModeEnvOverride proves that SERVER_MODE overrides
// server.mode from the config file (P2-12: without an explicit BindEnv,
// viper's AutomaticEnv looks up "SERVER.MODE", which never exists, and
// production containers silently ran gin in debug mode).
func TestLoadConfigServerModeEnvOverride(t *testing.T) {
	t.Setenv("SERVER_MODE", "production")

	cfg, err := LoadConfig(writeTestConfig(t, "development"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Server.Mode != "production" {
		t.Errorf("Server.Mode = %q, want %q (SERVER_MODE must override the config file)", cfg.Server.Mode, "production")
	}
}
