package tui

import (
	"fmt"
	"testing"
)

// envFlagNames maps optional TUIFeatures field names to their environment variable names.
// BlockRenderer and VisualV2 are always on and not listed here.
var envFlagNames = []struct {
	envKey string
	get    func(TUIFeatures) bool
	name   string
}{
	{"OS_TUI_VIRTUAL_SCROLL", func(f TUIFeatures) bool { return f.VirtualScroll }, "VirtualScroll"},
	{"OS_TUI_OVERLAY_STACK", func(f TUIFeatures) bool { return f.OverlayStack }, "OverlayStack"},
	{"OS_TUI_LAYOUT_REGION", func(f TUIFeatures) bool { return f.LayoutRegion }, "LayoutRegion"},
	{"OS_TUI_VIM_MODE", func(f TUIFeatures) bool { return f.VimMode }, "VimMode"},
}

// clearAllFeatureEnvs sets all OS_TUI_* variables to empty string in the test
// environment so there is no bleed-through from the parent process.
func clearAllFeatureEnvs(t *testing.T) {
	t.Helper()
	for _, flag := range envFlagNames {
		t.Setenv(flag.envKey, "")
	}
}

// TestFeaturesCombination_AlwaysOnFlags verifies that BlockRenderer and VisualV2
// are always enabled regardless of env vars.
func TestFeaturesCombination_AlwaysOnFlags(t *testing.T) {
	clearAllFeatureEnvs(t)
	f := FeaturesFromEnv()
	if !f.BlockRenderer {
		t.Error("BlockRenderer should always be true")
	}
	if !f.VisualV2 {
		t.Error("VisualV2 should always be true")
	}
}

// TestFeaturesCombination_AllOptionalOff verifies that with no env vars set
// all optional features are off but always-on features remain on.
func TestFeaturesCombination_AllOptionalOff(t *testing.T) {
	clearAllFeatureEnvs(t)
	f := FeaturesFromEnv()
	for _, flag := range envFlagNames {
		if flag.get(f) {
			t.Errorf("%s: expected false when env unset", flag.name)
		}
	}
}

// TestFeaturesCombination_EnvVarParsing tests exhaustive combinations of optional
// feature flags via environment variables.
func TestFeaturesCombination_EnvVarParsing(t *testing.T) {
	n := len(envFlagNames)
	total := 1 << n
	for mask := 0; mask < total; mask++ {
		t.Run(fmt.Sprintf("mask_%05b", mask), func(t *testing.T) {
			for i, flag := range envFlagNames {
				if mask&(1<<i) != 0 {
					t.Setenv(flag.envKey, "1")
				} else {
					t.Setenv(flag.envKey, "")
				}
			}
			f := FeaturesFromEnv()
			// Always-on flags
			if !f.BlockRenderer {
				t.Error("BlockRenderer should always be true")
			}
			if !f.VisualV2 {
				t.Error("VisualV2 should always be true")
			}
			// Optional flags match their env var
			for i, flag := range envFlagNames {
				expected := mask&(1<<i) != 0
				if flag.get(f) != expected {
					t.Errorf("%s: expected %v, got %v (mask=%05b)", flag.name, expected, flag.get(f), mask)
				}
			}
		})
	}
}

// TestFeaturesCombination_SanitizeForces verifies Sanitize always forces
// BlockRenderer and VisualV2 on.
func TestFeaturesCombination_SanitizeForces(t *testing.T) {
	f := TUIFeatures{}.Sanitize()
	if !f.BlockRenderer {
		t.Error("Sanitize should force BlockRenderer on")
	}
	if !f.VisualV2 {
		t.Error("Sanitize should force VisualV2 on")
	}
}

// TestFeaturesCombination_DependencyValidation verifies Validate always passes
// since BlockRenderer and VisualV2 are always on.
func TestFeaturesCombination_DependencyValidation(t *testing.T) {
	tests := []struct {
		name string
		f    TUIFeatures
	}{
		{
			name: "all features enabled",
			f:    TUIFeatures{BlockRenderer: true, VisualV2: true, VirtualScroll: true, OverlayStack: true, LayoutRegion: true, VimMode: true},
		},
		{
			name: "only always-on features",
			f:    DefaultFeatures(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.f.Validate(); err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}
