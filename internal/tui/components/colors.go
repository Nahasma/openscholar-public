package components

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// IsDarkTheme indicates whether the terminal has a dark background.
var IsDarkTheme = true

// Shared color tokens — single source of truth for all TUI colors.
var (
	ColorBrandPurple lipgloss.TerminalColor
	ColorGreen       lipgloss.TerminalColor
	ColorOrange      lipgloss.TerminalColor
	ColorRed         lipgloss.TerminalColor
	ColorYellow      lipgloss.TerminalColor
	ColorWhite       lipgloss.TerminalColor
	ColorGrayBright  lipgloss.TerminalColor
	ColorGrayUser    lipgloss.TerminalColor
	ColorGrayMedium  lipgloss.TerminalColor
	ColorGrayDim     lipgloss.TerminalColor
	ColorGrayDark    lipgloss.TerminalColor
	ColorBlue        lipgloss.TerminalColor
	ColorCyan        lipgloss.TerminalColor
	ColorDiffAddBg   lipgloss.TerminalColor // low-saturation green bg for added lines
	ColorDiffDelBg   lipgloss.TerminalColor // low-saturation red bg for removed lines
	ColorDiffAddFg   lipgloss.TerminalColor // text color on add bg
	ColorDiffDelFg   lipgloss.TerminalColor // text color on del bg
)

func init() {
	// Force 256-color profile if auto-detection yields Ascii (no color).
	// This ensures colors render on terminals like macOS Terminal.app or
	// zsh sessions where TERM/COLORTERM may not be set correctly.
	profile := lipgloss.ColorProfile()
	if profile == termenv.Ascii {
		lipgloss.SetColorProfile(termenv.ANSI256)
	}

	IsDarkTheme = detectDarkBackground()
	if IsDarkTheme {
		Theme = darkThemeTokens()
	} else {
		Theme = lightThemeTokens()
	}
	if IsDarkTheme {
		ColorBrandPurple = lipgloss.Color("170")
		ColorGreen = lipgloss.Color("42")
		ColorOrange = lipgloss.Color("214")
		ColorRed = lipgloss.Color("196")
		ColorYellow = lipgloss.Color("226")
		ColorWhite = lipgloss.Color("255")
		ColorGrayBright = lipgloss.Color("252")
		ColorGrayUser = lipgloss.Color("245")
		ColorGrayMedium = lipgloss.Color("243")
		ColorGrayDim = lipgloss.Color("240")
		ColorGrayDark = lipgloss.Color("237")
		ColorBlue = lipgloss.Color("33")
		ColorCyan = lipgloss.Color("38")
		ColorDiffAddBg = lipgloss.Color("22")  // dark green background
		ColorDiffDelBg = lipgloss.Color("52")  // dark red background
		ColorDiffAddFg = lipgloss.Color("78")  // bright green text for added lines
		ColorDiffDelFg = lipgloss.Color("168") // bright red text for removed lines
	} else {
		ColorBrandPurple = lipgloss.Color("93")
		ColorGreen = lipgloss.Color("28")
		ColorOrange = lipgloss.Color("208")
		ColorRed = lipgloss.Color("160")
		ColorYellow = lipgloss.Color("178")
		ColorWhite = lipgloss.Color("16")
		ColorGrayBright = lipgloss.Color("236")
		ColorGrayUser = lipgloss.Color("241")
		ColorGrayMedium = lipgloss.Color("244")
		ColorGrayDim = lipgloss.Color("248")
		ColorGrayDark = lipgloss.Color("253")
		ColorBlue = lipgloss.Color("27")
		ColorCyan = lipgloss.Color("30")
		ColorDiffAddBg = lipgloss.Color("194") // light green background
		ColorDiffDelBg = lipgloss.Color("224") // light red background
		ColorDiffAddFg = lipgloss.Color("22")  // dark green text on light green bg
		ColorDiffDelFg = lipgloss.Color("124") // dark red text on light red bg
	}

	// Colors are now set — initialize all component styles
	initChatStyles()
	initWelcomeStyles()
	initHelpStyles()
	initStatusStyles()
	initDialogStyles()
	initSessionStyles()
	initInputStyles()
	initHeaderStyles()
	initCommandPickerStyles()
	initCommandOutputStyles()
	initDiffStyles()
	initModelDialogStyles()
	initThemeStyles()
}

// initThemeStyles is a placeholder for future theme extensions.
// V2 styles are now initialized as part of initChatStyles().
func initThemeStyles() {
	// no-op: V2 styles merged into initChatStyles()
}

// detectDarkBackground safely detects whether the terminal has a dark background.
// Falls back to dark theme if detection fails or panics.
func detectDarkBackground() (isDark bool) {
	isDark = true // default to dark
	defer func() {
		if r := recover(); r != nil {
			isDark = true
		}
	}()

	output := termenv.NewOutput(os.Stderr)
	isDark = output.HasDarkBackground()
	return isDark
}
