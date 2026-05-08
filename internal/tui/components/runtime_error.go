package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func RenderRuntimeErrorDetail(summary, detail string, width int) string {
	body := strings.TrimSpace(detail)
	if body == "" {
		body = strings.TrimSpace(summary)
	}
	if body == "" {
		return ""
	}

	panelWidth := TerminalSafeWidth(width)
	if panelWidth > 2 {
		panelWidth -= 2
	}
	if panelWidth < 8 {
		panelWidth = 8
	}
	innerWidth := panelWidth - 4
	if innerWidth < 4 {
		innerWidth = 4
	}
	lines := strings.Split(ansiWrap(body, innerWidth), "\n")
	if len(lines) > 8 {
		lines = append(lines[:7], "... (truncated)")
	}
	trimmed := strings.Join(lines, "\n")
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Theme.Danger).
		Foreground(Theme.TextPrimary).
		Width(panelWidth).
		Padding(0, 1)
	return style.Render(trimmed)
}
