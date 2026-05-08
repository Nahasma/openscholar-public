package components

import (
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	helpKeyStyle    lipgloss.Style
	helpDescStyle   lipgloss.Style
	helpTitleStyle  lipgloss.Style
	helpBorderStyle lipgloss.Style
)

func initHelpStyles() {
	helpKeyStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorGrayBright)
	helpDescStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
	helpTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBrandPurple)
	helpBorderStyle = lipgloss.NewStyle().
		Foreground(ColorGrayDim)
}

type shortcutEntry struct {
	Key  string
	Desc string
}

// RenderHelpOverlay renders a keyboard shortcuts help panel.
func RenderHelpOverlay(width int) string {
	if width < 20 {
		width = 20
	}

	newlineKey := "alt+enter"
	if runtime.GOOS == "darwin" {
		newlineKey = "⌥+return"
	}

	shortcuts := []shortcutEntry{
		{"enter", "send message"},
		{newlineKey, "insert newline"},
		{"shift+tab", "cycle mode (default/auto/plan/research)"},
		{"ctrl+o", "toggle transcript / focused expand"},
		{"esc", "cancel current request"},
		{"ctrl+c", "quit (or cancel if processing)"},
		{"mouse drag", "select and copy text"},
		{"ctrl+y", "copy last response"},
		{"ctrl+b", "copy code block"},
		{"ctrl+p", "session browser"},
		{"ctrl+n", "new session"},
		{"?", "toggle this help panel"},
	}

	innerWidth := width - 6 // borders + padding
	if innerWidth < 30 {
		innerWidth = 30
	}

	var sb strings.Builder

	// Top border with title
	title := " Keyboard Shortcuts "
	topLen := innerWidth + 2
	titleStart := 2
	top := "╭" + strings.Repeat("─", titleStart) + title + strings.Repeat("─", topLen-titleStart-len(title)) + "╮"
	sb.WriteString(helpBorderStyle.Render(top) + "\n")

	// Empty line
	sb.WriteString(helpBorderStyle.Render("│") + strings.Repeat(" ", innerWidth+2) + helpBorderStyle.Render("│") + "\n")

	// Shortcut rows
	keyColWidth := 18
	for _, s := range shortcuts {
		key := helpKeyStyle.Render(s.Key)

		keyPad := keyColWidth - lipgloss.Width(key)
		if keyPad < 1 {
			keyPad = 1
		}

		// Truncate description to fit within available width
		descMaxWidth := innerWidth - keyColWidth - 1
		descText := s.Desc
		if descMaxWidth > 0 {
			descText = truncateDisplay(descText, descMaxWidth)
		}
		desc := helpDescStyle.Render(descText)

		line := " " + key + strings.Repeat(" ", keyPad) + desc
		lineWidth := lipgloss.Width(line)
		rightPad := innerWidth + 2 - lineWidth
		if rightPad < 0 {
			rightPad = 0
		}

		sb.WriteString(helpBorderStyle.Render("│") + line + strings.Repeat(" ", rightPad) + helpBorderStyle.Render("│") + "\n")
	}

	// Empty line
	sb.WriteString(helpBorderStyle.Render("│") + strings.Repeat(" ", innerWidth+2) + helpBorderStyle.Render("│") + "\n")

	// Hint line
	hint := helpDescStyle.Render("  Press any key to close")
	hintPad := innerWidth + 2 - lipgloss.Width(hint)
	if hintPad < 0 {
		hintPad = 0
	}
	sb.WriteString(helpBorderStyle.Render("│") + hint + strings.Repeat(" ", hintPad) + helpBorderStyle.Render("│") + "\n")

	// Bottom border
	bottom := "╰" + strings.Repeat("─", innerWidth+2) + "╯"
	sb.WriteString(helpBorderStyle.Render(bottom))

	return sb.String()
}
