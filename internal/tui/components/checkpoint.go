package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
)

// RenderCheckpointDialog renders the phase checkpoint confirmation dialog.
func RenderCheckpointDialog(event tools.CheckpointEvent, selectedIdx int, feedbackText string, feedbackFocused bool, width int) string {
	if width < 30 {
		width = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite)
	contentStyle := lipgloss.NewStyle().Foreground(ColorGrayBright)
	optionStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	selectedOptionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite).Background(ColorBlue).PaddingLeft(1).PaddingRight(1)
	arrowStyle := lipgloss.NewStyle().Foreground(ColorBlue).Bold(true)
	inputBorderStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)
	hintStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	dividerStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)
	scoreStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen)

	var sb strings.Builder

	// Title
	sb.WriteString("\n")
	sb.WriteString(" " + titleStyle.Render("⏺ 阶段审核") + "\n")
	sb.WriteString(" " + dividerStyle.Render(strings.Repeat("─", width-2)) + "\n")

	// Phase info
	sb.WriteString("\n")
	sb.WriteString(" " + labelStyle.Render(fmt.Sprintf("阶段 %d: %s", event.PhaseOrder, event.PhaseName)) + "\n")

	// Summary
	if event.Summary != "" {
		sb.WriteString("\n")
		sb.WriteString(" " + labelStyle.Render("摘要:") + "\n")
		for _, line := range strings.Split(event.Summary, "\n") {
			sb.WriteString("   " + contentStyle.Render(line) + "\n")
		}
	}

	// Review score (shown if auto-reviewed)
	if event.ReviewScore > 0 {
		sb.WriteString("\n")
		sb.WriteString(" " + scoreStyle.Render(fmt.Sprintf("评审得分: %.0f/10", event.ReviewScore)) + "\n")
		if event.ReviewReport != "" {
			sb.WriteString(" " + labelStyle.Render("评审意见:") + "\n")
			for _, line := range strings.Split(event.ReviewReport, "\n") {
				sb.WriteString("   " + contentStyle.Render(line) + "\n")
			}
		}
	}

	sb.WriteString("\n")

	// Options
	options := []string{"通过 (继续下一阶段)", "打回 (要求修改)"}
	for i, opt := range options {
		if !feedbackFocused && i == selectedIdx {
			prefix := " " + arrowStyle.Render("→") + " "
			sb.WriteString(prefix + selectedOptionStyle.Render(opt) + "\n")
		} else {
			sb.WriteString("   " + optionStyle.Render(opt) + "\n")
		}
	}

	// Feedback input (when Reject is selected or in input mode)
	if selectedIdx == 1 || feedbackFocused {
		sb.WriteString("\n")
		border := "─"
		inputWidth := width - 8
		if inputWidth < 20 {
			inputWidth = 20
		}

		if feedbackFocused {
			sb.WriteString("  " + arrowStyle.Render("→") + " " + inputBorderStyle.Render("┌"+strings.Repeat(border, inputWidth)+"┐") + "\n")
		} else {
			sb.WriteString("    " + inputBorderStyle.Render("┌"+strings.Repeat(border, inputWidth)+"┐") + "\n")
		}

		displayText := feedbackText
		if displayText == "" {
			displayText = "输入修改意见..."
			if feedbackFocused {
				displayText = feedbackText + "█"
			}
		} else if feedbackFocused {
			displayText += "█"
		}

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
	hints := "↑↓ 选择  y 通过  n 打回  Tab 输入反馈  Enter 确认  Esc 打回"
	sb.WriteString(" " + hintStyle.Render(hints) + "\n")

	return sb.String()
}
