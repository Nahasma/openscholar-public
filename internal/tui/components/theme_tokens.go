package components

import (
	"github.com/charmbracelet/lipgloss"
)

// ThemeTokens defines semantic color tokens and spacing constants for the TUI.
// Each color provides TrueColor/ANSI256/ANSI16 fallbacks via CompleteColor.
type ThemeTokens struct {
	// Semantic colors
	Accent      lipgloss.CompleteColor
	TextPrimary lipgloss.CompleteColor
	TextMuted   lipgloss.CompleteColor
	SurfaceSub  lipgloss.CompleteColor
	BorderSub   lipgloss.CompleteColor
	Success     lipgloss.CompleteColor
	Warning     lipgloss.CompleteColor
	Danger      lipgloss.CompleteColor
	Info        lipgloss.CompleteColor

	// Role colors
	User      lipgloss.CompleteColor
	Assistant lipgloss.CompleteColor
	Tool      lipgloss.CompleteColor
	System    lipgloss.CompleteColor

	// UI element colors (CC visual alignment)
	Permission      lipgloss.CompleteColor // permission dialog border (CC blue)
	SecondaryBorder lipgloss.CompleteColor // input box default border
	UserBackground  lipgloss.CompleteColor // subtle user message background
	PromptBorder    lipgloss.CompleteColor // CC: fixed gray input border

	// Spacing constants
	GapXS       int // 1
	GapSM       int // 2
	InsetSM     int // 2
	InsetMD     int // 4
	GutterWidth int // 2 (left gutter)
}

// Theme is the global semantic theme instance, initialized in colors.go init().
var Theme ThemeTokens

// darkThemeTokens returns ThemeTokens for dark terminal backgrounds.
func darkThemeTokens() ThemeTokens {
	return ThemeTokens{
		// Semantic colors
		Accent:      lipgloss.CompleteColor{TrueColor: "#AF5FD7", ANSI256: "170", ANSI: "5"},
		TextPrimary: lipgloss.CompleteColor{TrueColor: "#E6E6E6", ANSI256: "252", ANSI: "7"},
		TextMuted:   lipgloss.CompleteColor{TrueColor: "#6C6C6C", ANSI256: "242", ANSI: "8"},
		SurfaceSub:  lipgloss.CompleteColor{TrueColor: "#2D2D2D", ANSI256: "236", ANSI: "0"},
		BorderSub:   lipgloss.CompleteColor{TrueColor: "#3E3E3E", ANSI256: "237", ANSI: "8"},
		Success:     lipgloss.CompleteColor{TrueColor: "#2C7A39", ANSI256: "34", ANSI: "2"},
		Warning:     lipgloss.CompleteColor{TrueColor: "#966C1E", ANSI256: "136", ANSI: "3"},
		Danger:      lipgloss.CompleteColor{TrueColor: "#AB2B3F", ANSI256: "124", ANSI: "1"},
		Info:        lipgloss.CompleteColor{TrueColor: "#5769F7", ANSI256: "63", ANSI: "4"},

		// Role colors
		User:      lipgloss.CompleteColor{TrueColor: "#58A6FF", ANSI256: "75", ANSI: "4"},
		Assistant: lipgloss.CompleteColor{TrueColor: "#AF5FD7", ANSI256: "170", ANSI: "5"},
		Tool:      lipgloss.CompleteColor{TrueColor: "#8B949E", ANSI256: "245", ANSI: "7"},
		System:    lipgloss.CompleteColor{TrueColor: "#6E7681", ANSI256: "243", ANSI: "8"},

		// UI element colors
		Permission:      lipgloss.CompleteColor{TrueColor: "#5769F7", ANSI256: "63", ANSI: "4"},  // CC blue
		SecondaryBorder: lipgloss.CompleteColor{TrueColor: "#3E3E3E", ANSI256: "237", ANSI: "8"},
		UserBackground:  lipgloss.CompleteColor{TrueColor: "#1E1E1E", ANSI256: "234", ANSI: "0"},
		PromptBorder:    lipgloss.CompleteColor{TrueColor: "#999999", ANSI256: "245", ANSI: "7"}, // CC fixed gray

		// Spacing
		GapXS:       1,
		GapSM:       2,
		InsetSM:     2,
		InsetMD:     4,
		GutterWidth: 2,
	}
}

// lightThemeTokens returns ThemeTokens for light terminal backgrounds.
func lightThemeTokens() ThemeTokens {
	return ThemeTokens{
		// Semantic colors
		Accent:      lipgloss.CompleteColor{TrueColor: "#8B5CF6", ANSI256: "93", ANSI: "5"},
		TextPrimary: lipgloss.CompleteColor{TrueColor: "#1A1A1A", ANSI256: "234", ANSI: "0"},
		TextMuted:   lipgloss.CompleteColor{TrueColor: "#6C6C6C", ANSI256: "242", ANSI: "8"},
		SurfaceSub:  lipgloss.CompleteColor{TrueColor: "#F0F0F0", ANSI256: "255", ANSI: "7"},
		BorderSub:   lipgloss.CompleteColor{TrueColor: "#D0D0D0", ANSI256: "252", ANSI: "7"},
		Success:     lipgloss.CompleteColor{TrueColor: "#2C7A39", ANSI256: "34", ANSI: "2"},
		Warning:     lipgloss.CompleteColor{TrueColor: "#966C1E", ANSI256: "136", ANSI: "3"},
		Danger:      lipgloss.CompleteColor{TrueColor: "#AB2B3F", ANSI256: "124", ANSI: "1"},
		Info:        lipgloss.CompleteColor{TrueColor: "#5769F7", ANSI256: "63", ANSI: "4"},

		// Role colors
		User:      lipgloss.CompleteColor{TrueColor: "#0969DA", ANSI256: "26", ANSI: "4"},
		Assistant: lipgloss.CompleteColor{TrueColor: "#8B5CF6", ANSI256: "93", ANSI: "5"},
		Tool:      lipgloss.CompleteColor{TrueColor: "#57606A", ANSI256: "241", ANSI: "7"},
		System:    lipgloss.CompleteColor{TrueColor: "#6E7681", ANSI256: "243", ANSI: "8"},

		// UI element colors
		Permission:      lipgloss.CompleteColor{TrueColor: "#5769F7", ANSI256: "63", ANSI: "4"},  // CC blue
		SecondaryBorder: lipgloss.CompleteColor{TrueColor: "#D0D0D0", ANSI256: "252", ANSI: "7"},
		UserBackground:  lipgloss.CompleteColor{TrueColor: "#F5F5F5", ANSI256: "255", ANSI: "7"},
		PromptBorder:    lipgloss.CompleteColor{TrueColor: "#999999", ANSI256: "245", ANSI: "7"}, // CC fixed gray

		// Spacing
		GapXS:       1,
		GapSM:       2,
		InsetSM:     2,
		InsetMD:     4,
		GutterWidth: 2,
	}
}
