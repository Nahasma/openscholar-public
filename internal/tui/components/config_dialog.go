package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/config"
)

// RenderConfigWizardDialog renders a single step of the config wizard.
func RenderConfigWizardDialog(
	step *config.CWStep,
	stepNum, totalSteps int,
	phaseLabel string,
	selectedIdx int,
	textInput string,
	textCursor int,
	changes []string,
	width int,
) string {
	if width < 30 {
		width = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorBrandPurple)
	questionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite)
	optionStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	selectedOptionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite).Background(ColorBlue).PaddingLeft(1).PaddingRight(1)
	arrowStyle := lipgloss.NewStyle().Foreground(ColorBlue).Bold(true)
	inputBorderStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)
	hintStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	dividerStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)
	statusOkStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	statusEmptyStyle := lipgloss.NewStyle().Foreground(ColorGrayDim)
	changeStyle := lipgloss.NewStyle().Foreground(ColorCyan)
	continueStyle := lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)

	var sb strings.Builder
	sb.WriteString("\n")

	// Title with phase progress
	title := fmt.Sprintf("⏺ 配置向导 (%d/%d) — %s", stepNum, totalSteps, phaseLabel)
	sb.WriteString(" " + titleStyle.Render(title) + "\n")
	sb.WriteString(" " + dividerStyle.Render(strings.Repeat("─", width-2)) + "\n")
	sb.WriteString("\n")

	switch step.Type {
	case config.CWStepProviderMenu:
		sb.WriteString(" " + questionStyle.Render(step.Label) + "\n")
		sb.WriteString("\n")
		renderProviderTableMenuOptions(&sb, step.Options, selectedIdx, arrowStyle, selectedOptionStyle, optionStyle, continueStyle, dividerStyle, width)
		sb.WriteString("\n")
		sb.WriteString(" " + hintStyle.Render("↑↓ 选择  Enter 配置  Esc 取消") + "\n")

	case config.CWStepProviderKey:
		sb.WriteString(" " + questionStyle.Render(step.Label) + "\n")
		if step.CurrentValue != "" {
			sb.WriteString(" " + statusOkStyle.Render("  当前: "+step.CurrentValue) + "\n")
		} else {
			sb.WriteString(" " + statusEmptyStyle.Render("  当前: 未配置") + "\n")
		}
		envHint := config.ProviderEnvHint(step.Provider)
		if envHint != "" {
			sb.WriteString(" " + hintStyle.Render("  环境变量: "+envHint) + "\n")
		}
		sb.WriteString("\n")
		sb.WriteString(renderTextInput(maskInputForStep(step, textInput), textCursor, step.Placeholder, arrowStyle, inputBorderStyle, width))
		sb.WriteString("\n")
		sb.WriteString(" " + hintStyle.Render("Enter 确认  Esc 返回") + "\n")

	case config.CWStepProviderBaseURL:
		sb.WriteString(" " + questionStyle.Render(step.Label) + "\n")
		if step.CurrentValue != "" {
			sb.WriteString(" " + statusOkStyle.Render("  当前: "+step.CurrentValue) + "\n")
		}
		sb.WriteString("\n")
		sb.WriteString(renderTextInput(textInput, textCursor, step.Placeholder, arrowStyle, inputBorderStyle, width))
		sb.WriteString("\n")
		sb.WriteString(" " + hintStyle.Render("Enter 确认  Esc 返回") + "\n")

	case config.CWStepDefaultProvider:
		sb.WriteString(" " + questionStyle.Render(step.Label) + "\n")
		sb.WriteString(" " + hintStyle.Render("  当前: "+step.CurrentValue) + "\n")
		sb.WriteString("\n")

		for i, opt := range step.Options {
			numStr := fmt.Sprintf("%d. %s", i+1, opt.Label)
			if i == selectedIdx {
				prefix := " " + arrowStyle.Render("→") + " "
				sb.WriteString(prefix + selectedOptionStyle.Render(numStr) + "\n")
			} else {
				sb.WriteString("   " + optionStyle.Render(numStr) + "\n")
			}
		}
		sb.WriteString("\n")
		sb.WriteString(" " + hintStyle.Render("↑↓ 选择  Enter 确认  Esc 跳过") + "\n")

	case config.CWStepAgentMenu:
		sb.WriteString(" " + questionStyle.Render(step.Label) + "\n")
		sb.WriteString("\n")
		renderMenuOptions(&sb, step.Options, selectedIdx, arrowStyle, selectedOptionStyle, optionStyle, continueStyle, dividerStyle, width)
		sb.WriteString("\n")
		sb.WriteString(" " + hintStyle.Render("↑↓ 选择  Enter 配置模型  Esc 取消") + "\n")

	case config.CWStepAgentProvider:
		sb.WriteString(" " + questionStyle.Render(step.Label) + "\n")
		sb.WriteString(" " + hintStyle.Render("  当前: "+step.CurrentValue) + "\n")
		sb.WriteString("\n")

		for i, opt := range step.Options {
			numStr := fmt.Sprintf("%d. %s", i+1, opt.Label)
			if i == selectedIdx {
				prefix := " " + arrowStyle.Render("→") + " "
				sb.WriteString(prefix + selectedOptionStyle.Render(numStr) + "\n")
			} else {
				sb.WriteString("   " + optionStyle.Render(numStr) + "\n")
			}
		}
		sb.WriteString("\n")
		sb.WriteString(" " + hintStyle.Render("↑↓ 选择  Enter 确认  Esc 返回") + "\n")

	case config.CWStepAgentModel:
		sb.WriteString(" " + questionStyle.Render(step.Label) + "\n")
		if step.CurrentValue != "" {
			sb.WriteString(" " + hintStyle.Render("  当前: "+step.CurrentValue) + "\n")
		}
		sb.WriteString("\n")

		if len(step.Options) == 0 {
			sb.WriteString("   " + statusEmptyStyle.Render("无可用模型") + "\n")
		} else {
			// Show scrollable model list (max 8 visible)
			maxVisible := 8
			startIdx := 0
			if selectedIdx >= maxVisible {
				startIdx = selectedIdx - maxVisible + 1
			}
			endIdx := startIdx + maxVisible
			if endIdx > len(step.Options) {
				endIdx = len(step.Options)
			}

			if startIdx > 0 {
				sb.WriteString("   " + hintStyle.Render("↑ more") + "\n")
			}
			for i := startIdx; i < endIdx; i++ {
				opt := step.Options[i]
				// Mark current model
				marker := "  "
				if opt.Value == step.CurrentValue {
					marker = statusOkStyle.Render("→ ")
				}
				if i == selectedIdx {
					prefix := " " + arrowStyle.Render("❯") + " "
					sb.WriteString(prefix + marker + selectedOptionStyle.Render(opt.Label) + "\n")
				} else {
					sb.WriteString("   " + marker + optionStyle.Render(opt.Label) + "\n")
				}
			}
			if endIdx < len(step.Options) {
				sb.WriteString("   " + hintStyle.Render("↓ more") + "\n")
			}
		}
		sb.WriteString("\n")
		sb.WriteString(" " + hintStyle.Render("↑↓ 选择  Enter 确认  Esc 返回") + "\n")

	case config.CWStepSummary:
		sb.WriteString(" " + questionStyle.Render(step.Label) + "\n")
		sb.WriteString("\n")

		if len(changes) == 0 {
			sb.WriteString("   " + statusEmptyStyle.Render("没有需要保存的更改") + "\n")
		} else {
			for _, c := range changes {
				sb.WriteString("   " + changeStyle.Render("• "+c) + "\n")
			}
		}

		sb.WriteString("\n")
		if len(changes) > 0 {
			sb.WriteString(" " + hintStyle.Render("Enter 保存  Esc 取消") + "\n")
		} else {
			sb.WriteString(" " + hintStyle.Render("Enter/Esc 退出") + "\n")
		}
	}

	return sb.String()
}

func renderProviderTableMenuOptions(
	sb *strings.Builder,
	options []config.CWOption,
	selectedIdx int,
	arrowStyle, selectedOptionStyle, optionStyle, continueStyle, dividerStyle lipgloss.Style,
	width int,
) {
	boxWidth := width - 6
	if boxWidth < 24 {
		boxWidth = 24
	}
	contentBudget := boxWidth - 2
	primaryWidth := contentBudget / 2
	if primaryWidth > 24 {
		primaryWidth = 24
	}
	if primaryWidth < 10 {
		primaryWidth = 10
	}
	secondaryWidth := contentBudget - primaryWidth - 3
	if secondaryWidth < 8 {
		secondaryWidth = 8
	}
	contentWidth := primaryWidth + 3 + secondaryWidth

	border := strings.Repeat("─", boxWidth)
	selectedRowStyle := selectedOptionStyle.PaddingLeft(0).PaddingRight(0)
	sb.WriteString("   " + dividerStyle.Render("┌"+border+"┐") + "\n")
	header := formatProviderTableRow("Provider", "API Key", primaryWidth, secondaryWidth)
	sb.WriteString("   " + dividerStyle.Render("│") + " " + optionStyle.Bold(true).Render(header) + strings.Repeat(" ", boxWidth-2-contentWidth) + " " + dividerStyle.Render("│") + "\n")
	sb.WriteString("   " + dividerStyle.Render("├"+strings.Repeat("─", boxWidth)+"┤") + "\n")

	for i, opt := range options {
		if opt.Value == "continue" {
			continue
		}
		primary := opt.Label
		secondary := ""
		if opt.Display != nil {
			if strings.TrimSpace(opt.Display.Primary) != "" {
				primary = opt.Display.Primary
			}
			secondary = opt.Display.Secondary
		}
		line := formatProviderTableRow(primary, secondary, primaryWidth, secondaryWidth)
		pad := strings.Repeat(" ", boxWidth-2-contentWidth)
		if i == selectedIdx {
			sb.WriteString(" " + arrowStyle.Render("→") + " " + dividerStyle.Render("│") + " " + selectedRowStyle.Render(line+pad) + " " + dividerStyle.Render("│") + "\n")
		} else {
			sb.WriteString("   " + dividerStyle.Render("│") + " " + optionStyle.Render(line) + pad + " " + dividerStyle.Render("│") + "\n")
		}
	}
	sb.WriteString("   " + dividerStyle.Render("└"+strings.Repeat("─", boxWidth)+"┘") + "\n")

	for i, opt := range options {
		if opt.Value != "continue" {
			continue
		}
		if i == selectedIdx {
			sb.WriteString(" " + arrowStyle.Render("→") + " " + selectedOptionStyle.Render(opt.Label) + "\n")
		} else {
			sb.WriteString("   " + continueStyle.Render(opt.Label) + "\n")
		}
	}
}

func formatProviderTableRow(primary, secondary string, primaryWidth, secondaryWidth int) string {
	return padDisplayRight(primary, primaryWidth) + " │ " + padDisplayRight(secondary, secondaryWidth)
}

func padDisplayRight(s string, width int) string {
	s = truncateDisplay(s, width)
	if pad := width - displayWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// renderMenuOptions renders a menu list with a separator before the "continue" item.
func renderMenuOptions(
	sb *strings.Builder,
	options []config.CWOption,
	selectedIdx int,
	arrowStyle, selectedOptionStyle, optionStyle, continueStyle, dividerStyle lipgloss.Style,
	width int,
) {
	for i, opt := range options {
		isContinue := opt.Value == "continue"

		// Draw separator before continue option
		if isContinue {
			sb.WriteString("   " + dividerStyle.Render(strings.Repeat("─", width-6)) + "\n")
		}

		if i == selectedIdx {
			prefix := " " + arrowStyle.Render("→") + " "
			if isContinue {
				sb.WriteString(prefix + selectedOptionStyle.Render(opt.Label) + "\n")
			} else {
				sb.WriteString(prefix + selectedOptionStyle.Render(opt.Label) + "\n")
			}
		} else {
			if isContinue {
				sb.WriteString("   " + continueStyle.Render(opt.Label) + "\n")
			} else {
				sb.WriteString("   " + optionStyle.Render(opt.Label) + "\n")
			}
		}
	}
}

func renderTextInput(text string, cursorPos int, placeholder string, arrowStyle, borderStyle lipgloss.Style, width int) string {
	var sb strings.Builder

	placeholderStyle := lipgloss.NewStyle().Foreground(ColorGrayDim)

	inputWidth := width - 8
	if inputWidth < 20 {
		inputWidth = 20
	}

	border := "─"
	sb.WriteString("  " + arrowStyle.Render("→") + " " + borderStyle.Render("┌"+strings.Repeat(border, inputWidth)+"┐") + "\n")

	isPlaceholder := text == ""

	if isPlaceholder {
		// Show placeholder in dim with cursor at start
		displayText := placeholder
		runes := []rune(displayText)
		if len(runes) > inputWidth-3 {
			runes = runes[:inputWidth-3]
			displayText = string(runes)
		}
		content := "█" + placeholderStyle.Render(displayText)
		visLen := 1 + len(runes) // cursor + placeholder
		padding := inputWidth - visLen
		if padding < 0 {
			padding = 0
		}
		sb.WriteString("    " + borderStyle.Render("│") + " " + content + strings.Repeat(" ", padding-1) + borderStyle.Render("│") + "\n")
	} else {
		// Insert block cursor at cursorPos
		runes := []rune(text)
		if cursorPos > len(runes) {
			cursorPos = len(runes)
		}
		if cursorPos < 0 {
			cursorPos = 0
		}

		before := string(runes[:cursorPos])
		var cursorChar string
		var after string
		if cursorPos < len(runes) {
			cursorChar = string(runes[cursorPos])
			after = string(runes[cursorPos+1:])
		} else {
			cursorChar = " "
			after = ""
		}

		// Build display with cursor highlighting
		cursorStyle := lipgloss.NewStyle().Reverse(true)
		content := before + cursorStyle.Render(cursorChar) + after

		visLen := len(runes)
		if cursorPos >= len(runes) {
			visLen++ // trailing cursor space
		}

		// Scroll if content exceeds width
		maxContent := inputWidth - 2
		if visLen > maxContent {
			// Keep cursor visible: show a window around cursor position
			start := cursorPos - maxContent/2
			if start < 0 {
				start = 0
			}
			end := start + maxContent
			if end > len(runes) {
				end = len(runes)
				start = end - maxContent
				if start < 0 {
					start = 0
				}
			}

			windowRunes := runes[start:end]
			adjCursor := cursorPos - start
			before = string(windowRunes[:adjCursor])
			if adjCursor < len(windowRunes) {
				cursorChar = string(windowRunes[adjCursor])
				after = string(windowRunes[adjCursor+1:])
			} else {
				cursorChar = " "
				after = ""
			}
			content = before + cursorStyle.Render(cursorChar) + after
			visLen = len(windowRunes)
			if adjCursor >= len(windowRunes) {
				visLen++
			}
		}

		padding := inputWidth - visLen
		if padding < 0 {
			padding = 0
		}
		sb.WriteString("    " + borderStyle.Render("│") + " " + content + strings.Repeat(" ", padding-1) + borderStyle.Render("│") + "\n")
	}

	sb.WriteString("    " + borderStyle.Render("└"+strings.Repeat(border, inputWidth)+"┘") + "\n")

	return sb.String()
}

func maskInputForStep(step *config.CWStep, text string) string {
	if step == nil {
		return text
	}
	if step.InputKind != config.CWInputKindSensitive || text == "" {
		return text
	}
	return strings.Repeat("*", len([]rune(text)))
}
