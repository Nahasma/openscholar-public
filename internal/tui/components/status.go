package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// StatusMetrics carries runtime metrics for the enhanced status bar.
type StatusMetrics struct {
	// PromptTokens is the current input/context window usage. It is not a
	// cumulative session total.
	PromptTokens int64
	// CompletionTokens is the most recent output window usage, or a streaming
	// estimate while the response is still arriving.
	CompletionTokens int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	CostUSD          float64
	ContextPct       int
	ContextLimit     int64
	OutputLimit      int64
	PromptEstimated  bool
	OutputEstimated  bool
	LastLatency      time.Duration
}

var (
	statusShortcutStyle lipgloss.Style
	statusHintStyle     lipgloss.Style
	statusSpinnerStyle  lipgloss.Style
	ctxUsageGreenStyle  lipgloss.Style
	ctxUsageYellowStyle lipgloss.Style
	ctxUsageRedStyle    lipgloss.Style
	statusTokenStyle    lipgloss.Style
	statusCostStyle     lipgloss.Style
	statusSepStyle      lipgloss.Style
)

func initStatusStyles() {
	statusShortcutStyle = lipgloss.NewStyle().
		Foreground(ColorGrayDim)
	statusHintStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
	statusSpinnerStyle = lipgloss.NewStyle().
		Foreground(ColorBrandPurple)
	ctxUsageGreenStyle = lipgloss.NewStyle().
		Foreground(ColorGreen)
	ctxUsageYellowStyle = lipgloss.NewStyle().
		Foreground(ColorYellow)
	ctxUsageRedStyle = lipgloss.NewStyle().
		Foreground(ColorRed)
	statusTokenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	statusCostStyle = lipgloss.NewStyle().Foreground(ColorYellow)
	statusSepStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
}

// RenderBottomStatusLine renders the bottom status line with three states:
//   - ctrlCPending: show exit hint
//   - hasInput or isProcessing: show current mode
//   - idle (no input): show shortcuts
//
// contextPct is the context window usage percentage (0-100); 0 means unknown/hidden.
func RenderBottomStatusLine(mode string, isProcessing bool, hasInput bool, ctrlCPending bool, width int, contextPct int) string {
	width = TerminalSafeWidth(width)
	if width < 20 {
		width = 20
	}

	ctxBadge := renderContextBadge(contextPct)

	// State 1: Ctrl+C pending — replace entire bar with exit hint
	if ctrlCPending {
		return " " + statusHintStyle.Render("Press ctrl+c again to exit")
	}

	// State 2: Has input, processing, or non-default mode — show current mode
	if hasInput || isProcessing || mode != "" {
		var modeText string
		switch mode {
		case "auto":
			modeText = modeAutoStyle.Render(" ⏵⏵ auto accept edits") + statusHintStyle.Render(" (shift+tab to cycle)")
		case "research":
			modeText = modeResearchStyle.Render(" ⠋ research mode") + statusHintStyle.Render(" (shift+tab to cycle)")
		default:
			modeText = statusHintStyle.Render(" ⏵ default mode (shift+tab to cycle)")
		}
		if ctxBadge != "" {
			modeWidth := lipgloss.Width(modeText)
			badgeWidth := lipgloss.Width(ctxBadge)
			gap := width - modeWidth - badgeWidth - 1
			if gap < 1 {
				gap = 1
			}
			return modeText + strings.Repeat(" ", gap) + ctxBadge + " "
		}
		return modeText
	}

	// State 3: Idle — CC alignment: minimal, hints moved to input footer
	left := " " + statusHintStyle.Render("shift+tab to cycle mode")

	// Right side: context badge + shift+tab hint
	right := ""
	if ctxBadge != "" {
		right = ctxBadge + "  " + statusHintStyle.Render("shift+tab") + " "
	} else {
		right = statusHintStyle.Render("shift+tab") + " "
	}

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := width - leftWidth - rightWidth
	if gap < 0 {
		gap = 0
	}

	return left + strings.Repeat(" ", gap) + right
}

// renderContextBadge returns a colored "CTX XX%" string based on usage percentage.
// Returns empty string if pct <= 0.
func renderContextBadge(pct int) string {
	if pct <= 0 {
		return ""
	}
	if pct > 100 {
		pct = 100
	}
	text := fmt.Sprintf("CTX %d%%", pct)
	switch {
	case pct >= 80:
		return ctxUsageRedStyle.Render(text)
	case pct >= 60:
		return ctxUsageYellowStyle.Render(text)
	default:
		return ctxUsageGreenStyle.Render(text)
	}
}

func percentageOf(used, limit int64) int {
	if used <= 0 || limit <= 0 {
		return 0
	}
	pct := int((used*100 + limit - 1) / limit)
	if pct < 1 {
		return 1
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func renderTokenWindow(label string, used, limit int64, estimated bool) string {
	if used <= 0 {
		return ""
	}
	prefix := ""
	if estimated {
		prefix = "~"
	}
	if limit <= 0 {
		return fmt.Sprintf("%s %s%s", label, prefix, formatTokenCount(used))
	}
	return fmt.Sprintf("%s %s%s/%s %d%%",
		label,
		prefix,
		formatTokenCount(used),
		formatTokenCount(limit),
		percentageOf(used, limit),
	)
}

func tokenWindowSegments(metrics StatusMetrics) []statusSegment {
	var segments []statusSegment
	if text := renderTokenWindow("IN", metrics.PromptTokens, metrics.ContextLimit, metrics.PromptEstimated); text != "" {
		segments = append(segments, statusSegment{
			text:     statusTokenStyle.Render(text),
			priority: 1,
		})
	} else if ctxBadge := renderContextBadge(metrics.ContextPct); ctxBadge != "" {
		segments = append(segments, statusSegment{text: ctxBadge, priority: 1})
	}
	if text := renderTokenWindow("OUT", metrics.CompletionTokens, metrics.OutputLimit, metrics.OutputEstimated); text != "" {
		segments = append(segments, statusSegment{
			text:     statusTokenStyle.Render(text),
			priority: 2,
		})
	}
	return segments
}

// formatTokenCount formats token count as human-readable (e.g., 13200 -> "13.2k").
func formatTokenCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// statusSegment is a piece of the enhanced status bar with a rendering priority.
// Lower priority value = higher importance (trimmed last).
type statusSegment struct {
	text     string
	priority int
}

func joinStatusSegments(parts []string, sep string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, sep)
}

func renderRightAnchoredStatus(left, right, sep string, width int) string {
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	sepWidth := lipgloss.Width(sep)

	switch {
	case left == "":
		gap := width - rightWidth
		if gap < 0 {
			gap = 0
		}
		return strings.Repeat(" ", gap) + right
	case right == "":
		return left
	default:
		gap := width - leftWidth - sepWidth - rightWidth
		if gap < 0 {
			gap = 0
		}
		return left + strings.Repeat(" ", gap) + sep + right
	}
}

// RenderEnhancedStatusLine renders the status bar with token and cost telemetry.
// It preserves the original mode/processing/idle semantics on the left side and adds
// metrics segments (CTX, tokens, cost) on the right side, trimmed by priority.
func RenderEnhancedStatusLine(mode string, isProcessing bool, hasInput bool, ctrlCPending bool, metrics StatusMetrics, width int) string {
	width = TerminalSafeWidth(width)
	if width < 20 {
		width = 20
	}

	// Ctrl+C pending — always fall back to simple hint
	if ctrlCPending {
		return " " + statusHintStyle.Render("Press ctrl+c again to exit")
	}

	// If metrics are all zero, fall back to original renderer
	metricsEmpty := metrics.PromptTokens == 0 && metrics.CompletionTokens == 0 &&
		metrics.CostUSD == 0
	if metricsEmpty {
		return RenderBottomStatusLine(mode, isProcessing, hasInput, ctrlCPending, width, metrics.ContextPct)
	}

	// Build left side — preserving the original mode/idle semantics
	var leftText string
	if hasInput || isProcessing || mode != "" {
		switch mode {
		case "auto":
			leftText = " " + modeAutoStyle.Render("⏵⏵ auto") + statusHintStyle.Render(" (shift+tab)")
		case "research":
			leftText = " " + modeResearchStyle.Render("⠋ research") + statusHintStyle.Render(" (shift+tab)")
		default:
			leftText = " " + statusHintStyle.Render("⏵ default (shift+tab)")
		}
	} else {
		leftText = " " + statusShortcutStyle.Render("? shortcuts") + "  " + statusHintStyle.Render("scroll ↑↓")
	}

	sep := statusSepStyle.Render(" | ")

	// Build right-side metric segments in priority order (lower = more important)
	var segments []statusSegment

	if metrics.PromptTokens > 0 || metrics.CompletionTokens > 0 {
		tokenText := statusTokenStyle.Render(
			fmt.Sprintf("IN %s OUT %s",
				formatTokenCount(metrics.PromptTokens),
				formatTokenCount(metrics.CompletionTokens),
			),
		)
		segments = append(segments, statusSegment{text: tokenText, priority: 2})
	}

	if metrics.CostUSD > 0 {
		costText := statusCostStyle.Render(fmt.Sprintf("$%.3f", metrics.CostUSD))
		segments = append(segments, statusSegment{text: costText, priority: 3})
	}

	ctxBadge := renderContextBadge(metrics.ContextPct)
	if ctxBadge != "" {
		segments = append(segments, statusSegment{text: ctxBadge, priority: 1})
	}

	// Join segments with separator
	buildRight := func(segs []statusSegment) string {
		parts := make([]string, 0, len(segs))
		for _, s := range segs {
			parts = append(parts, s.text)
		}
		return joinStatusSegments(parts, sep)
	}

	leftWidth := lipgloss.Width(leftText)
	sepWidth := lipgloss.Width(sep)

	// Trim lowest-priority metric segments until they fit
	for len(segments) > 0 {
		rightStr := buildRight(segments)
		rightWidth := lipgloss.Width(rightStr)
		total := leftWidth + sepWidth + rightWidth
		if total <= width {
			return renderRightAnchoredStatus(leftText, rightStr, sep, width)
		}
		// Remove the lowest-priority segment
		maxPri := -1
		maxIdx := -1
		for i, s := range segments {
			if s.priority > maxPri {
				maxPri = s.priority
				maxIdx = i
			}
		}
		segments = append(segments[:maxIdx], segments[maxIdx+1:]...)
	}

	// No metrics fit — just show left side
	return leftText
}

// StatusV2Params carries all state needed for the V2 status bar.
type StatusV2Params struct {
	Mode              string // "" | "auto" | "research" | "plan"
	IsProcessing      bool
	CtrlCPending      bool
	ProcessingPhase   uint8  // Wave 4: 0=Idle, 1=Thinking, 2=ToolRunning, 3=Streaming, 4=Compacting
	ProcessingVerb    string // "reasoning", "writing", etc.
	ProcessingLabel   string // tool/query detail for the current phase
	ProcessingElapsed int    // seconds
	OverlayName       string // active overlay name
	Metrics           StatusMetrics
	HasInput          bool
	ModelName         string
	VimMode           string // "" | "normal" | "insert"
	Width             int
	SpinnerFrame      int
	BgTaskCount       int
	MemoryCount       int
	ResearchProgress  string
	Workspace         string
}

// FooterDockParams carries unified bottom-dock state shared by fullscreen/main.
type FooterDockParams struct {
	Width              int
	Mode               string
	ModelName          string
	Workspace          string
	Metrics            StatusMetrics
	IsProcessing       bool
	CanInterrupt       bool
	CtrlCPending       bool
	HasInput           bool
	HasBlockingOverlay bool
	HasInputOwner      bool
	HasEscapeOwner     bool
	Notice             TransientNotice
	ProcessingPhase    uint8
	ProcessingVerb     string
	ProcessingLabel    string
	ProcessingElapsed  int
	SpinnerFrame       int
	BgTaskCount        int
	MemoryCount        int
	ResearchProgress   string
	OverlayName        string
	VimMode            string
}

// ShouldRenderBottomProcessing returns whether the fixed bottom status row
// should own active runtime progress. Runtime progress belongs to the rail
// directly above the input; the status row keeps only stable metadata/hints.
func ShouldRenderBottomProcessing(isProcessing bool, phase uint8, width int) bool {
	_ = isProcessing
	_ = phase
	_ = width
	return false
}

func statusModeLabel(mode string) string {
	switch mode {
	case "auto":
		return modeAutoStyle.Render("auto mode")
	case "plan":
		return lipgloss.NewStyle().Foreground(Theme.Warning).Render("plan mode")
	case "research":
		return modeResearchStyle.Render("research mode")
	default:
		return ""
	}
}

func statusModelName(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ""
	}
	if idx := strings.LastIndex(modelName, "/"); idx >= 0 {
		modelName = modelName[idx+1:]
	}
	return modelName
}

func renderStatusV2Left(p StatusV2Params) string {
	switch {
	case strings.TrimSpace(p.Mode) != "":
		if label := statusModeLabel(p.Mode); label != "" {
			return " " + label
		}
		return " " + lipgloss.NewStyle().Foreground(Theme.Accent).Render(p.Mode+" mode")
	default:
		var parts []string
		if model := statusModelName(p.ModelName); model != "" {
			parts = append(parts, lipgloss.NewStyle().Foreground(Theme.TextPrimary).Render(model))
		}
		if workspace := strings.TrimSpace(p.Workspace); workspace != "" {
			parts = append(parts, lipgloss.NewStyle().Foreground(Theme.TextMuted).Render(shortenPath(workspace)))
		}
		if len(parts) > 0 {
			return " " + strings.Join(parts, statusSepStyle.Render(" · "))
		}
	}
	return " " + statusShortcutStyle.Render("?") + " " + statusHintStyle.Render("shortcuts")
}

// RenderStatusV2 renders the three-segment V2 status bar with priority trimming.
//
// Layout:
//
//	Left                     Center                          Right
//	mode/vim/processing      research/workspace              CTX · tokens · cost · memory · tasks
//
// Three states:
//   - CtrlCPending: full-width "Press Ctrl+C again to exit"
//   - IsProcessing: spinner + verb + elapsed | metrics
//   - Idle/input:   ? shortcuts or model/path | IN/OUT token windows
func RenderStatusV2(p StatusV2Params) string {
	if p.Width < 20 {
		p.Width = 20
	}

	sepStyle := lipgloss.NewStyle().Foreground(Theme.TextMuted)
	sep := " " + sepStyle.Render(FigBullet) + " "

	// ── State 1: Ctrl+C pending ───────────────────────────────────────────────
	if p.CtrlCPending {
		return " " + lipgloss.NewStyle().Foreground(Theme.Warning).Render("Press Ctrl+C again to exit")
	}

	// ── State 2: Overlay / Vim ownership ─────────────────────────────────────
	// Overlay and Vim retain ownership of the bottom row and are not preempted
	// by processing mirrors.
	if p.OverlayName != "" {
		overlayStyle := lipgloss.NewStyle().Foreground(Theme.Accent)
		leftText := " " + overlayStyle.Render("["+p.OverlayName+"]")
		return truncateToWidth(leftText, p.Width, "…")
	}
	if p.VimMode != "" {
		var leftText string
		switch strings.ToLower(p.VimMode) {
		case "normal":
			leftText = " " + lipgloss.NewStyle().Foreground(Theme.Accent).Render("NORMAL")
		case "insert":
			leftText = " " + lipgloss.NewStyle().Foreground(Theme.Success).Render("INSERT")
		default:
			leftText = " " + lipgloss.NewStyle().Foreground(Theme.TextPrimary).Render(strings.ToUpper(p.VimMode))
		}
		return truncateToWidth(leftText, p.Width, "…")
	}

	// ── State 3: Processing ─────────────────────────────────────────────────
	if ShouldRenderBottomProcessing(p.IsProcessing, p.ProcessingPhase, p.Width) {
		// Wave 4: phase-specific spinner color
		var phaseColor lipgloss.TerminalColor
		switch p.ProcessingPhase {
		case PhaseToolRunning:
			phaseColor = Theme.Info
		case PhaseCompacting:
			phaseColor = Theme.Warning
		default: // Streaming
			phaseColor = Theme.Accent
		}
		// Wave 4: CC-style spinner with phase-specific color
		glyph := CCSpinnerChars[p.SpinnerFrame%len(CCSpinnerChars)]
		spinner := lipgloss.NewStyle().Foreground(phaseColor).Render(glyph)

		verb := p.ProcessingVerb
		if verb == "" {
			verb = "processing"
		}
		verbStyle := lipgloss.NewStyle().Foreground(Theme.TextPrimary)
		elapsedStyle := lipgloss.NewStyle().Foreground(Theme.TextMuted)

		leftText := " " + spinner + " " + verbStyle.Render(verb) +
			" " + elapsedStyle.Render(fmt.Sprintf("· %ds", p.ProcessingElapsed))

		var rightParts []string
		for _, segment := range tokenWindowSegments(p.Metrics) {
			rightParts = append(rightParts, segment.text)
		}

		if len(rightParts) == 0 {
			return leftText
		}
		rightText := strings.Join(rightParts, sep)
		leftW := lipgloss.Width(leftText)
		rightW := lipgloss.Width(rightText)
		gap := p.Width - leftW - rightW - 1
		if gap < 1 {
			gap = 1
		}
		return leftText + strings.Repeat(" ", gap) + rightText + " "
	}

	// ── State 4: Stable status row ──────────────────────────────────────────
	left := renderStatusV2Left(p)

	var centerSegs []statusSegment
	if p.ResearchProgress != "" {
		centerSegs = append(centerSegs, statusSegment{
			text:     lipgloss.NewStyle().Foreground(Theme.Info).Render(truncateToWidth(p.ResearchProgress, max(12, p.Width/4), "…")),
			priority: 2,
		})
	}
	rightSegs := tokenWindowSegments(p.Metrics)
	if p.Metrics.CostUSD > 0 {
		rightSegs = append(rightSegs, statusSegment{
			text:     statusCostStyle.Render(fmt.Sprintf("$%.3f", p.Metrics.CostUSD)),
			priority: 4,
		})
	}
	if p.MemoryCount > 0 {
		rightSegs = append(rightSegs, statusSegment{
			text:     lipgloss.NewStyle().Foreground(Theme.TextMuted).Render(fmt.Sprintf("MEM %d", p.MemoryCount)),
			priority: 3,
		})
	}
	if p.BgTaskCount > 0 {
		rightSegs = append(rightSegs, statusSegment{
			text:     lipgloss.NewStyle().Foreground(Theme.TextMuted).Render(fmt.Sprintf("TASK %d", p.BgTaskCount)),
			priority: 5,
		})
	}

	joinSegs := func(segs []statusSegment) string {
		parts := make([]string, 0, len(segs))
		for _, s := range segs {
			parts = append(parts, s.text)
		}
		return joinStatusSegments(parts, sep)
	}
	dropLowestPriority := func(segs []statusSegment) []statusSegment {
		if len(segs) == 0 {
			return segs
		}
		maxPri, maxIdx := -1, -1
		for i, s := range segs {
			if s.priority > maxPri {
				maxPri = s.priority
				maxIdx = i
			}
		}
		return append(segs[:maxIdx], segs[maxIdx+1:]...)
	}

	for {
		center := joinSegs(centerSegs)
		right := joinSegs(rightSegs)
		leftGroup := joinStatusSegments([]string{left, center}, sep)
		out := joinStatusSegments([]string{leftGroup, right}, sep)
		if lipgloss.Width(out) <= p.Width {
			return renderRightAnchoredStatus(leftGroup, right, sep, p.Width)
		}
		if len(rightSegs) > 0 {
			rightSegs = dropLowestPriority(rightSegs)
			continue
		}
		if len(centerSegs) > 0 {
			centerSegs = dropLowestPriority(centerSegs)
			continue
		}
		return truncateToWidth(out, p.Width, "…")
	}
}

// RenderFooterDock renders a unified fixed footer/status dock.
// It owns status metadata and enter/esc hints so fullscreen has one stable owner.
func RenderFooterDock(p FooterDockParams) string {
	safeWidth := TerminalSafeWidth(p.Width)
	if safeWidth <= 0 {
		return ""
	}
	statusParams := StatusV2Params{
		Mode:              p.Mode,
		IsProcessing:      p.IsProcessing,
		CtrlCPending:      p.CtrlCPending,
		ProcessingPhase:   p.ProcessingPhase,
		ProcessingVerb:    p.ProcessingVerb,
		ProcessingLabel:   p.ProcessingLabel,
		ProcessingElapsed: p.ProcessingElapsed,
		OverlayName:       p.OverlayName,
		Metrics:           p.Metrics,
		HasInput:          p.HasInput,
		ModelName:         p.ModelName,
		VimMode:           p.VimMode,
		Width:             safeWidth,
		SpinnerFrame:      p.SpinnerFrame,
		BgTaskCount:       p.BgTaskCount,
		MemoryCount:       p.MemoryCount,
		ResearchProgress:  p.ResearchProgress,
		Workspace:         p.Workspace,
	}
	if p.CtrlCPending {
		return renderFooterRow(RenderStatusV2(statusParams), "", p.Width)
	}
	if msg := strings.TrimSpace(p.Notice.Text); msg != "" {
		style := lipgloss.NewStyle().Foreground(Theme.Info)
		switch p.Notice.Kind {
		case NoticeError:
			style = lipgloss.NewStyle().Foreground(Theme.Danger)
		case NoticeSuccess:
			style = lipgloss.NewStyle().Foreground(Theme.Success)
		case NoticeWarning:
			style = lipgloss.NewStyle().Foreground(Theme.Warning)
		}
		return renderFooterRow("  "+style.Render(truncateToWidth(msg, max(0, safeWidth-4), "…")), "", p.Width)
	}

	status := RenderStatusV2(statusParams)

	var primaryHint string
	dim := lipgloss.NewStyle().Foreground(Theme.TextMuted)
	switch {
	case p.CtrlCPending:
		primaryHint = ""
	case p.CanInterrupt && !p.HasEscapeOwner:
		primaryHint = dim.Render("esc to interrupt")
	case p.HasInput && !p.IsProcessing && !p.HasBlockingOverlay && !p.HasInputOwner:
		primaryHint = dim.Render("enter to send")
	}
	modeHint := dim.Render("shift+tab mode")
	hint := modeHint
	if primaryHint != "" {
		hint = primaryHint + dim.Render(" · ") + modeHint
	}
	if lipgloss.Width(hint) >= safeWidth {
		hint = primaryHint
	}
	return renderFooterRow(status, hint, p.Width)
}
