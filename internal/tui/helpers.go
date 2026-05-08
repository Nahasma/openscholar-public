package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/picker"
	"github.com/Nahasma/openscholar-public/internal/research"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// resetSessionState clears all session-scoped state when switching or creating sessions.
// Prevents state leakage (e.g. researchPipelineID, focused tool call) between sessions.
func (m *Model) resetSessionState() {
	m.chat.messages = nil
	m.chat.toolMessages = make(map[string]message.Message)
	m.chat.expandedToolCalls = make(map[string]bool)
	m.chat.expandedNodes = make(map[string]components.ExpandState)
	m.chat.transcriptExpanded = false
	m.chat.inlineErrors = nil
	m.chat.focusedToolCallID = ""
	m.chat.selection = TextSelection{}
	m.chat.scrollMode = ScrollAutoFollow
	m.chat.dirtyMessageIDs = make(map[string]bool)
	m.chat.renderedContent = ""
	m.sessionSummaryMessageID = ""
	m.resetMainScreenTranscriptState()
	m.chat.renderCache = components.NewRenderCache()
	m.resetTranscriptRenderState()
	m.composer.commandOutput = ""
	m.composer.commandDialog = CommandDialogState{}
	m.composer.pendingCommandActivity = CommandActivityState{}

	m.status.researchPipelineID = ""
	m.status.isProcessing = false
	m.status.isCompacting = false
	m.status.ticking = false
	m.clearNotice()
	m.status.processing = ProcessingState{Phase: PhaseIdle}
	m.status.toolRuntime = nil
	m.status.runtimeErr = RuntimeErrorState{}

	// Wave 3: Clear memory UI state on session switch
	m.status.memoryCount = 0
	m.status.memoryToast = components.MemoryToastManager{}
	m.status.bgTaskCount = 0
}

func (m *Model) refreshBackgroundTaskCount() {
	if m.app == nil || m.app.TaskRegistry == nil {
		m.status.bgTaskCount = 0
		return
	}
	if m.sessionID != "" {
		m.status.bgTaskCount = len(m.app.TaskRegistry.BackgroundTasksForSession(m.sessionID))
		return
	}
	m.status.bgTaskCount = 0
}

func (m *Model) resetTranscriptRenderState() {
	bl := components.NewBlockList()
	hc := components.NewHeightCache()
	m.chat.blockList = bl
	m.chat.heightCache = hc
	m.chat.virtualList = components.NewVirtualMessageList(bl, hc)
	m.chat.contentLines = nil
	m.chat.pendingAnchor = nil
}

func (m *Model) clearChatState() tea.Cmd {
	// Clear DB messages and reset session summary for the current session.
	if m.sessionID != "" && m.app != nil {
		if m.app.Messages != nil {
			_ = m.app.Messages.DeleteSessionMessages(m.ctx, m.sessionID)
		}
		if m.app.Sessions != nil {
			if sess, err := m.app.Sessions.Get(m.ctx, m.sessionID); err == nil {
				sess.SummaryMessageID = ""
				_, _ = m.app.Sessions.Save(m.ctx, sess)
			}
		}
	}
	if m.app != nil && m.app.CoderAgent != nil {
		m.app.CoderAgent.ResetTelemetry()
	}

	m.chat.messages = nil
	m.chat.toolMessages = make(map[string]message.Message)
	m.chat.expandedToolCalls = make(map[string]bool)
	m.chat.expandedNodes = make(map[string]components.ExpandState)
	m.chat.inlineErrors = nil
	m.chat.renderedContent = ""
	m.chat.scrollMode = ScrollAutoFollow
	m.chat.renderCache = components.NewRenderCache()
	m.sessionSummaryMessageID = ""
	m.resetMainScreenTranscriptState()
	m.resetTranscriptRenderState()

	m.composer.commandOutput = ""
	m.composer.commandDialog = CommandDialogState{}
	m.composer.pendingCommandActivity = CommandActivityState{}
	m.status.isCompacting = false
	m.status.runtimeErr = RuntimeErrorState{}
	m.dlg.clarification.pending = nil
	m.dlg.planApproval.pending = nil
	m.dlg.checkpoint.pending = nil

	return m.updateViewportContent()
}

func (m *Model) clearRuntimeError() {
	m.status.runtimeErr = RuntimeErrorState{}
}

func (m *Model) setNotice(kind components.NoticeKind, text string) {
	next := components.TransientNotice{
		Text: strings.TrimSpace(text),
		Kind: kind,
	}
	if m.status.notice == next {
		return
	}
	before, ok := m.captureFullscreenLayoutSnapshot()
	m.status.notice = components.TransientNotice{
		Text: next.Text,
		Kind: kind,
	}
	_ = m.relayoutFullscreenIfChanged(before, ok)
}

func (m *Model) clearNotice() {
	m.status.notice = components.TransientNotice{}
}

func (m *Model) currentPermissionMode() permission.Mode {
	switch m.status.mode {
	case "auto":
		return permission.ModeAuto
	case "plan":
		return permission.ModePlan
	case "research":
		return permission.ModeResearch
	default:
		return permission.ModeDefault
	}
}

func permissionModeStatusString(mode permission.Mode) string {
	switch mode {
	case permission.ModeAuto:
		return "auto"
	case permission.ModePlan:
		return "plan"
	case permission.ModeResearch:
		return "research"
	default:
		return ""
	}
}

func (m *Model) setPermissionModeStatus(mode permission.Mode) {
	m.status.mode = permissionModeStatusString(mode)
	if mode == permission.ModeDefault {
		m.status.researchPipelineID = ""
	}
}

func (m *Model) persistSessionMode(sessionID string, mode permission.Mode) {
	if m.app == nil || m.app.Sessions == nil || sessionID == "" {
		return
	}
	sess, err := m.app.Sessions.Get(m.ctx, sessionID)
	if err != nil {
		return
	}
	sess.Mode = permissionModeStatusString(mode)
	_, _ = m.app.Sessions.Save(m.ctx, sess)
}

type sessionModeTransitionOptions struct {
	allowPlanExit bool
}

func (m *Model) transitionSessionMode(to permission.Mode) {
	m.transitionSessionModeWithOptions(to, sessionModeTransitionOptions{})
}

func (m *Model) cycleSessionModeFromUserInput() {
	m.transitionSessionModeWithOptions(permission.NextMode(m.currentPermissionMode()), sessionModeTransitionOptions{
		allowPlanExit: true,
	})
}

func (m *Model) transitionSessionModeWithOptions(to permission.Mode, opts sessionModeTransitionOptions) {
	if !opts.allowPlanExit && m.currentPermissionMode() == permission.ModePlan && to != permission.ModePlan {
		m.setNotice(components.NoticeWarning, "Plan mode requires ExitPlanMode approval to leave")
		return
	}
	m.setPermissionModeStatus(to)

	if m.app == nil || m.app.Permissions == nil || m.sessionID == "" {
		return
	}
	m.app.Permissions.TransitionSessionMode(m.sessionID, to)
	if to == permission.ModePlan && m.app.Plans != nil {
		_, _ = m.app.Plans.Ensure(m.ctx, m.sessionID)
	}
	m.persistSessionMode(m.sessionID, to)
	m.app.Permissions.DenialTracker().Reset()
}

func (m *Model) applyCurrentModeToSession(sessionID string) {
	if m.app == nil || m.app.Permissions == nil || sessionID == "" {
		return
	}
	m.app.Permissions.TransitionSessionMode(sessionID, m.currentPermissionMode())
	if m.currentPermissionMode() == permission.ModePlan && m.app.Plans != nil {
		_, _ = m.app.Plans.Ensure(m.ctx, sessionID)
	}
}

func (m *Model) applySessionModeToUI(sess session.Session) {
	if m.app == nil || m.app.Permissions == nil {
		return
	}
	mode := strings.TrimSpace(sess.Mode)
	switch mode {
	case "auto":
		m.status.mode = "auto"
		m.app.Permissions.TransitionSessionMode(sess.ID, permission.ModeAuto)
	case "plan":
		m.status.mode = "plan"
		m.app.Permissions.TransitionSessionMode(sess.ID, permission.ModePlan)
		if m.app.Plans != nil {
			_, _ = m.app.Plans.Ensure(m.ctx, sess.ID)
		}
	case "research":
		m.status.mode = "research"
		m.app.Permissions.TransitionSessionMode(sess.ID, permission.ModeResearch)
	default:
		m.status.mode = ""
		m.app.Permissions.TransitionSessionMode(sess.ID, permission.ModeDefault)
	}
}

func (m *Model) captureFullscreenLayoutSnapshot() (LayoutSnapshot, bool) {
	if m == nil || !m.isFullscreenMode() {
		return LayoutSnapshot{}, false
	}
	return m.currentLayoutSnapshot(), true
}

func (m *Model) relayoutFullscreenIfChanged(before LayoutSnapshot, ok bool) tea.Cmd {
	if !ok || m == nil || !m.isFullscreenMode() {
		return nil
	}
	after := m.currentLayoutSnapshot()
	if before == after {
		return nil
	}
	m.recalcLayout()
	return m.updateViewportContent()
}

func (m *Model) refreshFullscreenChromeLayout() {
	if m == nil || !m.isFullscreenMode() {
		return
	}
	m.recalcLayout()
	_ = m.updateViewportContent()
}

func (m *Model) resetMainScreenTranscriptState() {
	m.chat.flushedAnchor = nil
	m.chat.liveTailAnchor = nil
	m.chat.activeStreamFlush = nil
	m.chat.mainScreenIntroFlushed = false
	m.resetMainScreenIntroCache()
	m.chat.mainScreenViewportOwned = false
	m.chat.mainScreenOwnedStart = nil
	m.chat.mainScreenOwnedStream = nil
	m.chat.mainScreenFrame = nil
	m.chat.pendingAnchor = nil
	m.chat.resumePinnedStart = 0
	m.chat.resumePinned = false
}

func (m *Model) resetMainScreenIntroCache() {
	m.chat.mainScreenIntroCache = ""
	m.chat.mainScreenIntroWidth = 0
	m.chat.mainScreenIntroCached = false
}

func (m *Model) pinLoadedTranscriptTail() {
	if len(m.chat.messages) == 0 {
		m.resetMainScreenTranscriptState()
		return
	}
	m.resetMainScreenTranscriptState()
}

func appendCmd(cmds *[]tea.Cmd, cmd tea.Cmd) {
	if cmd != nil {
		*cmds = append(*cmds, cmd)
	}
}

func (m Model) hasBlockingOverlay() bool {
	if m.features.OverlayStack && m.overlays != nil && !m.overlays.IsEmpty() {
		return m.overlays.Top().BlocksInput()
	}
	return m.state != stateChat
}

func (m Model) hasEscapeOwner() bool {
	if m.features.OverlayStack && m.overlays != nil && !m.overlays.IsEmpty() {
		return true
	}
	return m.hasPromptInputOwner() || m.state != stateChat
}

func (m Model) hasPromptInputOwner() bool {
	if m.search.historySearch.Visible() || m.search.textSearch.Visible() {
		return true
	}
	if m.composer.commandDialog.Visible || m.composer.commandOutput != "" || m.composer.commandPickerActive {
		return true
	}
	return m.composer.filePicker != nil && m.composer.filePicker.Active
}

func (m Model) activeOverlayName() string {
	if name := strings.TrimSpace(m.status.overlayName); name != "" {
		return name
	}
	if m.features.OverlayStack && m.overlays != nil && !m.overlays.IsEmpty() {
		return m.overlays.Top().ID()
	}
	switch m.state {
	case statePermission:
		return "permission"
	case stateHelp:
		return "help"
	case stateSessionBrowser:
		return "session-browser"
	case statePlanApproval:
		return "plan-approval"
	case stateClarification:
		return "clarification"
	case stateModelSelection:
		return "model-select"
	case stateInitRequired:
		return "init-required"
	case stateInitWizard:
		return "init-wizard"
	case stateConfigWizard:
		return "config-wizard"
	case stateCheckpoint:
		return "checkpoint"
	case stateTemplateSelection:
		return "template"
	case stateWorkspaceSelection:
		return "workspace"
	case stateResearchSuggestion:
		return "research-suggestion"
	case stateCopyMode:
		return "copy-mode"
	default:
		return ""
	}
}

func (m Model) canInterruptCurrentTurn() bool {
	if !m.status.isProcessing || m.sessionID == "" || m.app == nil || m.app.CoderAgent == nil {
		return false
	}
	return m.app.CoderAgent.IsSessionBusy(m.sessionID) || m.hasRunningToolCalls() || m.status.processing.Phase != PhaseIdle
}

func (m *Model) ensureSession() error {
	if m.sessionID != "" {
		return nil
	}
	if m.app == nil || m.app.Sessions == nil {
		return fmt.Errorf("session service is unavailable")
	}
	sess, err := m.app.Sessions.Create(m.ctx, "")
	if err != nil {
		return err
	}
	sess.ProjectPath = config.WorkingDirectory()
	sess.Worktree = config.WorkingDirectory()
	switch m.currentPermissionMode() {
	case permission.ModeAuto:
		sess.Mode = "auto"
	case permission.ModePlan:
		sess.Mode = "plan"
	case permission.ModeResearch:
		sess.Mode = "research"
	default:
		sess.Mode = ""
	}
	if sess.RootSessionID == "" {
		sess.RootSessionID = sess.ID
	}
	sess, _ = m.app.Sessions.Save(m.ctx, sess)
	m.sessionID = sess.ID
	m.sessionSummaryMessageID = sess.SummaryMessageID
	m.applyCurrentModeToSession(m.sessionID)
	return nil
}

// --- File Picker (@) support ---

type fileIndexMsg struct {
	root    string
	entries []picker.FileEntry
}

func loadFileIndexCmd(root string) tea.Cmd {
	return func() tea.Msg {
		return fileIndexMsg{
			root:    root,
			entries: picker.IndexFiles(root, picker.DefaultIndexConfig()),
		}
	}
}

// extractAtQuery finds the @ query from the input text.
// Returns the query string after @ and true if @ is found in a valid position.
func extractAtQuery(input string) (string, bool) {
	// Find the last @ that's preceded by space or is at start
	idx := -1
	for i := len(input) - 1; i >= 0; i-- {
		if input[i] == '@' {
			if i == 0 || input[i-1] == ' ' || input[i-1] == '\n' || input[i-1] == '\t' {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return "", false
	}

	query := input[idx+1:]
	// Don't trigger if there's a space after the query (selection already done)
	if strings.HasSuffix(query, " ") {
		return "", false
	}
	return query, true
}

// replaceAtQuery replaces the @query in the input with the selected path.
func (m *Model) replaceAtQuery(replacement string) {
	value := m.composer.input.Value()
	idx := strings.LastIndex(value, "@")
	if idx < 0 {
		return
	}
	newValue := value[:idx] + "@" + replacement
	m.composer.input.SetValue(newValue)
}

// buildResearchContext builds ephemeral research pipeline context for system prompt injection.
// This is NOT stored in DB — it is only sent to the LLM on each request.
func (m *Model) buildResearchContext() string {
	if m.app.ResearchEngine == nil || m.status.researchPipelineID == "" {
		return ""
	}
	p, err := m.app.ResearchEngine.Get(m.status.researchPipelineID)
	if err != nil {
		return ""
	}
	phases, _ := m.app.ResearchEngine.GetPhases(p.ID)

	var sb strings.Builder
	fmt.Fprintf(&sb, "你正在领导一个科研项目。\n\n")
	fmt.Fprintf(&sb, "## 研究主题\n%s\n\n", p.Topic)
	fmt.Fprintf(&sb, "## 论文类型\n%s\n\n", p.Template)
	fmt.Fprintf(&sb, "## 工作目录\n%s/\n\n", p.WorkDir)
	fmt.Fprintf(&sb, "## 阶段列表\n")
	for _, ph := range phases {
		cp := ""
		if ph.Checkpoint {
			cp = " [检查点]"
		}
		status := ""
		switch ph.Status {
		case research.PhaseRunning:
			status = " [进行中]"
		case research.PhaseCompleted:
			status = " [已完成]"
		}
		fmt.Fprintf(&sb, "%d. %s (max %d workers)%s%s\n", ph.Order, ph.Name, ph.MaxWorkers, cp, status)
	}
	// Highlight the currently active phase
	fmt.Fprintf(&sb, "\n## 当前阶段\n")
	hasRunning := false
	for _, ph := range phases {
		if ph.Status == research.PhaseRunning {
			fmt.Fprintf(&sb, "**%s** (阶段 %d)\n", ph.Name, ph.Order)
			hasRunning = true
			break
		}
	}
	if !hasRunning {
		fmt.Fprintf(&sb, "无进行中的阶段\n")
	}

	fmt.Fprintf(&sb, "\n## 当前模式\n%s\n\n", p.Mode)

	fmt.Fprintf(&sb, "## 预算\n$%.0f (已用 $%.2f)\n\n", p.Budget.Limit, p.Budget.Spent)

	fmt.Fprintf(&sb, "## 阶段控制\n")
	fmt.Fprintf(&sb, "- 默认链路：leader -> worker -> verify -> ResearchPipeline(action=\"advance\") / checkpoint dialog\n")
	fmt.Fprintf(&sb, "- 完成当前阶段：ResearchPipeline(action=\"advance\", summary=\"...\")\n")
	fmt.Fprintf(&sb, "  auto 模式：verify worker 自动评审，达标自动推进\n")
	fmt.Fprintf(&sb, "  default/strict 模式：等待用户确认后推进\n")
	fmt.Fprintf(&sb, "  审核未通过或评审失败时不会自动推进，请修复后重试\n")
	fmt.Fprintf(&sb, "- 查看进度：ResearchPipeline(action=\"status\")\n")
	fmt.Fprintf(&sb, "- 暂停流水线：ResearchPipeline(action=\"pause\")\n")
	fmt.Fprintf(&sb, "- 切换模式：ResearchPipeline(action=\"set_mode\", mode=\"auto|default|strict\")\n")
	fmt.Fprintf(&sb, "- 兼容旧调用：ResearchControl(...) 仍可用，但不再是默认路径\n\n")

	fmt.Fprintf(&sb, "## 写入权限\n")
	fmt.Fprintf(&sb, "Leader 默认不直接写文件；需要通过 Task 派发 Worker 在工作目录 %s/ 内使用 Write/Edit。\n\n", p.WorkDir)

	fmt.Fprintf(&sb, "按照你的系统提示中的工作方式派遣 Worker Agent。\n")
	return sb.String()
}

// syncPipelineProgress updates the pipeline progress data from the research engine.
// Called on each tick when in research mode.
func (m *Model) syncPipelineProgress() {
	if m.app.ResearchEngine == nil || m.status.researchPipelineID == "" {
		m.status.pipelineProgress = components.PipelineProgressData{}
		return
	}
	p, err := m.app.ResearchEngine.Get(m.status.researchPipelineID)
	if err != nil {
		m.status.pipelineProgress = components.PipelineProgressData{}
		return
	}
	phases, _ := m.app.ResearchEngine.GetPhases(p.ID)
	if len(phases) == 0 {
		m.status.pipelineProgress = components.PipelineProgressData{}
		return
	}

	items := make([]components.PhaseProgressItem, len(phases))
	currentIdx := -1
	currentLabel := ""
	for i, ph := range phases {
		var status uint8
		switch ph.Status {
		case research.PhaseCompleted:
			status = components.PhaseProgressCompleted
		case research.PhaseRunning:
			status = components.PhaseProgressRunning
			currentIdx = i
			currentLabel = ph.Name
		case research.PhasePaused:
			status = components.PhaseProgressRunning // show paused as "in progress" visually
			if currentIdx < 0 {
				currentIdx = i
				currentLabel = ph.Name + " (paused)"
			}
		case research.PhaseFailed:
			status = components.PhaseProgressFailed
		default:
			status = components.PhaseProgressPending
		}
		items[i] = components.PhaseProgressItem{Name: ph.Name, Status: status}
	}

	// Handle no running phase: find last completed or default to first
	if currentIdx < 0 {
		allCompleted := true
		for i, ph := range phases {
			if ph.Status == research.PhaseCompleted {
				currentIdx = i
				currentLabel = ph.Name
			} else {
				allCompleted = false
			}
		}
		if allCompleted && currentIdx >= 0 {
			currentLabel = "Completed"
		} else if currentIdx < 0 {
			currentIdx = 0
			currentLabel = phases[0].Name
		}
	}

	m.status.pipelineProgress = components.PipelineProgressData{
		Phases:       items,
		CurrentPhase: currentIdx,
		CurrentLabel: currentLabel,
		WorkerStatus: m.status.processingVerb,
	}
}

// collectExpandableIDs collects all expandable IDs (tool calls + thinking + summaries + groups) in order.
func (m *Model) collectExpandableIDs() []string {
	var ids []string
	for _, msg := range m.chat.messages {
		if msg.Role == message.Assistant {
			toolCalls := msg.ToolCalls()
			if m.features.ToolGroups {
				// Wave 3: group consecutive same-name tool calls into a single navigable group
				i := 0
				for i < len(toolCalls) {
					tc := toolCalls[i]
					// Scan for consecutive same-name tool calls
					j := i + 1
					for j < len(toolCalls) && toolCalls[j].Name == tc.Name {
						j++
					}
					if j-i >= 2 {
						// Group: use group key
						groupKey := "group_" + tc.ID
						ids = append(ids, groupKey)
						// If group is expanded (including via transcript), also add individual items
						if m.chat.GetExpandState(groupKey) >= components.ExpandInline {
							for k := i; k < j; k++ {
								if m.isExpandableToolCall(toolCalls[k]) {
									ids = append(ids, toolCalls[k].ID)
								}
							}
						}
						i = j
					} else {
						// Single tool call
						if m.isExpandableToolCall(tc) {
							ids = append(ids, tc.ID)
						}
						i++
					}
				}
			} else {
				for _, tc := range toolCalls {
					if m.isExpandableToolCall(tc) {
						ids = append(ids, tc.ID)
					}
				}
			}
			for _, part := range msg.Parts {
				if _, ok := part.(message.ReasoningContent); ok {
					key := "thinking_" + msg.ID
					// Skip finished thinking when collapsed (invisible — can't focus)
					if msg.IsFinished() && m.chat.GetExpandState(key) == components.ExpandCollapsed {
						continue
					}
					ids = append(ids, key)
				}
			}
		}
		if msg.Role == message.User {
			content := msg.Content().String()
			if strings.HasPrefix(content, "[Conversation Summary]") {
				ids = append(ids, "summary_"+msg.ID)
			}
		}
	}
	return ids
}

func (m *Model) hasToolResult(toolCallID string) bool {
	toolMsg, ok := m.chat.toolMessages[toolCallID]
	if !ok {
		return false
	}
	for _, part := range toolMsg.Parts {
		if tr, ok := part.(message.ToolResult); ok && tr.ToolCallID == toolCallID {
			return true
		}
	}
	return false
}

func (m *Model) isExpandableToolCall(tc message.ToolCall) bool {
	return tc.IsTerminal() && m.hasToolResult(tc.ID)
}

// navigateToolCall moves the focus to the next/previous expandable item.
func (m *Model) navigateToolCall(delta int) {
	ids := m.collectExpandableIDs()
	if len(ids) == 0 {
		return
	}

	currentIdx := -1
	for i, id := range ids {
		if id == m.chat.focusedToolCallID {
			currentIdx = i
			break
		}
	}

	newIdx := currentIdx + delta
	if newIdx < 0 {
		newIdx = len(ids) - 1
	}
	if newIdx >= len(ids) {
		newIdx = 0
	}
	m.chat.focusedToolCallID = ids[newIdx]
}

func (m *Model) ensureTicking(cmds *[]tea.Cmd) {
	if !m.status.ticking {
		m.status.ticking = true
		*cmds = append(*cmds, tickCmd())
	}
}

func (m *Model) hasRunningToolCalls() bool {
	for _, msg := range m.chat.messages {
		if msg.Role == message.Assistant {
			for _, tc := range msg.ToolCalls() {
				if !tc.IsTerminal() {
					return true
				}
			}
		}
	}
	return false
}

// hasStreamingAssistant returns true if the last assistant message is still streaming.
func (m Model) hasStreamingAssistant() bool {
	for i := len(m.chat.messages) - 1; i >= 0; i-- {
		if m.chat.messages[i].Role == message.Assistant {
			return !m.chat.messages[i].IsFinished()
		}
	}
	return false
}

func (m Model) usesVirtualTranscript() bool {
	return m.isFullscreenMode() && m.features.VirtualScroll
}

func (m *Model) refreshVirtualTranscriptAfterScroll() {
	if !m.usesVirtualTranscript() {
		return
	}
	if m.chat.scrollMode == ScrollManualLocked && m.chat.blockList.Len() > 0 {
		anchor := m.chat.virtualList.AnchorFromFrameYOffset(m.chat.viewport.YOffset)
		m.chat.pendingAnchor = &anchor
	}
	_ = m.updateViewportContent()
}

func (m *Model) syncScrollModeFromViewport() {
	if m.usesVirtualTranscript() {
		maxTop := max(0, m.chat.virtualList.LastTotalHeight()-m.chat.viewport.Height)
		currentTop := m.chat.virtualList.LastFrameTopLine() + m.chat.viewport.YOffset
		if currentTop >= maxTop {
			m.chat.scrollMode = ScrollAutoFollow
		} else {
			m.chat.scrollMode = ScrollManualLocked
		}
		return
	}
	if m.chat.viewport.AtBottom() {
		m.chat.scrollMode = ScrollAutoFollow
	} else {
		m.chat.scrollMode = ScrollManualLocked
	}
}

func (m *Model) applyViewportKeyScroll(msg tea.KeyMsg) tea.Cmd {
	if m.usesVirtualTranscript() {
		switch msg.Type {
		case tea.KeyDown:
			m.scrollVirtualTranscriptBy(1)
		case tea.KeyUp:
			m.scrollVirtualTranscriptBy(-1)
		case tea.KeyPgDown:
			m.scrollVirtualTranscriptBy(max(1, m.chat.viewport.Height))
		case tea.KeyPgUp:
			m.scrollVirtualTranscriptBy(-max(1, m.chat.viewport.Height))
		}
		return nil
	}
	var cmd tea.Cmd
	m.chat.viewport, cmd = m.chat.viewport.Update(msg)
	m.syncScrollModeFromViewport()
	m.refreshVirtualTranscriptAfterScroll()
	return cmd
}

func (m *Model) applyViewportHalfPageDown() {
	if m.usesVirtualTranscript() {
		m.scrollVirtualTranscriptBy(max(1, m.chat.viewport.Height/2))
		return
	}
	m.chat.viewport.HalfPageDown()
	m.syncScrollModeFromViewport()
	m.refreshVirtualTranscriptAfterScroll()
}

func (m *Model) applyViewportHalfPageUp() {
	if m.usesVirtualTranscript() {
		m.scrollVirtualTranscriptBy(-max(1, m.chat.viewport.Height/2))
		return
	}
	m.chat.viewport.HalfPageUp()
	m.syncScrollModeFromViewport()
	m.refreshVirtualTranscriptAfterScroll()
}

func (m *Model) scrollVirtualTranscriptBy(delta int) {
	if !m.usesVirtualTranscript() || delta == 0 {
		return
	}
	m.syncBlockList()
	m.chat.virtualList.SetViewportHeight(m.chat.viewport.Height)
	m.chat.virtualList.ScrollBy(delta)
	if m.chat.virtualList.AutoFollow() {
		m.chat.scrollMode = ScrollAutoFollow
	} else {
		m.chat.scrollMode = ScrollManualLocked
	}
	_ = m.updateViewportContent()
}

// scrollToMessage scrolls the viewport to the first block of message msgIdx.
func (m *Model) scrollToMessage(msgIdx int) {
	if len(m.chat.messages) == 0 {
		return
	}
	if msgIdx < 0 {
		msgIdx = 0
	}
	if msgIdx >= len(m.chat.messages) {
		msgIdx = len(m.chat.messages) - 1
	}

	m.chat.scrollMode = ScrollManualLocked
	_ = m.updateViewportContent()

	if msgIdx <= 0 {
		if m.usesVirtualTranscript() {
			m.chat.virtualList.SetAutoFollow(false)
			m.chat.virtualList.SetAnchor(components.AnchorForBlock(m.chat.blockList.All(), 0, 0))
			m.chat.pendingAnchor = nil
			_ = m.updateViewportContent()
			return
		}
		m.chat.viewport.GotoTop()
		m.chat.scrollMode = ScrollManualLocked
		m.refreshVirtualTranscriptAfterScroll()
		return
	}

	targetMsgID := m.chat.messages[msgIdx].ID
	m.syncBlockList()
	targetBlocks := m.chat.blockList.BlocksForMsg(targetMsgID)
	if len(targetBlocks) == 0 {
		return
	}

	blocks := m.chat.blockList.All()
	if m.usesVirtualTranscript() {
		m.chat.virtualList.SetAutoFollow(false)
		anchor := components.AnchorForBlock(blocks, targetBlocks[0], 0)
		m.chat.virtualList.SetAnchor(anchor)
		m.chat.pendingAnchor = nil
		_ = m.updateViewportContent()
		targetLine := anchor.ResolveDisplayLine(m.chat.heightCache, blocks, components.DefaultBlockHeight)
		if m.chat.viewport.Height > 2 {
			targetLine -= m.chat.viewport.Height / 2
		}
		maxYOffset := max(0, components.DisplayTotalHeight(m.chat.heightCache, blocks, components.DefaultBlockHeight)-m.chat.viewport.Height)
		targetLine = min(max(0, targetLine), maxYOffset)
		centeredAnchor := components.AnchorFromDisplayLine(m.chat.heightCache, blocks, targetLine, components.DefaultBlockHeight)
		m.chat.virtualList.SetAnchor(centeredAnchor)
		m.chat.pendingAnchor = nil
		_ = m.updateViewportContent()
		return
	}

	targetLine := components.DisplayHeightBefore(m.chat.heightCache, blocks, targetBlocks[0], 1)
	maxYOffset := max(0, m.chat.viewport.TotalLineCount()-m.chat.viewport.Height)
	if targetLine > maxYOffset {
		targetLine = maxYOffset
	}
	m.chat.viewport.SetYOffset(max(0, targetLine))
	m.chat.scrollMode = ScrollManualLocked
	m.refreshVirtualTranscriptAfterScroll()
}

// hasAnyRecentAssistant returns true if there is any assistant message after the last user message.
func (m Model) hasAnyRecentAssistant() bool {
	for i := len(m.chat.messages) - 1; i >= 0; i-- {
		if m.chat.messages[i].Role == message.User {
			return false
		}
		if m.chat.messages[i].Role == message.Assistant {
			return true
		}
	}
	return false
}

// --- Model selection ---

// initModelSelection initializes the model selection dialog state.
func (m *Model) initModelSelection() tea.Cmd {
	currentModel := m.app.CoderAgent.Model()
	cfg := config.Get()

	// Collect providers that have models
	var providers []models.ModelProvider
	for _, prov := range models.ProviderDisplayOrder() {
		if len(config.ModelOptionsForProvider(cfg, prov, nil)) == 0 {
			continue
		}
		providers = append(providers, prov)
	}

	m.dlg.modelSelect.providers = providers
	m.dlg.modelSelect.providerIdx = 0
	m.dlg.modelSelect.selectedModelID = currentModel.ID
	m.dlg.modelSelect.discovered = make(map[models.ModelProvider][]models.Model)
	m.dlg.modelSelect.loadingProvider = ""
	m.dlg.modelSelect.listWarning = ""

	// Find current provider index
	for i, p := range providers {
		if p == currentModel.Provider {
			m.dlg.modelSelect.providerIdx = i
			break
		}
	}

	m.setupModelListForProvider()
	return m.discoverCurrentModelSelectionProviderCmd()
}

// setupModelListForProvider populates the model list for the currently selected provider.
func (m *Model) setupModelListForProvider() {
	if m.dlg.modelSelect.providerIdx >= len(m.dlg.modelSelect.providers) {
		return
	}
	prov := m.dlg.modelSelect.providers[m.dlg.modelSelect.providerIdx]
	options := config.ModelOptionsForProvider(config.Get(), prov, m.dlg.modelSelect.discovered[prov])
	modelList := make([]models.Model, 0, len(options))
	for _, opt := range options {
		modelList = append(modelList, opt.Model)
	}

	m.dlg.modelSelect.list = modelList
	if len(modelList) == 0 {
		m.dlg.modelSelect.listIdx = 0
		m.dlg.modelSelect.listScroll = 0
		return
	}

	selectedIdx := -1
	for i, mdl := range modelList {
		if mdl.ID == m.dlg.modelSelect.selectedModelID {
			selectedIdx = i
			break
		}
	}
	if selectedIdx == -1 {
		selectedIdx = 0
		m.dlg.modelSelect.selectedModelID = modelList[0].ID
	}
	m.dlg.modelSelect.listIdx = selectedIdx
	m.ensureModelSelectionVisible()
}

func (m *Model) discoverCurrentModelSelectionProviderCmd() tea.Cmd {
	if m.dlg.modelSelect.providerIdx < 0 || m.dlg.modelSelect.providerIdx >= len(m.dlg.modelSelect.providers) {
		return nil
	}
	prov := m.dlg.modelSelect.providers[m.dlg.modelSelect.providerIdx]
	cmd := discoverModelsForProviderCmd(prov)
	if cmd == nil {
		m.dlg.modelSelect.loadingProvider = ""
		return nil
	}
	m.dlg.modelSelect.loadingProvider = prov
	m.dlg.modelSelect.listWarning = ""
	return cmd
}

func (m *Model) applyModelListLoaded(msg modelListLoadedMsg) {
	if m.dlg.modelSelect.loadingProvider == msg.Provider {
		m.dlg.modelSelect.loadingProvider = ""
	}
	if msg.Err != nil {
		m.dlg.modelSelect.listWarning = "模型列表刷新失败: " + msg.Err.Error()
		return
	}
	if m.dlg.modelSelect.discovered == nil {
		m.dlg.modelSelect.discovered = make(map[models.ModelProvider][]models.Model)
	}
	m.dlg.modelSelect.discovered[msg.Provider] = append([]models.Model(nil), msg.Models...)
	config.MergeProviderModelsInMemory(msg.Provider, modelConfigsFromDiscovered(msg.Models))
	if len(msg.Models) > 0 {
		m.dlg.modelSelect.listWarning = "已加载 provider 返回的模型列表"
	} else {
		m.dlg.modelSelect.listWarning = ""
	}
	if m.dlg.modelSelect.providerIdx >= 0 &&
		m.dlg.modelSelect.providerIdx < len(m.dlg.modelSelect.providers) &&
		m.dlg.modelSelect.providers[m.dlg.modelSelect.providerIdx] == msg.Provider {
		m.setupModelListForProvider()
	}
}

func (m *Model) ensureModelSelectionVisible() {
	const maxVisible = 10
	n := len(m.dlg.modelSelect.list)
	if n == 0 {
		m.dlg.modelSelect.listIdx = 0
		m.dlg.modelSelect.listScroll = 0
		return
	}
	m.dlg.modelSelect.listIdx = max(0, min(m.dlg.modelSelect.listIdx, n-1))
	visible := min(maxVisible, n)
	minScroll := m.dlg.modelSelect.listIdx - visible + 1
	if minScroll < 0 {
		minScroll = 0
	}
	maxScroll := m.dlg.modelSelect.listIdx
	if m.dlg.modelSelect.listScroll < minScroll {
		m.dlg.modelSelect.listScroll = minScroll
	}
	if m.dlg.modelSelect.listScroll > maxScroll {
		m.dlg.modelSelect.listScroll = maxScroll
	}
	maxAllowedScroll := max(0, n-visible)
	m.dlg.modelSelect.listScroll = max(0, min(m.dlg.modelSelect.listScroll, maxAllowedScroll))
	if m.dlg.modelSelect.listIdx >= 0 && m.dlg.modelSelect.listIdx < len(m.dlg.modelSelect.list) {
		m.dlg.modelSelect.selectedModelID = m.dlg.modelSelect.list[m.dlg.modelSelect.listIdx].ID
	}
}

func (m *Model) handleModelSelectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.state = stateChat
		return m, m.composer.input.Focus()

	case tea.KeyUp:
		if len(m.dlg.modelSelect.list) == 0 {
			return m, nil
		}
		if m.dlg.modelSelect.listIdx > 0 {
			m.dlg.modelSelect.listIdx--
		} else {
			m.dlg.modelSelect.listIdx = len(m.dlg.modelSelect.list) - 1
		}
		m.ensureModelSelectionVisible()
		return m, nil

	case tea.KeyDown:
		if len(m.dlg.modelSelect.list) == 0 {
			return m, nil
		}
		if m.dlg.modelSelect.listIdx < len(m.dlg.modelSelect.list)-1 {
			m.dlg.modelSelect.listIdx++
		} else {
			m.dlg.modelSelect.listIdx = 0
		}
		m.ensureModelSelectionVisible()
		return m, nil

	case tea.KeyLeft:
		if len(m.dlg.modelSelect.providers) > 1 {
			if m.dlg.modelSelect.providerIdx > 0 {
				m.dlg.modelSelect.providerIdx--
			} else {
				m.dlg.modelSelect.providerIdx = len(m.dlg.modelSelect.providers) - 1
			}
			m.setupModelListForProvider()
			return m, m.discoverCurrentModelSelectionProviderCmd()
		}
		return m, nil

	case tea.KeyRight:
		if len(m.dlg.modelSelect.providers) > 1 {
			if m.dlg.modelSelect.providerIdx < len(m.dlg.modelSelect.providers)-1 {
				m.dlg.modelSelect.providerIdx++
			} else {
				m.dlg.modelSelect.providerIdx = 0
			}
			m.setupModelListForProvider()
			return m, m.discoverCurrentModelSelectionProviderCmd()
		}
		return m, nil

	case tea.KeyEnter:
		if len(m.dlg.modelSelect.list) > 0 && m.dlg.modelSelect.listIdx < len(m.dlg.modelSelect.list) {
			selected := m.dlg.modelSelect.list[m.dlg.modelSelect.listIdx]
			m.dlg.modelSelect.selectedModelID = selected.ID
			if err := m.app.SetModelProvider(selected.Provider, string(selected.ID)); err != nil {
				m.composer.commandOutput = fmt.Sprintf("Error: %v", err)
			} else {
				m.composer.commandOutput = fmt.Sprintf("已切换为 %s (%s)", selected.Name, selected.ID)
			}
			m.state = stateChat
			return m, m.composer.input.Focus()
		}
		return m, nil
	}

	// vim-style keys
	switch msg.String() {
	case "j":
		msg.Type = tea.KeyDown
		return m.handleModelSelectionKey(msg)
	case "k":
		msg.Type = tea.KeyUp
		return m.handleModelSelectionKey(msg)
	case "h":
		msg.Type = tea.KeyLeft
		return m.handleModelSelectionKey(msg)
	case "l":
		msg.Type = tea.KeyRight
		return m.handleModelSelectionKey(msg)
	}

	return m, nil
}

// --- Session management ---

func (m Model) fetchSessionListCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.app.Resume.List(m.ctx, config.WorkingDirectory(), 50)
		if err != nil {
			return nil
		}
		return sessionListMsg(sessions)
	}
}

func (m Model) loadSessionByIDCmd(sessionID string) tea.Cmd {
	return func() tea.Msg {
		loaded, err := m.app.Resume.Load(m.ctx, sessionID)
		if err != nil {
			return nil
		}
		return loadSessionMsg{
			session:      loaded.Session,
			messages:     loaded.Messages,
			toolMessages: loaded.ToolMessageByCall,
		}
	}
}
