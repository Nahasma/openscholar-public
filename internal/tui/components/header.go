package components

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	modeAutoStyle     lipgloss.Style
	modeResearchStyle lipgloss.Style
	borderColor       lipgloss.TerminalColor
)

func initHeaderStyles() {
	modeAutoStyle = lipgloss.NewStyle().
		Foreground(ColorGreen)
	modeResearchStyle = lipgloss.NewStyle().
		Foreground(ColorCyan)
	borderColor = ColorGrayMedium
}
