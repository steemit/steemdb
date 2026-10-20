package utils

import (
	"os"
	"testing"
)

const testUmamiWebsiteID = "94db1cb1-74f4-4a40-ad6c-9623636803c4"

// TestNormalizeAnalyticsFailClosed verifies that the analytics config
// canonicalization degrades to "none" for every broken or hostile input, so
// the SPA never receives a half-configured snippet (e.g. a provider without
// its credential, or a GA "ID" carrying script characters).
func TestNormalizeAnalyticsFailClosed(t *testing.T) {
	cases := []struct {
		name            string
		in              AnalyticsConfig
		wantProvider    string
		wantGAID        string
		wantUmamiURL    string
		wantUmamiSiteID string
	}{
		{
			name:            "zero value stays none",
			in:              AnalyticsConfig{},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "unknown provider downgrades",
			in:              AnalyticsConfig{Provider: "matomo", GAID: "G-ABC"},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "ga without id downgrades",
			in:              AnalyticsConfig{Provider: "ga"},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "ga with legacy UA id downgrades",
			in:              AnalyticsConfig{Provider: "ga", GAID: "UA-12345-1"},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "uppercase provider is accepted",
			in:              AnalyticsConfig{Provider: "GA", GAID: "G-ABC123"},
			wantProvider:    "ga",
			wantGAID:        "G-ABC123",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "ga id with script characters is scrubbed then fails format",
			in:              AnalyticsConfig{Provider: "ga", GAID: "G-ABC);alert(1);//"},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name: "ga drops the umami credentials it does not use",
			in: AnalyticsConfig{
				Provider:       "ga",
				GAID:           "G-ABC123",
				UmamiURL:       "https://umami.example.com",
				UmamiWebsiteID: testUmamiWebsiteID,
			},
			wantProvider:    "ga",
			wantGAID:        "G-ABC123",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "umami without url downgrades",
			in:              AnalyticsConfig{Provider: "umami", UmamiWebsiteID: testUmamiWebsiteID},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "umami without website id downgrades",
			in:              AnalyticsConfig{Provider: "umami", UmamiURL: "https://umami.example.com"},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "umami with non-uuid website id downgrades",
			in:              AnalyticsConfig{Provider: "umami", UmamiURL: "https://umami.example.com", UmamiWebsiteID: "not-a-uuid"},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "umami relative url downgrades",
			in:              AnalyticsConfig{Provider: "umami", UmamiURL: "/umami", UmamiWebsiteID: testUmamiWebsiteID},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "umami javascript url downgrades",
			in:              AnalyticsConfig{Provider: "umami", UmamiURL: "javascript:alert(1)", UmamiWebsiteID: testUmamiWebsiteID},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name: "umami valid url and website id are kept (trailing slash trimmed)",
			in: AnalyticsConfig{
				Provider:       "umami",
				UmamiURL:       " https://umami.example.com/ ",
				UmamiWebsiteID: " " + testUmamiWebsiteID + " ",
			},
			wantProvider:    "umami",
			wantGAID:        "",
			wantUmamiURL:    "https://umami.example.com",
			wantUmamiSiteID: testUmamiWebsiteID,
		},
		{
			name: "umami drops the ga credential it does not use",
			in: AnalyticsConfig{
				Provider:       "umami",
				GAID:           "G-ABC123",
				UmamiURL:       "https://umami.example.com",
				UmamiWebsiteID: testUmamiWebsiteID,
			},
			wantProvider:    "umami",
			wantGAID:        "",
			wantUmamiURL:    "https://umami.example.com",
			wantUmamiSiteID: testUmamiWebsiteID,
		},
		{
			name:            "unset provider with valid ga id falls back to ga (legacy)",
			in:              AnalyticsConfig{GAID: "G-LEGACY1"},
			wantProvider:    "ga",
			wantGAID:        "G-LEGACY1",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "unset provider with invalid ga id stays none",
			in:              AnalyticsConfig{GAID: "G-abc"},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
		},
		{
			name:            "explicit none ignores a valid ga id",
			in:              AnalyticsConfig{Provider: "none", GAID: "G-ABC123"},
			wantProvider:    "none",
			wantGAID:        "",
			wantUmamiURL:    "",
			wantUmamiSiteID: "",
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
			if got.UmamiWebsiteID != tc.wantUmamiSiteID {
				t.Errorf("UmamiWebsiteID = %q, want %q", got.UmamiWebsiteID, tc.wantUmamiSiteID)
			}
		})
	}
}

// TestLoadConfigAnalyticsEnvBinding proves the flat operator env surface
// (ANALYTICS_PROVIDER, GA_ID, UMAMI_URL, UMAMI_WEBSITE_ID) reaches the
// analytics config: ANALYTICS_PROVIDER resolves through its explicit BindEnv
// (no default, so "unset" stays distinguishable from "none"); the credential
// keys need explicit BindEnv because the nested names would otherwise be
// ANALYTICS_GA_ID / ANALYTICS_UMAMI_URL / ANALYTICS_UMAMI_WEBSITE_ID.
func TestLoadConfigAnalyticsEnvBinding(t *testing.T) {
	t.Setenv("ANALYTICS_PROVIDER", "umami")
	t.Setenv("GA_ID", "")
	t.Setenv("UMAMI_URL", "https://umami.example.com")
	t.Setenv("UMAMI_WEBSITE_ID", testUmamiWebsiteID)

	cfg, err := LoadConfig(writeTestConfig(t, "development"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg.Analytics.Normalize()

	if cfg.Analytics.Provider != "umami" {
		t.Errorf("Analytics.Provider = %q, want %q", cfg.Analytics.Provider, "umami")
	}
	if cfg.Analytics.GAID != "" {
		t.Errorf("Analytics.GAID = %q, want empty", cfg.Analytics.GAID)
	}
	if cfg.Analytics.UmamiURL != "https://umami.example.com" {
		t.Errorf("Analytics.UmamiURL = %q, want %q", cfg.Analytics.UmamiURL, "https://umami.example.com")
	}
	if cfg.Analytics.UmamiWebsiteID != testUmamiWebsiteID {
		t.Errorf("Analytics.UmamiWebsiteID = %q, want %q", cfg.Analytics.UmamiWebsiteID, testUmamiWebsiteID)
	}
}

// TestLoadConfigLegacyGAIDOnly verifies the migration path for operators of
// the pre-/api/v1/meta deployment: with ANALYTICS_PROVIDER unset, a valid
// GA_ID alone still enables Google Analytics — nobody silently loses
// tracking on upgrade.
func TestLoadConfigLegacyGAIDOnly(t *testing.T) {
	os.Unsetenv("ANALYTICS_PROVIDER")
	t.Setenv("GA_ID", "G-ENVLEGACY")
	os.Unsetenv("UMAMI_URL")
	os.Unsetenv("UMAMI_WEBSITE_ID")

	cfg, err := LoadConfig(writeTestConfig(t, "development"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg.Analytics.Normalize()

	if cfg.Analytics.Provider != "ga" {
		t.Errorf("Analytics.Provider = %q, want %q (legacy GA_ID-only fallback)", cfg.Analytics.Provider, "ga")
	}
	if cfg.Analytics.GAID != "G-ENVLEGACY" {
		t.Errorf("Analytics.GAID = %q, want %q", cfg.Analytics.GAID, "G-ENVLEGACY")
	}
}

// TestLoadConfigAnalyticsDefaultsNone verifies tracking is off by default:
// with no env vars and no config-file entry the provider is "none".
func TestLoadConfigAnalyticsDefaultsNone(t *testing.T) {
	os.Unsetenv("ANALYTICS_PROVIDER")
	os.Unsetenv("GA_ID")
	os.Unsetenv("UMAMI_URL")
	os.Unsetenv("UMAMI_WEBSITE_ID")

	cfg, err := LoadConfig(writeTestConfig(t, "development"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg.Analytics.Normalize()

	if cfg.Analytics.Provider != "none" {
		t.Errorf("Analytics.Provider = %q, want %q", cfg.Analytics.Provider, "none")
	}
}
