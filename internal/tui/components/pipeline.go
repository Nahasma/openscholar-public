package components

import (
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/research"
)

// RenderPipelineBar 渲染紧凑模式的流水线进度条（单行，嵌入状态栏）
func RenderPipelineBar(phases []*research.Phase, budget research.Budget, width int) string {
	if len(phases) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("⠋ Research: ")
	for i, ph := range phases {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(phaseIcon(ph.Status))
		sb.WriteString(" ")
		sb.WriteString(ph.Name)
	}

	if budget.Limit > 0 {
		budgetStr := fmt.Sprintf("  $%.2f/$%.0f", budget.Spent, budget.Limit)
		sb.WriteString(budgetStr)
	}

	result := sb.String()
	result = truncateDisplay(result, width)
	return result
}

// RenderPipelineExpanded 渲染展开模式的流水线进度（多行，Ctrl+R 切换）
func RenderPipelineExpanded(phases []*research.Phase, budget research.Budget, width int) string {
	if len(phases) == 0 {
		return ""
	}
	var sb strings.Builder
	border := strings.Repeat("─", width-4)
	sb.WriteString("┌─ Research Pipeline " + border[:max(0, len(border)-20)] + "┐\n")

	for _, ph := range phases {
		icon := phaseIcon(ph.Status)
		line := fmt.Sprintf("│ %s %d/%d %s", icon, ph.Order, len(phases), ph.Name)

		switch ph.Status {
		case research.PhaseRunning:
			line += fmt.Sprintf(" (%d workers)", ph.MaxWorkers)
		case research.PhaseCompleted:
			line += " ✓"
		}

		// Pad to width
		padding := width - 2 - visibleLen(line)
		if padding > 0 {
			line += strings.Repeat(" ", padding)
		}
		line += "│\n"
		sb.WriteString(line)
	}

	// Budget line
	if budget.Limit > 0 {
		budgetStr := fmt.Sprintf("$%.2f/$%.0f", budget.Spent, budget.Limit)
		padding := width - 4 - len(budgetStr)
		if padding > 0 {
			sb.WriteString("│ " + strings.Repeat(" ", padding) + budgetStr + " │\n")
		}
	}

	sb.WriteString("└" + strings.Repeat("─", width-2) + "┘\n")
	return sb.String()
}

// (RenderCheckpointDialog moved to checkpoint.go with expanded functionality)

func phaseIcon(status research.PhaseStatus) string {
	switch status {
	case research.PhasePending:
		return "⏳"
	case research.PhaseRunning:
		return "⠋"
	case research.PhaseCompleted:
		return "✓"
	case research.PhaseFailed:
		return "✗"
	case research.PhasePaused:
		return "⏸"
	default:
		return "?"
	}
}

// visibleLen estimates visible character length (rough, ignoring ANSI codes)
func visibleLen(s string) int {
	return len([]rune(s))
}
