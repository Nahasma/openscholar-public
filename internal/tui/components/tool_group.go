package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/message"
)

// Lazily initialized styles for tool group rendering.
var (
	groupHeaderDoneStyle    lipgloss.Style
	groupHeaderRunningStyle lipgloss.Style
	groupHeaderNameStyle    lipgloss.Style
	groupHeaderMetaStyle    lipgloss.Style
	groupItemConnStyle      lipgloss.Style
	groupItemDoneStyle      lipgloss.Style
	groupItemRunningStyle   lipgloss.Style
	groupItemWaitingStyle   lipgloss.Style
	groupItemParamStyle     lipgloss.Style
	groupItemErrStyle       lipgloss.Style
	groupStylesReady        bool
)

func ensureGroupStyles() {
	if groupStylesReady {
		return
	}
	groupStylesReady = true

	groupHeaderDoneStyle = lipgloss.NewStyle().Foreground(Theme.Success)
	groupHeaderRunningStyle = lipgloss.NewStyle().Foreground(Theme.Accent)
	groupHeaderNameStyle = lipgloss.NewStyle().Bold(true) // CC: bold, default foreground
	groupHeaderMetaStyle = lipgloss.NewStyle().Foreground(Theme.TextMuted)
	groupItemConnStyle = lipgloss.NewStyle().Foreground(Theme.TextMuted)
	groupItemDoneStyle = lipgloss.NewStyle().Foreground(Theme.Success)
	groupItemRunningStyle = lipgloss.NewStyle().Foreground(Theme.Accent)
	groupItemWaitingStyle = lipgloss.NewStyle().Foreground(Theme.TextMuted)
	groupItemParamStyle = lipgloss.NewStyle().Foreground(Theme.Tool)
	groupItemErrStyle = lipgloss.NewStyle().Foreground(Theme.Danger)
}

func groupRunningDot(frame int) string {
	if (frame/5)%2 == 0 {
		return groupItemRunningStyle.Render(FigBlackCircle)
	}
	return " "
}

// RenderToolGroupHeader renders the header line for a tool group.
//
// Collapsed + done:  "    Read  3 files"  (no icon, no elapsed)
// Expanded + done:   "  ▼ Read  3 files"
// Running:           "  ⠹ Read  2/3 files"
// Waiting:           "  ○ Read  0/3 files"
//
// When collapsed + done and block.Content is non-empty, a hint line is appended:
//
//	"  ⎿  <lastParam>"
func RenderToolGroupHeader(block *BlockVM, ctx BlockRenderContext) string {
	ensureGroupStyles()

	meta := block.Meta

	toolName := meta.GroupToolName
	if toolName == "" {
		toolName = meta.ToolName
	}
	displayName := getToolDisplayName(toolName)

	total := meta.GroupTotal
	finished := meta.GroupFinished
	allDone := total > 0 && finished >= total

	const indent = "  "

	nameRendered := groupHeaderNameStyle.Render(displayName)

	// Build meta segment using ToolPresenter for semantic noun
	presenter := GetPresenter(toolName)
	var metaParts []string
	if total > 0 {
		metaParts = append(metaParts, presenter.GroupNoun(total, finished, allDone))
	}

	var sb strings.Builder

	switch {
	case allDone && meta.GroupExpanded:
		// CC alignment: expanded done with green ⏺ dot
		greenDot := groupHeaderDoneStyle.Render(FigBlackCircle)
		sb.WriteString(indent + greenDot + " " + nameRendered)
		if len(metaParts) > 0 {
			sb.WriteString("  " + groupHeaderMetaStyle.Render(strings.Join(metaParts, "  ")))
		}
		sb.WriteString("\n")
	case allDone:
		// CC alignment: collapsed done with green ⏺ dot
		greenDot := groupHeaderDoneStyle.Render(FigBlackCircle)
		sb.WriteString(indent + greenDot + " " + nameRendered)
		if len(metaParts) > 0 {
			sb.WriteString("  " + groupHeaderMetaStyle.Render(strings.Join(metaParts, "  ")))
		}
		sb.WriteString("\n")
		// Hint line: show lastParam from GroupHint metadata (truncated for readability)
		if lastParam := meta.GroupHint; lastParam != "" {
			maxHintWidth := 50
			if ctx.Width-8 < maxHintWidth {
				maxHintWidth = ctx.Width - 8
			}
			if maxHintWidth < 10 {
				maxHintWidth = 10
			}
			truncatedHint := truncateToWidth(lastParam, maxHintWidth, "…")
			sb.WriteString(ComposeResponseRow(groupHeaderMetaStyle.Render(truncatedHint), ctx.Width, false) + "\n")
		}
	case meta.GroupRunning > 0 || finished > 0:
		// Running or partial progress: animated dot
		iconRendered := groupRunningDot(ctx.SpinnerFrame)
		sb.WriteString(indent + iconRendered + " " + nameRendered)
		if len(metaParts) > 0 {
			sb.WriteString("  " + groupHeaderMetaStyle.Render(strings.Join(metaParts, "  ")))
		}
		sb.WriteString("\n")
		if meta.GroupHint != "" {
			sb.WriteString(ComposeResponseRow(groupHeaderMetaStyle.Render(truncateToWidth(meta.GroupHint, max(20, ctx.Width-6), "…")), ctx.Width, false) + "\n")
		}
	default:
		// All queued: static dim dot
		waitIcon := groupItemWaitingStyle.Render(FigBlackCircle)
		sb.WriteString(indent + waitIcon + " " + nameRendered)
		if total > 0 {
			metaParts = []string{presenter.GroupNoun(total, 0, false)}
		}
		if len(metaParts) > 0 {
			sb.WriteString("  " + groupHeaderMetaStyle.Render(strings.Join(metaParts, "  ")))
		}
		sb.WriteString("\n")
		if meta.GroupHint != "" {
			sb.WriteString(ComposeResponseRow(groupHeaderMetaStyle.Render(truncateToWidth(meta.GroupHint, max(20, ctx.Width-6), "…")), ctx.Width, false) + "\n")
		}
	}

	return sb.String()
}

// RenderToolGroupItem renders a single tool call item within a group.
//
// Done:    "  ⎿  internal/tui/view.go  (247 lines)"
// Running: "  │ ⠹ internal/tui/model.go"
// Waiting: "  │ ○ internal/tui/update.go"
func RenderToolGroupItem(block *BlockVM, ctx BlockRenderContext) string {
	ensureGroupStyles()

	meta := block.Meta
	paramSummary := extractToolParamsSummary(meta.ToolName, meta.ToolInput)
	toolFinished := meta.ToolFinished || message.IsTerminalToolState(meta.ToolState)

	const indent = "  "

	var sb strings.Builder

	switch {
	case meta.IsError:
		// Error: use ⎿ connector with error-styled param
		sb.WriteString(indent + connectorStyle.Render(FigConnector) + "  ")
		if paramSummary != "" {
			sb.WriteString(groupItemErrStyle.Render(paramSummary))
		}
	case toolFinished:
		// Done: use ⎿ connector with green color for visual completion feedback
		sb.WriteString(indent + groupItemDoneStyle.Render(FigConnector) + "  ")
		if paramSummary != "" {
			sb.WriteString(groupItemParamStyle.Render(paramSummary))
		}
		// Append result summary when finished
		if ctx.ToolMessages != nil {
			tc := message.ToolCall{
				ID:       meta.ToolCallID,
				Name:     meta.ToolName,
				Input:    meta.ToolInput,
				Finished: toolFinished,
				State:    meta.ToolState,
			}
			resultSummary := getToolResultSummary(tc, ctx.ToolMessages)
			if resultSummary != "" {
				sb.WriteString("  " + groupHeaderMetaStyle.Render("("+resultSummary+")"))
			}
		}
	case meta.ToolState == message.ToolCallQueued:
		// Queued: static dim dot
		connector := groupItemConnStyle.Render(FigTreeVert)
		waitIcon := groupItemWaitingStyle.Render(FigBlackCircle)
		sb.WriteString(indent + connector + " " + waitIcon)
		if paramSummary != "" {
			sb.WriteString(" " + groupItemParamStyle.Render(paramSummary))
		}
	default:
		// Running: animated spinner
		connector := groupItemConnStyle.Render(FigTreeVert)
		iconRendered := groupItemRunningStyle.Render(groupRunningDot(ctx.SpinnerFrame))
		sb.WriteString(indent + connector + " " + iconRendered)
		if paramSummary != "" {
			sb.WriteString(" " + groupItemParamStyle.Render(paramSummary))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}
