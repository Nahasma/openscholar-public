package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/agent"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/picker"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

func (m Model) sendMessage() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.composer.input.Value())
	if text == "" {
		return m, nil
	}

	beforeChrome, trackChrome := (&m).captureFullscreenLayoutSnapshot()
	finish := func(model Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
		relayout := (&model).relayoutFullscreenIfChanged(beforeChrome, trackChrome)
		return model, tea.Batch(cmd, relayout)
	}

	m.composer.input.PushHistory(text)
	m.composer.input.Reset()

	// Command interception
	if m.dispatcher != nil && m.dispatcher.IsCommand(text) {
		invocation := strings.TrimSpace(text)
		isResearchStart := strings.HasPrefix(strings.TrimSpace(text), "/research start")
		if isResearchStart && m.sessionID == "" {
			if err := m.ensureSession(); err != nil {
				m.composer.commandOutput = fmt.Sprintf("Error: %v", err)
				return finish(m, nil)
			}
		}

		if isResearchStart && m.sessionID != "" {
			if sess, err := m.app.Sessions.Get(m.ctx, m.sessionID); err == nil && sess.ParentSessionID != "" {
				m.composer.commandOutput = "Error: /research start 需要主会话，当前子任务会话不支持直接启动研究流水线。"
				return finish(m, nil)
			}
		}

		execCtx := m.buildExecutionContext()
		result := m.dispatcher.Dispatch(command.Context{
			App:         m.app,
			SessionID:   m.sessionID,
			ExecContext: execCtx,
		}, text)

		if result.Error != nil {
			m.composer.commandOutput = fmt.Sprintf("Error: %v", result.Error)
			return finish(m, nil)
		}

		if result.SessionID != "" {
			m.sessionID = result.SessionID
		}

		if result.ClearChat {
			if cmd := m.clearChatState(); cmd != nil {
				return finish(m, cmd)
			}
		}
		if handled, cmd := m.applyCommandActions(result, invocation); handled {
			return finish(m, cmd)
		}

		if result.Output != "" {
			cmd := m.applyCommandOutput(invocation, result)
			return finish(m, cmd)
		}

		if result.Prompt != "" {
			next, cmd := m.sendToAgentWithRuntime(result.Prompt, result.Runtime)
			if updated, ok := next.(Model); ok {
				return finish(updated, cmd)
			}
			return next, cmd
		}

		return finish(m, nil)
	}

	// Research intent detection: suggest /research start when not in research mode
	if m.status.mode != "research" && hasResearchIntent(text) {
		if m.features.OverlayStack {
			m.pushResearchSuggestionOverlay(text)
		} else {
			m.dlg.researchSuggest.idx = 0
			m.dlg.researchSuggest.text = text
			m.state = stateResearchSuggestion
		}
		return finish(m, nil)
	}

	next, cmd := m.sendToAgent(text)
	if updated, ok := next.(Model); ok {
		return finish(updated, cmd)
	}
	return next, cmd
}

func (m *Model) showCommandDialog(invocation string, result command.Result) {
	m.composer.commandOutput = ""
	dismissText := strings.TrimSpace(result.OutputDismissText)
	if dismissText == "" {
		dismissText = "Dialog dismissed"
	}
	m.composer.commandDialog = CommandDialogState{
		Visible:         true,
		Invocation:      strings.TrimSpace(invocation),
		Title:           strings.TrimSpace(result.OutputTitle),
		Body:            strings.TrimRight(result.Output, "\n"),
		DismissText:     dismissText,
		RecordOnDismiss: !result.SuppressDismissRecord,
	}
}

func (m *Model) hasCommandDialog() bool {
	return m.composer.commandDialog.Visible
}

func (m *Model) appendCommandActivity(invocation, summary string) {
	invocation = strings.TrimSpace(invocation)
	summary = strings.TrimSpace(summary)
	if invocation == "" || summary == "" {
		return
	}
	now := time.Now()
	m.chat.messages = append(m.chat.messages, message.Message{
		ID:        fmt.Sprintf("command-activity-%d", now.UnixNano()),
		SessionID: m.sessionID,
		Role:      message.System,
		Parts: []message.ContentPart{
			message.TextContent{Text: summary},
			message.Finish{Reason: message.FinishReasonEndTurn, Time: now.Unix()},
		},
		Meta: map[string]any{
			"ui_command_activity": true,
			"command_invocation":  invocation,
			"command_summary":     summary,
		},
		CreatedAt: now.Unix(),
		UpdatedAt: now.Unix(),
	})
}

func (m *Model) dismissCommandDialog() tea.Cmd {
	dialog := m.composer.commandDialog
	m.composer.commandDialog = CommandDialogState{}
	if dialog.RecordOnDismiss {
		m.appendCommandActivity(dialog.Invocation, dialog.DismissText)
	}
	m.recalcLayout()
	return tea.Batch(m.updateViewportContent(), m.composer.input.Focus())
}

func (m *Model) applyCommandOutput(invocation string, result command.Result) tea.Cmd {
	if result.Output == "" {
		return nil
	}
	switch result.EffectiveOutputKind() {
	case command.OutputCommandBlock:
		m.showCommandDialog(invocation, result)
		return nil
	case command.OutputTranscriptBlock:
		m.appendCommandTranscriptBlock(result)
		return m.updateViewportContent()
	default:
		m.composer.commandOutput = result.Output
		return nil
	}
}

func (m *Model) appendCommandTranscriptBlock(result command.Result) {
	output := strings.TrimRight(result.Output, "\n")
	if output == "" {
		return
	}
	if title := strings.TrimSpace(result.OutputTitle); title != "" {
		output = title + "\n\n" + output
	}
	now := time.Now()
	m.chat.messages = append(m.chat.messages, message.Message{
		ID:        fmt.Sprintf("command-output-%d", now.UnixNano()),
		SessionID: m.sessionID,
		Role:      message.System,
		Parts: []message.ContentPart{
			message.TextContent{Text: output},
			message.Finish{Reason: message.FinishReasonEndTurn, Time: now.Unix()},
		},
		Meta: map[string]any{
			"command_output": true,
			"output_kind":    string(result.EffectiveOutputKind()),
		},
		CreatedAt: now.Unix(),
		UpdatedAt: now.Unix(),
	})
}

func (m Model) startManualCompact(focus string) (tea.Model, tea.Cmd) {
	if m.sessionID == "" {
		m.composer.commandOutput = "No active session."
		return m, nil
	}
	if m.app == nil || m.app.CoderAgent == nil {
		m.composer.commandOutput = "Compaction is unavailable."
		return m, nil
	}
	if m.app.CoderAgent.IsSessionBusy(m.sessionID) {
		m.composer.commandOutput = "Session is busy"
		return m, nil
	}

	m.status.isProcessing = true
	m.clearNotice()
	m.clearRuntimeError()
	m.status.processing = ProcessingState{
		Phase:     PhaseCompacting,
		Label:     "compacting",
		StartedAt: time.Now(),
	}
	var cmds []tea.Cmd
	if !m.status.ticking {
		m.status.ticking = true
		cmds = append(cmds, tickCmd())
	}

	ctx := m.buildExecutionContext()
	go func() {
		_ = m.app.CoderAgent.CompactSession(ctx, m.sessionID, focus)
	}()
	return m, tea.Batch(cmds...)
}

func (m Model) buildExecutionContext() context.Context {
	ctx := context.WithValue(m.ctx, tools.ResearchModeContextKey, m.status.mode == "research")
	ctx = permission.WithMode(ctx, m.currentPermissionMode())
	if m.status.mode == "research" {
		ctx = context.WithValue(ctx, tools.ResearchToolProfileContextKey, tools.ResearchToolProfileLeader)
	}
	ctx = context.WithValue(ctx, tools.WorkspaceDirContextKey, config.WorkingDirectory())
	if m.status.mode == "research" && m.app != nil && m.app.ResearchEngine != nil && m.status.researchPipelineID != "" {
		ctx = context.WithValue(ctx, tools.ResearchContextContextKey, m.buildResearchContext())
		if p, err := m.app.ResearchEngine.Get(m.status.researchPipelineID); err == nil {
			ctx = context.WithValue(ctx, tools.ResearchWorkDirContextKey, p.WorkDir)
		}
	}
	return ctx
}

// hasResearchIntent detects if user input expresses research project intent.
func hasResearchIntent(text string) bool {
	lower := strings.ToLower(text)
	keywords := []string{
		"进行研究", "写论文", "文献调研", "科研", "论文写作", "学术研究",
		"research project", "survey paper", "paper writing", "literature review",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// sendToAgent handles session creation and agent invocation.
func (m Model) sendToAgent(text string) (tea.Model, tea.Cmd) {
	return m.sendToAgentWithRuntime(text, nil)
}

func (m Model) sendToAgentWithRuntime(text string, runtime *command.RuntimeOverride) (tea.Model, tea.Cmd) {
	beforeChrome, trackChrome := (&m).captureFullscreenLayoutSnapshot()
	finish := func(cmd tea.Cmd) (tea.Model, tea.Cmd) {
		relayout := (&m).relayoutFullscreenIfChanged(beforeChrome, trackChrome)
		return m, tea.Batch(cmd, relayout)
	}

	m.status.isProcessing = true
	m.clearNotice()
	// Wave 2: Start thinking phase
	m.status.processing = ProcessingState{
		Phase:     PhaseThinking,
		Label:     "thinking",
		StartedAt: time.Now(),
	}
	// Wave 4: Pick random session verb for this turn
	m.status.sessionVerb = pickSessionVerb()
	m.status.telemetry = ProgressTelemetry{}
	m.status.toolRuntime = make(map[string]ToolRuntimeState)
	m.clearRuntimeError()
	// Clear old inline errors
	m.chat.inlineErrors = nil
	// Reset scroll to auto-follow when user sends a new message
	m.chat.scrollMode = ScrollAutoFollow

	// Create session if needed
	if m.sessionID == "" {
		sess, err := m.app.Sessions.Create(m.ctx, "")
		if err != nil {
			m.setNotice(components.NoticeError, fmt.Sprintf("Error: %v", err))
			m.status.isProcessing = false
			m.status.processing = ProcessingState{Phase: PhaseIdle}
			return finish(nil)
		}
		m.sessionID = sess.ID

		m.applyCurrentModeToSession(m.sessionID)

		// Load history
		if history, err := m.app.Messages.List(m.ctx, m.sessionID); err == nil && len(history) > 0 {
			m.chat.messages = history
		}
	}

	// Detect PDF file paths in the message
	var pdfParts []message.ContentPart
	pdfRegex := regexp.MustCompile(`\S+\.pdf\b`)
	pdfPaths := pdfRegex.FindAllString(text, -1)
	for _, pdfPath := range pdfPaths {
		// Resolve relative paths
		absPath := pdfPath
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(os.Getenv("PWD"), absPath)
		}
		data, err := os.ReadFile(absPath)
		if err == nil {
			pdfParts = append(pdfParts, message.BinaryContent{
				Path:     absPath,
				MIMEType: "application/pdf",
				Data:     data,
			})
		}
	}
	var cmds []tea.Cmd
	if !m.status.ticking {
		m.status.ticking = true
		cmds = append(cmds, tickCmd())
	}

	// Guard: don't start if agent is already busy for this session
	if m.app.CoderAgent.IsSessionBusy(m.sessionID) {
		m.status.isProcessing = false
		m.status.processing = ProcessingState{Phase: PhaseIdle}
		m.setNotice(components.NoticeWarning, "Session is busy")
		return finish(nil)
	}

	ctx := m.buildExecutionContext()

	// Research mode: create pipeline once, inject context on every turn
	if m.status.mode == "research" && m.app.ResearchEngine != nil {
		if m.status.researchPipelineID == "" {
			// Check if a pipeline already exists for this session (e.g. created by /research start)
			if existing, err := m.app.ResearchEngine.GetBySession(m.sessionID); err == nil {
				m.status.researchPipelineID = existing.ID
			} else {
				// First research message: show template selection dialog
				m.dlg.template.options = []string{"empirical", "aris_empirical", "survey", "theoretical"}
				m.dlg.template.idx = 0
				m.dlg.template.folderName = ""
				m.dlg.template.picker = picker.New(8)
				m.dlg.template.fileIndex = nil
				m.dlg.template.pendingText = text
				m.state = stateTemplateSelection
				return finish(nil)
			}
		}
		// Inject research pipeline context (not stored in DB, only sent to LLM)
		if m.status.researchPipelineID != "" {
			researchCtx := m.buildResearchContext()
			ctx = context.WithValue(ctx, tools.ResearchContextContextKey, researchCtx)
			// Inject workspace directory for tool sandbox validation
			if p, err := m.app.ResearchEngine.Get(m.status.researchPipelineID); err == nil {
				ctx = context.WithValue(ctx, tools.ResearchWorkDirContextKey, p.WorkDir)
			}
		}
		// text remains the user's raw input — no wrapping
	}
	if runtime != nil {
		ctx = agent.WithRequestRuntime(ctx, agent.RequestRuntime{
			AllowedTools:           append([]string(nil), runtime.AllowedTools...),
			Model:                  runtime.Model,
			DisableModelInvocation: runtime.DisableModelInvocation,
		})
	}

	go func() {
		ch, err := m.app.CoderAgent.Run(ctx, m.sessionID, text, pdfParts...)
		if err != nil {
			m.app.CoderAgent.Publish(pubsub.CreatedEvent, agent.AgentEvent{
				SessionID: m.sessionID,
				Error:     err,
				Done:      true,
			})
			return
		}
		// Drain the channel to unblock agent goroutine
		for range ch {
		}
	}()

	return finish(tea.Batch(cmds...))
}

// toggleLastToolCall finds the last finished tool call and toggles its expanded state.
func (m *Model) toggleLastToolCall() tea.Cmd {
	// Collect all finished tool calls in all messages
	var toolCallIDs []string
	for i := 0; i < len(m.chat.messages); i++ {
		msg := m.chat.messages[i]
		if msg.Role == message.Assistant {
			for _, tc := range msg.ToolCalls() {
				if m.isExpandableToolCall(tc) {
					toolCallIDs = append(toolCallIDs, tc.ID)
				}
			}
			// Also check thinking content
			for _, part := range msg.Parts {
				if _, ok := part.(message.ReasoningContent); ok {
					key := "thinking_" + msg.ID
					toolCallIDs = append(toolCallIDs, key)
				}
			}
		}
		// Also collect summary keys from user messages
		if msg.Role == message.User {
			content := msg.Content().String()
			if strings.HasPrefix(content, "[Conversation Summary]") {
				toolCallIDs = append(toolCallIDs, "summary_"+msg.ID)
			}
		}
	}

	// If no expandable nodes exist, nothing to toggle.
	if len(toolCallIDs) == 0 {
		return nil
	}

	// Toggle only the last finished tool call
	lastID := toolCallIDs[len(toolCallIDs)-1]
	m.cycleExpandState(lastID)
	return nil
}

// toggleTranscriptView flips the global transcript expansion state so thinking
// blocks and tool previews follow a Claude Code-like "expand the transcript"
// behavior even when no specific node is focused.
func (m *Model) toggleTranscriptView() tea.Cmd {
	m.chat.transcriptExpanded = !m.chat.transcriptExpanded
	if m.chat.heightCache != nil {
		m.chat.heightCache.InvalidateAll()
	}
	if m.features.ToolGroups && m.chat.blockList != nil {
		m.chat.blockList.RebuildAll(nil)
	}
	// Preserve scroll position: switch to manual lock so the viewport
	// anchor system keeps the current reading position instead of
	// jumping to the bottom via auto-follow.
	if !m.chat.viewport.AtBottom() {
		m.chat.scrollMode = ScrollManualLocked
	}
	return m.updateViewportContent()
}

// cycleExpandState cycles the expand state for a given node ID:
// Collapsed ↔ Inline (toggle). ExpandOverlay is reserved for Wave 3
// and intentionally excluded from the current cycle per spec.
func (m *Model) cycleExpandState(id string) {
	current := m.chat.expandedNodes[id]
	switch current {
	case components.ExpandCollapsed:
		m.chat.expandedNodes[id] = components.ExpandInline
	default:
		// ExpandInline or ExpandOverlay → collapse
		m.chat.expandedNodes[id] = components.ExpandCollapsed
	}
	// Sync legacy expandedToolCalls for callers that still read it
	m.chat.expandedToolCalls[id] = m.chat.expandedNodes[id] >= components.ExpandInline
	// Invalidate height cache for this block
	if m.chat.heightCache != nil {
		m.chat.heightCache.InvalidateAll()
	}

	// Wave 3: Group expand/collapse changes block topology.
	// Force a full rebuild so BuildBlocksGrouped re-generates the block list.
	if strings.HasPrefix(id, "group_") && m.features.ToolGroups {
		m.chat.blockList.RebuildAll(nil) // clear → syncBlockList will rebuild
	}
}

// renderMessageSlice renders a subset of messages using the block pipeline,
// reusing BuildBlocksGrouped / RenderBlockV2 for visual consistency.
func (m *Model) renderMessageSlice(msgs []message.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	var blocks []components.BlockVM
	if m.features.ToolGroups {
		blocks = components.BuildBlocksGrouped(msgs, m.chat.toolMessages,
			m.chat.expandedToolCalls, m.chat.focusedToolCallID, m.chat.GetExpandState)
	} else {
		blocks = components.BuildBlocks(msgs, m.chat.toolMessages,
			m.chat.expandedToolCalls, m.chat.focusedToolCallID)
	}

	ctx := components.BlockRenderContext{
		Width:             m.width,
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

	var sb strings.Builder
	prevMsgID := ""
	for i := range blocks {
		rendered := ctx.Render(&blocks[i])
		if rendered == "" {
			continue
		}
		if prevMsgID != "" && blocks[i].MsgID != prevMsgID {
			sb.WriteString("\n")
		}
		sb.WriteString(rendered)
		prevMsgID = blocks[i].MsgID
	}
	return sb.String()
}
