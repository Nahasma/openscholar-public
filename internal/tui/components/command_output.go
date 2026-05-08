package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	commandOutputBorderStyle  lipgloss.Style
	commandOutputContentStyle lipgloss.Style
)

func initCommandOutputStyles() {
	commandOutputBorderStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
	commandOutputContentStyle = lipgloss.NewStyle().
		Foreground(ColorGrayBright)
}

// RenderCommandOutput renders command output in a bordered area that replaces the input box.
func RenderCommandOutput(title, body, footer string, width int) string {
	contentWidth := width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	border := commandOutputBorderStyle.Render(strings.Repeat("─", contentWidth))

	var sb strings.Builder
	sb.WriteString("  " + border + "\n")
	if strings.TrimSpace(title) != "" {
		sb.WriteString("  " + commandOutputContentStyle.Bold(true).Render(title) + "\n")
		if strings.TrimSpace(body) != "" {
			sb.WriteString("  \n")
		}
	}
	for _, line := range strings.Split(body, "\n") {
		sb.WriteString("  " + commandOutputContentStyle.Render(line) + "\n")
	}
	sb.WriteString("  " + border + "\n")
	if strings.TrimSpace(footer) == "" {
		footer = "press any key to dismiss"
	}
	sb.WriteString("  " + lipgloss.NewStyle().Foreground(ColorGrayDim).Render(footer) + "\n")

	return sb.String()
}
