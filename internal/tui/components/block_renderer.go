package components

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/openscholar/openscholar/internal/message"
)

// BlockRenderFunc is the signature for block rendering functions.
type BlockRenderFunc func(block *BlockVM, ctx BlockRenderContext) string

// BlockRenderContext carries the runtime context needed to render a block.
type BlockRenderContext struct {
	Width        int
	SpinnerFrame int
	ToolMessages map[string]message.Message
	RenderCache  *RenderCache
	Expanded     map[string]bool        // legacy: bool expand state
	ExpandNodes  map[string]ExpandState // Wave 2: three-state expand
	RenderFn     BlockRenderFunc        // optional override; nil defaults to RenderBlock

	// ResolveExpand is an optional callback that resolves the ExpandState for a
	// given semantic key. When set, IsExpandedV2 delegates to it first, which
	// allows the TUI layer (ChatFeature.GetExpandState) to inject transcript-wide
	// expand logic (e.g. transcriptExpanded fallback) without duplicating state.
	ResolveExpand func(string) ExpandState

	// Wave 2: Processing state for three-phase thinking animation
	ProcessingPhase   uint8 // 0=Idle, 1=Thinking, 2=ToolRunning, 3=Streaming, 4=Compacting
	ProcessingElapsed int   // seconds since processing started

	// InMessageResponse indicates we're already inside a ⎿ response container.
	// Used by ComposeResponseRow to prevent nested connectors.
	InMessageResponse bool
}

// IsExpandedV2 returns the ExpandState for a given key.
// Priority: ResolveExpand callback > ExpandNodes > legacy Expanded map.
func (ctx BlockRenderContext) IsExpandedV2(key string) ExpandState {
	if ctx.ResolveExpand != nil {
		return ctx.ResolveExpand(key)
	}
	if ctx.ExpandNodes != nil {
		return ctx.ExpandNodes[key]
	}
	if ctx.Expanded != nil && ctx.Expanded[key] {
		return ExpandInline
	}
	return ExpandCollapsed
}

// Render dispatches to RenderFn if set, otherwise to RenderBlock.
func (ctx BlockRenderContext) Render(block *BlockVM) string {
	if ctx.RenderFn != nil {
		return ctx.RenderFn(block, ctx)
	}
	return RenderBlock(block, ctx)
}

// RenderBlock renders a BlockVM using V2 visual style, with caching.
// It updates block.Rendered, block.WidthKey, block.Height, and block.Dirty in place.
// Note: V1 rendering path has been removed; this now delegates to V2 renderers directly.
func RenderBlock(block *BlockVM, ctx BlockRenderContext) string {
	return RenderBlockV2(block, ctx)
}

// renderSummaryBlock renders a [Conversation Summary] user message as a collapsible block.
func renderSummaryBlock(block *BlockVM, ctx BlockRenderContext) string {
	content := block.Content
	summaryContent := strings.TrimPrefix(content, "[Conversation Summary]\n")

	focusPrefix := ""
	if block.Meta.IsFocused {
		focusPrefix = "▶ "
	}

	var sb strings.Builder
	if block.Meta.IsExpanded {
		sb.WriteString("  " + focusPrefix + thinkingLabelStyle.Render("📋 Conversation Summary") + "\n")
		for _, line := range strings.Split(summaryContent, "\n") {
			wrapped := wrapLine(line, ctx.Width-6)
			for i, seg := range wrapped {
				if i == 0 {
					sb.WriteString("     " + thinkingStyle.Render(seg) + "\n")
				} else {
					sb.WriteString("       " + thinkingStyle.Render(seg) + "\n")
				}
			}
		}
	} else {
		firstLine := strings.TrimSpace(strings.SplitN(summaryContent, "\n", 2)[0])
		maxPreview := ctx.Width - 10
		if maxPreview < 20 {
			maxPreview = 20
		}
		firstLine = truncateToWidth(firstLine, maxPreview, "…")
		sb.WriteString("  " + focusPrefix + thinkingLabelStyle.Render("📋") + " " +
			thinkingStyle.Render(firstLine) + "\n")
		lines := strings.Split(summaryContent, "\n")
		if len(lines) > 1 {
			sb.WriteString("  " + connectorStyle.Render(FigConnector) + "  " +
				toolResultMutedStyle.Render(fmt.Sprintf("… +%d lines (ctrl+o to expand)", len(lines)-1)) + "\n")
		}
	}
	return sb.String()
}

// renderToolPreviewBlock renders the collapsed preview of a tool result.
// Edit tool → compact diff preview; others → compact output preview.
// NOTE: V2 path uses renderToolPreviewBlockV2 instead; this is kept for
// non-V2 callers (e.g. BlockToolDetail fallback).
func renderToolPreviewBlock(block *BlockVM, ctx BlockRenderContext) string {
	meta := block.Meta
	if ctx.ToolMessages == nil {
		return ""
	}
	toolMsg, ok := ctx.ToolMessages[meta.ToolCallID]
	if !ok {
		return ""
	}

	tc := message.ToolCall{
		ID:       meta.ToolCallID,
		Name:     meta.ToolName,
		Input:    meta.ToolInput,
		Finished: meta.ToolFinished || message.IsTerminalToolState(meta.ToolState),
		State:    meta.ToolState,
	}
	toolResultsMap := map[string]message.Message{meta.ToolCallID: toolMsg}

	// Use RenderStructuredDiff for Edit tools in the block path
	if meta.ToolName == "Edit" && block.Content != "" {
		lang := langFromToolInput(meta.ToolInput)
		return RenderStructuredDiffLang(block.Content, lang, ctx.Width, true)
	}

	var sb strings.Builder
	if meta.ToolName == "Edit" {
		renderCompactDiff(&sb, tc, toolResultsMap, ctx.Width)
	} else {
		renderCompactOutput(&sb, tc, toolResultsMap, ctx.Width)
	}
	return sb.String()
}

// renderToolDetailBlock renders the expanded detail of a tool result (max 200 lines).
func renderToolDetailBlock(block *BlockVM, ctx BlockRenderContext) string {
	meta := block.Meta

	// Use RenderStructuredDiff for Edit tools in the block path (full mode)
	if meta.ToolName == "Edit" && block.Content != "" {
		lang := langFromToolInput(meta.ToolInput)
		expandedContent := RenderStructuredDiffLang(block.Content, lang, ctx.Width-8, false)
		const maxExpandedLines = 200
		lines := strings.Split(expandedContent, "\n")
		if len(lines) > maxExpandedLines {
			remaining := len(lines) - maxExpandedLines
			return strings.Join(lines[:maxExpandedLines], "\n") + "\n" +
				"     " + toolResultMutedStyle.Render(fmt.Sprintf("… +%d lines (ctrl+o to collapse)", remaining)) + "\n"
		}
		return expandedContent
	}

	if ctx.ToolMessages == nil {
		return ""
	}
	toolMsg, ok := ctx.ToolMessages[meta.ToolCallID]
	if !ok {
		return ""
	}

	tc := message.ToolCall{
		ID:       meta.ToolCallID,
		Name:     meta.ToolName,
		Input:    meta.ToolInput,
		Finished: meta.ToolFinished || message.IsTerminalToolState(meta.ToolState),
		State:    meta.ToolState,
	}
	toolResultsMap := map[string]message.Message{meta.ToolCallID: toolMsg}

	var expandedSB strings.Builder
	renderToolResult(&expandedSB, tc, toolResultsMap, ctx.Expanded, ctx.Width)
	expandedContent := expandedSB.String()

	const maxExpandedLines = 200
	lines := strings.Split(expandedContent, "\n")
	if len(lines) > maxExpandedLines {
		remaining := len(lines) - maxExpandedLines
		return strings.Join(lines[:maxExpandedLines], "\n") + "\n" +
			"     " + toolResultMutedStyle.Render(fmt.Sprintf("… +%d lines (ctrl+o to collapse)", remaining)) + "\n"
	}
	return expandedContent
}

// renderMessageResponse wraps content with the CC-style ⎿ connector prefix.
// Uses ComposeResponseRow for unified layout. The inResponse parameter prevents
// nesting when already inside a response container.
func renderMessageResponse(content string) string {
	return ComposeResponseRow(content, 0, false)
}

// langFromToolInput extracts programming language from tool input JSON's file_path.
// Returns "" if extraction fails.
func langFromToolInput(input string) string {
	if input == "" {
		return ""
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return ""
	}
	fp, _ := params["file_path"].(string)
	if fp == "" {
		return ""
	}
	return extToLang(filepath.Ext(fp))
}

// renderCompactBoundaryBlock renders a compact boundary marker.
// CC alignment: uses ✻ fleuron with dimColor, marginY=1.
func renderCompactBoundaryBlock(block *BlockVM, _ BlockRenderContext) string {
	text := "Conversation compacted (ctrl+o for history)"
	if block.Content != "" {
		text = block.Content
	}
	dimStyle := lipgloss.NewStyle().Faint(true)
	return dimStyle.Render("  "+FigFleuron+" "+text) + "\n"
}

// RenderSystemDivider renders a centered system divider line: ━━━ text ━━━
func RenderSystemDivider(text string, width int) string {
	systemStyle := lipgloss.NewStyle().Foreground(Theme.TextMuted)
	textWidth := runewidth.StringWidth(text)
	// 2 spaces gutter + 1 space padding each side of text
	lineLen := (width - textWidth - 2 - 2) / 2
	if lineLen < 3 {
		lineLen = 3
	}
	line := strings.Repeat(FigHeavyLine, lineLen)
	return "  " + systemStyle.Render(line+" "+text+" "+line)
}

// ── V2 block rendering (feature-gated by VisualV2) ──────────────────────────

// RenderBlockV2 renders a BlockVM using V2 visual style.
func RenderBlockV2(block *BlockVM, ctx BlockRenderContext) string {
	if IsDisplayHidden(*block) {
		return ""
	}
	if !block.Dirty && block.WidthKey == ctx.Width && block.Rendered != "" {
		return block.Rendered
	}

	var result string
	switch block.Kind {
	case BlockUser:
		result = renderUserBlockV2(block, ctx)
	case BlockAssistantMarkdown:
		result = renderMarkdownBlockV2(block, ctx)
	case BlockThinking:
		result = renderThinkingBlockV2(block, ctx)
	case BlockToolHeader:
		result = renderToolHeaderBlockV2(block, ctx)
	case BlockToolPreview:
		// When ExpandInline, render inline expand instead of collapsed preview
		if block.Meta.SemanticKey != "" && ctx.IsExpandedV2(block.Meta.SemanticKey) == ExpandInline {
			result = renderToolInlineExpandV2(block, ctx)
		} else {
			result = renderToolPreviewBlockV2(block, ctx)
		}
	case BlockToolDetail:
		// When ExpandInline, render inline expand instead of full detail
		if block.Meta.SemanticKey != "" && ctx.IsExpandedV2(block.Meta.SemanticKey) == ExpandInline {
			result = renderToolInlineExpandV2(block, ctx)
		} else {
			result = renderToolDetailBlock(block, ctx)
		}
	case BlockStreamTail:
		result = renderStreamTailBlockV2(block, ctx)
	case BlockInlineError:
		result = renderErrorBlockV2(block, ctx)
	case BlockCompactBoundary:
		result = renderCompactBoundaryBlock(block, ctx)
	case BlockToolGroupHeader:
		result = RenderToolGroupHeader(block, ctx)
	case BlockToolGroupItem:
		result = RenderToolGroupItem(block, ctx)
	case BlockSystem:
		result = renderSystemBlockV2(block, ctx)
	case BlockCommandActivity:
		result = renderCommandActivityBlockV2(block, ctx)
	default:
		result = ""
	}

	block.Rendered = result
	block.WidthKey = ctx.Width
	block.Height = MeasureRenderedHeight(result)
	block.Dirty = false
	return result
}

func renderSystemBlockV2(block *BlockVM, ctx BlockRenderContext) string {
	content := block.Content
	if strings.TrimSpace(content) == "" {
		return ""
	}
	availWidth := BodyWidth(ctx.Width)
	rendered := RenderMarkdown(content, availWidth)
	dot := systemLineStyleV2.Render(FigDiamondOpen)
	return ComposeMessageRow(dot, rendered, ctx.Width)
}

func renderCommandActivityBlockV2(block *BlockVM, _ BlockRenderContext) string {
	invocation := strings.TrimSpace(block.Meta.CommandInvocation)
	summary := strings.TrimSpace(block.Meta.CommandSummary)
	if invocation == "" || summary == "" {
		return ""
	}
	return "  " + FigPrompt + " " + invocation + "\n  " + FigConnector + "  " + summary + "\n"
}

func renderUserBlockV2(block *BlockVM, ctx BlockRenderContext) string {
	content := block.Content
	if content == "" {
		return ""
	}
	if block.Meta.IsSummary {
		return renderSummaryBlock(block, ctx)
	}

	dot := userPrefixStyleV2.Render(FigBlackCircle)
	availWidth := BodyWidth(ctx.Width) - 1
	if availWidth < 1 {
		availWidth = 1
	}
	rendered := RenderMarkdown(content, availWidth)
	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return ""
	}

	var bodyLines []string
	for _, line := range strings.Split(rendered, "\n") {
		bodyLines = append(bodyLines, userContentStyleV2.Render(line))
	}
	rendered = strings.Join(bodyLines, "\n")

	result := ComposeMessageRow(dot, rendered, ctx.Width)

	if len(block.Meta.Attachments) > 0 {
		var sb strings.Builder
		sb.WriteString(result)
		continuation := strings.Repeat(" ", MessagePrefixWidth())
		for _, att := range block.Meta.Attachments {
			sb.WriteString(continuation + toolParamStyle.Render(FigAttachment+" "+att.FileName) + "\n")
		}
		return sb.String()
	}
	return result
}

func renderMarkdownBlockV2(block *BlockVM, ctx BlockRenderContext) string {
	CleanupStreamRenderer(block.MsgID)
	content := block.Content
	if content == "" {
		return ""
	}

	hasDot := block.Meta.IsFirstContentBlock
	// Use appropriate width: dot prefix is wider than plain continuation indent
	var availWidth int
	if hasDot {
		availWidth = BodyWidth(ctx.Width)
	} else {
		availWidth = ContinuationBodyWidth(ctx.Width)
	}

	cacheKey := fmt.Sprintf("block-v2:%s:%d:%d:%v", block.ID, len(content), ctx.Width, hasDot)
	if ctx.RenderCache != nil {
		if cached, ok := ctx.RenderCache.Get(cacheKey); ok {
			return cached
		}
	}

	rendered := RenderMarkdown(content, availWidth)
	// RenderMarkdown now only strips newlines, preserving Glamour's internal
	// indentation (tables, nested lists, block quotes).

	var result string
	if hasDot {
		dot := assistantPrefixStyleV2.Render(FigBlackCircle)
		result = ComposeMessageRow(dot, rendered, ctx.Width)
	} else {
		var sb strings.Builder
		for _, line := range strings.Split(rendered, "\n") {
			sb.WriteString(ComposeContinuation(line) + "\n")
		}
		result = sb.String()
	}

	if ctx.RenderCache != nil {
		ctx.RenderCache.Set(cacheKey, result)
	}
	return result
}

// StreamTailRenderSplit describes the rendered stream-tail block and a safe
// split boundary for non-fullscreen/main-screen incremental flushing.
//
// Split rule: only source paragraphs that are closed by a stable Markdown
// boundary are returned in CommittedLines. The active paragraph remains in
// LiveLines because later tokens can reflow its visual lines.
type StreamTailRenderSplit struct {
	Rendered           string
	RenderedLines      []string
	CommittedLines     []string
	LiveLines          []string
	CommittedSourceEnd int
	Content            string

	// Layout metadata for callers to detect and avoid advancing across resize.
	Width      int
	BodyWidth  int
	HasDot     bool
	MessageID  string
	ContentLen int
}

// CommittedRenderedAfter returns only the committed content whose source offset
// is strictly after the provided offset.
func (s StreamTailRenderSplit) CommittedRenderedAfter(sourceEnd int) string {
	if len(s.CommittedLines) == 0 || sourceEnd >= s.CommittedSourceEnd {
		return ""
	}
	if sourceEnd > 0 {
		return s.renderSourceRange(sourceEnd, s.CommittedSourceEnd)
	}
	return strings.Join(s.CommittedLines, "\n")
}

// RenderedSuffixAfter returns the rendered stream suffix whose source offset is
// after sourceEnd. Callers can use it to flush only unflushed source once.
func (s StreamTailRenderSplit) RenderedSuffixAfter(sourceEnd int) string {
	if sourceEnd <= 0 {
		return renderLinesWithTrailingNewline(s.RenderedLines)
	}
	return s.renderSourceRangeWithTrailingNewline(sourceEnd, len(s.Content))
}

func (s StreamTailRenderSplit) renderSourceRange(start, end int) string {
	return strings.TrimRight(s.renderSourceRangeWithTrailingNewline(start, end), "\n")
}

func (s StreamTailRenderSplit) renderSourceRangeWithTrailingNewline(start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s.Content) {
		end = len(s.Content)
	}
	if start >= end {
		return ""
	}
	body := renderStreamingMarkdownFresh(s.Content[start:end], s.BodyWidth)
	lines := decorateStreamTailLines(body, s.HasDot, start > 0)
	return renderLinesWithTrailingNewline(lines)
}

func renderLinesWithTrailingNewline(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func stableStreamPrefixLen(content string, width int) int {
	paragraphs := strings.Split(content, "\n\n")
	if len(paragraphs) <= 1 {
		return stablePlainTextPrefixLen(content, width)
	}

	cacheableCount := 0
	openFenceMarker := ""
	for i := 0; i < len(paragraphs)-1; i++ {
		openFenceMarker = trackFenceMarkerState(paragraphs[i], openFenceMarker)
		if openFenceMarker == "" {
			cacheableCount = i + 1
		}
	}
	if cacheableCount == 0 {
		return stablePlainTextPrefixLen(content, width)
	}
	prefix := strings.Join(paragraphs[:cacheableCount], "\n\n") + "\n\n"
	if len(prefix) > len(content) {
		return len(content)
	}
	if tailPrefix := stablePlainTextPrefixLen(content[len(prefix):], width); tailPrefix > 0 {
		return len(prefix) + tailPrefix
	}
	return len(prefix)
}

const streamLiveTailVisualLines = 3

func stablePlainTextPrefixLen(content string, width int) int {
	if content == "" || width <= 0 || strings.Contains(content, "\n\n") || !isPlainTextForFastPath(content) {
		return 0
	}

	lines := strings.Split(content, "\n")
	stableLinePrefixEnd := 0
	activeLine := content
	if len(lines) > 1 {
		stableLinePrefixEnd = len(strings.Join(lines[:len(lines)-1], "\n")) + 1
		activeLine = content[stableLinePrefixEnd:]
	}

	if visualEnd := stablePlainTextVisualPrefixLen(activeLine, width, streamLiveTailVisualLines); visualEnd > 0 {
		return stableLinePrefixEnd + visualEnd
	}
	return stableLinePrefixEnd
}

func stablePlainTextVisualPrefixLen(line string, width, keepLines int) int {
	if line == "" || width <= 0 || keepLines <= 0 {
		return 0
	}
	totalWidth := runewidth.StringWidth(line)
	if totalWidth <= width*(keepLines+1) {
		return 0
	}
	targetWidth := totalWidth - width*keepLines
	seenWidth := 0
	lastWordBoundary := 0
	for idx, r := range line {
		rw := runewidth.RuneWidth(r)
		if seenWidth+rw > targetWidth {
			if lastWordBoundary > 0 {
				return lastWordBoundary
			}
			return idx
		}
		seenWidth += rw
		if r == ' ' || r == '\t' {
			lastWordBoundary = idx + len(string(r))
		}
	}
	if lastWordBoundary > 0 && lastWordBoundary < len(line) {
		return lastWordBoundary
	}
	return 0
}

func renderStreamingMarkdownFresh(content string, width int) string {
	rendered := NewStreamingMarkdownRenderer().Render(content, width)
	var cleanLines []string
	for _, line := range strings.Split(rendered, "\n") {
		cleanLines = append(cleanLines, strings.TrimLeft(line, " \t"))
	}
	return strings.Join(cleanLines, "\n")
}

func decorateStreamTailLines(body string, hasDot bool, dotAlreadyRendered bool) []string {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	bodyLines := strings.Split(body, "\n")
	lines := make([]string, 0, len(bodyLines))
	switch {
	case hasDot && !dotAlreadyRendered:
		dot := assistantPrefixStyleV2.Render(FigBlackCircle)
		continuation := strings.Repeat(" ", MessagePrefixWidth())
		for i, line := range bodyLines {
			if i == 0 {
				lines = append(lines, "  "+dot+" "+line)
			} else {
				lines = append(lines, continuation+line)
			}
		}
	case hasDot:
		continuation := strings.Repeat(" ", MessagePrefixWidth())
		for _, line := range bodyLines {
			lines = append(lines, continuation+line)
		}
	default:
		for _, line := range bodyLines {
			lines = append(lines, ComposeContinuation(line))
		}
	}
	return lines
}

// RenderStreamTailBlockSplitV2 renders a stream-tail block exactly as V2 does
// and returns both full output and a safe committed/live visual-line split.
func RenderStreamTailBlockSplitV2(block *BlockVM, ctx BlockRenderContext) StreamTailRenderSplit {
	split := StreamTailRenderSplit{
		Width:      ctx.Width,
		HasDot:     block.Meta.IsFirstContentBlock,
		MessageID:  block.MsgID,
		ContentLen: len(block.Content),
		Content:    block.Content,
	}
	if block.Content == "" {
		return split
	}

	var availWidth int
	if split.HasDot {
		availWidth = BodyWidth(ctx.Width)
	} else {
		availWidth = ContinuationBodyWidth(ctx.Width)
	}
	split.BodyWidth = availWidth

	rendered := renderStreamingMarkdown(block.Content, block.MsgID, availWidth, ctx.RenderCache)
	split.RenderedLines = decorateStreamTailLines(rendered, split.HasDot, false)
	if len(split.RenderedLines) == 0 {
		return split
	}

	stableLen := stableStreamPrefixLen(block.Content, availWidth)
	split.CommittedSourceEnd = stableLen
	switch {
	case stableLen <= 0:
		split.LiveLines = append(split.LiveLines, split.RenderedLines...)
	case stableLen >= len(block.Content):
		split.CommittedLines = append(split.CommittedLines, split.RenderedLines...)
	default:
		prefix := block.Content[:stableLen]
		tail := block.Content[stableLen:]
		prefixBody := renderStreamingMarkdownFresh(prefix, availWidth)
		split.CommittedLines = append(split.CommittedLines, decorateStreamTailLines(prefixBody, split.HasDot, false)...)

		tailBody := renderStreamingMarkdownFresh(tail, availWidth)
		split.LiveLines = append(split.LiveLines, decorateStreamTailLines(tailBody, split.HasDot, stableLen > 0)...)
	}
	split.Rendered = strings.Join(split.RenderedLines, "\n") + "\n"
	return split
}

func renderStreamTailBlockV2(block *BlockVM, ctx BlockRenderContext) string {
	return RenderStreamTailBlockSplitV2(block, ctx).Rendered
}

func renderErrorBlockV2(block *BlockVM, _ BlockRenderContext) string {
	// CC alignment: errors use ⎿ connector prefix
	return renderMessageResponse(errorStyle.Render(block.Content)) + "\n"
}

// renderToolHeaderBlockV2 renders tool headers with unified Unicode figures.
// For finished tools, the result summary line uses the format "(N lines, X.Xs)" when
// line count information is available.
func renderToolHeaderBlockV2(block *BlockVM, ctx BlockRenderContext) string {
	meta := block.Meta
	toolSummary := getToolDisplayName(meta.ToolName)
	paramsSummary := extractToolParamsSummary(meta.ToolName, meta.ToolInput)
	toolFinished := meta.ToolFinished || message.IsTerminalToolState(meta.ToolState)
	detail := FormatToolDetail(meta.ToolName, meta.ToolInput, "", string(meta.ToolState))

	var sb strings.Builder

	if toolFinished {
		focusPrefix := ""
		if meta.IsFocused {
			focusPrefix = FigCollapse + " "
		}

		// CC alignment: V2 finished tools show green dot (or red on error) + bold name + params
		dotColor := Theme.Success
		if meta.IsError {
			dotColor = Theme.Danger
		}
		greenDot := lipgloss.NewStyle().Foreground(dotColor).Render(FigBlackCircle)
		toolNameRendered := toolNameStyleV2.Render(toolSummary)
		overhead := 2 + 2 + lipgloss.Width(toolSummary) + 3
		maxParamsWidth := ctx.Width - overhead
		if maxParamsWidth < 10 {
			maxParamsWidth = 10
		}
		truncatedParams := truncateToWidth(paramsSummary, maxParamsWidth, "…")
		// CC alignment: params with parentheses (CC's AssistantToolUseMessage wraps in parens)
		headerLine := "  " + focusPrefix + greenDot + " " + toolNameRendered + " " +
			toolParamStyle.Render("("+truncatedParams+")")
		sb.WriteString(headerLine + "\n")
		// Result summary is now rendered by renderToolPreviewBlockV2 —
		// header only shows the tool name + params line.
	} else if meta.ToolState == message.ToolCallQueued {
		waitIcon := lipgloss.NewStyle().Foreground(Theme.TextMuted).Render(FigBlackCircle)
		toolNameRendered := toolNameStyleV2.Render(toolSummary)
		overheadQueued := 2 + 2 + lipgloss.Width(toolSummary) + 2
		maxParamsQueued := ctx.Width - overheadQueued
		if maxParamsQueued < 10 {
			maxParamsQueued = 10
		}
		paramsSummaryQueued := truncateToWidth(paramsSummary, maxParamsQueued, "…")
		sb.WriteString("  " + waitIcon + " " + toolNameRendered + " " +
			toolParamStyle.Render("("+paramsSummaryQueued+")") + "\n")
		if detail != "" {
			sb.WriteString(renderMessageResponse(toolResultMutedStyle.Render(detail)) + "\n")
		}
	} else {
		// CC alignment: blinking ⏺ dot for running tools (~1s period = 5 frames × 100ms × 2)
		isBlinkOn := (ctx.SpinnerFrame/5)%2 == 0
		var dot string
		if isBlinkOn {
			dimDotStyle := lipgloss.NewStyle().Foreground(Theme.Info)
			dot = dimDotStyle.Render(FigBlackCircle)
		} else {
			dot = " "
		}

		toolNameRendered := toolNameStyleV2.Render(toolSummary)
		overheadRunning := 2 + 2 + lipgloss.Width(toolSummary) + 2
		maxParamsRunning := ctx.Width - overheadRunning
		if maxParamsRunning < 10 {
			maxParamsRunning = 10
		}
		paramsSummaryRunning := truncateToWidth(paramsSummary, maxParamsRunning, "…")
		sb.WriteString("  " + dot + " " + toolNameRendered + " " +
			toolParamStyle.Render("("+paramsSummaryRunning+")") + "\n")
		if detail != "" {
			sb.WriteString(renderMessageResponse(toolResultMutedStyle.Render(detail)) + "\n")
		}
	}

	return sb.String()
}

// renderThinkingBlockV2 renders thinking blocks with three-state expand using FigThinking.
// ExpandCollapsed: single line summary with hidden line count hint.
// ExpandInline:    label + indented content lines.
// ExpandOverlay:   falls through to ExpandInline (overlay handled externally).
func renderThinkingBlockV2(block *BlockVM, ctx BlockRenderContext) string {
	content := block.Content
	if content == "" {
		return ""
	}

	// Determine expand state using V2 three-state (fallback to legacy bool).
	expandState := ExpandCollapsed
	if block.Meta.SemanticKey != "" {
		expandState = ctx.IsExpandedV2(block.Meta.SemanticKey)
	} else if block.Meta.IsExpanded {
		expandState = ExpandInline
	}

	// Suppress consecutive thinking blocks when collapsed to reduce visual noise.
	if block.Meta.IsConsecutiveThinking && expandState == ExpandCollapsed {
		return ""
	}

	// CC alignment: hide thinking after message completion (non-transcript mode).
	// During streaming, show collapsed hint so user knows AI is thinking.
	// After completion, thinking disappears — ctrl+o (transcript) re-shows it.
	if block.Meta.IsMessageFinished && expandState == ExpandCollapsed {
		return ""
	}

	focusPrefix := ""
	if block.Meta.IsFocused {
		focusPrefix = FigExpand + " "
	}

	var sb strings.Builder

	// CC alignment: dimColor + italic for thinking labels
	dimItalicStyle := lipgloss.NewStyle().Faint(true).Italic(true)
	dimContentStyle := lipgloss.NewStyle().Faint(true)

	switch expandState {
	case ExpandInline, ExpandOverlay:
		// CC alignment: "∴ Thinking…" dimColor italic, content rendered as markdown with dimColor
		sb.WriteString("  " + focusPrefix + dimItalicStyle.Render(FigThinking+" Thinking…") + "\n")
		mdRendered := RenderMarkdown(content, ctx.Width-6)
		for _, line := range strings.Split(strings.TrimRight(mdRendered, "\n"), "\n") {
			sb.WriteString("      " + dimContentStyle.Render(line) + "\n")
		}
	default: // ExpandCollapsed
		// CC alignment: "∴ Thinking" + "(ctrl+o to expand)" — no line count
		hint := "(ctrl+o to expand)"
		sb.WriteString("  " + focusPrefix + dimItalicStyle.Render(FigThinking+" Thinking") +
			"  " + connectorStyle.Render(hint) + "\n")
	}
	return sb.String()
}

// renderToolInlineExpandV2 renders a tool call in ExpandInline state.
// Shows the tool header with a collapse marker, followed by indented input params and
// up to 10 lines of the tool result with a truncation notice when the result is large.
//
//	▼ Read  internal/tui/view.go  (247 lines)
//	│ file_path: "internal/tui/view.go"
//	│ offset: 0, limit: 100
//	├─ Result (247 lines)
//	│   1│ package tui
//	│  ...
//	└─
func renderToolInlineExpandV2(block *BlockVM, ctx BlockRenderContext) string {
	meta := block.Meta

	// Header line: ▼ ToolName  params  (N lines)
	toolSummary := getToolDisplayName(meta.ToolName)
	paramsSummary := extractToolParamsSummary(meta.ToolName, meta.ToolInput)

	var sb strings.Builder

	dot := toolDoneStyle.Render(FigCollapse)
	toolNameRendered := toolNameStyle.Render(toolSummary)
	overhead := 2 + lipgloss.Width(FigCollapse+" ") + lipgloss.Width(toolSummary) + 2
	maxParamsWidth := ctx.Width - overhead
	if maxParamsWidth < 10 {
		maxParamsWidth = 10
	}
	truncatedParams := truncateToWidth(paramsSummary, maxParamsWidth, "…")

	// Build result summary for header (line count).
	resultLineSummary := ""
	var resultContent string
	if ctx.ToolMessages != nil {
		if result, ok := toolResultForCall(meta.ToolCallID, ctx.ToolMessages); ok {
			resultContent = result.Content
		}
	}
	if resultContent != "" {
		lc := strings.Count(resultContent, "\n") + 1
		resultLineSummary = fmt.Sprintf("(%d lines)", lc)
	}

	headerParts := "  " + dot + " " + toolNameRendered + "  " + toolParamStyle.Render(truncatedParams)
	if resultLineSummary != "" {
		headerParts += "  " + toolResultMutedStyle.Render(resultLineSummary)
	}
	sb.WriteString(headerParts + "\n")

	// Params section: parse JSON input and emit key: value pairs.
	if meta.ToolInput != "" {
		var params map[string]any
		if err := json.Unmarshal([]byte(meta.ToolInput), &params); err == nil && len(params) > 0 {
			// Emit params in insertion-stable order (map iteration is random; sort for determinism).
			keys := make([]string, 0, len(params))
			for k := range params {
				keys = append(keys, k)
			}
			// Simple sort: shorter keys first, then alphabetic.
			for i := 0; i < len(keys)-1; i++ {
				for j := i + 1; j < len(keys); j++ {
					if keys[i] > keys[j] {
						keys[i], keys[j] = keys[j], keys[i]
					}
				}
			}
			for _, k := range keys {
				v := params[k]
				valStr := ""
				switch tv := v.(type) {
				case string:
					// Quote strings
					valStr = fmt.Sprintf("%q", tv)
				case float64:
					// Integers display without decimal
					if tv == float64(int64(tv)) {
						valStr = fmt.Sprintf("%d", int64(tv))
					} else {
						valStr = fmt.Sprintf("%g", tv)
					}
				default:
					valStr = fmt.Sprintf("%v", v)
				}
				line := toolResultMutedStyle.Render(k+": ") + toolParamStyle.Render(valStr)
				sb.WriteString("  " + connectorStyle.Render(FigTreeVert) + " " + line + "\n")
			}
		}
	}

	// Result section: up to 10 lines with line numbers.
	if resultContent != "" {
		resultLines := strings.Split(resultContent, "\n")
		totalLines := len(resultLines)
		const maxInlineLines = 10
		const fullViewThreshold = 200

		// Result header row.
		resultHeader := fmt.Sprintf("Result (%d lines)", totalLines)
		sb.WriteString("  " + connectorStyle.Render(FigTreeMid) + " " +
			toolResultMutedStyle.Render(resultHeader) + "\n")

		displayLines := resultLines
		truncated := false
		if totalLines > maxInlineLines {
			displayLines = resultLines[:maxInlineLines]
			truncated = true
		}

		// Determine line number width for padding.
		lastNum := totalLines
		if truncated {
			lastNum = maxInlineLines
		}
		numWidth := len(fmt.Sprintf("%d", lastNum))
		if numWidth < 1 {
			numWidth = 1
		}

		for i, line := range displayLines {
			lineNum := fmt.Sprintf("%*d", numWidth, i+1)
			numPart := toolResultMutedStyle.Render(lineNum + "│ ")
			sb.WriteString("  " + connectorStyle.Render(FigTreeVert) + "  " + numPart + thinkingStyle.Render(line) + "\n")
		}

		if truncated {
			remaining := totalLines - maxInlineLines
			moreMsg := fmt.Sprintf("… (%d more lines)", remaining)
			if totalLines > fullViewThreshold {
				moreMsg += " — ctrl+o to collapse"
			}
			sb.WriteString("  " + connectorStyle.Render(FigTreeVert) + "  " +
				toolResultMutedStyle.Render(moreMsg) + "\n")
		}
	}

	// Closing tree end.
	sb.WriteString("  " + connectorStyle.Render(FigTreeEnd) + "\n")

	return sb.String()
}

// getToolResultText extracts the full result text from a tool message.
func getToolResultText(meta BlockMeta, toolMessages map[string]message.Message) string {
	if result, ok := toolResultForCall(meta.ToolCallID, toolMessages); ok {
		return result.Content
	}
	return ""
}

// renderToolPreviewBlockV2 renders the collapsed preview of a tool result in V2 style.
// Single line result:  ⎿ result text
// Multi-line result:   ⎿ first line
// Truncated:           ⎿ … +N lines (ctrl+o to expand)
func renderToolPreviewBlockV2(block *BlockVM, ctx BlockRenderContext) string {
	meta := block.Meta

	// Edit tool with structured diff content uses the existing diff renderer.
	if meta.ToolName == "Edit" && block.Content != "" {
		lang := langFromToolInput(meta.ToolInput)
		return RenderStructuredDiffLang(block.Content, lang, ctx.Width, true)
	}

	resultText := getToolResultText(meta, ctx.ToolMessages)
	if resultText == "" {
		return ""
	}

	// Normalize trailing newlines — tool output like "hello\n" is a single
	// logical line, not two. Without this, split produces a phantom empty line.
	resultText = strings.TrimRight(resultText, "\n")
	lines := strings.Split(resultText, "\n")
	totalLines := len(lines)

	// CC alignment: show multiple preview lines (3 default, 5 for Bash)
	maxPreview := 3
	if meta.ToolName == "Bash" {
		maxPreview = 5
	}

	var sb strings.Builder
	showCount := totalLines
	if showCount > maxPreview {
		showCount = maxPreview
	}
	for i := 0; i < showCount; i++ {
		truncated := truncateToWidth(lines[i], ctx.Width-8, "…")
		sb.WriteString(renderMessageResponse(toolResultMutedStyle.Render(truncated)) + "\n")
	}
	if totalLines > maxPreview {
		remaining := totalLines - maxPreview
		hint := fmt.Sprintf("… +%d lines (ctrl+o to expand)", remaining)
		sb.WriteString(renderMessageResponse(toolResultMutedStyle.Render(hint)) + "\n")
	}
	return sb.String()
}

// HeightCache caches measured block heights keyed by block ID.
// generation is bumped on InvalidateAll so callers can detect stale entries.
type HeightCache struct {
	heights            map[string]int
	generation         int
	resizeFreezeFrames int
}

// NewHeightCache returns an initialized HeightCache.
func NewHeightCache() *HeightCache {
	return &HeightCache{heights: make(map[string]int)}
}

// Get returns the cached height for blockID and whether it was found.
func (hc *HeightCache) Get(blockID string) (int, bool) {
	v, ok := hc.heights[blockID]
	return v, ok
}

// GetOrDefault returns the cached height for blockID, or def if not cached.
func (hc *HeightCache) GetOrDefault(blockID string, def int) int {
	if v, ok := hc.heights[blockID]; ok {
		return v
	}
	return def
}

// Set stores the height for blockID.
func (hc *HeightCache) Set(blockID string, height int) {
	hc.heights[blockID] = height
}

// MeasureRenderedHeight returns terminal row count for an already-rendered
// block. Renderers are expected to have applied wrapping before this point, so
// height is line count after ignoring trailing newlines from row composers.
func MeasureRenderedHeight(rendered string) int {
	rendered = strings.TrimRight(rendered, "\n")
	if rendered == "" {
		return 0
	}
	return strings.Count(rendered, "\n") + 1
}

// IsDisplayHidden reports whether a block has been measured or pre-marked as
// occupying no transcript rows.
func IsDisplayHidden(block BlockVM) bool {
	return block.WidthKey > 0 && block.Height == 0 && block.Rendered == ""
}

// MarkDisplayHidden pre-marks a block that is known to render empty in the
// current transcript expand state. This lets scroll math skip off-screen hidden
// blocks before the renderer has visited them.
func MarkDisplayHidden(block *BlockVM, width int) {
	block.Rendered = ""
	block.WidthKey = width
	block.Height = 0
	block.Dirty = false
}

// DisplayHeight returns the rendered block height and whether the block
// contributes any transcript rows. Message-boundary rows are not included here.
func DisplayHeight(hc *HeightCache, block BlockVM, defaultH int) (int, bool) {
	if IsDisplayHidden(block) {
		return 0, false
	}
	if block.Height > 0 {
		return block.Height, true
	}
	if hc != nil {
		if h, ok := hc.Get(block.ID); ok {
			if h <= 0 {
				return 0, false
			}
			return h, true
		}
	}
	if defaultH <= 0 {
		return 0, false
	}
	return defaultH, true
}

// ScaleForWidthChange applies a coarse height scaling fallback when terminal
// width changes. This keeps anchor/virtual-scroll math stable during rapid
// resize drags until exact heights are re-measured by subsequent renders.
func (hc *HeightCache) ScaleForWidthChange(oldWidth, newWidth int) {
	if oldWidth <= 0 || newWidth <= 0 || oldWidth == newWidth {
		return
	}
	for id, h := range hc.heights {
		if h <= 1 {
			hc.heights[id] = 1
			continue
		}
		scaled := (h*oldWidth + (newWidth / 2)) / newWidth
		if scaled < 1 {
			scaled = 1
		}
		hc.heights[id] = scaled
	}
	if hc.resizeFreezeFrames < 2 {
		hc.resizeFreezeFrames = 2
	}
}

// ResizeFreezeFrames reports how many virtual render passes should still prefer
// scaled height estimates over an immediate second-pass correction.
func (hc *HeightCache) ResizeFreezeFrames() int {
	return hc.resizeFreezeFrames
}

// ConsumeResizeFreeze consumes one resize-freeze render pass.
func (hc *HeightCache) ConsumeResizeFreeze() bool {
	if hc.resizeFreezeFrames <= 0 {
		return false
	}
	hc.resizeFreezeFrames--
	return true
}

// InvalidateAll clears all cached heights and bumps the generation counter.
func (hc *HeightCache) InvalidateAll() {
	hc.heights = make(map[string]int)
	hc.resizeFreezeFrames = 0
	hc.generation++
}

// Generation returns the current generation counter, which is incremented on every InvalidateAll call.
func (hc *HeightCache) Generation() int {
	return hc.generation
}

// TotalHeight returns the sum of heights for all blocks in the slice.
// Blocks without a cached height use defaultH.
func (hc *HeightCache) TotalHeight(blocks []BlockVM, defaultH int) int {
	total := 0
	for i := range blocks {
		if h, ok := DisplayHeight(hc, blocks[i], defaultH); ok {
			total += h
		}
	}
	return total
}

// HeightBefore returns the sum of heights for all blocks before index idx.
// Blocks without a cached height use defaultH.
func (hc *HeightCache) HeightBefore(blocks []BlockVM, idx int, defaultH int) int {
	if idx <= 0 || len(blocks) == 0 {
		return 0
	}
	if idx > len(blocks) {
		idx = len(blocks)
	}
	total := 0
	for i := 0; i < idx; i++ {
		if h, ok := DisplayHeight(hc, blocks[i], defaultH); ok {
			total += h
		}
	}
	return total
}
