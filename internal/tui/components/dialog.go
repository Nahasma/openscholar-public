package components

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/plan"
)

var (
	warnStyle           lipgloss.Style
	dialogTextStyle     lipgloss.Style
	dialogMutedStyle    lipgloss.Style
	btnNormalStyle      lipgloss.Style
	btnSelectedStyle    lipgloss.Style
	btnArrowStyle       lipgloss.Style
	previewToolStyle    lipgloss.Style
	previewLabelStyle   lipgloss.Style
	previewLineNumStyle lipgloss.Style
	previewContentStyle lipgloss.Style
	previewDividerStyle lipgloss.Style
	diffAddStyle        lipgloss.Style
	diffRemoveStyle     lipgloss.Style
)

func initDialogStyles() {
	warnStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorYellow)
	dialogTextStyle = lipgloss.NewStyle().
		Foreground(ColorWhite)
	dialogMutedStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
	btnNormalStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium).
		PaddingLeft(1).
		PaddingRight(1)
	btnSelectedStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorWhite).
		Background(ColorBlue).
		PaddingLeft(1).
		PaddingRight(1)
	btnArrowStyle = lipgloss.NewStyle().
		Foreground(ColorBlue).
		Bold(true)
	previewToolStyle = lipgloss.NewStyle().
		Foreground(ColorBrandPurple).
		Bold(true)
	previewLabelStyle = lipgloss.NewStyle().
		Foreground(ColorGrayBright)
	previewLineNumStyle = lipgloss.NewStyle().
		Foreground(ColorGrayDim)
	previewContentStyle = lipgloss.NewStyle().
		Foreground(ColorWhite)
	previewDividerStyle = lipgloss.NewStyle().
		Foreground(ColorGrayDark)
	diffAddStyle = lipgloss.NewStyle().
		Foreground(ColorDiffAddFg).
		Background(ColorDiffAddBg)
	diffRemoveStyle = lipgloss.NewStyle().
		Foreground(ColorDiffDelFg).
		Background(ColorDiffDelBg)
}

// RenderPermissionPreview renders the file/command preview in the viewport (scrollable).
func RenderPermissionPreview(perm permission.PermissionRequest, width int) string {
	if width < 20 {
		width = 20
	}

	var sb strings.Builder

	// CC alignment: rounded top border with embedded title
	permBorderStyle := lipgloss.NewStyle().Foreground(Theme.Permission)
	title := " " + perm.ToolName + " wants to " + getPermissionVerb(perm.ToolName, perm.Action) + " "
	titleWidth := lipgloss.Width(title)
	remainWidth := width - 2 - titleWidth - 3 // 2 for "╭──", 3 for "──╮"
	if remainWidth < 2 {
		remainWidth = 2
	}
	topBorder := permBorderStyle.Render("╭──" + title + strings.Repeat("─", remainWidth) + "╮")
	sb.WriteString(" " + topBorder + "\n")

	// Action label + path/command
	actionLabel := getPermissionActionLabel(perm.ToolName, perm.Action)
	sb.WriteString(" " + permBorderStyle.Render("│") + " " + previewLabelStyle.Render(actionLabel) + "\n")
	sb.WriteString(" " + permBorderStyle.Render("│") + " " + dialogTextStyle.Render(perm.Description) + "\n")

	// Content preview
	content, hasContent := extractPreviewContent(perm)
	if hasContent {
		sb.WriteString(" " + permBorderStyle.Render(strings.Repeat("╌", width-2)) + "\n")

		lines := strings.Split(content, "\n")
		contentWidth := width - 8

		if perm.ToolName == "Edit" {
			// Render old/new sections with diff colors
			inOld := false
			inNew := false
			for _, line := range lines {
				if line == "--- old" {
					inOld, inNew = true, false
					sb.WriteString(" " + previewLineNumStyle.Render("  old ") + diffRemoveStyle.Render("─── old content") + "\n")
					continue
				}
				if line == "+++ new" {
					inOld, inNew = false, true
					sb.WriteString(" " + previewLineNumStyle.Render("  new ") + diffAddStyle.Render("─── new content") + "\n")
					continue
				}
				var style lipgloss.Style
				var prefix string
				if inOld {
					style = diffRemoveStyle
					prefix = "- "
				} else if inNew {
					style = diffAddStyle
					prefix = "+ "
				} else {
					style = previewContentStyle
					prefix = ""
				}
				wrapped := wrapLine(line, contentWidth-2)
				for j, seg := range wrapped {
					if j == 0 {
						sb.WriteString(" " + previewLineNumStyle.Render("      ") + style.Render(prefix+seg) + "\n")
					} else {
						sb.WriteString(" " + previewLineNumStyle.Render("      ") + style.Render("  "+seg) + "\n")
					}
				}
			}
		} else {
			contPad := " " + strings.Repeat(" ", 5) // align with content after line number
			for i, line := range lines {
				lineNum := previewLineNumStyle.Render(fmt.Sprintf("%4d ", i+1))
				wrapped := wrapLine(line, contentWidth)
				for j, seg := range wrapped {
					if j == 0 {
						sb.WriteString(" " + lineNum + previewContentStyle.Render(seg) + "\n")
					} else {
						sb.WriteString(contPad + previewContentStyle.Render(seg) + "\n")
					}
				}
			}
		}
	}

	// Bottom divider
	sb.WriteString(" " + permBorderStyle.Render(strings.Repeat("╌", width-2)) + "\n")

	return sb.String()
}

// RenderPermissionDialog renders the bottom permission choices (fixed area below viewport).
func RenderPermissionDialog(perm permission.PermissionRequest, selectedIdx int, width int) string {
	var sb strings.Builder

	// CC alignment: inline permission options
	permStyle := lipgloss.NewStyle().Foreground(Theme.Permission)
	dimStyle := lipgloss.NewStyle().Foreground(Theme.TextMuted)

	sb.WriteString("\n")
	sb.WriteString(" " + dialogTextStyle.Render("Allow?") + "  ")

	// Inline options: y Yes  n No  a Always
	options := []struct {
		key   string
		label string
	}{
		{"y", "Yes"},
		{"n", "No"},
		{"a", "Always"},
	}

	for i, opt := range options {
		if i == selectedIdx {
			sb.WriteString(permStyle.Render(opt.key) + " " + lipgloss.NewStyle().Bold(true).Render(opt.label))
		} else {
			sb.WriteString(dimStyle.Render(opt.key) + " " + dimStyle.Render(opt.label))
		}
		if i < len(options)-1 {
			sb.WriteString("  ")
		}
	}
	sb.WriteString("\n")

	return sb.String()
}

// RenderPermissionRequestDialog renders the full permission request as a single
// top-border-only container, closer to Claude Code's PermissionDialog layout.
func RenderPermissionRequestDialog(perm permission.PermissionRequest, selectedIdx int, width int, maxPreviewLines int) string {
	if width < 20 {
		width = 20
	}

	permStyle := lipgloss.NewStyle().Foreground(Theme.Permission)
	titleStyle := lipgloss.NewStyle().Foreground(Theme.Permission).Bold(true)
	title := fmt.Sprintf("%s wants to %s", perm.ToolName, getPermissionVerb(perm.ToolName, perm.Action))
	title = " " + title + " "
	ruleWidth := width - lipgloss.Width(title) - 2
	if ruleWidth < 6 {
		ruleWidth = 6
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(" " + permStyle.Render(strings.Repeat("─", 2)) + titleStyle.Render(title) +
		permStyle.Render(strings.Repeat("─", ruleWidth)) + "\n")

	actionLabel := getPermissionActionLabel(perm.ToolName, perm.Action)
	if actionLabel != "" {
		sb.WriteString("  " + previewLabelStyle.Render(actionLabel) + "\n")
	}
	if perm.Description != "" {
		sb.WriteString("  " + dialogTextStyle.Render(perm.Description) + "\n")
	}
	if perm.Path != "" {
		sb.WriteString("  " + dialogMutedStyle.Render(truncateToWidth(shortenPath(perm.Path), width-4, "…")) + "\n")
	}

	if previewBody, hiddenLines := renderPermissionPreviewBody(perm, width, maxPreviewLines); previewBody != "" {
		sb.WriteString(" " + previewDividerStyle.Render(strings.Repeat("─", width-2)) + "\n")
		sb.WriteString(previewBody)
		if hiddenLines > 0 {
			sb.WriteString("  " + dialogMutedStyle.Render(fmt.Sprintf("… +%d lines hidden", hiddenLines)) + "\n")
		}
	}

	sb.WriteString(" " + previewDividerStyle.Render(strings.Repeat("─", width-2)) + "\n")
	sb.WriteString(" " + dialogTextStyle.Render("Allow?") + "  ")

	options := []struct {
		key   string
		label string
	}{
		{"y", "Yes"},
		{"n", "No"},
		{"a", "Always"},
	}

	for i, opt := range options {
		if i == selectedIdx {
			sb.WriteString(permStyle.Render(opt.key) + " " + lipgloss.NewStyle().Bold(true).Render(opt.label))
		} else {
			sb.WriteString(dialogMutedStyle.Render(opt.key) + " " + dialogMutedStyle.Render(opt.label))
		}
		if i < len(options)-1 {
			sb.WriteString("  ")
		}
	}
	sb.WriteString("\n")

	return sb.String()
}

var planApprovalOptions = []string{"Approve", "Approve Auto", "Reject", "Edit Plan"}

// RenderPlanApprovalDialog renders the plan approval choices after the agent finishes in plan mode.
func RenderPlanApprovalDialog(event tools.PlanApprovalEvent, selectedIdx int, feedback string, inputMode bool, width int) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(" " + warnStyle.Render("Plan approval") + " " + dialogTextStyle.Render("Review the proposed plan.") + "\n")
	sb.WriteString(" " + dialogMutedStyle.Render(truncateToWidth(shortenPath(event.PlanPath), width-4, "…")) + "\n")
	sb.WriteString("\n")

	preview := strings.TrimSpace(event.PlanBody)
	if preview == "" {
		preview = plan.DisplayBody(event.Plan)
	}
	lines := strings.Split(preview, "\n")
	maxLines := 10
	if len(lines) < maxLines {
		maxLines = len(lines)
	}
	for i := 0; i < maxLines; i++ {
		sb.WriteString("  " + dialogTextStyle.Render(truncateToWidth(lines[i], width-4, "…")) + "\n")
	}
	if len(lines) > maxLines {
		sb.WriteString("  " + dialogMutedStyle.Render(fmt.Sprintf("... +%d lines", len(lines)-maxLines)) + "\n")
	}
	sb.WriteString("\n")

	for i, opt := range planApprovalOptions {
		numStr := fmt.Sprintf("%d. %s", i+1, opt)
		if i == selectedIdx {
			prefix := " " + btnArrowStyle.Render("❯") + " "
			sb.WriteString(prefix + btnSelectedStyle.Render(numStr) + "\n")
		} else {
			sb.WriteString("   " + btnNormalStyle.Render(numStr) + "\n")
		}
	}

	sb.WriteString("\n")
	if inputMode {
		label := "Feedback"
		if selectedIdx == 3 {
			label = "Edited plan"
		}
		if feedback == "" {
			sb.WriteString(" " + dialogMutedStyle.Render(label+": type here, Enter to submit") + "\n")
		} else {
			sb.WriteString(" " + dialogTextStyle.Render(label+": "+truncateToWidth(feedback, width-4, "…")) + "\n")
		}
	} else {
		sb.WriteString(" " + dialogMutedStyle.Render("1-4 select · arrows navigate · enter confirm · tab type · esc reject") + "\n")
	}

	return sb.String()
}

// getToolSummary returns a brief summary for the tool header.
func getToolSummary(perm permission.PermissionRequest) string {
	return perm.Description
}

// getPermissionActionLabel returns the action label like "Create file", "Edit file", etc.
func getPermissionActionLabel(toolName, action string) string {
	switch toolName {
	case "Write":
		return "Create file"
	case "Edit":
		return "Edit file"
	case "Bash":
		return "Run command"
	case "WebSearch":
		return "Search web"
	case "WebFetch":
		return "Fetch URL"
	default:
		return action
	}
}

// getPermissionVerb returns a verb for the confirmation question.
func getPermissionVerb(toolName, action string) string {
	switch toolName {
	case "Write":
		return "create"
	case "Edit":
		return "edit"
	case "Bash":
		return "run"
	case "WebSearch":
		return "search"
	case "WebFetch":
		return "fetch"
	default:
		return action
	}
}

// extractPreviewContent extracts displayable content from the permission params.
func extractPreviewContent(perm permission.PermissionRequest) (string, bool) {
	if perm.Params == nil {
		return "", false
	}

	// Marshal to JSON then unmarshal to map to avoid cross-package type assertions
	data, err := json.Marshal(perm.Params)
	if err != nil {
		return "", false
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return "", false
	}

	switch perm.ToolName {
	case "Write":
		if content, ok := m["content"].(string); ok && content != "" {
			return content, true
		}
	case "Edit":
		var preview strings.Builder
		if oldStr, ok := m["old_string"].(string); ok && oldStr != "" {
			preview.WriteString("--- old\n")
			preview.WriteString(oldStr)
			if !strings.HasSuffix(oldStr, "\n") {
				preview.WriteString("\n")
			}
		}
		if newStr, ok := m["new_string"].(string); ok && newStr != "" {
			preview.WriteString("+++ new\n")
			preview.WriteString(newStr)
		}
		if preview.Len() > 0 {
			return preview.String(), true
		}
	case "Bash":
		if cmd, ok := m["command"].(string); ok && cmd != "" {
			return "$ " + cmd, true
		}
	}

	return "", false
}

func renderPermissionPreviewBody(perm permission.PermissionRequest, width int, maxPreviewLines int) (string, int) {
	content, hasContent := extractPreviewContent(perm)
	if !hasContent {
		return "", 0
	}

	lines := strings.Split(content, "\n")
	if maxPreviewLines <= 0 {
		return "", len(lines)
	}
	hiddenLines := 0
	if len(lines) > maxPreviewLines {
		hiddenLines = len(lines) - maxPreviewLines
		lines = lines[:maxPreviewLines]
	}

	contentWidth := width - 8
	if contentWidth < 12 {
		contentWidth = 12
	}

	var sb strings.Builder
	if perm.ToolName == "Edit" {
		inOld := false
		inNew := false
		for _, line := range lines {
			if line == "--- old" {
				inOld, inNew = true, false
				sb.WriteString("  " + previewLineNumStyle.Render(" old ") + diffRemoveStyle.Render("─── old content") + "\n")
				continue
			}
			if line == "+++ new" {
				inOld, inNew = false, true
				sb.WriteString("  " + previewLineNumStyle.Render(" new ") + diffAddStyle.Render("─── new content") + "\n")
				continue
			}

			var style lipgloss.Style
			var prefix string
			switch {
			case inOld:
				style = diffRemoveStyle
				prefix = "- "
			case inNew:
				style = diffAddStyle
				prefix = "+ "
			default:
				style = previewContentStyle
			}

			wrapped := wrapLine(line, contentWidth-2)
			for j, seg := range wrapped {
				if j == 0 {
					sb.WriteString("  " + previewLineNumStyle.Render("     ") + style.Render(prefix+seg) + "\n")
				} else {
					sb.WriteString("  " + previewLineNumStyle.Render("     ") + style.Render("  "+seg) + "\n")
				}
			}
		}
		return sb.String(), hiddenLines
	}

	for i, line := range lines {
		lineNum := previewLineNumStyle.Render(fmt.Sprintf("%4d ", i+1))
		wrapped := wrapLine(line, contentWidth)
		for j, seg := range wrapped {
			if j == 0 {
				sb.WriteString("  " + lineNum + previewContentStyle.Render(seg) + "\n")
			} else {
				sb.WriteString("  " + strings.Repeat(" ", 5) + previewContentStyle.Render(seg) + "\n")
			}
		}
	}

	return sb.String(), hiddenLines
}

// RenderPermissionPreviewCollapsed renders a collapsed summary of a permission preview
// in Claude Code style: header + first few lines + "… +N lines" indicator.
func RenderPermissionPreviewCollapsed(perm permission.PermissionRequest, width int) string {
	var sb strings.Builder

	summary := getToolSummary(perm)

	// Header: ⏺ Write(file_path) — same style as finished tool call in chat
	dot := toolDoneStyle.Render("⏺")
	sb.WriteString("  " + dot + " " + toolNameStyle.Render(perm.ToolName) +
		toolParamStyle.Render("("+summary+")") + "\n")

	// Content preview
	content, hasContent := extractPreviewContent(perm)
	if hasContent {
		lines := strings.Split(content, "\n")
		totalLines := len(lines)

		// Summary line with ⎿ connector
		sb.WriteString("  " + connectorStyle.Render("⎿") + "  " +
			toolResultMutedStyle.Render(fmt.Sprintf("%d lines", totalLines)) + "\n")

		// First few lines of content
		maxPreview := 5
		if totalLines < maxPreview {
			maxPreview = totalLines
		}
		for i := 0; i < maxPreview; i++ {
			num := previewLineNumStyle.Render(fmt.Sprintf("%4d ", i+1))
			wrapped := wrapLine(lines[i], width-12)
			for j, seg := range wrapped {
				if j == 0 {
					sb.WriteString("     " + num + seg + "\n")
				} else {
					sb.WriteString("     " + strings.Repeat(" ", 5) + seg + "\n")
				}
			}
		}

		// Remaining lines indicator
		if totalLines > maxPreview {
			remaining := totalLines - maxPreview
			sb.WriteString("     " +
				toolResultMutedStyle.Render(fmt.Sprintf("… +%d lines (ctrl+o to expand)", remaining)) + "\n")
		}
	}

	return sb.String()
}

var researchSuggestionOptions = []string{"切换到研究模式", "继续普通模式"}

// RenderResearchSuggestionDialog renders the research intent detection dialog.
func RenderResearchSuggestionDialog(selectedIdx int, width int) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(" " + warnStyle.Render("⏺ 检测到研究意图") + " " + dialogTextStyle.Render("建议使用科研流水线（自动文献调研→实验→写作→审稿）。") + "\n")
	sb.WriteString("\n")

	for i, opt := range researchSuggestionOptions {
		numStr := fmt.Sprintf("%d. %s", i+1, opt)
		if i == selectedIdx {
			prefix := " " + btnArrowStyle.Render("❯") + " "
			sb.WriteString(prefix + btnSelectedStyle.Render(numStr) + "\n")
		} else {
			sb.WriteString("   " + btnNormalStyle.Render(numStr) + "\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(" " + dialogMutedStyle.Render("1-2 select · y/n · ↑↓ navigate · enter confirm · esc skip") + "\n")

	return sb.String()
}

var workspaceOptions = []string{"使用当前目录", "更换目录"}

// RenderWorkspaceDialog renders the workspace selection dialog at startup.
func RenderWorkspaceDialog(currentDir string, selectedIdx int, inputMode bool, inputText string, width int) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(" " + warnStyle.Render("⏺ 工作目录") + " " + dialogTextStyle.Render("所有文件操作将限制在此目录内。") + "\n")
	sb.WriteString("   " + dialogMutedStyle.Render(currentDir) + "\n")
	sb.WriteString("\n")

	if inputMode {
		sb.WriteString("   " + dialogTextStyle.Render("输入新路径:") + "\n")
		cursor := "█"
		sb.WriteString("   " + btnSelectedStyle.Render(inputText+cursor) + "\n")
		sb.WriteString("\n")
		sb.WriteString(" " + dialogMutedStyle.Render("enter confirm · esc cancel") + "\n")
	} else {
		for i, opt := range workspaceOptions {
			numStr := fmt.Sprintf("%d. %s", i+1, opt)
			if i == selectedIdx {
				prefix := " " + btnArrowStyle.Render("❯") + " "
				sb.WriteString(prefix + btnSelectedStyle.Render(numStr) + "\n")
			} else {
				sb.WriteString("   " + btnNormalStyle.Render(numStr) + "\n")
			}
		}
		sb.WriteString("\n")
		sb.WriteString(" " + dialogMutedStyle.Render("1-2 select · ↑↓ navigate · enter confirm") + "\n")
	}

	return sb.String()
}

var initRequiredOptions = []string{"开始初始化", "跳过"}

// RenderInitRequiredDialog renders the init required dialog when no profile exists.
func RenderInitRequiredDialog(selectedIdx int, width int) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(" " + warnStyle.Render("⏺ 首次使用") + " " + dialogTextStyle.Render("检测到尚未配置用户画像。建议先完成初始化以获得个性化体验。") + "\n")
	sb.WriteString("\n")

	for i, opt := range initRequiredOptions {
		numStr := fmt.Sprintf("%d. %s", i+1, opt)
		if i == selectedIdx {
			prefix := " " + btnArrowStyle.Render("❯") + " "
			sb.WriteString(prefix + btnSelectedStyle.Render(numStr) + "\n")
		} else {
			sb.WriteString("   " + btnNormalStyle.Render(numStr) + "\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(" " + dialogMutedStyle.Render("1-2 select · ↑↓ navigate · enter confirm · esc skip") + "\n")

	return sb.String()
}

// InitWizardStep describes a single step of the init wizard.
type InitWizardStep struct {
	Question    string
	Options     []string // nil for text input step
	IsTextInput bool
}

// RenderInitWizardDialog renders a single step of the init wizard.
func RenderInitWizardDialog(step InitWizardStep, stepNum, totalSteps, selectedIdx int, textInput string, isTextFocused bool, width int) string {
	if width < 30 {
		width = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow)
	questionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite)
	optionStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	selectedOptionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite).Background(ColorBlue).PaddingLeft(1).PaddingRight(1)
	arrowStyle := lipgloss.NewStyle().Foreground(ColorBlue).Bold(true)
	inputBorderStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)
	hintStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	dividerStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)

	var sb strings.Builder

	sb.WriteString("\n")
	title := fmt.Sprintf("⏺ 个性化配置 (%d/%d)", stepNum, totalSteps)
	sb.WriteString(" " + titleStyle.Render(title) + "\n")
	sb.WriteString(" " + dividerStyle.Render(strings.Repeat("─", width-2)) + "\n")
	sb.WriteString("\n")
	sb.WriteString(" " + questionStyle.Render(step.Question) + "\n")
	sb.WriteString("\n")

	if step.IsTextInput || isTextFocused {
		// Text input mode
		inputWidth := width - 8
		if inputWidth < 20 {
			inputWidth = 20
		}
		border := "─"
		sb.WriteString("  " + arrowStyle.Render("→") + " " + inputBorderStyle.Render("┌"+strings.Repeat(border, inputWidth)+"┐") + "\n")

		displayText := textInput
		if displayText == "" {
			displayText = "输入内容..."
		}
		displayText += "█"
		if displayWidth(displayText) > inputWidth-2 {
			displayText = truncateDisplayStart(displayText, inputWidth-2)
		}
		padding := inputWidth - displayWidth(displayText)
		if padding < 0 {
			padding = 0
		}
		sb.WriteString("    " + inputBorderStyle.Render("│") + " " + displayText + strings.Repeat(" ", padding-1) + inputBorderStyle.Render("│") + "\n")
		sb.WriteString("    " + inputBorderStyle.Render("└"+strings.Repeat(border, inputWidth)+"┘") + "\n")
		sb.WriteString("\n")
		sb.WriteString(" " + hintStyle.Render("Enter 确认  Esc 跳过") + "\n")
	} else {
		// Option selection mode
		for i, opt := range step.Options {
			numStr := fmt.Sprintf("%d. %s", i+1, opt)
			if i == selectedIdx {
				prefix := " " + arrowStyle.Render("→") + " "
				sb.WriteString(prefix + selectedOptionStyle.Render(numStr) + "\n")
			} else {
				sb.WriteString("   " + optionStyle.Render(numStr) + "\n")
			}
		}
		sb.WriteString("\n")
		sb.WriteString(" " + hintStyle.Render("↑↓ 选择  Enter 确认  Esc 跳过") + "\n")
	}

	return sb.String()
}

// truncateLine truncates a line to fit within the given display width.
func truncateLine(line string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	return truncateDisplay(line, maxWidth)
}
