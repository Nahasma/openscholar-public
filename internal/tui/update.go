package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/llm/agent"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/picker"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/research"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/task"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	skipComposerPickerSync := false

	switch msg := msg.(type) {
	case tickMsg:
		m.status.spinnerFrame++

		// Wave 2: Sync ProcessingState with spinner frame and update StatusV2 fields
		m.status.processing.SpinnerFrame = m.status.spinnerFrame
		if m.status.processing.Phase != PhaseIdle {
			elapsed := m.status.processing.ElapsedSeconds()
			m.status.processingElapsed = elapsed

			// Update processingVerb based on phase
			switch m.status.processing.Phase {
			case PhaseThinking:
				thinkingVerbs := []string{"thinking", "reasoning", "analyzing", "composing"}
				verbIdx := (m.status.spinnerFrame / 80) % len(thinkingVerbs)
				m.status.processingVerb = thinkingVerbs[verbIdx]
			case PhaseToolQueued:
				m.status.processingVerb = "Queued tools"
			case PhaseToolRunning:
				m.status.processingVerb = "Running tools"
			case PhaseStreaming:
				m.status.processingVerb = "Streaming response"
			case PhaseCompacting:
				m.status.processingVerb = "Compacting context"
			}
		}

		// Wave 4: Stalled detection (only during streaming without active tools)
		if m.status.processing.Phase == PhaseStreaming && !m.status.telemetry.LastTokenAt.IsZero() {
			sinceLast := time.Since(m.status.telemetry.LastTokenAt)
			if sinceLast > 3*time.Second {
				// Fade from 0→1 over 2 seconds (3s→5s)
				fadeElapsed := sinceLast - 3*time.Second
				m.status.telemetry.StalledIntensity = min(float64(fadeElapsed)/float64(2*time.Second), 1.0)
			} else {
				m.status.telemetry.StalledIntensity = 0
			}
		} else {
			m.status.telemetry.StalledIntensity = 0
		}

		// Wave 4: Smooth token counter
		if m.status.telemetry.DisplayedTokenCount < m.status.telemetry.LastTokenCount {
			m.status.telemetry.DisplayedTokenCount = components.SmoothTokenStep(
				m.status.telemetry.DisplayedTokenCount,
				m.status.telemetry.LastTokenCount,
			)
		}

		// Wave 3: Tick memory toast TTL
		if m.features.MemoryUI {
			m.status.memoryToast.Tick()
		}

		// Wave 4: Sync pipeline progress every 10 ticks (~1s)
		if m.status.mode == "research" && m.status.spinnerFrame%10 == 0 {
			if m.syncPipelineProgressLayoutChanged() {
				appendCmd(&cmds, m.updateViewportContent())
			}
		}

		// Dirty flush: re-render only if there are dirty messages
		if len(m.chat.dirtyMessageIDs) > 0 {
			appendCmd(&cmds, m.updateViewportContent())
			m.chat.dirtyMessageIDs = make(map[string]bool)
		}

		// Fallback safety net: if processing but no message events for 3s, force refresh
		if m.status.isProcessing {
			m.chat.ticksSinceLastMsg++
			if m.chat.ticksSinceLastMsg > 30 { // 30 ticks × 100ms = 3s
				m.chat.ticksSinceLastMsg = 0
				appendCmd(&cmds, m.updateViewportContent())
			}
		}

		if m.status.isProcessing || m.hasRunningToolCalls() {
			cmds = append(cmds, tickCmd())
		} else {
			m.status.ticking = false
		}
		return m, tea.Batch(cmds...)

	case ctrlCResetMsg:
		before, ok := m.captureFullscreenLayoutSnapshot()
		m.status.ctrlCPending = false
		return m, m.relayoutFullscreenIfChanged(before, ok)

	case clearCopyFeedbackMsg:
		before, ok := m.captureFullscreenLayoutSnapshot()
		m.clearNotice()
		return m, m.relayoutFullscreenIfChanged(before, ok)

	case screenModeAppliedMsg:
		m.terminalState = TerminalState{
			AltScreen: msg.target.IsFullscreen(),
			Mouse:     msg.target.IsFullscreen(),
		}
		if msg.target.IsFullscreen() {
			m.markFullscreenFrameDirty()
			return m, m.recalcFullscreenLayoutAndViewport(msg.prevWidth)
		}
		m.scheduleMainScreenVisibleReset("return-main")
		return m, m.recalcLayoutAndViewport(true)

	case tea.MouseMsg:
		if !m.isFullscreenMode() {
			// In main-screen mode we rely on native terminal selection and
			// scrollback rather than Bubble Tea mouse capture.
			return m, nil
		}
		headerHeight := m.headerLayoutHeight()
		localY := msg.Y - headerHeight
		transcriptHit := localY >= 0 && localY < m.chat.viewport.Height
		isWheel := msg.Button == tea.MouseButtonWheelUp ||
			msg.Button == tea.MouseButtonWheelDown ||
			msg.Button == tea.MouseButtonWheelLeft ||
			msg.Button == tea.MouseButtonWheelRight
		clampedScreenY := msg.Y
		clampedLocalY := localY
		if m.chat.viewport.Height > 0 {
			minScreenY := headerHeight
			maxScreenY := headerHeight + m.chat.viewport.Height - 1
			if clampedScreenY < minScreenY {
				clampedScreenY = minScreenY
			}
			if clampedScreenY > maxScreenY {
				clampedScreenY = maxScreenY
			}
			if clampedLocalY < 0 {
				clampedLocalY = 0
			}
			if clampedLocalY >= m.chat.viewport.Height {
				clampedLocalY = m.chat.viewport.Height - 1
			}
		} else {
			clampedLocalY = 0
		}

		// Handle text selection with left mouse button.
		switch {
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && transcriptHit:
			row, col, coord, ok := m.screenToSelectionPos(msg.X, msg.Y)
			if ok {
				m.chat.selection = TextSelection{
					Active:     true,
					StartRow:   row,
					StartCol:   col,
					EndRow:     row,
					EndCol:     col,
					StartCoord: coord,
					EndCoord:   coord,
				}
				appendCmd(&cmds, m.updateViewportContent())
			}
			return m, nil

		case msg.Action == tea.MouseActionMotion && m.chat.selection.Active:
			// Clamp to transcript bounds so drag updates survive header/footer crossings.
			row, col, coord, ok := m.screenToSelectionPos(msg.X, clampedScreenY)
			if ok {
				m.chat.selection.EndRow = row
				m.chat.selection.EndCol = col
				m.chat.selection.EndCoord = coord
				appendCmd(&cmds, m.updateViewportContent())
			}
			return m, nil

		case msg.Action == tea.MouseActionRelease && m.chat.selection.Active:
			// Finish selection even if the pointer is released outside transcript bounds.
			row, col, coord, ok := m.screenToSelectionPos(msg.X, clampedScreenY)
			if ok {
				m.chat.selection.EndRow = row
				m.chat.selection.EndCol = col
				m.chat.selection.EndCoord = coord
			}
			m.chat.selection.Active = false

			text := m.extractSelectedText()
			if text != "" {
				m.chat.selection.HasRange = true
				if errMsg := copyToClipboard(text); errMsg != "" {
					m.setNotice(components.NoticeError, errMsg)
				} else {
					m.setNotice(components.NoticeSuccess, "Copied selection ✓")
				}
				appendCmd(&cmds, m.updateViewportContent())
				return m, clearStatusAfterDelay()
			}
			m.chat.selection = TextSelection{}
			appendCmd(&cmds, m.updateViewportContent())
			return m, nil
		}

		// Clear selection on non-selection clicks.
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && !transcriptHit {
			m.clearSelection()
		}

		// Forward mouse events to viewport for scroll support, including wheel
		// events above the persistent header or below the transcript.
		if transcriptHit || isWheel {
			if m.usesVirtualTranscript() && isWheel {
				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.scrollVirtualTranscriptBy(-3)
				case tea.MouseButtonWheelDown:
					m.scrollVirtualTranscriptBy(3)
				}
				return m, nil
			}
			localMsg := msg
			localMsg.Y = clampedLocalY
			var cmd tea.Cmd
			m.chat.viewport, cmd = m.chat.viewport.Update(localMsg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m.syncScrollModeFromViewport()
			m.refreshVirtualTranscriptAfterScroll()
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case initialWindowSizeMsg:
		prevWidth, _, changed := m.applyWindowSize(msg.Width, msg.Height, msg.Source)
		if !changed {
			return m, nil
		}
		m.syncGeometryToControls()
		if m.isFullscreenMode() {
			m.markFullscreenFrameDirty()
			return m, m.recalcFullscreenLayoutAndViewport(prevWidth)
		}
		return m, m.recalcLayoutAndViewport(false)

	case tea.WindowSizeMsg:
		prevGeometry := m.geometry
		prevWidth, prevHeight, changed := m.applyWindowSize(msg.Width, msg.Height, geometrySourceWindow)
		if !changed {
			return m, nil
		}
		m.syncGeometryToControls()

		var cmds []tea.Cmd
		if m.features.OverlayStack && !m.overlays.IsEmpty() {
			overlayCmd, _ := m.overlays.RouteMsg(msg)
			appendCmd(&cmds, overlayCmd)
		}

		if m.isFullscreenMode() {
			m.markFullscreenFrameDirty()
			appendCmd(&cmds, m.recalcFullscreenLayoutAndViewport(prevWidth))
		} else {
			invalidateWrapCache := false
			if prevGeometry.Valid && (prevWidth != m.width || prevHeight != m.height) {
				invalidateWrapCache = true
			}
			appendCmd(&cmds, m.recalcLayoutAndViewport(invalidateWrapCache))
		}
		return m, tea.Batch(cmds...)

	case modelListLoadedMsg:
		if m.features.OverlayStack && m.overlays != nil && !m.overlays.IsEmpty() {
			if top := m.overlays.Top(); top != nil && top.ID() == "model-select" {
				updated, _, cmd := top.Update(msg)
				m.overlays.ReplaceTop(updated)
				appendCmd(&cmds, cmd)
				appendCmd(&cmds, m.recalcLayoutAndViewport(false))
				return m, tea.Batch(cmds...)
			}
		}
		if m.state == stateModelSelection {
			m.applyModelListLoaded(msg)
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
			return m, tea.Batch(cmds...)
		}

	case pubsub.Event[session.Session]:
		// Session updated

	case pubsub.Event[message.Message]:
		// Filter: only process events for the current session
		if msg.Payload.SessionID != "" && msg.Payload.SessionID != m.sessionID {
			break
		}
		if msg.Type == pubsub.CreatedEvent || msg.Type == pubsub.UpdatedEvent {
			payload := msg.Payload
			layoutDirty := false
			if payload.Role == message.Tool {
				for _, tr := range payload.ToolResults() {
					m.chat.toolMessages[tr.ToolCallID] = payload
				}
			}

			// Wave 2: Drive ProcessingState transitions from message events
			if m.status.isProcessing && payload.Role == message.Assistant {
				content := payload.Content().String()
				toolCalls := payload.ToolCalls()

				// Count tool states
				var runningTool string
				runningCount := 0
				queuedCount := 0
				finishedCount := 0
				for _, tc := range toolCalls {
					switch tc.EffectiveState() {
					case message.ToolCallRunning:
						runningCount++
						if runningTool == "" {
							runningTool = tc.Name
						}
					case message.ToolCallQueued:
						queuedCount++
					case message.ToolCallCompleted, message.ToolCallErrored, message.ToolCallCanceled:
						finishedCount++
					}
				}
				hasRunning := runningCount > 0
				hasQueued := queuedCount > 0
				allFinished := len(toolCalls) > 0 && finishedCount == len(toolCalls)

				switch {
				case hasRunning && m.status.processing.Phase != PhaseToolRunning:
					layoutDirty = true
					m.status.processing = ProcessingState{
						Phase:       PhaseToolRunning,
						ActiveTool:  runningTool,
						StartedAt:   time.Now(),
						QueuedCount: queuedCount,
						TotalTools:  len(toolCalls),
					}
				case !hasRunning && hasQueued && m.status.processing.Phase != PhaseToolQueued:
					// Tools known but none started → PhaseToolQueued
					layoutDirty = true
					m.status.processing = ProcessingState{
						Phase:       PhaseToolQueued,
						Label:       "queued",
						StartedAt:   time.Now(),
						QueuedCount: queuedCount,
						TotalTools:  len(toolCalls),
					}
				case content != "" && !hasRunning && !hasQueued &&
					(m.status.processing.Phase == PhaseThinking || m.status.processing.Phase == PhaseToolRunning || m.status.processing.Phase == PhaseToolQueued):
					// Content received → PhaseStreaming
					layoutDirty = true
					m.status.processing = ProcessingState{
						Phase:     PhaseStreaming,
						Label:     "streaming",
						StartedAt: time.Now(),
					}
					m.status.telemetry.LastTokenAt = time.Now()
					m.status.telemetry.LastTokenCount = len([]rune(content)) / 4
				case allFinished && content == "" && m.status.processing.Phase == PhaseToolRunning:
					// All tools finished but no content yet → back to thinking
					layoutDirty = true
					m.status.processing = ProcessingState{
						Phase:     PhaseThinking,
						Label:     "thinking",
						StartedAt: time.Now(),
					}
				}
			}

			// Wave 4: Update token telemetry during streaming
			if m.status.processing.Phase == PhaseStreaming && payload.Role == message.Assistant {
				newContent := payload.Content().String()
				newTokenEst := len([]rune(newContent)) / 4
				if newTokenEst > m.status.telemetry.LastTokenCount {
					m.status.telemetry.LastTokenAt = time.Now()
					m.status.telemetry.LastTokenCount = newTokenEst
				}
			}

			m.chat.messages = updateMessages(m.chat.messages, payload)
			// Refresh transcript deterministically for every message event so
			// main-screen and fullscreen stay in sync with the same message source.
			m.chat.dirtyMessageIDs[payload.ID] = true
			m.chat.ticksSinceLastMsg = 0
			if layoutDirty {
				appendCmd(&cmds, m.recalcLayoutAndViewport(false))
			} else {
				appendCmd(&cmds, m.updateViewportContent())
			}
			m.chat.dirtyMessageIDs = make(map[string]bool)
			m.ensureTicking(&cmds)
		}

	case pubsub.Event[permission.PermissionRequest]:
		if msg.Type == pubsub.CreatedEvent {
			perm := msg.Payload
			if m.features.OverlayStack {
				m.overlays.Push(NewPermissionOverlay(&perm))
			} else {
				m.dlg.perm.pending = &perm
				m.dlg.perm.optionIdx = 0
				m.state = statePermission
			}
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
		}

	case pubsub.Event[tools.ClarificationEvent]:
		if msg.Type == pubsub.CreatedEvent {
			event := msg.Payload
			if m.features.OverlayStack {
				m.overlays.Push(NewClarificationOverlay(&event))
			} else {
				m.dlg.clarification.pending = &event
				m.dlg.clarification.idx = 0
				m.dlg.clarification.input = ""
				m.dlg.clarification.focused = false
				m.state = stateClarification
			}
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
		}

	case pubsub.Event[tools.PlanApprovalEvent]:
		if msg.Type == pubsub.CreatedEvent {
			event := msg.Payload
			if event.SessionID != "" && event.SessionID != m.sessionID {
				break
			}
			if m.features.OverlayStack {
				m.overlays.Push(NewPlanApprovalOverlay(&event))
			} else {
				m.dlg.planApproval.pending = &event
				m.dlg.planApproval.optionIdx = 0
				m.dlg.planApproval.feedback = ""
				m.dlg.planApproval.inputMode = false
				m.state = statePlanApproval
			}
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
		}

	case pubsub.Event[tools.CheckpointEvent]:
		if msg.Type == pubsub.CreatedEvent {
			event := msg.Payload
			if m.features.OverlayStack {
				m.overlays.Push(NewCheckpointOverlay(&event))
			} else {
				m.dlg.checkpoint.pending = &event
				m.dlg.checkpoint.optionIdx = 0
				m.dlg.checkpoint.feedback = ""
				m.dlg.checkpoint.inputMode = false
				m.state = stateCheckpoint
			}
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
		}

	case pubsub.Event[research.ResearchEvent]:
		// Wave 4: Immediately sync pipeline progress on research events
		if m.status.mode == "research" && m.status.researchPipelineID != "" {
			if msg.Payload.PipelineID == m.status.researchPipelineID {
				m.syncPipelineProgressLayoutChanged()
				appendCmd(&cmds, m.updateViewportContent())
			}
		}

	case pubsub.Event[agent.AgentEvent]:
		// Filter: only process events for the current session
		if msg.Payload.SessionID != "" && msg.Payload.SessionID != m.sessionID {
			break
		}
		if msg.Payload.ToolLifecycle != nil {
			m.applyToolLifecycle(*msg.Payload.ToolLifecycle)
			m.ensureTicking(&cmds)
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
		}
		if msg.Payload.Type == agent.AgentEventTypeCompacting {
			m.status.isCompacting = true
			m.clearRuntimeError()
			m.setNotice(components.NoticeInfo, "Compacting conversation history...")
			// Wave 2: Transition to compacting phase
			m.status.processing = ProcessingState{
				Phase:     PhaseCompacting,
				Label:     "compacting",
				StartedAt: time.Now(),
			}
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
		} else if msg.Payload.Type == agent.AgentEventTypeCompactDone {
			m.status.isCompacting = false
			m.setNotice(components.NoticeSuccess, "Context compacted ✓")
			m.refreshSessionSummaryBoundary()
			// Wave 2: Compaction done — if still processing, the next message event
			// will drive the correct phase transition; reset to Thinking as safe default.
			if m.status.isProcessing {
				m.status.processing = ProcessingState{
					Phase:     PhaseThinking,
					Label:     "thinking",
					StartedAt: time.Now(),
				}
			} else {
				m.status.processing = ProcessingState{Phase: PhaseIdle}
			}
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
		} else if msg.Payload.Error != nil {
			// Error check BEFORE Done check: events may carry both Error and Done=true
			summary, detail := agent.ErrorDisplay(msg.Payload.Error, msg.Payload.TerminalReason)
			if summary == "" {
				summary = "Request failed. Press Ctrl+O for details."
			}
			if detail == "" {
				detail = fmt.Sprintf("%v", msg.Payload.Error)
			}
			m.status.runtimeErr = RuntimeErrorState{
				Summary: summary,
				Detail:  detail,
			}
			m.setNotice(components.NoticeError, summary)
			m.status.isProcessing = false
			m.status.isCompacting = false
			m.status.ticking = false
			m.status.processing = ProcessingState{Phase: PhaseIdle}
			m.status.toolRuntime = nil
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
		} else if msg.Payload.Done {
			m.status.isProcessing = false
			m.status.isCompacting = false
			m.status.ticking = false
			m.status.processing = ProcessingState{Phase: PhaseIdle}
			m.status.toolRuntime = nil
			if msg.Payload.TerminalReason == agent.ReasonCompleted {
				m.clearRuntimeError()
			}
			if msg.Payload.Warning != "" {
				m.setNotice(components.NoticeWarning, msg.Payload.Warning)
			} else {
				m.clearNotice()
			}
			appendCmd(&cmds, m.recalcLayoutAndViewport(false))
		}

	case loadSessionMsg:
		m.resetSessionState()
		m.sessionID = msg.session.ID
		m.refreshBackgroundTaskCount()
		m.sessionSummaryMessageID = msg.session.SummaryMessageID
		m.applySessionModeToUI(msg.session)
		m.chat.messages = msg.messages
		m.chat.toolMessages = msg.toolMessages
		m.pinLoadedTranscriptTail()
		// Update viewport with loaded session messages
		appendCmd(&cmds, m.updateViewportContent())

	case sessionListMsg:
		if m.features.OverlayStack {
			m.overlays.Push(NewSessionBrowserOverlay([]session.Session(msg)))
		} else {
			m.dlg.sessionBrowser.list = []session.Session(msg)
			m.dlg.sessionBrowser.listIdx = 0
			m.dlg.sessionBrowser.listScroll = 0
			m.state = stateSessionBrowser
		}
		appendCmd(&cmds, m.recalcLayoutAndViewport(false))

	case fileIndexMsg:
		m.composer.fileIndexRefreshing = false
		if msg.root != "" && m.composer.fileIndexRoot != "" &&
			filepath.Clean(msg.root) != filepath.Clean(m.composer.fileIndexRoot) {
			return m, tea.Batch(cmds...)
		}
		m.composer.fileIndexRoot = msg.root
		m.composer.fileIndex = msg.entries
		m.syncComposerPickersWithOptions(&cmds, true, false)
		skipComposerPickerSync = true

	case pubsub.Event[app.MemoryEvent]:
		// Wave 3: Memory update event → toast notification + update MEM pill
		// Filter: only process events for the current session
		if m.features.MemoryUI && (msg.Payload.SessionID == "" || msg.Payload.SessionID == m.sessionID) {
			memEvent := msg.Payload
			m.status.memoryToast.Push(components.MemoryToast{
				FilePath:  memEvent.FilePath,
				Action:    memEvent.Action,
				CreatedAt: time.Now(),
			})
			// Refresh actual memory file count from disk
			m.status.memoryCount = len(m.collectMemoryFileEntries())
			m.ensureTicking(&cmds)
		}

	case pubsub.Event[task.RegistryEvent]:
		m.refreshBackgroundTaskCount()
	}

	// Recalculate layout if input height changed
	currentInputHeight := m.composer.input.Height()
	if currentInputHeight != m.composer.prevInputHeight {
		m.composer.prevInputHeight = currentInputHeight
		appendCmd(&cmds, m.recalcLayoutAndViewport(false))
	}

	if !skipComposerPickerSync {
		m.syncComposerPickers(&cmds)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) syncComposerPickers(cmds *[]tea.Cmd) {
	m.syncComposerPickersWithOptions(cmds, false, true)
}

func (m *Model) syncComposerPickersWithOptions(cmds *[]tea.Cmd, forceFileRefresh bool, queueFileRefresh bool) {
	hasBlockingOverlay := m.features.OverlayStack && m.overlays != nil && m.overlays.HasBlocking()
	inChat := m.state == stateChat && !hasBlockingOverlay
	if !inChat || m.status.isProcessing {
		return
	}
	value := m.composer.input.Value()
	if strings.HasPrefix(value, "/") {
		if m.dispatcher != nil && m.dispatcher.Registry() != nil {
			_, matched := m.dispatcher.Registry().CompleteInvocation(value, m.composer.input.CursorOffset())
			m.composer.filteredCompletions = matched
			m.composer.commandPickerActive = len(matched) > 0
			if m.composer.commandPickerIdx >= len(matched) {
				m.composer.commandPickerIdx = 0
			}
		} else {
			m.composer.filteredCompletions = nil
			m.composer.commandPickerActive = false
		}
	} else {
		m.composer.commandPickerActive = false
	}
	if m.composer.commandPickerActive {
		if m.composer.filePicker != nil && m.composer.filePicker.Active {
			m.composer.filePicker.Reset()
		}
		return
	}
	if atQuery, ok := extractAtQuery(value); ok {
		if m.composer.filePicker == nil {
			m.composer.filePicker = picker.New(8)
		}
		if m.composer.fileIndex == nil {
			cwd, _ := os.Getwd()
			m.composer.fileIndexRoot = cwd
			m.composer.fileIndex = picker.IndexFilesShallow(cwd, picker.DefaultIndexConfig())
			m.composer.filePicker.UpdateQuery(atQuery, m.composer.fileIndex)
		}
		if queueFileRefresh && !m.composer.fileIndexRefreshing {
			m.composer.fileIndexRefreshing = true
			appendCmd(cmds, loadFileIndexCmd(m.composer.fileIndexRoot))
		}
		if forceFileRefresh || atQuery != m.composer.filePicker.Query || !m.composer.filePicker.Active {
			m.composer.filePicker.UpdateQuery(atQuery, m.composer.fileIndex)
		}
		return
	}
	if m.composer.filePicker != nil && m.composer.filePicker.Active {
		m.composer.filePicker.Reset()
	}
}

func updateMessages(existing []message.Message, updated message.Message) []message.Message {
	for i, msg := range existing {
		if msg.ID == updated.ID {
			existing[i] = updated
			return existing
		}
	}
	return append(existing, updated)
}

func (m *Model) applyToolLifecycle(ev agent.ToolLifecycleEvent) {
	if m.status.toolRuntime == nil {
		m.status.toolRuntime = make(map[string]ToolRuntimeState)
	}

	// New batch start signal: first queued item in a batch.
	if ev.State == message.ToolCallQueued && ev.Order == 0 {
		for id, rt := range m.status.toolRuntime {
			if rt.BatchID == ev.BatchID {
				delete(m.status.toolRuntime, id)
			}
		}
	}

	m.status.toolRuntime[ev.ToolCallID] = ToolRuntimeState{
		ToolName:      ev.ToolName,
		BatchID:       ev.BatchID,
		Input:         ev.Input,
		State:         ev.State,
		Order:         ev.Order,
		Total:         ev.Total,
		DurationMs:    ev.DurationMs,
		ResultSummary: ev.ResultSummary,
	}

	presenter := components.GetPresenter(ev.ToolName)
	displayName := presenter.DisplayName()

	queuedCount := 0
	runningCount := 0
	finishedCount := 0
	for _, rt := range m.status.toolRuntime {
		if rt.BatchID != ev.BatchID {
			continue
		}
		switch rt.State {
		case message.ToolCallQueued:
			queuedCount++
		case message.ToolCallRunning:
			runningCount++
		case message.ToolCallCompleted, message.ToolCallErrored, message.ToolCallCanceled:
			finishedCount++
		}
	}

	allDone := ev.Total > 0 && finishedCount >= ev.Total
	groupLabel := components.FormatToolBatchLabel(ev.ToolName, ev.Total, finishedCount, allDone)
	intent := components.FormatToolDetail(ev.ToolName, ev.Input, ev.ResultSummary, string(ev.State))

	switch {
	case ev.State == message.ToolCallQueued || queuedCount > 0:
		m.status.processing = ProcessingState{
			Phase:       PhaseToolQueued,
			Label:       groupLabel,
			StartedAt:   time.Now(),
			QueuedCount: queuedCount,
			TotalTools:  ev.Total,
		}
	case ev.State == message.ToolCallRunning || runningCount > 0:
		m.status.processing = ProcessingState{
			Phase:       PhaseToolRunning,
			Label:       intent,
			ActiveTool:  displayName,
			StartedAt:   time.Now(),
			QueuedCount: queuedCount,
			TotalTools:  ev.Total,
		}
		m.status.telemetry.ActiveToolStartedAt = time.Now()
	case allDone:
		m.status.processing = ProcessingState{
			Phase:     PhaseThinking,
			Label:     "thinking",
			StartedAt: time.Now(),
		}
		for id, rt := range m.status.toolRuntime {
			if rt.BatchID == ev.BatchID {
				delete(m.status.toolRuntime, id)
			}
		}
	}
}

func (m *Model) syncPipelineProgressLayoutChanged() bool {
	prevRail := m.renderProgressRail()
	m.syncPipelineProgress()
	if m.renderProgressRail() == prevRail {
		return false
	}
	m.recalcLayout()
	return true
}
