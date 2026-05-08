package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/tui/components"
)

type commandCompletionAcceptResult struct {
	Applied      bool
	ChangedInput bool
	CursorWasEnd bool
	ExactNoop    bool
}

func (m *Model) acceptCommandCompletion(item command.CompletionItem, addTrailingSpace bool) commandCompletionAcceptResult {
	current := m.composer.input.Value()
	cursor := m.composer.input.CursorOffset()
	result := commandCompletionAcceptResult{
		CursorWasEnd: cursor == len(current),
	}
	switch item.Kind {
	case "subcommand":
		inv := command.ParseInvocation(current, cursor)
		if inv.Command == "" {
			return result
		}
		next, nextCursor, ok := replaceFirstArgAtCursor(current, cursor, item.Subcommand, addTrailingSpace)
		if !ok {
			next = "/" + inv.Command + " " + item.Subcommand
			nextCursor = len(next)
			if addTrailingSpace {
				next += " "
				nextCursor = len(next)
			}
		}
		result.Applied = true
		result.ChangedInput = next != current || nextCursor != cursor
		result.ExactNoop = !result.ChangedInput
		m.composer.input.SetValueWithCursor(next, nextCursor)
	default:
		next, nextCursor, ok := replaceCommandAtCursor(current, cursor, item.Value, addTrailingSpace)
		if !ok {
			next = "/" + item.Value
			nextCursor = len(next)
			if addTrailingSpace {
				next += " "
				nextCursor = len(next)
			}
		}
		result.Applied = true
		result.ChangedInput = next != current || nextCursor != cursor
		result.ExactNoop = !result.ChangedInput
		m.composer.input.SetValueWithCursor(next, nextCursor)
	}
	return result
}

func replaceCommandAtCursor(input string, cursor int, replacement string, addTrailingSpace bool) (string, int, bool) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(input) {
		cursor = len(input)
	}
	if input == "" || input[0] != '/' {
		return "", 0, false
	}
	tokenStart := 1
	tokenEnd := tokenStart
	for tokenEnd < len(input) && input[tokenEnd] != ' ' {
		tokenEnd++
	}
	if cursor < tokenStart || cursor > tokenEnd {
		return "", 0, false
	}
	next := input[:tokenStart] + replacement + input[tokenEnd:]
	nextCursor := tokenStart + len(replacement)
	if addTrailingSpace {
		if tokenEnd < len(input) && input[tokenEnd] == ' ' {
			nextCursor++
		} else {
			next = input[:tokenStart] + replacement + " " + input[tokenEnd:]
			nextCursor++
		}
	}
	return next, nextCursor, true
}

func replaceFirstArgAtCursor(input string, cursor int, replacement string, addTrailingSpace bool) (string, int, bool) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(input) {
		cursor = len(input)
	}
	if input == "" || input[0] != '/' {
		return "", 0, false
	}
	commandEndRel := strings.IndexByte(input[1:], ' ')
	if commandEndRel < 0 {
		return "", 0, false
	}
	argsStart := commandEndRel + 2
	if cursor < argsStart {
		return "", 0, false
	}

	firstTokenStart := argsStart
	for firstTokenStart < len(input) && input[firstTokenStart] == ' ' {
		firstTokenStart++
	}
	if cursor <= firstTokenStart {
		end := cursor
		for end < len(input) && input[end] == ' ' {
			end++
		}
		insert := replacement
		cursorAfterReplacement := cursor + len(replacement)
		if addTrailingSpace || end < len(input) {
			insert += " "
		}
		if addTrailingSpace {
			cursorAfterReplacement++
		}
		return input[:cursor] + insert + input[end:], cursorAfterReplacement, true
	}

	tokenStart := firstTokenStart
	tokenEnd := tokenStart
	for tokenEnd < len(input) && input[tokenEnd] != ' ' {
		tokenEnd++
	}
	if cursor < tokenStart || cursor > tokenEnd {
		return "", 0, false
	}
	next := input[:tokenStart] + replacement + input[tokenEnd:]
	nextCursor := tokenStart + len(replacement)
	if addTrailingSpace {
		if tokenEnd < len(input) && input[tokenEnd] == ' ' {
			nextCursor++
		} else {
			next = input[:tokenStart] + replacement + " " + input[tokenEnd:]
			nextCursor++
		}
	}
	return next, nextCursor, true
}

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Clear text selection on any key press
	m.clearSelection()

	// Phase 4: Overlay stack key routing (feature-gated).
	// Non-blocking overlays (Help, CopyMode) only consume Esc; all other keys
	// pass through to the chat layer. Blocking overlays consume all keys.
	if m.features.OverlayStack && !m.overlays.IsEmpty() {
		top := m.overlays.Top()
		topID := top.ID()

		// Non-blocking overlays: only route Esc, let other keys fall through
		if top.Kind() == OverlayNonBlocking && msg.Type != tea.KeyEsc {
			// Key passes through to chat layer below
		} else {
			updated, result, cmd := top.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if result != nil {
				// Overlay completed — pop and process side effects
				m.overlays.Pop()
				m2, resultCmd := m.processOverlayResult(topID, result)
				_ = m2 // m is a pointer receiver, already mutated
				if resultCmd != nil {
					cmds = append(cmds, resultCmd)
				}
				m.recalcLayout()
				return m, tea.Batch(cmds...)
			}
			// Overlay consumed the key but didn't finish
			m.overlays.ReplaceTop(updated)
			if top.Kind() == OverlayBlocking {
				m.recalcLayout()
				return m, tea.Batch(cmds...)
			}
		}
	}

	// History search active: delegate all keys
	if m.search.historySearch.Visible() {
		var cmd tea.Cmd
		m.search.historySearch, cmd = m.search.historySearch.Update(msg)
		if msg.Type == tea.KeyEnter {
			if selected, ok := m.search.historySearch.Selected(); ok {
				m.composer.input.SetValue(selected)
			}
			m.search.historySearch.Hide()
			m.recalcLayout()
			return m, m.composer.input.Focus()
		}
		if !m.search.historySearch.Visible() {
			// Esc was pressed
			m.recalcLayout()
			return m, m.composer.input.Focus()
		}
		return m, cmd
	}

	// Text search active: delegate all keys
	if m.search.textSearch.Visible() {
		prevQuery := m.search.textSearch.Query()
		var cmd tea.Cmd
		m.search.textSearch, cmd = m.search.textSearch.Update(msg)
		if !m.search.textSearch.Visible() {
			// Search closed (Esc) — if vim is enabled, keep matches for n/N
			// navigation and restore normal mode.
			if m.vim.Enabled() && m.search.textSearch.HasMatches() {
				// Matches were preserved by HideKeepMatches via Update→Hide,
				// but Update calls Hide() which clears them. We need to
				// re-search to preserve matches for vim n/N.
				m.search.textSearch.Search(m.chat.messages)
				m.vim.EnterNormal()
				m.recalcLayout()
				return m, m.updateViewportContent()
			}
			m.chat.scrollMode = ScrollAutoFollow
			m.recalcLayout()
			return m, tea.Batch(m.updateViewportContent(), m.composer.input.Focus())
		}
		// Only re-search when query text actually changes (not on Enter/navigation)
		if m.search.textSearch.Query() != prevQuery {
			m.search.textSearch.Search(m.chat.messages)
		}
		// Scroll to current match
		if match, ok := m.search.textSearch.CurrentMatch(); ok {
			m.scrollToMessage(match.MessageIdx)
		}
		return m, cmd
	}

	// Config wizard owns blocking keys before transient composer UI handling.
	// Keep processing Esc cancellation priority.
	if m.hasCommandDialog() {
		return m, m.dismissCommandDialog()
	}

	// Config wizard owns blocking keys before transient composer UI handling.
	// Keep processing Esc cancellation priority.
	if m.state == stateConfigWizard {
		if msg.Type == tea.KeyEsc && m.status.isProcessing && m.sessionID != "" {
			m.app.CoderAgent.Cancel(m.sessionID)
			m.status.isProcessing = false
			m.status.ticking = false
			m.status.processing = ProcessingState{Phase: PhaseIdle}
			m.setNotice(components.NoticeWarning, "Cancelled")
			return m, nil
		}
		return m.handleConfigWizardKey(msg)
	}

	// Dismiss legacy command output on any key and restore input focus
	if m.composer.commandOutput != "" {
		m.composer.commandOutput = ""
		m.recalcLayout()
		return m, m.composer.input.Focus()
	}

	// Any key other than Ctrl+C resets the Ctrl+C confirmation
	if msg.Type != tea.KeyCtrlC {
		m.status.ctrlCPending = false
	}

	// Command picker key handling
	if m.composer.commandPickerActive {
		switch msg.Type {
		case tea.KeyUp:
			if m.composer.commandPickerIdx > 0 {
				m.composer.commandPickerIdx--
			}
			return m, nil
		case tea.KeyDown:
			if m.composer.commandPickerIdx < len(m.composer.filteredCompletions)-1 {
				m.composer.commandPickerIdx++
			}
			return m, nil
		case tea.KeyEnter:
			if m.composer.commandPickerIdx < len(m.composer.filteredCompletions) {
				selected := m.composer.filteredCompletions[m.composer.commandPickerIdx]
				acceptResult := m.acceptCommandCompletion(selected, false)
				m.composer.commandPickerActive = false
				if acceptResult.Applied && acceptResult.ExactNoop && acceptResult.CursorWasEnd {
					return m.sendMessage()
				}
			}
			return m, nil
		case tea.KeyTab:
			if m.composer.commandPickerIdx < len(m.composer.filteredCompletions) {
				selected := m.composer.filteredCompletions[m.composer.commandPickerIdx]
				_ = m.acceptCommandCompletion(selected, true)
				m.composer.commandPickerActive = false
			}
			return m, nil
		case tea.KeyEsc, tea.KeyCtrlC:
			m.composer.commandPickerActive = false
			return m, nil
		}
	}

	// File picker key handling (@ mentions)
	if m.composer.filePicker != nil && m.composer.filePicker.Active && len(m.composer.filePicker.Items) > 0 {
		switch msg.Type {
		case tea.KeyUp:
			m.composer.filePicker.MoveUp()
			return m, nil
		case tea.KeyDown:
			m.composer.filePicker.MoveDown()
			return m, nil
		case tea.KeyEnter, tea.KeyTab:
			entry, isDir := m.composer.filePicker.Select()
			if isDir {
				m.replaceAtQuery(entry.RelPath + "/")
				// Re-trigger search with new path prefix
				if atQuery, ok := extractAtQuery(m.composer.input.Value()); ok {
					m.composer.filePicker.UpdateQuery(atQuery, m.composer.fileIndex)
				}
			} else {
				m.replaceAtQuery(entry.RelPath + " ")
				m.composer.filePicker.Reset()
			}
			return m, nil
		case tea.KeyEsc:
			m.composer.filePicker.Reset()
			return m, nil
		}
	}

	// Phase 6: Vim normal mode intercept.
	// When vim is enabled and in normal mode, HandleKey processes navigation keys
	// before the main switch block to prevent them from reaching the textarea.
	if m.state == stateChat && (msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown) {
		return m, m.applyViewportKeyScroll(msg)
	}

	if m.vim.IsNormalMode() && m.state == stateChat {
		action, consumed := m.vim.HandleKey(msg)
		if consumed {
			cmd := m.executeVimAction(action)
			return m, cmd
		}
	}

	switch {
	// Shift+Tab: in permission state, accept and enter auto mode (Always=index 2)
	case msg.Type == tea.KeyShiftTab && m.state == statePermission && m.dlg.perm.pending != nil:
		m.dlg.perm.optionIdx = 2
		m.transitionSessionMode(permission.ModeAuto)
		return m.executePermissionChoice()

	// Shift+Tab: cycle mode (default → auto → plan → research → default)
	case msg.Type == tea.KeyShiftTab:
		m.cycleSessionModeFromUserInput()
		return m, nil

	// Copy mode: Esc exits (must be before the generic Esc handler)
	case msg.Type == tea.KeyEsc && m.state == stateCopyMode:
		return m.handleCopyModeKey(msg)

	// Escape: cancel processing, or enter vim normal mode
	case msg.Type == tea.KeyEsc:
		if m.status.isProcessing && m.sessionID != "" {
			m.app.CoderAgent.Cancel(m.sessionID)
			m.status.isProcessing = false
			m.status.ticking = false
			m.status.processing = ProcessingState{Phase: PhaseIdle}
			m.setNotice(components.NoticeWarning, "Cancelled")
			return m, nil
		}
		// Enter vim normal mode if enabled and not processing
		if m.vim.Enabled() && m.state == stateChat {
			m.vim.EnterNormal()
			return m, nil
		}
		return m, nil

	// Ctrl+C: cancel processing, or double-press to quit
	case msg.Type == tea.KeyCtrlC:
		if m.status.isProcessing && m.sessionID != "" {
			m.app.CoderAgent.Cancel(m.sessionID)
			m.status.isProcessing = false
			m.status.ticking = false
			m.status.processing = ProcessingState{Phase: PhaseIdle}
			m.setNotice(components.NoticeWarning, "Cancelled")
			m.status.ctrlCPending = false
			return m, nil
		}
		if m.status.ctrlCPending {
			return m, tea.Quit
		}
		m.status.ctrlCPending = true
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return ctrlCResetMsg{}
		})

	// Ctrl+L: redraw screen (alt-screen mode)
	case msg.Type == tea.KeyCtrlL:
		return m, tea.ClearScreen

	case m.state == statePlanApproval && m.dlg.planApproval.pending != nil:
		return m.handlePlanApprovalKey(msg)

	// Init wizard state
	case m.state == stateInitWizard:
		return m.handleInitWizardKey(msg)

	// Research suggestion state
	case m.state == stateResearchSuggestion:
		return m.handleResearchSuggestionKey(msg)

	// Workspace selection state
	case m.state == stateWorkspaceSelection:
		return m.handleWorkspaceSelectionKey(msg)

	// Init required state: arrow keys and enter
	case m.state == stateInitRequired:
		switch msg.Type {
		case tea.KeyUp, tea.KeyLeft:
			if m.dlg.initWizard.choiceIdx > 0 {
				m.dlg.initWizard.choiceIdx--
			}
			return m, nil
		case tea.KeyDown, tea.KeyRight:
			if m.dlg.initWizard.choiceIdx < 1 {
				m.dlg.initWizard.choiceIdx++
			}
			return m, nil
		case tea.KeyEnter:
			return m.executeInitChoice()
		case tea.KeyEsc:
			// Escape = skip
			m.dlg.initWizard.choiceIdx = 1
			return m.executeInitChoice()
		}
		switch msg.String() {
		case "1":
			m.dlg.initWizard.choiceIdx = 0
			return m.executeInitChoice()
		case "2":
			m.dlg.initWizard.choiceIdx = 1
			return m.executeInitChoice()
		}
		return m, nil

	// Model selection state: arrow keys, enter, esc
	case m.state == stateModelSelection:
		return m.handleModelSelectionKey(msg)

	// Clarification state: arrow keys, tab, enter, esc
	case m.state == stateClarification && m.dlg.clarification.pending != nil:
		return m.handleClarificationKey(msg)

	// Checkpoint confirmation state
	case m.state == stateCheckpoint && m.dlg.checkpoint.pending != nil:
		return m.handleCheckpointKey(msg)

	// Template selection state
	case m.state == stateTemplateSelection:
		return m.handleTemplateSelectionKey(msg)

	// Permission state: arrow keys and enter
	case m.state == statePermission && m.dlg.perm.pending != nil:
		switch msg.Type {
		case tea.KeyLeft, tea.KeyUp:
			if m.dlg.perm.optionIdx > 0 {
				m.dlg.perm.optionIdx--
			}
			return m, nil
		case tea.KeyRight, tea.KeyDown:
			if m.dlg.perm.optionIdx < 2 {
				m.dlg.perm.optionIdx++
			}
			return m, nil
		case tea.KeyEnter:
			return m.executePermissionChoice()
		}
		// CC alignment: y/n/a shortcuts match inline order (Yes=0, No=1, Always=2)
		switch msg.String() {
		case "y", "Y":
			m.dlg.perm.optionIdx = 0
			return m.executePermissionChoice()
		case "n", "N":
			m.dlg.perm.optionIdx = 1
			return m.executePermissionChoice()
		case "a", "A":
			m.dlg.perm.optionIdx = 2
			return m.executePermissionChoice()
		}
		return m, nil

	// Up arrow: history navigation (when in chat, not processing, single-line input)
	case msg.Type == tea.KeyUp && m.state == stateChat && !m.status.isProcessing:
		if !strings.Contains(m.composer.input.Value(), "\n") {
			m.composer.input.HistoryUp()
			return m, nil
		}

	// Down arrow: history navigation
	case msg.Type == tea.KeyDown && m.state == stateChat && !m.status.isProcessing:
		if !strings.Contains(m.composer.input.Value(), "\n") {
			m.composer.input.HistoryDown()
			return m, nil
		}

	// Enter in chat mode: send message (Alt+Enter inserts newline)
	case msg.Type == tea.KeyEnter && m.state == stateChat && !m.status.isProcessing:
		if msg.Alt {
			// Alt+Enter: let textarea handle it for newline insertion
			break // falls through to textarea Update below
		}
		return m.sendMessage()

	// Block Enter from reaching textarea while processing (prevents phantom newlines)
	case msg.Type == tea.KeyEnter && m.state == stateChat && m.status.isProcessing:
		return m, nil

	// Help state: any key closes (legacy path — overlay handles this in OverlayStack mode)
	case m.state == stateHelp && !m.features.OverlayStack:
		m.state = stateChat
		m.recalcLayout()
		cmd := m.composer.input.Focus()
		return m, cmd

	// Session browser state
	case m.state == stateSessionBrowser:
		return m.handleSessionBrowserKey(msg)

	// Ctrl+R: history search (chat state, not processing)
	case msg.Type == tea.KeyCtrlR && m.state == stateChat && !m.status.isProcessing:
		// Collect history from input
		var entries []string
		for _, msg := range m.chat.messages {
			if msg.Role == message.User {
				for _, part := range msg.Parts {
					if tc, ok := part.(message.TextContent); ok && strings.TrimSpace(tc.Text) != "" {
						entries = append(entries, tc.Text)
					}
				}
			}
		}
		if len(entries) == 0 {
			return m, nil
		}
		// Reverse: most recent first
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
		m.search.historySearch.SetEntries(entries)
		m.search.historySearch.SetWidth(m.width)
		cmd := m.search.historySearch.Show()
		m.recalcLayout()
		return m, cmd

	// Ctrl+F: text search (chat state)
	case msg.Type == tea.KeyCtrlF && m.state == stateChat && !m.status.isProcessing:
		m.search.textSearch.SetWidth(m.width)
		cmd := m.search.textSearch.Show()
		m.recalcLayout()
		return m, cmd

	// ? key: show help when input is empty and in chat state
	case msg.Type == tea.KeyRunes && msg.String() == "?" && m.state == stateChat && !m.status.isProcessing:
		if strings.TrimSpace(m.composer.input.Value()) == "" {
			if m.features.OverlayStack {
				m.pushHelpOverlay()
			} else {
				m.state = stateHelp
				m.recalcLayout()
			}
			return m, nil
		}

	// Ctrl+P: open session browser
	case msg.Type == tea.KeyCtrlP && m.state == stateChat && !m.status.isProcessing:
		return m, m.fetchSessionListCmd()

	// Ctrl+N: new session
	case msg.Type == tea.KeyCtrlN && m.state == stateChat && !m.status.isProcessing:
		m.sessionID = ""
		m.resetSessionState()
		return m, m.updateViewportContent()

	// Ctrl+J: navigate to next expandable item
	case msg.Type == tea.KeyCtrlJ && m.state == stateChat:
		m.navigateToolCall(1)
		return m, m.updateViewportContent()

	// Ctrl+K: navigate to previous expandable item
	case msg.Type == tea.KeyCtrlK && m.state == stateChat:
		m.navigateToolCall(-1)
		return m, m.updateViewportContent()

	// Copy mode state: handle selection keys
	case m.state == stateCopyMode:
		return m.handleCopyModeKey(msg)

	// Ctrl+Y: copy last assistant response (or focused item) to clipboard
	case msg.Type == tea.KeyCtrlY && m.state == stateChat:
		var text string
		if m.chat.focusedToolCallID != "" {
			text = m.extractFocusedContent()
		}
		if text == "" {
			text = m.extractLastAssistantText()
		}
		if text == "" {
			m.setNotice(components.NoticeWarning, "No content to copy")
			return m, clearStatusAfterDelay()
		}
		if errMsg := copyToClipboard(text); errMsg != "" {
			m.setNotice(components.NoticeError, errMsg)
		} else {
			m.setNotice(components.NoticeSuccess, "Copied to clipboard ✓")
		}
		return m, clearStatusAfterDelay()

	// Ctrl+B: copy code block(s) from last assistant response
	case msg.Type == tea.KeyCtrlB && m.state == stateChat && !m.status.isProcessing:
		text := m.extractLastAssistantText()
		blocks := extractCodeBlocks(text)
		if len(blocks) == 0 {
			m.setNotice(components.NoticeWarning, "No code blocks found")
			return m, clearStatusAfterDelay()
		}
		if len(blocks) == 1 {
			if errMsg := copyToClipboard(blocks[0].Content); errMsg != "" {
				m.setNotice(components.NoticeError, errMsg)
			} else {
				m.setNotice(components.NoticeSuccess, "Copied code block ✓")
			}
			return m, clearStatusAfterDelay()
		}
		if m.features.OverlayStack {
			m.pushCopyModeOverlay(blocks)
		} else {
			m.chat.copyModeBlocks = blocks
			m.state = stateCopyMode
			m.recalcLayout()
		}
		return m, nil

	// Toggle tool call expand with Ctrl+O
	case msg.Type == tea.KeyCtrlO && m.state == stateChat:
		return m, m.togglePrimaryExpansion()

	}

	// Pass to input (only events not handled above reach the textarea)
	beforeLayout, trackLayout := m.captureFullscreenLayoutSnapshot()
	cmd := m.composer.input.Update(msg)
	cmds = append(cmds, cmd)
	m.syncComposerPickers(&cmds)
	appendCmd(&cmds, m.relayoutFullscreenIfChanged(beforeLayout, trackLayout))

	return m, tea.Batch(cmds...)
}

// handleCopyModeKey handles key events in copy mode (code block selection).
func (m *Model) handleCopyModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	exitCopyMode := func() {
		m.state = stateChat
		m.chat.copyModeBlocks = nil
		m.recalcLayout()
	}

	switch {
	case msg.Type == tea.KeyEsc:
		exitCopyMode()
		return m, m.composer.input.Focus()

	case msg.Type == tea.KeyRunes && msg.String() == "a":
		// Copy all blocks concatenated
		var all strings.Builder
		for i, b := range m.chat.copyModeBlocks {
			if i > 0 {
				all.WriteString("\n\n")
			}
			all.WriteString(b.Content)
		}
		if errMsg := copyToClipboard(all.String()); errMsg != "" {
			m.setNotice(components.NoticeError, errMsg)
		} else {
			m.setNotice(components.NoticeSuccess, "Copied all code blocks ✓")
		}
		exitCopyMode()
		return m, tea.Batch(m.composer.input.Focus(), clearStatusAfterDelay())

	case msg.Type == tea.KeyRunes:
		r := msg.String()
		if len(r) == 1 && r[0] >= '1' && r[0] <= '9' {
			idx := int(r[0] - '1')
			if idx < len(m.chat.copyModeBlocks) {
				if errMsg := copyToClipboard(m.chat.copyModeBlocks[idx].Content); errMsg != "" {
					m.setNotice(components.NoticeError, errMsg)
				} else {
					m.setNotice(components.NoticeSuccess, "Copied code block ✓")
				}
			}
			exitCopyMode()
			return m, tea.Batch(m.composer.input.Focus(), clearStatusAfterDelay())
		}
		// Unknown key: exit copy mode
		exitCopyMode()
		return m, m.composer.input.Focus()

	default:
		exitCopyMode()
		return m, m.composer.input.Focus()
	}
}

// executeVimAction translates a VimAction returned by VimMode.HandleKey into
// the corresponding viewport mutation or mode switch. Returns a tea.Cmd if needed.
func (m *Model) executeVimAction(action VimAction) tea.Cmd {
	switch action {
	case VimActionScrollDown:
		return m.applyViewportKeyScroll(tea.KeyMsg{Type: tea.KeyDown})
	case VimActionScrollUp:
		return m.applyViewportKeyScroll(tea.KeyMsg{Type: tea.KeyUp})
	case VimActionGotoTop:
		if m.usesVirtualTranscript() {
			m.syncBlockList()
			m.chat.virtualList.SetAutoFollow(false)
			m.chat.virtualList.SetAnchor(components.AnchorForBlock(m.chat.blockList.All(), 0, 0))
			m.chat.scrollMode = ScrollManualLocked
			_ = m.updateViewportContent()
			return nil
		}
		m.chat.viewport.GotoTop()
		m.chat.scrollMode = ScrollManualLocked
		m.refreshVirtualTranscriptAfterScroll()
	case VimActionGotoBottom:
		if m.usesVirtualTranscript() {
			m.syncBlockList()
			m.chat.virtualList.ScrollToBottom()
			m.chat.scrollMode = ScrollAutoFollow
			_ = m.updateViewportContent()
			return nil
		}
		m.chat.viewport.GotoBottom()
		m.chat.scrollMode = ScrollAutoFollow
		m.refreshVirtualTranscriptAfterScroll()
	case VimActionHalfPageDown:
		m.applyViewportHalfPageDown()
	case VimActionHalfPageUp:
		m.applyViewportHalfPageUp()
	case VimActionSearch:
		// Trigger text search (same as Ctrl+F), exit vim normal mode
		// so the search input can receive keystrokes.
		m.vim.EnterInsert()
		m.search.textSearch.SetWidth(m.width)
		cmd := m.search.textSearch.Show()
		m.recalcLayout()
		return cmd
	case VimActionNextMatch:
		// Navigate to next match (works even after search UI is hidden)
		if m.search.textSearch.HasMatches() {
			if match, ok := m.search.textSearch.NextMatch(); ok {
				m.scrollToMessage(match.MessageIdx)
			}
		}
	case VimActionPrevMatch:
		// Navigate to previous match (works even after search UI is hidden)
		if m.search.textSearch.HasMatches() {
			if match, ok := m.search.textSearch.PrevMatch(); ok {
				m.scrollToMessage(match.MessageIdx)
			}
		}
	case VimActionToggleFold:
		return m.togglePrimaryExpansion()
	case VimActionYankBlock:
		var text string
		if m.chat.focusedToolCallID != "" {
			text = m.extractFocusedContent()
		}
		if text == "" {
			text = m.extractLastAssistantText()
		}
		if text == "" {
			m.setNotice(components.NoticeWarning, "No content to copy")
			return clearStatusAfterDelay()
		}
		if errMsg := copyToClipboard(text); errMsg != "" {
			m.setNotice(components.NoticeError, errMsg)
		} else {
			m.setNotice(components.NoticeSuccess, "Copied to clipboard ✓")
		}
		return clearStatusAfterDelay()
	case VimActionInsertMode:
		m.vim.EnterInsert()
		return m.composer.input.Focus()
	}

	// Update scroll mode based on final position
	m.syncScrollModeFromViewport()
	return nil
}

func (m *Model) hasPendingRuntimeError() bool {
	return strings.TrimSpace(m.status.runtimeErr.Detail) != "" &&
		m.status.notice.Kind == components.NoticeError &&
		strings.TrimSpace(m.status.notice.Text) == strings.TrimSpace(m.status.runtimeErr.Summary)
}

func (m *Model) togglePrimaryExpansion() tea.Cmd {
	if m.hasPendingRuntimeError() {
		m.status.runtimeErr.Expanded = !m.status.runtimeErr.Expanded
		return m.recalcLayoutAndViewport(false)
	}
	if m.chat.focusedToolCallID != "" {
		m.cycleExpandState(m.chat.focusedToolCallID)
		return m.updateViewportContent()
	}
	return m.toggleTranscriptView()
}
