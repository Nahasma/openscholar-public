package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

// SafeLineBudget describes the physical columns a rendered line may occupy.
// TerminalWidth is the real terminal width; LeftIndent is visible indentation
// written before the line content.
type SafeLineBudget struct {
	TerminalWidth int
	LeftIndent    int
}

// Width returns the content budget after reserving the terminal's final column.
func (b SafeLineBudget) Width() int {
	return TerminalSafeWidthWithIndent(b.TerminalWidth, b.LeftIndent)
}

// TerminalSafeWidth reserves the last terminal column to avoid pending-wrap.
func TerminalSafeWidth(width int) int {
	if width <= 1 {
		return 0
	}
	return width - 1
}

// TerminalSafeWidthWithIndent applies safe-width budgeting with left indent.
func TerminalSafeWidthWithIndent(width, leftIndent int) int {
	safe := TerminalSafeWidth(width)
	if leftIndent <= 0 {
		return safe
	}
	if leftIndent >= safe {
		return 0
	}
	return safe - leftIndent
}

// SafeTruncateLine trims trailing plain spaces and truncates to safe width.
func SafeTruncateLine(line string, width int) string {
	trimmed := strings.TrimRight(line, " \t")
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(trimmed) <= width {
		return trimmed
	}
	return xansi.Truncate(trimmed, width, "")
}

// SafeTruncateLineWithBudget truncates using a terminal-aware safe budget.
func SafeTruncateLineWithBudget(line string, budget SafeLineBudget) string {
	return SafeTruncateLine(line, budget.Width())
}

// SafeBlankLine is the only safe fullscreen filler row.
func SafeBlankLine() string {
	return ""
}

// SafeRule renders a single-line rule constrained to terminal-safe width.
func SafeRule(width, leftIndent int, glyph string) string {
	budget := TerminalSafeWidthWithIndent(width, leftIndent)
	if budget <= 0 || glyph == "" {
		return ""
	}
	glyphWidth := lipgloss.Width(glyph)
	if glyphWidth <= 0 {
		return ""
	}
	count := budget / glyphWidth
	if count <= 0 {
		return ""
	}
	return strings.Repeat(glyph, count)
}

// SafePadRight pads a line only up to the terminal-safe budget.
func SafePadRight(line string, budget SafeLineBudget) string {
	width := budget.Width()
	line = SafeTruncateLine(line, width)
	if width <= 0 {
		return ""
	}
	if pad := width - lipgloss.Width(line); pad > 0 {
		return line + strings.Repeat(" ", pad)
	}
	return line
}
