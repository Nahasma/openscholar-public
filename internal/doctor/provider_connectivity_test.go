package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/connectivity"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestFormatProviderConnectivityReport_MiniMaxMetadataAndNoKeyLeak(t *testing.T) {
	report := ProviderConnectivityReport{
		Rows: []ProviderConnectivityRow{
			{
				Provider: "minimax",
				Model:    "MiniMax-M2.7",
				Status:   "MATRIX",
				MiniMax: &MiniMaxConnectivity{
					Provider:          models.ProviderMiniMax,
					Model:             "MiniMax-M2.7",
					ConfiguredBaseURL: "https://api.minimaxi.com/anthropic",
					AuthSource:        "env",
					KeyPresent:        true,
					KeyLength:         len("super-secret-key"),
					KeyFingerprint:    connectivity.KeyFingerprint("super-secret-key"),
					Probes: []MiniMaxProbe{
						{
							EndpointFlavor: "token-plan",
							BaseURL:        "https://api.minimaxi.com/anthropic",
							AuthMode:       connectivity.ProbeAuthModeBearer,
							HTTPStatus:     401,
							ErrorType:      "authentication_error",
							ErrorMessage:   "invalid key [REDACTED]",
							RequestID:      "req-1",
						},
					},
					Recommendation: "env key overrides config key",
				},
			},
		},
	}
	out := FormatProviderConnectivityReport(report)
	for _, expected := range []string{
		"configured_base_url: https://api.minimaxi.com/anthropic",
		"auth_source: env",
		"key_present: true",
		"key_length: 16",
		"key_fingerprint:",
		"flavor=token-plan auth_mode=bearer base_url=https://api.minimaxi.com/anthropic http_status=401",
		"error_type=authentication_error",
		"request_id=req-1",
		"recommendation: env key overrides config key",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("missing %q in output:\n%s", expected, out)
		}
	}
	if strings.Contains(out, "super-secret-key") {
		t.Fatalf("output leaked raw key:\n%s", out)
	}
}

func TestCollectProviderConnectivity_MiniMaxLoadedConfigKeySource(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	t.Setenv("MINIMAX_API_KEY", "")

	dir := t.TempDir()
	key := "config-only-minimax-key"
	if err := config.SaveFull(dir, config.Config{
		DefaultProvider: models.ProviderMiniMax,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderMiniMax: {APIKey: key, Model: "MiniMax-M2.7"},
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := config.ProviderAuthSource(loaded, models.ProviderMiniMax); got != string(config.AuthConfig) {
		t.Fatalf("precondition auth source = %q, want config", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report := CollectProviderConnectivity(ctx, loaded)
	if len(report.Rows) != 1 || report.Rows[0].MiniMax == nil {
		t.Fatalf("expected one MiniMax row, got %+v", report.Rows)
	}
	m := report.Rows[0].MiniMax
	if m.AuthSource != string(config.AuthConfig) {
		t.Fatalf("MiniMax auth source = %q, want config", m.AuthSource)
	}
	if strings.Contains(m.Recommendation, "env key overrides") {
		t.Fatalf("config-only key should not produce env override recommendation: %q", m.Recommendation)
	}
}

func TestMiniMaxFixedBaseURLDedupesTrailingSlash(t *testing.T) {
	for _, base := range []string{
		"https://api.minimaxi.com/anthropic/",
		"https://api.minimax.io/anthropic/",
	} {
		if !isMiniMaxFixedBaseURL(base) {
			t.Fatalf("expected fixed MiniMax base URL with trailing slash to dedupe: %q", base)
		}
	}
}

func TestEffectiveConfiguredBaseURL_MiniMaxProfileLegacy(t *testing.T) {
	got := effectiveConfiguredBaseURL("", models.ProviderMiniMax, "legacy", "")
	if got != "https://api.minimax.io/anthropic" {
		t.Fatalf("configured baseURL=%q, want legacy endpoint", got)
	}
}

func TestEffectiveConfiguredBaseURL_MiniMaxDefaultUsesOfficialEndpoint(t *testing.T) {
	got := effectiveConfiguredBaseURL("", models.ProviderMiniMax, "", "")
	if got != "https://api.minimaxi.com/anthropic" {
		t.Fatalf("configured baseURL=%q, want official endpoint", got)
	}
}
