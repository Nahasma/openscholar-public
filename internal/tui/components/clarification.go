package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/llm/tools"
)

// RenderClarificationDialog renders the AskUser clarification dialog.
func RenderClarificationDialog(event tools.ClarificationEvent, selectedIdx int, freeformText string, freeformFocused bool, width int) string {
	if width < 30 {
		width = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow)
	questionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite)
	contextStyle := lipgloss.NewStyle().Foreground(ColorGrayBright)
	optionStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	selectedOptionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite).Background(ColorBlue).PaddingLeft(1).PaddingRight(1)
	arrowStyle := lipgloss.NewStyle().Foreground(ColorBlue).Bold(true)
	inputBorderStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)
	hintStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	dividerStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)

	var sb strings.Builder

	// Title
	sb.WriteString("\n")
	sb.WriteString(" " + titleStyle.Render("⏺ 确认") + "\n")
	sb.WriteString(" " + dividerStyle.Render(strings.Repeat("─", width-2)) + "\n")

	// Question
	sb.WriteString("\n")
	sb.WriteString(" " + questionStyle.Render(event.Question) + "\n")

	// Context (if provided)
	if event.Context != "" {
		sb.WriteString("\n")
		sb.WriteString(" " + contextStyle.Render(event.Context) + "\n")
	}

	sb.WriteString("\n")

	// Options
	for i, opt := range event.Options {
		numStr := fmt.Sprintf("%d. %s", i+1, opt)
		if !freeformFocused && i == selectedIdx {
			prefix := " " + arrowStyle.Render("→") + " "
			sb.WriteString(prefix + selectedOptionStyle.Render(numStr) + "\n")
		} else {
			sb.WriteString("   " + optionStyle.Render(numStr) + "\n")
		}
	}

	// Freeform input (if allowed)
	if event.AllowFreeform {
		sb.WriteString("\n")
		border := "─"
		inputWidth := width - 8
		if inputWidth < 20 {
			inputWidth = 20
		}

		if freeformFocused {
			sb.WriteString("  " + arrowStyle.Render("→") + " " + inputBorderStyle.Render("┌"+strings.Repeat(border, inputWidth)+"┐") + "\n")
		} else {
			sb.WriteString("    " + inputBorderStyle.Render("┌"+strings.Repeat(border, inputWidth)+"┐") + "\n")
		}

		displayText := freeformText
		if displayText == "" {
			displayText = "或输入你的想法..."
			if freeformFocused {
				displayText = freeformText + "█"
			}
		} else if freeformFocused {
			displayText += "█"
		}

		// Truncate if too long
		if displayWidth(displayText) > inputWidth-2 {
			displayText = truncateDisplayStart(displayText, inputWidth-2)
		}

		padding := inputWidth - displayWidth(displayText)
		if padding < 0 {
			padding = 0
		}

		sb.WriteString("    " + inputBorderStyle.Render("│") + " " + displayText + strings.Repeat(" ", padding-1) + inputBorderStyle.Render("│") + "\n")
		sb.WriteString("    " + inputBorderStyle.Render("└"+strings.Repeat(border, inputWidth)+"┘") + "\n")
	}

	// Hints
	sb.WriteString("\n")
	hints := "↑↓ 选择  Enter 确认  Esc 跳过"
	if event.AllowFreeform {
		hints = "↑↓ 选择  Tab 输入框  Enter 确认  Esc 跳过"
	}
	sb.WriteString(" " + hintStyle.Render(hints) + "\n")

	return sb.String()
}
