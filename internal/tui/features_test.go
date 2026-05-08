package tui

import (
	"testing"
)

func TestFeatures_Validate(t *testing.T) {
	tests := []struct {
		name    string
		f       TUIFeatures
		wantErr bool
	}{
		{
			name:    "default features",
			f:       DefaultFeatures(),
			wantErr: false,
		},
		{
			name:    "all optional features",
			f:       TUIFeatures{BlockRenderer: true, VisualV2: true, VirtualScroll: true, OverlayStack: true},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.f.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFeatures_Sanitize(t *testing.T) {
	t.Run("forces BlockRenderer and VisualV2 on", func(t *testing.T) {
		f := TUIFeatures{}.Sanitize()
		if !f.BlockRenderer {
			t.Error("Sanitize should force BlockRenderer on")
		}
		if !f.VisualV2 {
			t.Error("Sanitize should force VisualV2 on")
		}
	})
	t.Run("keeps valid combination", func(t *testing.T) {
		f := TUIFeatures{BlockRenderer: true, VisualV2: true, VirtualScroll: true}.Sanitize()
		if !f.VirtualScroll || !f.BlockRenderer || !f.VisualV2 {
			t.Error("Sanitize should keep valid combination unchanged")
		}
	})
}

func TestDefaultFeatures(t *testing.T) {
	f := DefaultFeatures()
	if !f.BlockRenderer {
		t.Error("DefaultFeatures should have BlockRenderer enabled")
	}
	if !f.VisualV2 {
		t.Error("DefaultFeatures should have VisualV2 enabled")
	}
	if f.VirtualScroll || f.OverlayStack {
		t.Error("DefaultFeatures should have optional flags disabled")
	}
}

func TestFeaturesFromEnv(t *testing.T) {
	t.Run("defaults_always_on", func(t *testing.T) {
		t.Setenv("OS_TUI_VIRTUAL_SCROLL", "")
		t.Setenv("OS_TUI_OVERLAY_STACK", "")
		f := FeaturesFromEnv()
		if !f.BlockRenderer || !f.VisualV2 {
			t.Error("BlockRenderer and VisualV2 should always be on")
		}
		if f.VirtualScroll || f.OverlayStack {
			t.Error("optional flags should be off by default")
		}
	})

	t.Run("optional_flags", func(t *testing.T) {
		t.Setenv("OS_TUI_VIRTUAL_SCROLL", "1")
		t.Setenv("OS_TUI_OVERLAY_STACK", "1")
		f := FeaturesFromEnv()
		if !f.VirtualScroll || !f.OverlayStack {
			t.Error("optional flags should be enabled when set")
		}
	})

	t.Run("wave3_defaults_enabled", func(t *testing.T) {
		f := FeaturesFromEnv()
		if !f.ToolGroups {
			t.Error("ToolGroups should be enabled by default")
		}
		if !f.MemoryUI {
			t.Error("MemoryUI should be enabled by default")
		}
		if !f.StatusV2 {
			t.Error("StatusV2 should be enabled by default")
		}
	})
}

func TestFullscreenEnabled(t *testing.T) {
	t.Run("default unset is disabled", func(t *testing.T) {
		t.Setenv("OS_TUI_FULLSCREEN", "")
		if FullscreenEnabled() {
			t.Fatal("FullscreenEnabled should default to false when unset/empty")
		}
	})

	t.Run("truthy values enable fullscreen", func(t *testing.T) {
		for _, raw := range []string{"1", "true", "TRUE", " on ", "fullscreen", " FullScreen "} {
			t.Setenv("OS_TUI_FULLSCREEN", raw)
			if !FullscreenEnabled() {
				t.Fatalf("FullscreenEnabled should be true for %q", raw)
			}
		}
	})

	t.Run("falsy values keep main screen", func(t *testing.T) {
		for _, raw := range []string{"0", "false", "FALSE", "off", "no", " No "} {
			t.Setenv("OS_TUI_FULLSCREEN", raw)
			if FullscreenEnabled() {
				t.Fatalf("FullscreenEnabled should be false for %q", raw)
			}
		}
	})
}

func TestScreenModeFromEnv(t *testing.T) {
	t.Run("default is main", func(t *testing.T) {
		t.Setenv("OS_TUI_FULLSCREEN", "")
		if got := ScreenModeFromEnv(); got != ScreenModeMain {
			t.Fatalf("ScreenModeFromEnv() = %q, want %q", got, ScreenModeMain)
		}
	})

	t.Run("truthy enables fullscreen", func(t *testing.T) {
		t.Setenv("OS_TUI_FULLSCREEN", "true")
		if got := ScreenModeFromEnv(); got != ScreenModeFullscreen {
			t.Fatalf("ScreenModeFromEnv() = %q, want %q", got, ScreenModeFullscreen)
		}
	})

	t.Run("falsy uses main", func(t *testing.T) {
		t.Setenv("OS_TUI_FULLSCREEN", "off")
		if got := ScreenModeFromEnv(); got != ScreenModeMain {
			t.Fatalf("ScreenModeFromEnv() = %q, want %q", got, ScreenModeMain)
		}
	})
}

func TestMainScreenResetModeFromEnv(t *testing.T) {
	t.Run("default full", func(t *testing.T) {
		t.Setenv("OS_TUI_MAIN_RESET", "")
		if got := MainScreenResetModeFromEnv(); got != MainScreenResetModeFull {
			t.Fatalf("MainScreenResetModeFromEnv() = %q, want %q", got, MainScreenResetModeFull)
		}
	})
	t.Run("visible emergency override", func(t *testing.T) {
		t.Setenv("OS_TUI_MAIN_RESET", "visible")
		if got := MainScreenResetModeFromEnv(); got != MainScreenResetModeVisible {
			t.Fatalf("MainScreenResetModeFromEnv() = %q, want %q", got, MainScreenResetModeVisible)
		}
	})
	t.Run("full explicit", func(t *testing.T) {
		t.Setenv("OS_TUI_MAIN_RESET", "full")
		if got := MainScreenResetModeFromEnv(); got != MainScreenResetModeFull {
			t.Fatalf("MainScreenResetModeFromEnv() = %q, want %q", got, MainScreenResetModeFull)
		}
	})
}
