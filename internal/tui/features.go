package tui

import (
	"os"
	"strings"
)

type MainScreenResetMode string

const (
	MainScreenResetModeFull    MainScreenResetMode = "full"
	MainScreenResetModeVisible MainScreenResetMode = "visible"
)

// TUIFeatures controls experimental TUI features via environment variables.
// These flags are consumed by the rendering/scrolling/overlay code paths
// in later refactoring phases. Phase 0 only establishes the mechanism;
// actual branching is wired in Phase 1+ (see docs/重构/tui-重构/).
type TUIFeatures struct {
	BlockRenderer bool // Always true — block-based rendering is the only path
	VirtualScroll bool // TODO(phase2): wire into viewport update logic
	OverlayStack  bool // TODO(phase3): wire into dialogs.go state machine
	LayoutRegion  bool // TODO(phase5): declarative height calculation via Region interface
	VimMode       bool // Phase 6: vim normal mode for chat viewport navigation
	VisualV2      bool // Always true — V2 visual style is the only path
	StatusV2      bool // Wave 1: three-segment status bar
	ToolGroups    bool // Wave 3: consecutive same-type tool calls auto-grouped
	MemoryUI      bool // Wave 3: memory status pill + toast + selector overlay
}

// DefaultFeatures returns features with all core rendering paths enabled.
func DefaultFeatures() TUIFeatures {
	return TUIFeatures{
		BlockRenderer: true,
		VisualV2:      true,
		StatusV2:      true,
		ToolGroups:    true,
		MemoryUI:      true,
	}
}

// Validate checks feature flag dependencies and returns an error
// describing any invalid combinations.
func (f TUIFeatures) Validate() error {
	return nil
}

// Sanitize disables flags whose dependencies are not met, ensuring a valid
// combination without failing. Logs a warning when flags are auto-disabled.
// Returns the corrected features.
func (f TUIFeatures) Sanitize() TUIFeatures {
	// BlockRenderer and VisualV2 are always forced on — legacy paths removed.
	f.BlockRenderer = true
	f.VisualV2 = true
	return f
}

// FeaturesFromEnv reads feature flags from environment variables.
// Set OS_TUI_BLOCK_RENDERER=1, OS_TUI_VIRTUAL_SCROLL=1, OS_TUI_OVERLAY_STACK=1,
// OS_TUI_LAYOUT_REGION=1 to enable respective features.
func FeaturesFromEnv() TUIFeatures {
	f := DefaultFeatures() // BlockRenderer + VisualV2 always on
	if os.Getenv("OS_TUI_VIRTUAL_SCROLL") == "1" {
		f.VirtualScroll = true
	}
	if os.Getenv("OS_TUI_OVERLAY_STACK") == "1" {
		f.OverlayStack = true
	}
	if os.Getenv("OS_TUI_LAYOUT_REGION") == "1" {
		f.LayoutRegion = true
	}
	if os.Getenv("OS_TUI_VIM_MODE") == "1" {
		f.VimMode = true
	}
	if os.Getenv("OS_TUI_STATUS_V2") == "1" {
		f.StatusV2 = true
	}
	if os.Getenv("OS_TUI_TOOL_GROUPS") == "1" {
		f.ToolGroups = true
	}
	if os.Getenv("OS_TUI_MEMORY_UI") == "1" {
		f.MemoryUI = true
	}
	return f
}

// FullscreenEnabled controls whether the interactive TUI should use the
// alternate screen buffer. Default is main-screen; fullscreen is opt-in via
// OS_TUI_FULLSCREEN truthy values: 1/true/on/fullscreen (trimmed,
// case-insensitive). Falsy values 0/false/off/no and empty/unset keep
// main-screen mode.
//
// Product contract: main-screen keeps native terminal scrollback and does not
// promise retroactive reflow of already flushed native rows after resize.
// Fullscreen/alt-screen is the supported full-history reflow path.
func FullscreenEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OS_TUI_FULLSCREEN"))) {
	case "1", "true", "on", "fullscreen":
		return true
	default:
		return false
	}
}

func MainScreenResetModeFromEnv() MainScreenResetMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OS_TUI_MAIN_RESET"))) {
	case "visible":
		return MainScreenResetModeVisible
	default:
		return MainScreenResetModeFull
	}
}
