package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// templateInfo holds display metadata for each research template.
type templateInfo struct {
	Label  string   // e.g. "实证研究"
	Phases []string // phase names in order
}

var templateMeta = map[string]templateInfo{
	"empirical": {
		Label:  "实证研究",
		Phases: []string{"文献调研", "研究设计", "实验实施", "论文写作", "审稿修订"},
	},
	"aris_empirical": {
		Label:  "ARIS 实证研究",
		Phases: []string{"Idea", "Method", "Plan", "Experiment", "Claims", "Review", "Handoff"},
	},
	"survey": {
		Label:  "综述研究",
		Phases: []string{"文献调研", "分类体系", "论文写作", "审稿修订"},
	},
	"theoretical": {
		Label:  "理论研究",
		Phases: []string{"文献调研", "理论构建", "论文写作", "审稿修订"},
	},
}

// RenderTemplateDialog renders the research template selection dialog.
// Input field is always active. @ triggers file picker (rendered separately).
func RenderTemplateDialog(options []string, selectedIdx int, folderName string, width int) string {
	if width < 30 {
		width = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite)
	optionNameStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite)
	selectedNameStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorWhite).Background(ColorBlue).PaddingLeft(1).PaddingRight(1)
	phaseStyle := lipgloss.NewStyle().Foreground(ColorGrayBright)
	arrowStyle := lipgloss.NewStyle().Foreground(ColorBlue).Bold(true)
	inputBorderStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)
	hintStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	dividerStyle := lipgloss.NewStyle().Foreground(ColorGrayDark)

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(" " + titleStyle.Render("⏺ 选择研究模板") + "\n")
	sb.WriteString(" " + dividerStyle.Render(strings.Repeat("─", width-2)) + "\n")
	sb.WriteString("\n")

	for i, key := range options {
		meta, ok := templateMeta[key]
		if !ok {
			meta = templateInfo{Label: key, Phases: []string{"unknown"}}
		}

		nameStr := fmt.Sprintf("%d. %s (%s)", i+1, key, meta.Label)
		phasesStr := strings.Join(meta.Phases, " → ")

		if i == selectedIdx {
			prefix := " " + arrowStyle.Render("→") + " "
			sb.WriteString(prefix + selectedNameStyle.Render(nameStr) + "\n")
		} else {
			sb.WriteString("   " + optionNameStyle.Render(nameStr) + "\n")
		}
		sb.WriteString("     " + phaseStyle.Render(phasesStr) + "\n")

		if i < len(options)-1 {
			sb.WriteString("\n")
		}
	}

	// Work directory input (always active)
	sb.WriteString("\n")
	sb.WriteString(" " + labelStyle.Render("工作目录") + " " + hintStyle.Render("(留空使用当前目录, @ 选择文件夹)") + "\n")

	border := "─"
	inputWidth := width - 8
	if inputWidth < 20 {
		inputWidth = 20
	}

	sb.WriteString("  " + arrowStyle.Render("→") + " " + inputBorderStyle.Render("┌"+strings.Repeat(border, inputWidth)+"┐") + "\n")

	displayText := folderName + "█"
	if folderName == "" {
		displayText = hintStyle.Render("当前目录") + "█"
	}

	// Truncate from left if too long
	plain := folderName + "█"
	if displayWidth(plain) > inputWidth-2 {
		displayText = truncateDisplayStart(plain, inputWidth-2)
	}

	padding := inputWidth - displayWidth(displayText)
	if padding < 0 {
		padding = 0
	}

	sb.WriteString("    " + inputBorderStyle.Render("│") + " " + displayText + strings.Repeat(" ", padding-1) + inputBorderStyle.Render("│") + "\n")
	sb.WriteString("    " + inputBorderStyle.Render("└"+strings.Repeat(border, inputWidth)+"┘") + "\n")

	// Hints
	sb.WriteString("\n")
	sb.WriteString(" " + hintStyle.Render("直接输入路径  ↑↓ 切换模板  @ 选择文件夹  Enter 确认  Esc 取消") + "\n")

	return sb.String()
}
