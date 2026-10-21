package utils

import (
	"os"
	"testing"
)

// TestNormalizeAnalyticsFailClosed verifies that the analytics config
// canonicalization degrades to "none" for every broken or hostile input, so
// the SPA never receives a half-configured snippet (e.g. a provider without
// its credential, or a GA "ID" carrying script characters).
func TestNormalizeAnalyticsFailClosed(t *testing.T) {
	cases := []struct {
		name           string
		in             AnalyticsConfig
		wantProvider   string
		wantGAID       string
		wantUmamiURL   string
	}{
		{
			name:           "zero value stays none",
			in:             AnalyticsConfig{},
			wantProvider:   "none",
			wantGAID:       "",
			wantUmamiURL:   "",
		},
		{
			name:           "unknown provider downgrades",
			in:             AnalyticsConfig{Provider: "matomo", GAID: "G-ABC"},
			wantProvider:   "none",
			wantGAID:       "",
			wantUmamiURL:   "",
		},
		{
			name:           "ga without id downgrades",
			in:             AnalyticsConfig{Provider: "ga"},
			wantProvider:   "none",
			wantGAID:       "",
			wantUmamiURL:   "",
		},
		{
			name:           "ga with legacy UA id downgrades",
			in:             AnalyticsConfig{Provider: "ga", GAID: "UA-12345-1"},
			wantProvider:   "none",
			wantGAID:       "",
			wantUmamiURL:   "",
		},
		{
			name:           "uppercase provider is accepted",
			in:             AnalyticsConfig{Provider: "GA", GAID: "G-ABC123"},
			wantProvider:   "ga",
			wantGAID:       "G-ABC123",
			wantUmamiURL:   "",
		},
		{
			name:           "ga id with script characters is scrubbed then fails format",
			in:             AnalyticsConfig{Provider: "ga", GAID: "G-ABC);alert(1);//"},
			wantProvider:   "none",
			wantGAID:       "",
			wantUmamiURL:   "",
		},
		{
			name:           "umami without url downgrades",
			in:             AnalyticsConfig{Provider: "umami"},
			wantProvider:   "none",
			wantGAID:       "",
			wantUmamiURL:   "",
		},
		{
			name:           "umami relative url downgrades",
			in:             AnalyticsConfig{Provider: "umami", UmamiURL: "/umami"},
			wantProvider:   "none",
			wantGAID:       "",
			wantUmamiURL:   "",
		},
		{
			name:           "umami javascript url downgrades",
			in:             AnalyticsConfig{Provider: "umami", UmamiURL: "javascript:alert(1)"},
			wantProvider:   "none",
			wantGAID:       "",
			wantUmamiURL:   "",
		},
		{
			name:           "umami valid url is kept",
			in:             AnalyticsConfig{Provider: "umami", UmamiURL: " https://umami.example.com "},
			wantProvider:   "umami",
			wantGAID:       "",
			wantUmamiURL:   "https://umami.example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in
			got.Normalize()
			if got.Provider != tc.wantProvider {
				t.Errorf("Provider = %q, want %q", got.Provider, tc.wantProvider)
			}
			if got.GAID != tc.wantGAID {
				t.Errorf("GAID = %q, want %q", got.GAID, tc.wantGAID)
			}
			if got.UmamiURL != tc.wantUmamiURL {
				t.Errorf("UmamiURL = %q, want %q", got.UmamiURL, tc.wantUmamiURL)
			}
		})
	}
}

// TestLoadConfigAnalyticsEnvBinding proves the flat operator env surface
// (ANALYTICS_PROVIDER, GA_ID, UMAMI_URL) reaches the analytics config:
// ANALYTICS_PROVIDER resolves through AutomaticEnv + the dot->underscore
// replacer; GA_ID/UMAMI_URL need explicit BindEnv because the nested keys
// would otherwise resolve to ANALYTICS_GA_ID/ANALYTICS_UMAMI_URL.
func TestLoadConfigAnalyticsEnvBinding(t *testing.T) {
	t.Setenv("ANALYTICS_PROVIDER", "ga")
	t.Setenv("GA_ID", "G-ENV123")
	t.Setenv("UMAMI_URL", "")

	cfg, err := LoadConfig(writeTestConfig(t, "development"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg.Analytics.Normalize()

	if cfg.Analytics.Provider != "ga" {
		t.Errorf("Analytics.Provider = %q, want %q", cfg.Analytics.Provider, "ga")
	}
	if cfg.Analytics.GAID != "G-ENV123" {
		t.Errorf("Analytics.GAID = %q, want %q", cfg.Analytics.GAID, "G-ENV123")
	}
	if cfg.Analytics.UmamiURL != "" {
		t.Errorf("Analytics.UmamiURL = %q, want empty", cfg.Analytics.UmamiURL)
	}
}

// TestLoadConfigAnalyticsDefaultsNone verifies tracking is off by default:
// with no env vars and no config-file entry the provider is "none".
func TestLoadConfigAnalyticsDefaultsNone(t *testing.T) {
	os.Unsetenv("ANALYTICS_PROVIDER")
	os.Unsetenv("GA_ID")
	os.Unsetenv("UMAMI_URL")

	cfg, err := LoadConfig(writeTestConfig(t, "development"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg.Analytics.Normalize()

	if cfg.Analytics.Provider != "none" {
		t.Errorf("Analytics.Provider = %q, want %q", cfg.Analytics.Provider, "none")
	}
}
