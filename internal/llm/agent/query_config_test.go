package agent

import (
	"testing"

	"github.com/openscholar/openscholar/internal/config"
)

func TestBuildQueryConfig_DefaultUsesAbsoluteAutoCompactBuffer(t *testing.T) {
	cfg := &config.Config{Harness: config.DefaultHarnessConfig()}
	q := BuildQueryConfig("s", "m", 200000, 20000, cfg)
	if q.Compact.AutoCompactBufferTokens != 13000 {
		t.Fatalf("expected auto buffer 13000, got %d", q.Compact.AutoCompactBufferTokens)
	}
	if q.Compact.BlockingBufferTokens != 3000 {
		t.Fatalf("expected blocking buffer 3000, got %d", q.Compact.BlockingBufferTokens)
	}
}

func TestBuildQueryConfig_LegacyRatioStillWorksWhenConfigured(t *testing.T) {
	cfg := &config.Config{
		Harness: config.HarnessConfig{
			MicroCompactEnabled:       true,
			AutoCompactThresholdRatio: 0.8,
		},
	}
	q := BuildQueryConfig("s", "m", 100000, 10000, cfg)
	wantBuffer := int(float64(90000) * (1.0 - 0.8))
	if q.Compact.AutoCompactBufferTokens != wantBuffer {
		t.Fatalf("expected legacy ratio buffer %d, got %d", wantBuffer, q.Compact.AutoCompactBufferTokens)
	}
}
