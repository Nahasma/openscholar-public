package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Phase constants for RenderProgressLine.
// These values must match the iota order in internal/tui/status_feature.go.
const (
	PhaseIdle        uint8 = 0
	PhaseThinking    uint8 = 1
	PhaseToolQueued  uint8 = 2 // Tools known but none started yet
	PhaseToolRunning uint8 = 3
	PhaseStreaming   uint8 = 4
	PhaseCompacting  uint8 = 5
)

// progressVerbs cycles through during active thinking phase.
var progressVerbs = []string{"thinking", "reasoning", "analyzing", "composing"}

// progressLineStyles holds lazily-initialized styles for the progress line.
// They are populated on first use via ensureProgressStyles().
var (
	progressSpinnerStyle  lipgloss.Style
	progressVerbStyle     lipgloss.Style
	progressTimerStyle    lipgloss.Style
	progressThinkingStyle lipgloss.Style
	progressToolDoneStyle lipgloss.Style
	progressToolErrStyle  lipgloss.Style
	progressStylesReady   bool
)

func ensureProgressStyles() {
	if progressStylesReady {
		return
	}
	progressStylesReady = true

	// Active spinner — accent color
	progressSpinnerStyle = lipgloss.NewStyle().Foreground(ColorOrange)
	// Verb text — muted primary
	progressVerbStyle = lipgloss.NewStyle().Foreground(ColorGrayBright).Italic(true)
	// Timer — dim
	progressTimerStyle = lipgloss.NewStyle().Foreground(ColorGrayDim)
	// Phase 1 silent thinking — dimmer
	progressThinkingStyle = lipgloss.NewStyle().Foreground(ColorGrayMedium).Italic(true)
	// Tool done — green
	progressToolDoneStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	// Tool error — red
	progressToolErrStyle = lipgloss.NewStyle().Foreground(ColorRed)
}

// capitalizeFirst uppercases the first rune of s.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

// spinnerFrame returns the braille character for the given frame index.
func spinnerChar(frame int) string {
	runes := []rune(FigSpinnerFrames)
	return string(runes[frame%len(runes)])
}

// formatElapsed returns a human-friendly duration string (e.g. "3s", "1m12s").
func formatElapsed(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	m := seconds / 60
	s := seconds % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

// ProgressRailOpts carries optional rendering parameters for the progress rail.
type ProgressRailOpts struct {
	StalledPct float64 // 0.0-1.0 stalled intensity (only for streaming)
	TokenCount int     // displayed token count (smooth-animated)
	Width      int     // terminal width for token label formatting
}

// RenderProgressLine renders the processing progress line for the chat area.
//
//   - phase: 0=Idle, 1=Thinking, 2=ToolQueued, 3=ToolRunning, 4=Streaming, 5=Compacting
//   - spinnerFrame: current animation frame (incremented every 100ms)
//   - elapsed: seconds since phase started
//   - activeTool: current tool name (when phase=ToolRunning)
//   - label: description of current activity (overrides default verb when non-empty)
//   - opts: optional rendering parameters (nil-safe)
func RenderProgressLine(phase uint8, spinnerFrame int, elapsed int, activeTool string, label string, opts ...ProgressRailOpts) string {
	_ = activeTool // kept for API compatibility; tool details render in queued lane/tool rows
	var railOpts ProgressRailOpts
	if len(opts) > 0 {
		railOpts = opts[0]
	}
	ensureProgressStyles()
	const indent = "  "

	switch phase {
	case PhaseIdle:
		return ""

	case PhaseStreaming:
		// Wave 4: streaming phase shows persistent progress rail.
		verb := "streaming"
		if label != "" {
			verb = label
		}
		verbText := capitalizeFirst(verb) + "…"
		stalledPct := railOpts.StalledPct
		spinner := SpinnerGlyph(spinnerFrame, stalledPct, false)
		// Stop shimmer when stalled
		shimmerIdx := -1
		if stalledPct < 0.1 {
			shimmerIdx = ShimmerIndex(spinnerFrame, len([]rune(verbText)), true)
		}
		baseColor := StalledColor(stalledPct)
		verbRendered := RenderShimmerText(verbText, shimmerIdx, baseColor, Theme.Accent)
		var timerRendered string
		if elapsed >= 30 {
			timerRendered = progressTimerStyle.Render("(" + formatElapsed(elapsed) + ")")
		}
		parts := indent + spinner + " " + verbRendered
		if timerRendered != "" {
			parts += " " + timerRendered
		}
		// Token counter
		if railOpts.TokenCount > 0 && railOpts.Width > 0 {
			tokenLabel := FormatTokenLabel(railOpts.TokenCount, railOpts.Width)
			if tokenLabel != "" {
				parts += " " + progressTimerStyle.Render(tokenLabel)
			}
		}
		return "\n" + parts + "\n"

	case PhaseThinking:
		if elapsed < 1 {
			// Phase 1 (0-1s): silent dim indicator
			return "\n" + indent + progressThinkingStyle.Render(FigThinking+" Thinking") + "\n"
		}
		// Phase 2 (>1s): CC-style spinner + shimmer verb + timer
		verb := label
		if verb == "" {
			verb = progressVerbs[(spinnerFrame/8)%len(progressVerbs)]
		}
		verbText := capitalizeFirst(verb) + "…"
		spinner := SpinnerGlyph(spinnerFrame, 0, false)
		shimmerIdx := ShimmerIndex(spinnerFrame, len([]rune(verbText)), false)
		verbRendered := RenderShimmerText(verbText, shimmerIdx, Theme.TextPrimary, Theme.Accent)
		var timerRendered string
		if elapsed >= 30 {
			timerRendered = progressTimerStyle.Render("(" + formatElapsed(elapsed) + ")")
		}
		result := indent + spinner + " " + verbRendered
		if timerRendered != "" {
			result += " " + timerRendered
		}
		return "\n" + result + "\n"

	case PhaseToolQueued:
		// Turn-level queued phase: details live in queued lane/tool rows.
		waitIcon := lipgloss.NewStyle().Foreground(Theme.TextMuted).Render(FigBlackCircle)
		verb := "Queued tools"
		if label != "" {
			verb = label
		}
		verbRendered := lipgloss.NewStyle().Foreground(Theme.TextMuted).Italic(true).Render(verb + "…")
		result := indent + waitIcon + " " + verbRendered
		return "\n" + result + "\n"

	case PhaseToolRunning:
		// Turn-level running phase: details live in queued lane/tool rows.
		spinner := SpinnerGlyph(spinnerFrame, 0, false)
		verb := "Running tools"
		if label != "" {
			verb = label
		}
		verbText := capitalizeFirst(verb) + "…"
		shimmerIdx := ShimmerIndex(spinnerFrame, len([]rune(verbText)), true)
		verbRendered := RenderShimmerText(verbText, shimmerIdx, Theme.TextPrimary, Theme.Info)
		result := indent + spinner + " " + verbRendered
		if elapsed > 0 {
			result += " " + progressTimerStyle.Render("("+formatElapsed(elapsed)+")")
		}
		return "\n" + result + "\n"

	case PhaseCompacting:
		compactingVerbs := []string{"compacting", "summarizing", "consolidating"}
		verb := compactingVerbs[(spinnerFrame/8)%len(compactingVerbs)]
		verbText := capitalizeFirst(verb) + "…"
		spinner := SpinnerGlyph(spinnerFrame, 0, false)
		// No shimmer for compacting (slow/background operation)
		verbRendered := lipgloss.NewStyle().Foreground(Theme.Warning).Render(verbText)
		parts := indent + spinner + " " + verbRendered
		if elapsed > 0 {
			parts += " " + progressTimerStyle.Render("("+formatElapsed(elapsed)+")")
		}
		return "\n" + parts + "\n"

	default:
		return ""
	}
}

// RenderToolRunningLine renders a running tool call line.
//
// Example: "  ⠹ Read  internal/tui/view.go"
func RenderToolRunningLine(toolName string, paramSummary string, spinnerFrame int) string {
	ensureProgressStyles()
	const indent = "  "

	spinner := progressSpinnerStyle.Render(spinnerChar(spinnerFrame))
	name := toolNameStyle.Render(toolName)

	if paramSummary == "" {
		return indent + spinner + " " + name + "\n"
	}
	param := toolParamStyle.Render("  " + paramSummary)
	return indent + spinner + " " + name + param + "\n"
}

// RenderToolDoneLine renders a completed tool call line with result summary.
//
// Example: "  ✓ Read  internal/tui/view.go  (247 lines, 0.3s)"
func RenderToolDoneLine(toolName string, paramSummary string, resultSummary string, elapsed float64) string {
	ensureProgressStyles()
	const indent = "  "

	check := progressToolDoneStyle.Render(FigCheckmark)
	name := toolDoneStyle.Render(toolName)

	parts := []string{indent + check + " " + name}

	if paramSummary != "" {
		parts = append(parts, toolParamStyle.Render("  "+paramSummary))
	}

	meta := ""
	if resultSummary != "" && elapsed > 0 {
		meta = fmt.Sprintf("(%s, %.1fs)", resultSummary, elapsed)
	} else if resultSummary != "" {
		meta = "(" + resultSummary + ")"
	} else if elapsed > 0 {
		meta = fmt.Sprintf("(%.1fs)", elapsed)
	}
	if meta != "" {
		parts = append(parts, toolResultMutedStyle.Render("  "+meta))
	}

	return strings.Join(parts, "") + "\n"
}

// RenderToolFailedLine renders a failed tool call line.
//
// Example: "  ✗ Read  /nonexistent.go  file not found"
func RenderToolFailedLine(toolName string, paramSummary string, errMsg string) string {
	ensureProgressStyles()
	const indent = "  "

	cross := progressToolErrStyle.Render(FigCross)
	name := toolErrorStyle.Render(toolName)

	parts := []string{indent + cross + " " + name}

	if paramSummary != "" {
		parts = append(parts, toolParamStyle.Render("  "+paramSummary))
	}
	if errMsg != "" {
		parts = append(parts, toolResultMutedStyle.Render("  "+errMsg))
	}

	return strings.Join(parts, "") + "\n"
}
