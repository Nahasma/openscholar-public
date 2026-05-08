package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/config"
	initwizard "github.com/openscholar/openscholar/internal/init"
	llmcontext "github.com/openscholar/openscholar/internal/llm/context"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/tui/components"
	"github.com/openscholar/openscholar/internal/version"
)

type screenVM struct {
	Header     string
	Transcript string
	Bottom     string
	Status     string
}

type promptAreaVM struct {
	Top      []string
	Input    string
	Footer   string
	Picker   string
	Override string
}

type toolBatchVM struct {
	BatchID   string
	ToolName  string
	Total     int
	Finished  int
	Queued    int
	Running   int
	AllDone   bool
	HasError  bool
	MinOrder  int
	Detail    string
	LastState message.ToolCallState
}

func (m Model) View() (rendered string) {
	defer func() {
		m.logFrame("view", rendered)
	}()

	if m.width == 0 {
		return "Loading..."
	}

	screen := m.buildScreenVM()
	if m.isFullscreenMode() {
		return m.buildFullscreenFrame(screen)
	}
	var parts []string
	for _, part := range []string{screen.Header, screen.Transcript, screen.Bottom} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if screen.Status != "" {
		parts = append(parts, screen.Status)
	}
	if len(parts) == 0 {
		return ""
	}
	frame := lipgloss.JoinVertical(lipgloss.Left, parts...)
	frame = sanitizeMainScreenFrame(frame, m.width)
	if m.mainOutput != nil {
		return m.mainOutput.StageFrame(MainScreenFrameInput{
			Frame:          frame,
			Width:          m.width,
			Height:         m.height,
			ViewportHeight: m.height,
			ResizeEpoch:    m.resizeEpoch,
			SliceAnchor:    m.mainScreenFrameSliceAnchor(),
			Reason:         (&m).consumePendingMainScreenResetReason(),
		})
	}
	if reset := (&m).maybeConsumeMainScreenVisibleReset(); reset != "" {
		return reset + frame
	}
	return frame
}

func (m Model) buildScreenVM() screenVM {
	if !m.isFullscreenMode() {
		header := ""
		if m.shouldRenderFixedHeader() {
			header = strings.TrimRight(m.renderHeaderBar(), "\n")
		}
		return screenVM{
			Header:     header,
			Transcript: m.mainScreenTranscript(),
			Bottom:     m.renderBottomSection(),
			Status:     m.renderStandaloneStatusBar(),
		}
	}

	snapshot := (&m).currentLayoutSnapshot()
	return screenVM{
		Header:     fitRenderedHeight(strings.TrimRight(m.renderHeaderBar(), "\n"), snapshot.HeaderHeight, snapshot.Width),
		Transcript: fitRenderedHeight(m.chat.viewport.View(), snapshot.TranscriptHeight, snapshot.Width),
		Bottom:     fitRenderedHeight(m.renderBottomSection(), snapshot.BottomHeight, snapshot.Width),
		Status:     fitRenderedHeight(m.renderStandaloneStatusBar(), snapshot.StatusHeight, snapshot.Width),
	}
}

func (m Model) shouldRenderFixedHeader() bool {
	return m.isFullscreenMode()
}

func (m Model) renderMainScreenIntro() string {
	if m.isFullscreenMode() || m.chat.mainScreenIntroFlushed {
		return ""
	}
	if m.chat.mainScreenIntroCached && m.chat.mainScreenIntroWidth == m.width {
		return m.chat.mainScreenIntroCache
	}
	return strings.TrimRight(m.renderHeaderBar(), "\n")
}

func (m Model) prependMainScreenIntro(content string) string {
	return joinMainScreenTranscriptParts(m.renderMainScreenIntro(), content)
}

func (m *Model) renderFrozenMainScreenIntro() string {
	if m == nil || m.isFullscreenMode() || m.chat.mainScreenIntroFlushed {
		return ""
	}
	if m.chat.mainScreenIntroCached && m.chat.mainScreenIntroWidth == m.width {
		return m.chat.mainScreenIntroCache
	}
	intro := strings.TrimRight(m.renderHeaderBar(), "\n")
	m.chat.mainScreenIntroCache = intro
	m.chat.mainScreenIntroWidth = m.width
	m.chat.mainScreenIntroCached = true
	return intro
}

func (m *Model) prependFrozenMainScreenIntro(content string) string {
	return joinMainScreenTranscriptParts(m.renderFrozenMainScreenIntro(), content)
}

func joinMainScreenTranscriptParts(parts ...string) string {
	joined := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "\n")
		if strings.TrimSpace(part) == "" {
			continue
		}
		joined = append(joined, part)
	}
	return strings.Join(joined, "\n")
}

func (m Model) shouldRenderMainScreenViewportSlice() bool {
	if m.isFullscreenMode() {
		return true
	}
	if m.chat.scrollMode != ScrollManualLocked {
		return false
	}
	return m.mainScreenTranscriptLineCount() > m.chat.viewport.Height
}

func (m Model) mainScreenFrameSliceAnchor() string {
	if frame := m.chat.mainScreenFrame; frame != nil && frame.SliceAnchor != "" {
		return frame.SliceAnchor
	}
	return "-"
}

func (m Model) mainScreenTranscript() string {
	if m.shouldRenderMainScreenViewportSlice() {
		return m.mainScreenViewportSlice()
	}
	return m.mainScreenTranscriptContent()
}

func (m Model) mainScreenTranscriptContent() string {
	transcript := strings.TrimRight(m.chat.renderedContent, "\n")
	if transcript != "" {
		return transcript
	}
	if len(m.chat.contentLines) > 0 {
		return strings.TrimRight(strings.Join(m.chat.contentLines, "\n"), "\n")
	}
	return m.prependMainScreenIntro(strings.TrimRight(m.renderAllMessages(), "\n"))
}

func (m Model) mainScreenTranscriptLineCount() int {
	if len(m.chat.contentLines) > 0 {
		return len(m.chat.contentLines)
	}
	content := m.mainScreenTranscriptContent()
	if content == "" {
		return 0
	}
	return len(strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n"))
}

func (m Model) mainScreenViewportSlice() string {
	lines := m.chat.contentLines
	if len(lines) == 0 {
		content := m.mainScreenTranscriptContent()
		if content == "" {
			return ""
		}
		lines = strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	}
	if len(lines) == 0 {
		return ""
	}

	start := min(max(m.chat.viewport.YOffset, 0), len(lines))
	end := len(lines)
	if m.chat.viewport.Height > 0 {
		end = min(start+m.chat.viewport.Height, len(lines))
	}
	if start >= end {
		return ""
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n")
}

func (m Model) renderStatusBar() string {
	if !m.shouldRenderStatusBar() {
		return ""
	}
	metrics := m.collectStatusMetrics()
	var vimMode string
	if m.features.VimMode && m.vim.Enabled() {
		if m.vim.IsNormalMode() {
			vimMode = "normal"
		} else {
			vimMode = "insert"
		}
	}
	processingLabel := m.status.processing.Label
	if processingLabel == "" {
		processingLabel = m.status.processing.ActiveTool
	}
	processingVerb := m.status.processingVerb
	if processingVerb == "" || processingVerb == "processing" {
		processingVerb = processingVerbForPhase(m.status.processing.Phase)
	}
	return components.RenderFooterDock(components.FooterDockParams{
		Width:              m.width,
		Mode:               m.status.mode,
		ModelName:          m.currentModelName(),
		Workspace:          config.WorkingDirectory(),
		Metrics:            metrics,
		IsProcessing:       m.status.isProcessing,
		CanInterrupt:       m.canInterruptCurrentTurn(),
		CtrlCPending:       m.status.ctrlCPending,
		HasInput:           strings.TrimSpace(m.composer.input.Value()) != "",
		HasBlockingOverlay: m.hasBlockingOverlay(),
		HasInputOwner:      m.hasPromptInputOwner(),
		HasEscapeOwner:     m.hasEscapeOwner(),
		Notice:             m.status.notice,
		ProcessingPhase:    uint8(m.status.processing.Phase),
		ProcessingVerb:     processingVerb,
		ProcessingLabel:    processingLabel,
		ProcessingElapsed:  m.status.processingElapsed,
		SpinnerFrame:       m.status.spinnerFrame,
		BgTaskCount:        m.status.bgTaskCount,
		MemoryCount:        m.status.memoryCount,
		ResearchProgress:   m.statusProgressLabel(),
		OverlayName:        m.activeOverlayName(),
		VimMode:            vimMode,
	})
}

func processingVerbForPhase(phase ProcessingPhase) string {
	switch phase {
	case PhaseThinking:
		return "Thinking"
	case PhaseToolQueued:
		return "Queued tools"
	case PhaseToolRunning:
		return "Running tools"
	case PhaseStreaming:
		return "Streaming"
	case PhaseCompacting:
		return "Compacting context"
	default:
		return "processing"
	}
}

func (m Model) renderBottomSection() string {
	if dialogOverlay := m.buildDialogOverlay(); dialogOverlay != "" {
		return dialogOverlay
	}

	area := m.buildPromptAreaVM()
	if area.Override != "" {
		return area.Override
	}

	var parts []string
	parts = append(parts, area.Top...)
	if progress := m.renderProgressRail(); progress != "" {
		parts = append(parts, strings.Trim(progress, "\n"))
	}
	parts = append(parts, area.Input)
	if area.Picker != "" {
		parts = append(parts, area.Picker)
	}
	if area.Footer != "" {
		parts = append(parts, area.Footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderStandaloneStatusBar() string {
	return m.renderStatusBar()
}

func (m Model) buildFullscreenFrame(screen screenVM) string {
	snapshot := (&m).currentLayoutSnapshot()
	if snapshot.Height <= 0 {
		return ""
	}
	rows := make([]string, 0, snapshot.Height)
	for _, section := range []struct {
		text   string
		height int
	}{
		{text: screen.Header, height: snapshot.HeaderHeight},
		{text: screen.Transcript, height: snapshot.TranscriptHeight},
		{text: screen.Bottom, height: snapshot.BottomHeight},
		{text: screen.Status, height: snapshot.StatusHeight},
	} {
		if section.height <= 0 {
			continue
		}
		rows = append(rows, strings.Split(fitRenderedHeight(section.text, section.height, snapshot.Width), "\n")...)
	}
	if len(rows) > snapshot.Height {
		rows = rows[:snapshot.Height]
	}
	for len(rows) < snapshot.Height {
		rows = append(rows, components.SafeBlankLine())
	}
	frame := strings.Join(rows, "\n")
	(&m).recordFullscreenFramePainted(snapshot.Width, snapshot.Height, snapshot.Epoch)
	return frame
}

// renderAllMessages renders all messages for the viewport content.
func (m Model) renderAllMessages() string {
	if len(m.chat.messages) == 0 && !m.status.isProcessing {
		return ""
	}

	if len(m.chat.messages) == 0 {
		return ""
	}

	content := m.renderViaBlocks()

	// Wave 3: Memory toast notification (above composer)
	if m.features.MemoryUI {
		if toast := m.status.memoryToast.Current(); toast != nil {
			content += components.RenderMemoryToast(*toast, m.width)
		}
	}

	return content
}

// renderViaBlocks renders all messages using the BlockVM pipeline.
// This is the new rendering path, gated behind the BlockRenderer feature flag.
// It incrementally updates the BlockList by per-message replacement, preserving
// cached Rendered/Height on unchanged blocks.
func (m Model) renderViaBlocks() string {
	m.syncBlockList()
	content := m.renderBlockSlice(m.chat.blockList.All())
	if len(m.chat.inlineErrors) == 0 {
		return content
	}
	return content + m.renderInlineErrors(m.blockRenderContext())
}

func (m Model) blockRenderContext() components.BlockRenderContext {
	return m.blockRenderContextWithWidth(m.width)
}

func (m Model) blockRenderContextWithWidth(width int) components.BlockRenderContext {
	renderWidth := components.TerminalSafeWidth(width)
	return components.BlockRenderContext{
		Width:             renderWidth,
		SpinnerFrame:      m.status.spinnerFrame,
		ToolMessages:      m.chat.toolMessages,
		RenderCache:       m.chat.renderCache,
		Expanded:          m.chat.expandedToolCalls,
		ExpandNodes:       m.chat.expandedNodes,
		ResolveExpand:     m.chat.GetExpandState,
		RenderFn:          components.RenderBlockV2,
		ProcessingPhase:   uint8(m.status.processing.Phase),
		ProcessingElapsed: m.status.processing.ElapsedSeconds(),
	}
}

func (m Model) renderBlockSlice(blocks []components.BlockVM) string {
	return m.renderBlockSliceWithWidthAndOverride(blocks, m.width, -1, "")
}

func renderedLineCount(rendered string) int {
	return components.MeasureRenderedHeight(rendered)
}

func (m Model) renderBlockSliceWithWidthAndOverride(blocks []components.BlockVM, width, overrideIdx int, override string) string {
	if len(blocks) == 0 {
		return ""
	}
	ctx := m.blockRenderContextWithWidth(width)

	var sb strings.Builder
	var prevVisible *components.BlockVM
	for i := range blocks {
		if components.IsDisplayHidden(blocks[i]) {
			continue
		}
		rendered := override
		if i != overrideIdx {
			rendered = ctx.Render(&blocks[i])
		}
		if rendered == "" {
			continue
		}
		if prevVisible != nil {
			if rows := components.BoundaryRowsBetween(*prevVisible, blocks[i]); rows > 0 {
				sb.WriteString(strings.Repeat("\n", rows))
			}
		}
		sb.WriteString(rendered)
		if width == m.width {
			// HeightCache stores pure block height. Inter-message separators are
			// accounted for by the display-line anchor helpers.
			effectiveHeight := blocks[i].Height
			if i == overrideIdx {
				effectiveHeight = renderedLineCount(rendered)
			}
			m.chat.heightCache.Set(blocks[i].ID, effectiveHeight)
		}
		prevVisible = &blocks[i]
	}

	return sb.String()
}

func (m Model) buildBlocksForMessages(messages []message.Message) []components.BlockVM {
	var blocks []components.BlockVM
	if m.features.ToolGroups {
		blocks = components.BuildBlocksGrouped(
			messages,
			m.chat.toolMessages,
			m.chat.expandedToolCalls,
			m.chat.focusedToolCallID,
			m.chat.GetExpandState,
		)
	} else {
		blocks = components.BuildBlocks(
			messages,
			m.chat.toolMessages,
			m.chat.expandedToolCalls,
			m.chat.focusedToolCallID,
		)
	}
	m.applyDisplayHiding(blocks)
	return blocks
}

func (m Model) renderMessagesSlice(messages []message.Message) string {
	if len(messages) == 0 {
		return ""
	}
	return m.renderBlockSlice(m.buildBlocksForMessages(messages))
}

func (m Model) renderInlineErrors(ctx components.BlockRenderContext) string {
	// Render inline errors as BlockInlineError blocks
	var sb strings.Builder
	for i, inlineErr := range m.chat.inlineErrors {
		errBlock := components.BlockVM{
			ID:      fmt.Sprintf("error-%d", i),
			Kind:    components.BlockInlineError,
			Content: inlineErr.Text,
			Dirty:   true,
		}
		rendered := ctx.Render(&errBlock)
		sb.WriteString(rendered)
	}
	return sb.String()
}

func resolveFlushedStart(messages []message.Message, anchor *messageSliceAnchor) int {
	if anchor == nil {
		return 0
	}
	if anchor.MessageID != "" {
		for i, msg := range messages {
			if msg.ID == anchor.MessageID {
				return min(i+1, len(messages))
			}
		}
	}
	return min(max(anchor.Idx+1, 0), len(messages))
}

func makeFlushedAnchor(messages []message.Message, frontier int) *messageSliceAnchor {
	if frontier <= 0 || frontier > len(messages) {
		return nil
	}
	idx := frontier - 1
	return &messageSliceAnchor{
		MessageID: messages[idx].ID,
		Idx:       idx,
	}
}

func (m *Model) scheduleMainScreenVisibleReset(reason string) {
	if reason == "" {
		return
	}
	frame := m.ensureMainScreenFrameState()
	frame.PendingReset = true
	frame.AppliedReset = false
	frame.Reason = reason
	frame.ResizeEpoch = m.resizeEpoch
	frame.Width = m.width
	frame.Height = m.height
}

func (m *Model) maybeConsumeMainScreenVisibleReset() string {
	if reason := m.consumePendingMainScreenResetReason(); reason != "" {
		return ClearTerminalSequence(m.mainResetMode)
	}
	return ""
}

func (m *Model) consumePendingMainScreenResetReason() string {
	if m.isFullscreenMode() {
		return ""
	}
	frame := m.chat.mainScreenFrame
	if frame == nil || !frame.PendingReset {
		return ""
	}
	if frame.ResizeEpoch != m.resizeEpoch || frame.Width != m.width || frame.Height != m.height {
		return ""
	}
	frame.PendingReset = false
	frame.AppliedReset = true
	return frame.Reason
}

func (m Model) mainScreenControllerMessages() ([]message.Message, string) {
	if len(m.chat.messages) == 0 {
		return nil, "-"
	}
	start := 0
	renderedOnce := m.mainOutput != nil && m.mainOutput.Stats().ConsumedFrameSeq > 0
	if renderedOnce && len(m.chat.messages) > nonFullscreenMessageCap+nonFullscreenMessageCapStep {
		start = max(0, len(m.chat.messages)-nonFullscreenMessageCap)
	}
	anchor := "-"
	if start > 0 && start < len(m.chat.messages) {
		anchor = m.chat.messages[start].ID
	}
	return m.chat.messages[start:], anchor
}

func resolveSliceAnchorStart(messages []message.Message, anchor *messageSliceAnchor, flushedBoundary bool) (int, *messageSliceAnchor) {
	if len(messages) == 0 || anchor == nil {
		return 0, nil
	}
	if anchor.MessageID != "" {
		for i, msg := range messages {
			if msg.ID == anchor.MessageID {
				if flushedBoundary {
					return min(i+1, len(messages)), anchor
				}
				return i, anchor
			}
		}
	}
	idx := min(max(anchor.Idx, 0), len(messages)-1)
	refreshed := &messageSliceAnchor{
		MessageID: messages[idx].ID,
		Idx:       idx,
	}
	if flushedBoundary {
		return min(idx+1, len(messages)), refreshed
	}
	return idx, refreshed
}

func computeMessageSliceStart(messages []message.Message, anchor *messageSliceAnchor, cap, step int) (int, *messageSliceAnchor) {
	if len(messages) == 0 {
		return 0, nil
	}
	start, _ := resolveSliceAnchorStart(messages, anchor, false)
	if len(messages)-start > cap+step {
		start = max(0, len(messages)-cap)
	}
	nextAnchor := &messageSliceAnchor{
		MessageID: messages[start].ID,
		Idx:       start,
	}
	return start, nextAnchor
}

func joinRenderedLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func renderOverrideLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return joinRenderedLines(lines) + "\n"
}

func findAssistantTextBlock(blocks []components.BlockVM, messageID string) int {
	for i := range blocks {
		if blocks[i].MsgID != messageID {
			continue
		}
		switch blocks[i].Kind {
		case components.BlockStreamTail, components.BlockAssistantMarkdown:
			return i
		}
	}
	return -1
}

func (m *Model) updateViewportContentNonFullscreen() tea.Cmd {
	m.syncBlockList()
	atBottom := m.chat.scrollMode == ScrollAutoFollow
	prevYOffset := m.chat.viewport.YOffset
	var savedAnchor *components.ScrollAnchor
	if !atBottom && m.chat.blockList.Len() > 0 {
		if m.chat.pendingAnchor != nil {
			savedAnchor = m.chat.pendingAnchor
			m.chat.pendingAnchor = nil
		} else {
			anchor := components.AnchorFromDisplayLine(m.chat.heightCache, m.chat.blockList.All(), m.chat.viewport.YOffset, 1)
			savedAnchor = &anchor
		}
	}
	content := ""
	switch {
	case len(m.chat.messages) > 0:
		m.chat.resumePinned = false
		m.chat.flushedAnchor = nil
		m.chat.liveTailAnchor = nil
		m.chat.activeStreamFlush = nil
		messages, sliceAnchor := m.mainScreenControllerMessages()
		frame := m.ensureMainScreenFrameState()
		frame.SliceAnchor = sliceAnchor
		content = m.prependFrozenMainScreenIntro(m.renderMessagesSlice(messages))
	case len(m.chat.messages) == 0 && !m.status.isProcessing:
		m.chat.liveTailAnchor = nil
		if frame := m.chat.mainScreenFrame; frame != nil {
			frame.SliceAnchor = "-"
		}
		content = m.renderFrozenMainScreenIntro()
	}
	if len(m.chat.inlineErrors) > 0 {
		content += m.renderInlineErrors(m.blockRenderContext())
	}

	if m.features.MemoryUI {
		if toast := m.status.memoryToast.Current(); toast != nil {
			content += components.RenderMemoryToast(*toast, m.width)
		}
	}

	m.chat.contentLines = strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if info := m.buildSearchHighlightInfo(); info.Query != "" {
		m.chat.contentLines = applySearchHighlight(m.chat.contentLines, info)
		content = strings.Join(m.chat.contentLines, "\n")
	}
	if m.chat.selection.Active || m.chat.selection.HasRange {
		m.syncSelectionRowsFromSemanticCoords()
		highlighted := applySelectionHighlight(m.chat.contentLines, m.chat.selection)
		content = strings.Join(highlighted, "\n")
	}

	m.chat.renderedContent = content
	m.chat.viewport.SetContent(content)
	if atBottom {
		m.chat.viewport.GotoBottom()
	} else if savedAnchor != nil && m.chat.blockList.Len() > 0 {
		resolved := savedAnchor.ResolveDisplayLine(m.chat.heightCache, m.chat.blockList.All(), 1)
		m.chat.viewport.SetYOffset(resolved)
	} else {
		maxYOffset := max(0, m.chat.viewport.TotalLineCount()-m.chat.viewport.Height)
		if prevYOffset > maxYOffset {
			prevYOffset = maxYOffset
		}
		m.chat.viewport.SetYOffset(prevYOffset)
	}
	return nil
}

// updateViewportContent re-renders messages and updates the viewport.
func (m *Model) updateViewportContent() tea.Cmd {
	// Virtual scroll path: delegate to VirtualMessageList
	if m.usesVirtualTranscript() {
		m.updateViewportVirtual()
		return nil
	}

	if !m.isFullscreenMode() {
		return m.updateViewportContentNonFullscreen()
	}

	usePendingAnchor := m.chat.pendingAnchor != nil
	atBottom := m.chat.scrollMode == ScrollAutoFollow

	// Save scroll anchor BEFORE re-rendering (from old block layout)
	var savedAnchor *components.ScrollAnchor
	if m.chat.blockList.Len() > 0 {
		if usePendingAnchor {
			savedAnchor = m.chat.pendingAnchor
			m.chat.pendingAnchor = nil
		} else if !atBottom {
			anchor := components.AnchorFromDisplayLine(m.chat.heightCache, m.chat.blockList.All(), m.chat.viewport.YOffset, 1)
			savedAnchor = &anchor
		}
	}

	content := m.renderAllMessages()

	// Cache content lines for selection coordinate mapping
	m.chat.contentLines = strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	// Apply search highlight if active
	if info := m.buildSearchHighlightInfo(); info.Query != "" {
		m.chat.contentLines = applySearchHighlight(m.chat.contentLines, info)
		content = strings.Join(m.chat.contentLines, "\n")
	}

	// Apply selection highlight if active
	if m.chat.selection.Active || m.chat.selection.HasRange {
		m.syncSelectionRowsFromSemanticCoords()
		highlighted := applySelectionHighlight(m.chat.contentLines, m.chat.selection)
		content = strings.Join(highlighted, "\n")
	}

	m.chat.renderedContent = content
	m.chat.viewport.SetContent(content)

	if atBottom {
		m.chat.viewport.GotoBottom()
	} else if savedAnchor != nil && m.chat.blockList.Len() > 0 {
		// Restore scroll position using new block layout
		resolved := savedAnchor.ResolveDisplayLine(m.chat.heightCache, m.chat.blockList.All(), 1)
		m.chat.viewport.SetYOffset(resolved)
	}
	return nil
}

// updateViewportVirtual renders using the VirtualMessageList, only rendering
// visible blocks with padding for off-screen content.
func (m *Model) updateViewportVirtual() {
	wasAutoFollow := m.chat.virtualList.AutoFollow()

	// Ensure block list is up to date (same logic as renderViaBlocks)
	m.syncBlockList()
	m.chat.virtualList.SetViewportHeight(m.chat.viewport.Height)

	// Sync viewport offset -> virtual anchor when manually locked so the visible
	// block window is rendered around the current viewport position.
	atBottom := m.chat.scrollMode == ScrollAutoFollow
	prevYOffset := m.chat.viewport.YOffset
	blocks := m.chat.blockList.All()
	if !atBottom && len(blocks) > 0 {
		var anchor components.ScrollAnchor
		if m.chat.pendingAnchor != nil {
			anchor = *m.chat.pendingAnchor
			m.chat.pendingAnchor = nil
		} else if wasAutoFollow {
			anchor = components.AnchorFromDisplayLine(m.chat.heightCache, blocks, prevYOffset, components.DefaultBlockHeight)
		} else {
			anchor = m.chat.virtualList.Anchor()
		}
		m.chat.virtualList.SetAnchor(anchor)
	}
	m.chat.virtualList.SetAutoFollow(atBottom)

	ctx := components.BlockRenderContext{
		Width:         components.TerminalSafeWidth(m.width),
		SpinnerFrame:  m.status.spinnerFrame,
		ToolMessages:  m.chat.toolMessages,
		RenderCache:   m.chat.renderCache,
		Expanded:      m.chat.expandedToolCalls,
		ExpandNodes:   m.chat.expandedNodes,
		ResolveExpand: m.chat.GetExpandState,
	}
	ctx.RenderFn = components.RenderBlockV2
	ctx.ProcessingPhase = uint8(m.status.processing.Phase)
	ctx.ProcessingElapsed = m.status.processing.ElapsedSeconds()

	frame := m.chat.virtualList.Frame(ctx)
	// The first pass measures visible block heights; the second pass rebuilds
	// the mounted frame against those measurements so offsets land on content.
	if m.chat.heightCache == nil || !m.chat.heightCache.ConsumeResizeFreeze() {
		frame = m.chat.virtualList.Frame(ctx)
	}
	content := frame.Content

	// Render inline errors (same as renderViaBlocks path)
	content += m.renderInlineErrors(ctx)

	// Wave 3: Memory toast notification (above composer) — virtual scroll path
	if m.features.MemoryUI {
		if toast := m.status.memoryToast.Current(); toast != nil {
			content += components.RenderMemoryToast(*toast, m.width)
		}
	}

	// Cache content lines for selection coordinate mapping
	m.chat.contentLines = strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	// Apply search highlight if active
	if info := m.buildSearchHighlightInfo(); info.Query != "" {
		m.chat.contentLines = applySearchHighlight(m.chat.contentLines, info)
		content = strings.Join(m.chat.contentLines, "\n")
	}

	// Apply selection highlight if active
	if m.chat.selection.Active || m.chat.selection.HasRange {
		m.syncSelectionRowsFromSemanticCoords()
		highlighted := applySelectionHighlight(m.chat.contentLines, m.chat.selection)
		content = strings.Join(highlighted, "\n")
	}

	totalContentHeight := components.MeasureRenderedHeight(content)
	m.chat.renderedContent = content
	m.chat.viewport.SetContent(content)
	if len(blocks) > 0 {
		yOffset := frame.ViewportYOffset
		if totalContentHeight > frame.ContentHeight &&
			(atBottom || yOffset+m.chat.viewport.Height >= frame.ContentHeight) {
			yOffset = max(0, totalContentHeight-m.chat.viewport.Height)
		}
		m.chat.viewport.SetYOffset(yOffset)
		return
	}
	maxYOffset := max(0, m.chat.viewport.TotalLineCount()-m.chat.viewport.Height)
	if prevYOffset > maxYOffset {
		prevYOffset = maxYOffset
	}
	m.chat.viewport.SetYOffset(max(0, prevYOffset))
}

// syncBlockList ensures the block list is up-to-date with current messages.
// Extracted from renderViaBlocks to share between full and virtual render paths.
func (m *Model) syncBlockList() {
	if m.chat.blockList == nil || m.chat.heightCache == nil || m.chat.virtualList == nil {
		m.resetTranscriptRenderState()
	}
	if len(m.chat.messages) == 0 {
		if m.chat.blockList.Len() > 0 {
			m.chat.blockList.RebuildAll(nil)
			m.chat.heightCache.InvalidateAll()
			m.chat.virtualList.SetAutoFollow(true)
			m.chat.virtualList.SetAnchor(components.ScrollAnchor{})
		}
		return
	}

	var blocks []components.BlockVM
	if m.features.ToolGroups {
		blocks = components.BuildBlocksGrouped(
			m.chat.messages,
			m.chat.toolMessages,
			m.chat.expandedToolCalls,
			m.chat.focusedToolCallID,
			m.chat.GetExpandState,
		)
	} else {
		blocks = components.BuildBlocks(
			m.chat.messages,
			m.chat.toolMessages,
			m.chat.expandedToolCalls,
			m.chat.focusedToolCallID,
		)
	}
	m.applyDisplayHiding(blocks)
	m.chat.blockList.RebuildAll(blocks)
}

func (m Model) applyDisplayHiding(blocks []components.BlockVM) {
	for i := range blocks {
		if m.shouldHideBlockForDisplay(blocks[i]) {
			components.MarkDisplayHidden(&blocks[i], max(m.width, 1))
		}
	}
}

func (m Model) shouldHideBlockForDisplay(block components.BlockVM) bool {
	if block.Kind != components.BlockThinking {
		return false
	}
	expandState := components.ExpandCollapsed
	if block.Meta.SemanticKey != "" {
		expandState = m.chat.GetExpandState(block.Meta.SemanticKey)
	} else if block.Meta.IsExpanded {
		expandState = components.ExpandInline
	}
	if expandState != components.ExpandCollapsed {
		return false
	}
	return block.Meta.IsConsecutiveThinking || block.Meta.IsMessageFinished
}

// buildBottomSection constructs the bottom section string (input + pickers/dialogs).
// This is extracted from View() to allow recalcLayout() to measure real heights.
func (m Model) buildBottomSection() string {
	area := m.buildPromptAreaVM()
	if area.Override != "" {
		return area.Override
	}

	var parts []string
	parts = append(parts, area.Top...)
	if progress := m.renderProgressRail(); progress != "" {
		parts = append(parts, strings.Trim(progress, "\n"))
	}
	parts = append(parts, area.Input)
	if area.Picker != "" {
		parts = append(parts, area.Picker)
	}
	if area.Footer != "" {
		parts = append(parts, area.Footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) buildPromptAreaVM() promptAreaVM {
	if m.search.historySearch.Visible() {
		return promptAreaVM{Override: m.search.historySearch.View()}
	}
	if m.search.textSearch.Visible() {
		return promptAreaVM{
			Top:   []string{m.search.textSearch.View()},
			Input: m.composer.input.View(),
		}
	}
	if m.composer.commandDialog.Visible {
		return promptAreaVM{
			Override: components.RenderCommandOutput(
				m.composer.commandDialog.Title,
				m.composer.commandDialog.Body,
				"press any key to dismiss",
				m.width,
			),
		}
	}
	if m.composer.commandOutput != "" {
		return promptAreaVM{Override: components.RenderCommandOutput("", m.composer.commandOutput, "press any key to dismiss", m.width)}
	}

	area := promptAreaVM{Input: m.composer.input.View()}
	if m.status.runtimeErr.Expanded && strings.TrimSpace(m.status.runtimeErr.Detail) != "" {
		area.Top = append(area.Top, components.RenderRuntimeErrorDetail(m.status.runtimeErr.Summary, m.status.runtimeErr.Detail, m.width))
	}

	if m.composer.commandPickerActive {
		pickerItems := make([]components.CommandItem, len(m.composer.filteredCompletions))
		for i, item := range m.composer.filteredCompletions {
			name := item.Value
			if item.Kind == "subcommand" {
				name = item.CommandName + " " + item.Subcommand
			}
			pickerItems[i] = components.CommandItem{
				Name:         name,
				Description:  item.Description,
				ArgumentHint: item.ArgumentHint,
				WhenToUse:    item.WhenToUse,
				Source:       item.Source,
			}
		}
		area.Picker = components.RenderCommandPicker(pickerItems, m.composer.commandPickerIdx, m.width)
	} else if m.composer.filePicker != nil && m.composer.filePicker.Active && len(m.composer.filePicker.VisibleItems()) > 0 {
		area.Picker = components.RenderFilePicker(m.composer.filePicker.VisibleItems(), m.composer.filePicker.VisibleSelectedIndex(), m.width)
	}

	area.Footer = components.RenderInputFooter(components.InputFooterParams{
		HasText:      strings.TrimSpace(m.composer.input.Value()) != "",
		IsProcessing: m.status.isProcessing,
		Notice:       components.TransientNotice{},
		Width:        m.width,
	})

	return area
}

func (m Model) currentModelName() string {
	if m.app == nil || m.app.CoderAgent == nil {
		return ""
	}
	return m.app.CoderAgent.Model().Name
}

func (m Model) shouldRenderStatusBar() bool {
	if m.status.ctrlCPending {
		return true
	}
	if m.activeOverlayName() != "" {
		return true
	}
	if m.features.VimMode && m.vim.Enabled() {
		return true
	}
	if components.ShouldRenderBottomProcessing(m.status.isProcessing, uint8(m.status.processing.Phase), m.width) {
		return true
	}
	if !m.features.StatusV2 {
		return false
	}
	return true
}

func lineCount(rendered string) int {
	if rendered == "" {
		return 0
	}
	return strings.Count(rendered, "\n") + 1
}

func clampRenderedHeight(rendered string, maxHeight int) string {
	if maxHeight <= 0 || rendered == "" {
		return ""
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) <= maxHeight {
		return rendered
	}
	return strings.Join(lines[:maxHeight], "\n")
}

func fitRenderedHeight(rendered string, height int, width int) string {
	if height <= 0 {
		return ""
	}
	rendered = strings.ReplaceAll(rendered, "\r\n", "\n")
	rendered = strings.TrimRight(rendered, "\n")
	var lines []string
	if rendered != "" {
		lines = strings.Split(rendered, "\n")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = components.SafeTruncateLineWithBudget(line, components.SafeLineBudget{TerminalWidth: width})
	}
	return strings.Join(lines, "\n")
}

func sanitizeMainScreenFrame(frame string, width int) string {
	if frame == "" {
		return ""
	}
	frame = strings.ReplaceAll(frame, "\r\n", "\n")
	lines := strings.Split(frame, "\n")
	if width <= 1 {
		for i := range lines {
			lines[i] = components.SafeBlankLine()
		}
		return strings.Join(lines, "\n")
	}
	budget := components.SafeLineBudget{TerminalWidth: width}
	for i, line := range lines {
		line = strings.ReplaceAll(line, "\t", "    ")
		lines[i] = components.SafeTruncateLineWithBudget(line, budget)
	}
	return strings.Join(lines, "\n")
}

func (m Model) headerLayoutHeight() int {
	if !m.shouldRenderFixedHeader() {
		return 0
	}
	return lineCount(strings.TrimRight(m.renderHeaderBar(), "\n"))
}

func (m Model) overlayAvailableHeight() int {
	available := m.height - m.headerLayoutHeight() - m.statusLayoutHeight()
	if available < 0 {
		return 0
	}
	return available
}

func (m Model) statusLayoutHeight() int {
	if !m.shouldRenderStatusBar() {
		return 0
	}
	return lineCount(strings.TrimRight(m.renderStandaloneStatusBar(), "\n"))
}

func (m Model) composerLayoutHeight() int {
	area := m.buildPromptAreaVM()
	if area.Override != "" {
		return lineCount(area.Override)
	}

	height := lineCount(area.Input)
	if progress := m.renderProgressRail(); progress != "" {
		height += lineCount(strings.Trim(progress, "\n"))
	}
	for _, top := range area.Top {
		height += lineCount(top)
	}

	if area.Picker != "" {
		height += lineCount(area.Picker)
	}
	if area.Footer != "" {
		height += lineCount(area.Footer)
	}
	if height < 1 {
		return 1
	}
	return height
}

func (m Model) overlayLayoutHeight() int {
	return lineCount(m.buildDialogOverlay())
}

func (m *Model) recalcLayoutAndViewport(invalidateWrapCache bool) tea.Cmd {
	if invalidateWrapCache && !m.isFullscreenMode() && m.chat.scrollMode != ScrollAutoFollow && m.chat.blockList.Len() > 0 {
		anchor := components.AnchorFromDisplayLine(m.chat.heightCache, m.chat.blockList.All(), m.chat.viewport.YOffset, 1)
		m.chat.pendingAnchor = &anchor
	}
	m.recalcLayout()
	if invalidateWrapCache && m.chat.heightCache != nil {
		m.chat.heightCache.InvalidateAll()
	}
	return m.updateViewportContent()
}

func (m *Model) recalcFullscreenLayoutAndViewport(prevWidth int) tea.Cmd {
	if m.chat.scrollMode != ScrollAutoFollow && m.chat.blockList.Len() > 0 {
		anchor := components.AnchorFromDisplayLine(
			m.chat.heightCache,
			m.chat.blockList.All(),
			m.chat.viewport.YOffset,
			components.DefaultBlockHeight,
		)
		if m.usesVirtualTranscript() {
			anchor = m.chat.virtualList.AnchorFromFrameYOffset(m.chat.viewport.YOffset)
		}
		m.chat.pendingAnchor = &anchor
	}
	if m.width != prevWidth && m.chat.heightCache != nil {
		m.chat.heightCache.ScaleForWidthChange(prevWidth, m.width)
	}
	m.recalcLayout()
	return m.updateViewportContent()
}

func (m Model) statusProgressLabel() string {
	if len(m.status.pipelineProgress.Phases) == 0 {
		return ""
	}
	current := m.status.pipelineProgress.CurrentPhase + 1
	total := len(m.status.pipelineProgress.Phases)
	label := strings.TrimSpace(m.status.pipelineProgress.CurrentLabel)
	if label == "" {
		return fmt.Sprintf("Phase %d/%d", current, total)
	}
	return fmt.Sprintf("Phase %d/%d %s", current, total, label)
}

func (m *Model) refreshSessionSummaryBoundary() {
	if m == nil || m.sessionID == "" || m.app == nil || m.app.Sessions == nil {
		return
	}
	if sess, err := m.app.Sessions.Get(m.ctx, m.sessionID); err == nil {
		m.sessionSummaryMessageID = sess.SummaryMessageID
	}
}

func (m Model) statusEstimateMessages() []message.Message {
	filtered := make([]message.Message, 0, len(m.chat.messages))
	for _, msg := range m.chat.messages {
		if msg.Role == message.System {
			if flag, ok := msg.Meta["ui_command_activity"].(bool); ok && flag {
				continue
			}
		}
		filtered = append(filtered, msg)
	}
	if m.sessionSummaryMessageID == "" {
		return filtered
	}
	for i, msg := range filtered {
		if msg.ID == m.sessionSummaryMessageID {
			return filtered[i:]
		}
	}
	return filtered
}

func (m Model) statusOutputLimit(model models.Model) int64 {
	if cfg := config.Get(); cfg != nil {
		if agentCfg, ok := cfg.Agents[config.AgentCoder]; ok && agentCfg.MaxTokens > 0 {
			return agentCfg.MaxTokens
		}
	}
	return model.DefaultMaxTokens
}

func (m Model) renderProgressRail() string {
	var sections []string
	if m.status.mode == "research" && len(m.status.pipelineProgress.Phases) > 0 {
		sections = append(sections, strings.Trim(components.RenderPipelineProgress(m.status.pipelineProgress, m.width), "\n"))
	}
	if runtime := m.renderRuntimeProgressRail(); runtime != "" {
		sections = append(sections, runtime)
	}
	return joinMainScreenTranscriptParts(sections...)
}

func (m Model) renderRuntimeProgressRail() string {
	if !m.status.isProcessing || m.status.processing.Phase == PhaseIdle {
		return ""
	}
	elapsed := m.status.processingElapsed
	if elapsed == 0 {
		elapsed = m.status.processing.ElapsedSeconds()
	}
	label := m.runtimeProgressLabel()
	rendered := components.RenderProgressLine(
		uint8(m.status.processing.Phase),
		m.status.spinnerFrame,
		elapsed,
		m.status.processing.ActiveTool,
		label,
		components.ProgressRailOpts{
			StalledPct: m.status.telemetry.StalledIntensity,
			TokenCount: m.status.telemetry.DisplayedTokenCount,
			Width:      m.width,
		},
	)
	return strings.Trim(rendered, "\n")
}

func (m Model) runtimeProgressLabel() string {
	verb := strings.TrimSpace(m.status.processingVerb)
	label := strings.TrimSpace(m.status.processing.Label)
	switch m.status.processing.Phase {
	case PhaseThinking, PhaseStreaming:
		if verb != "" && verb != "processing" {
			return verb
		}
		return label
	case PhaseToolQueued, PhaseToolRunning:
		if label != "" {
			return label
		}
		if verb != "" && verb != "processing" {
			return verb
		}
		return processingVerbForPhase(m.status.processing.Phase)
	default:
		return label
	}
}

func (m Model) renderQueuedCommandsLane() string {
	batches := m.collectToolBatches()
	if len(batches) == 0 {
		return ""
	}

	nameStyle := lipgloss.NewStyle().Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(components.Theme.TextMuted)
	infoStyle := lipgloss.NewStyle().Foreground(components.Theme.Info)
	successStyle := lipgloss.NewStyle().Foreground(components.Theme.Success)
	errorStyle := lipgloss.NewStyle().Foreground(components.Theme.Danger)
	queuedStyle := lipgloss.NewStyle().Foreground(components.Theme.TextMuted)

	var out []string
	for _, batch := range batches {
		presenter := components.GetPresenter(batch.ToolName)
		label := components.FormatToolBatchLabel(batch.ToolName, batch.Total, batch.Finished, batch.AllDone)
		dotStyle := queuedStyle
		switch {
		case batch.HasError:
			dotStyle = errorStyle
		case batch.AllDone:
			dotStyle = successStyle
		case batch.Running > 0:
			if (m.status.spinnerFrame/5)%2 == 1 {
				dotStyle = lipgloss.NewStyle()
			} else {
				dotStyle = infoStyle
			}
		}
		dot := dotStyle.Render(components.FigBlackCircle)
		header := "  " + dot + " " + nameStyle.Render(presenter.DisplayName()) + "  " + metaStyle.Render(presenter.GroupNoun(batch.Total, batch.Finished, batch.AllDone))
		out = append(out, header)

		detail := batch.Detail
		if detail == "" {
			detail = components.FormatToolDetail(batch.ToolName, "", "", string(batch.LastState))
		}
		if detail == "" {
			detail = label
		}
		out = append(out, "  "+metaStyle.Render(components.FigConnector)+"  "+metaStyle.Render(truncateInline(detail, max(20, m.width-6))))
	}

	return strings.Join(out, "\n")
}

func (m Model) collectToolBatches() []toolBatchVM {
	if len(m.status.toolRuntime) == 0 {
		return nil
	}

	batches := make(map[string]*toolBatchVM)
	for _, rt := range m.status.toolRuntime {
		batchID := rt.BatchID
		if batchID == "" {
			batchID = rt.ToolName
		}
		batch := batches[batchID]
		if batch == nil {
			batch = &toolBatchVM{
				BatchID:  batchID,
				ToolName: rt.ToolName,
				Total:    rt.Total,
				MinOrder: rt.Order,
			}
			batches[batchID] = batch
		}
		if rt.Order < batch.MinOrder {
			batch.MinOrder = rt.Order
		}
		if rt.Total > batch.Total {
			batch.Total = rt.Total
		}
		switch rt.State {
		case message.ToolCallQueued:
			batch.Queued++
			batch.LastState = rt.State
		case message.ToolCallRunning:
			batch.Running++
			batch.LastState = rt.State
			if detail := components.FormatToolDetail(rt.ToolName, rt.Input, rt.ResultSummary, string(rt.State)); detail != "" {
				batch.Detail = detail
			}
		case message.ToolCallCompleted:
			batch.Finished++
			batch.LastState = rt.State
			if detail := components.FormatToolDetail(rt.ToolName, rt.Input, rt.ResultSummary, string(rt.State)); detail != "" {
				batch.Detail = detail
			}
		case message.ToolCallErrored:
			batch.Finished++
			batch.HasError = true
			batch.LastState = rt.State
			if detail := components.FormatToolDetail(rt.ToolName, rt.Input, rt.ResultSummary, string(rt.State)); detail != "" {
				batch.Detail = detail
			}
		case message.ToolCallCanceled:
			batch.Finished++
			batch.LastState = rt.State
			if detail := components.FormatToolDetail(rt.ToolName, rt.Input, rt.ResultSummary, string(rt.State)); detail != "" {
				batch.Detail = detail
			}
		}
		if batch.Detail == "" {
			if detail := components.FormatToolDetail(rt.ToolName, rt.Input, rt.ResultSummary, string(rt.State)); detail != "" {
				batch.Detail = detail
			}
		}
	}

	result := make([]toolBatchVM, 0, len(batches))
	for _, batch := range batches {
		if batch.Total == 0 {
			batch.Total = batch.Finished + batch.Queued + batch.Running
		}
		batch.AllDone = batch.Total > 0 && batch.Finished >= batch.Total && batch.Running == 0 && batch.Queued == 0
		result = append(result, *batch)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AllDone != result[j].AllDone {
			return !result[i].AllDone
		}
		if result[i].MinOrder != result[j].MinOrder {
			return result[i].MinOrder < result[j].MinOrder
		}
		return result[i].BatchID < result[j].BatchID
	})
	return result
}

func truncateInline(s string, maxWidth int) string {
	runes := []rune(strings.ReplaceAll(s, "\n", " "))
	if len(runes) <= maxWidth || maxWidth <= 1 {
		return string(runes)
	}
	return string(runes[:maxWidth-1]) + "…"
}

// buildDialogOverlay renders the current dialog overlay (if any) and returns
// the rendered string. Returns "" if no dialog is active.
func (m Model) buildDialogOverlay() string {
	availableHeight := m.overlayAvailableHeight()
	dialogWidth := m.dialogContentWidth()

	// Phase 4: Overlay stack path
	if m.features.OverlayStack && !m.overlays.IsEmpty() {
		return m.overlays.Top().View(m.width, availableHeight)
	}

	switch {
	case m.state == statePermission && m.dlg.perm.pending != nil:
		maxPreviewLines := availableHeight - 8
		if maxPreviewLines < 0 {
			maxPreviewLines = 0
		}
		return clampRenderedHeight(
			components.RenderPermissionRequestDialog(*m.dlg.perm.pending, m.dlg.perm.optionIdx, dialogWidth, maxPreviewLines),
			availableHeight,
		)
	case m.state == statePlanApproval && m.dlg.planApproval.pending != nil:
		return clampRenderedHeight(
			components.RenderPlanApprovalDialog(
				*m.dlg.planApproval.pending, m.dlg.planApproval.optionIdx,
				m.dlg.planApproval.feedback, m.dlg.planApproval.inputMode, dialogWidth,
			),
			availableHeight,
		)
	case m.state == stateClarification && m.dlg.clarification.pending != nil:
		return components.RenderClarificationDialog(
			*m.dlg.clarification.pending, m.dlg.clarification.idx,
			m.dlg.clarification.input, m.dlg.clarification.focused, dialogWidth,
		)
	case m.state == stateCheckpoint && m.dlg.checkpoint.pending != nil:
		return components.RenderCheckpointDialog(
			*m.dlg.checkpoint.pending, m.dlg.checkpoint.optionIdx,
			m.dlg.checkpoint.feedback, m.dlg.checkpoint.inputMode, dialogWidth,
		)
	case m.state == stateTemplateSelection:
		overlay := components.RenderTemplateDialog(
			m.dlg.template.options, m.dlg.template.idx,
			m.dlg.template.folderName, dialogWidth,
		)
		if m.dlg.template.picker != nil && m.dlg.template.picker.Active {
			visible := m.dlg.template.picker.VisibleItems()
			visIdx := m.dlg.template.picker.VisibleSelectedIndex()
			overlay += components.RenderFilePicker(visible, visIdx, dialogWidth)
		}
		return overlay
	case m.state == stateModelSelection:
		warning := m.dlg.modelSelect.listWarning
		if m.dlg.modelSelect.loadingProvider != "" &&
			m.dlg.modelSelect.providerIdx >= 0 &&
			m.dlg.modelSelect.providerIdx < len(m.dlg.modelSelect.providers) &&
			m.dlg.modelSelect.providers[m.dlg.modelSelect.providerIdx] == m.dlg.modelSelect.loadingProvider {
			warning = "正在刷新 provider 模型列表..."
		}
		return components.RenderModelDialogWithNotice(
			m.dlg.modelSelect.providers, m.dlg.modelSelect.providerIdx,
			m.dlg.modelSelect.list, m.dlg.modelSelect.listIdx, m.dlg.modelSelect.listScroll,
			m.app.CoderAgent.Model().ID, dialogWidth, warning,
		)
	case m.state == stateCopyMode:
		copyBlocks := make([]components.CopyModeBlock, len(m.chat.copyModeBlocks))
		for i, b := range m.chat.copyModeBlocks {
			copyBlocks[i] = components.CopyModeBlock{Language: b.Language, Content: b.Content}
		}
		return components.RenderCopyModeOverlay(copyBlocks, dialogWidth)
	case m.state == stateHelp:
		return components.RenderHelpOverlay(m.width)
	case m.state == stateResearchSuggestion:
		return components.RenderResearchSuggestionDialog(m.dlg.researchSuggest.idx, dialogWidth)
	case m.state == stateWorkspaceSelection:
		return components.RenderWorkspaceDialog(config.WorkingDirectory(), m.dlg.workspace.idx, m.dlg.workspace.inputMode, m.dlg.workspace.input, dialogWidth)
	case m.state == stateInitRequired:
		return components.RenderInitRequiredDialog(m.dlg.initWizard.choiceIdx, dialogWidth)
	case m.state == stateInitWizard && m.dlg.initWizard.wizard != nil:
		wizardStep := m.dlg.initWizard.wizard.CurrentStep()
		if wizardStep != nil {
			stepUI := wizardStepToDialog(wizardStep)
			cur, total := m.dlg.initWizard.wizard.StepNumber()
			isTextMode := wizardStep.Type == initwizard.StepTextInput || m.dlg.initWizard.otherMode
			return components.RenderInitWizardDialog(
				stepUI, cur, total,
				m.dlg.initWizard.idx, m.dlg.initWizard.input, isTextMode, dialogWidth,
			)
		}
		return ""
	case m.state == stateConfigWizard && m.dlg.configWizard.wizard != nil:
		step := m.dlg.configWizard.wizard.CurrentStep()
		if step != nil {
			cur, total, phaseLabel := m.dlg.configWizard.wizard.PhaseProgress()
			return components.RenderConfigWizardDialog(
				step, cur, total, phaseLabel,
				m.dlg.configWizard.idx, m.dlg.configWizard.input, m.dlg.configWizard.cursor,
				m.dlg.configWizard.wizard.Changes(), dialogWidth,
			)
		}
		return ""
	case m.state == stateSessionBrowser:
		if availableHeight <= 0 {
			return ""
		}
		browserHeight := m.height / 2
		if browserHeight < 8 {
			browserHeight = 8
		}
		if browserHeight > availableHeight {
			browserHeight = availableHeight
		}
		return clampRenderedHeight(
			components.RenderSessionBrowser(m.dlg.sessionBrowser.list, m.dlg.sessionBrowser.listIdx, m.dlg.sessionBrowser.listScroll, m.width, browserHeight),
			availableHeight,
		)
	}
	return ""
}

// recalcLayout recalculates the viewport size based on terminal dimensions.
// When the LayoutRegion feature flag is enabled it delegates to the declarative
// Region-based path; otherwise it falls back to the original pre-render approach.
func (m *Model) recalcLayout() {
	if m.isFullscreenMode() || m.features.LayoutRegion {
		m.recalcLayoutRegion()
		return
	}

	statusHeight := 0
	if m.shouldRenderStatusBar() {
		statusHeight = 1
	}
	bottomHeight := m.composerLayoutHeight()
	if overlayHeight := m.overlayLayoutHeight(); overlayHeight > 0 {
		bottomHeight = overlayHeight
	}

	headerHeight := m.headerLayoutHeight()
	if headerHeight > m.height {
		headerHeight = m.height
	}
	remaining := m.height - headerHeight
	if bottomHeight > remaining {
		bottomHeight = remaining
	}
	remaining -= bottomHeight
	if statusHeight > remaining {
		statusHeight = remaining
	}
	remaining -= statusHeight

	chatHeight := remaining
	if chatHeight < 0 {
		chatHeight = 0
	}

	m.chat.viewport.Width = m.width
	m.chat.viewport.Height = chatHeight
	m.chat.virtualList.SetViewportHeight(chatHeight)
}

// recalcLayoutRegion calculates viewport dimensions using the declarative
// Region / LayoutManager approach.  It avoids the full pre-render of dialogs
// and the bottom section; each Region returns a height estimate instead.
func (m *Model) recalcLayoutRegion() {
	snapshot := m.currentLayoutSnapshot()
	chatHeight := snapshot.TranscriptHeight
	if chatHeight < 0 {
		chatHeight = 0
	}

	m.chat.viewport.Width = m.width
	m.chat.viewport.Height = chatHeight
	if m.chat.virtualList != nil {
		m.chat.virtualList.SetViewportHeight(chatHeight)
	}
}

// collectStatusMetrics gathers current window metrics for the status bar.
// The status bar shows the live context/output windows, not cumulative session
// totals. Cumulative usage and cost remain owned by /context and /cost.
func (m Model) collectStatusMetrics() components.StatusMetrics {
	if m.app == nil {
		return components.StatusMetrics{}
	}
	ag := m.app.CoderAgent
	if ag == nil {
		return components.StatusMetrics{}
	}

	model := ag.Model()
	contextLimit := models.RuntimeContextWindow(model)
	outputLimit := m.statusOutputLimit(model)
	snap := ag.ContextSnapshot()
	costState := ag.CostState()

	contextTokens := llmcontext.CurrentUsageTokens(snap.CurrentUsage)
	outputTokens := snap.LastResponseOutputTokens
	promptEstimated := false
	outputEstimated := false

	estimate := llmcontext.TokenCountWithEstimation(m.statusEstimateMessages())
	if estimate.TokenCount > contextTokens {
		contextTokens = estimate.TokenCount
		promptEstimated = estimate.EstimatedDelta > 0 || !estimate.HasRealUsage
	} else if contextTokens == 0 && estimate.TokenCount > 0 {
		contextTokens = estimate.TokenCount
		promptEstimated = !estimate.HasRealUsage || estimate.EstimatedDelta > 0
	}

	if m.status.processing.Phase == PhaseStreaming && int64(m.status.telemetry.LastTokenCount) > outputTokens {
		outputTokens = int64(m.status.telemetry.LastTokenCount)
		outputEstimated = true
	}

	contextPct := 0
	if contextLimit > 0 && contextTokens > 0 {
		contextPct = int(float64(contextTokens) / float64(contextLimit) * 100)
		if contextPct < 1 {
			contextPct = 1
		}
		if contextPct > 100 {
			contextPct = 100
		}
	}

	return components.StatusMetrics{
		PromptTokens:     contextTokens,
		CompletionTokens: outputTokens,
		CacheReadTokens:  snap.CurrentUsage.CacheReadInputTokens,
		CacheWriteTokens: snap.CurrentUsage.CacheCreationInputTokens,
		ContextPct:       contextPct,
		ContextLimit:     contextLimit,
		OutputLimit:      outputLimit,
		PromptEstimated:  promptEstimated,
		OutputEstimated:  outputEstimated,
		CostUSD:          costState.Total,
	}
}

// renderHeaderBar builds the condensed header bar string.
func (m Model) renderHeaderBar() string {
	var modelName, provider string
	if m.app != nil && m.app.CoderAgent != nil {
		mdl := m.app.CoderAgent.Model()
		modelName = mdl.Name
		provider = models.ProviderDisplayName(mdl.Provider)
	}
	return components.RenderHeaderBar(components.HeaderBarParams{
		Version:   version.Version,
		ModelName: modelName,
		Provider:  provider,
		Width:     max(0, m.width-2),
	})
}
