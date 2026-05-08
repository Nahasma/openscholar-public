package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/picker"
)

func (m *Model) executePermissionChoice() (tea.Model, tea.Cmd) {
	if m.dlg.perm.pending == nil {
		return m, nil
	}
	perm := *m.dlg.perm.pending
	// CC alignment: inline options order is y=Yes(0), n=No(1), a=Always(2)
	switch m.dlg.perm.optionIdx {
	case 0: // Yes — Allow (this session)
		m.app.Permissions.Grant(perm)
	case 1: // No — Deny
		m.app.Permissions.Deny(perm)
	case 2: // Always — Always Allow (persist to DB)
		m.app.Permissions.GrantAlways(perm)
		m.transitionSessionMode(permission.ModeAuto)
	}

	m.dlg.perm.pending = nil
	m.state = stateChat
	focusCmd := m.composer.input.Focus()
	return m, tea.Batch(m.updateViewportContent(), focusCmd)
}

func (m *Model) handlePlanApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	event := m.dlg.planApproval.pending
	if event == nil {
		return m, nil
	}

	if m.dlg.planApproval.inputMode {
		switch msg.Type {
		case tea.KeyTab:
			m.dlg.planApproval.inputMode = false
			return m, nil
		case tea.KeyEnter:
			return m.executePlanApprovalChoice()
		case tea.KeyEsc:
			return m.rejectPlanApproval()
		case tea.KeyBackspace:
			runes := []rune(m.dlg.planApproval.feedback)
			if len(runes) > 0 {
				m.dlg.planApproval.feedback = string(runes[:len(runes)-1])
			}
			return m, nil
		default:
			if len(msg.String()) == 1 || msg.Type == tea.KeySpace {
				ch := msg.String()
				if msg.Type == tea.KeySpace {
					ch = " "
				}
				m.dlg.planApproval.feedback += ch
			}
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyUp, tea.KeyLeft:
		if m.dlg.planApproval.optionIdx > 0 {
			m.dlg.planApproval.optionIdx--
		}
		return m, nil
	case tea.KeyDown, tea.KeyRight:
		if m.dlg.planApproval.optionIdx < 3 {
			m.dlg.planApproval.optionIdx++
		}
		return m, nil
	case tea.KeyTab:
		if m.dlg.planApproval.optionIdx == 2 || m.dlg.planApproval.optionIdx == 3 {
			m.dlg.planApproval.inputMode = true
		}
		return m, nil
	case tea.KeyEnter:
		if m.dlg.planApproval.optionIdx == 2 || m.dlg.planApproval.optionIdx == 3 {
			m.dlg.planApproval.inputMode = true
			return m, nil
		}
		return m.executePlanApprovalChoice()
	case tea.KeyEsc:
		return m.rejectPlanApproval()
	}

	switch msg.String() {
	case "1", "y", "Y":
		m.dlg.planApproval.optionIdx = 0
		return m.executePlanApprovalChoice()
	case "2", "a", "A":
		m.dlg.planApproval.optionIdx = 1
		return m.executePlanApprovalChoice()
	case "3", "n", "N":
		m.dlg.planApproval.optionIdx = 2
		m.dlg.planApproval.inputMode = true
		return m, nil
	case "4", "e", "E":
		m.dlg.planApproval.optionIdx = 3
		m.dlg.planApproval.inputMode = true
		return m, nil
	}

	return m, nil
}

func (m *Model) executePlanApprovalChoice() (tea.Model, tea.Cmd) {
	event := m.dlg.planApproval.pending
	if event == nil {
		return m, nil
	}

	resp := tools.PlanApprovalResponse{}
	switch m.dlg.planApproval.optionIdx {
	case 0:
		resp.Approved = true
		resp.TargetMode = permission.ModeRestore
	case 1:
		resp.Approved = true
		resp.TargetMode = permission.ModeAuto
	case 2:
		resp.Approved = false
		resp.Feedback = m.dlg.planApproval.feedback
	case 3:
		resp.Approved = true
		resp.TargetMode = permission.ModeRestore
		resp.EditedPlan = m.dlg.planApproval.feedback
	}

	select {
	case event.ResponseCh <- resp:
	default:
	}
	m.applyPlanApprovalUIState(event.SessionID, resp)
	m.dlg.planApproval.Reset()
	m.state = stateChat
	return m, m.composer.input.Focus()
}

func (m *Model) rejectPlanApproval() (tea.Model, tea.Cmd) {
	event := m.dlg.planApproval.pending
	if event == nil {
		return m, nil
	}
	resp := tools.PlanApprovalResponse{
		Approved: false,
		Feedback: m.dlg.planApproval.feedback,
	}
	select {
	case event.ResponseCh <- resp:
	default:
	}
	m.dlg.planApproval.Reset()
	m.state = stateChat
	return m, m.composer.input.Focus()
}

func (m *Model) applyPlanApprovalUIState(sessionID string, resp tools.PlanApprovalResponse) {
	targetSessionID := sessionID
	if targetSessionID == "" {
		targetSessionID = m.sessionID
	}
	if targetSessionID != "" && m.sessionID != "" && targetSessionID != m.sessionID {
		return
	}
	if !resp.Approved {
		m.status.mode = "plan"
		return
	}
	target := resp.TargetMode
	if target == permission.ModeRestore && m.app != nil && m.app.Permissions != nil && targetSessionID != "" {
		state := m.app.Permissions.SessionModeState(targetSessionID)
		target = state.PrePlanMode
		if target == permission.ModeDefault && state.Mode != permission.ModePlan {
			target = state.Mode
		}
	}
	if target == permission.ModeRestore || target == permission.ModePlan {
		target = permission.ModeDefault
	}
	m.setPermissionModeStatus(target)
	m.persistSessionMode(targetSessionID, target)
}

func (m *Model) handleClarificationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	event := m.dlg.clarification.pending
	if event == nil {
		return m, nil
	}

	if m.dlg.clarification.focused {
		// In freeform input mode
		switch msg.Type {
		case tea.KeyTab:
			m.dlg.clarification.focused = false
			return m, nil
		case tea.KeyEnter:
			if m.dlg.clarification.input != "" {
				return m.executeClarificationChoice()
			}
			return m, nil
		case tea.KeyEsc:
			return m.skipClarification()
		case tea.KeyBackspace:
			runes := []rune(m.dlg.clarification.input)
			if len(runes) > 0 {
				m.dlg.clarification.input = string(runes[:len(runes)-1])
			}
			return m, nil
		default:
			if len(msg.String()) == 1 || msg.Type == tea.KeySpace {
				ch := msg.String()
				if msg.Type == tea.KeySpace {
					ch = " "
				}
				m.dlg.clarification.input += ch
			}
			return m, nil
		}
	}

	// In option selection mode
	switch msg.Type {
	case tea.KeyUp:
		if m.dlg.clarification.idx > 0 {
			m.dlg.clarification.idx--
		}
		return m, nil
	case tea.KeyDown:
		if m.dlg.clarification.idx < len(event.Options)-1 {
			m.dlg.clarification.idx++
		}
		return m, nil
	case tea.KeyTab:
		if event.AllowFreeform {
			m.dlg.clarification.focused = true
		}
		return m, nil
	case tea.KeyEnter:
		return m.executeClarificationChoice()
	case tea.KeyEsc:
		return m.skipClarification()
	}

	// Number shortcuts
	switch msg.String() {
	case "1", "2", "3", "4", "5":
		idx := int(msg.String()[0]-'0') - 1
		if idx < len(event.Options) {
			m.dlg.clarification.idx = idx
			return m.executeClarificationChoice()
		}
	}

	return m, nil
}

func (m *Model) executeClarificationChoice() (tea.Model, tea.Cmd) {
	event := m.dlg.clarification.pending
	if event == nil {
		return m, nil
	}

	var resp tools.ClarificationResponse
	if m.dlg.clarification.focused && m.dlg.clarification.input != "" {
		resp = tools.ClarificationResponse{
			SelectedIndex: -1,
			SelectedText:  m.dlg.clarification.input,
		}
	} else {
		resp = tools.ClarificationResponse{
			SelectedIndex: m.dlg.clarification.idx,
			SelectedText:  event.Options[m.dlg.clarification.idx],
		}
	}

	// Send response (non-blocking)
	select {
	case event.ResponseCh <- resp:
	default:
	}

	m.dlg.clarification.pending = nil
	m.state = stateChat
	return m, m.composer.input.Focus()
}

func (m *Model) skipClarification() (tea.Model, tea.Cmd) {
	event := m.dlg.clarification.pending
	if event == nil {
		return m, nil
	}

	select {
	case event.ResponseCh <- tools.ClarificationResponse{Skipped: true}:
	default:
	}

	m.dlg.clarification.pending = nil
	m.state = stateChat
	return m, m.composer.input.Focus()
}

// --- Checkpoint confirmation handlers ---

func (m *Model) handleCheckpointKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	event := m.dlg.checkpoint.pending
	if event == nil {
		return m, nil
	}

	if m.dlg.checkpoint.inputMode {
		// In rejection feedback input mode
		switch msg.Type {
		case tea.KeyTab:
			m.dlg.checkpoint.inputMode = false
			return m, nil
		case tea.KeyEnter:
			if m.dlg.checkpoint.feedback != "" {
				return m.executeCheckpointChoice(false)
			}
			return m, nil
		case tea.KeyEsc:
			return m.executeCheckpointChoice(false)
		case tea.KeyBackspace:
			runes := []rune(m.dlg.checkpoint.feedback)
			if len(runes) > 0 {
				m.dlg.checkpoint.feedback = string(runes[:len(runes)-1])
			}
			return m, nil
		default:
			if len(msg.String()) == 1 || msg.Type == tea.KeySpace {
				ch := msg.String()
				if msg.Type == tea.KeySpace {
					ch = " "
				}
				m.dlg.checkpoint.feedback += ch
			}
			return m, nil
		}
	}

	// In option selection mode
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown:
		m.dlg.checkpoint.optionIdx = 1 - m.dlg.checkpoint.optionIdx // toggle 0↔1
		return m, nil
	case tea.KeyTab:
		if m.dlg.checkpoint.optionIdx == 1 { // Reject selected
			m.dlg.checkpoint.inputMode = true
		}
		return m, nil
	case tea.KeyEnter:
		return m.executeCheckpointChoice(m.dlg.checkpoint.optionIdx == 0)
	case tea.KeyEsc:
		return m.executeCheckpointChoice(false) // reject
	}

	// Shortcut keys
	switch msg.String() {
	case "y", "Y":
		return m.executeCheckpointChoice(true)
	case "n", "N":
		m.dlg.checkpoint.optionIdx = 1
		m.dlg.checkpoint.inputMode = true
		return m, nil
	}

	return m, nil
}

func (m *Model) executeCheckpointChoice(approved bool) (tea.Model, tea.Cmd) {
	event := m.dlg.checkpoint.pending
	if event == nil {
		return m, nil
	}

	resp := tools.CheckpointResponse{
		Approved: approved,
		Feedback: m.dlg.checkpoint.feedback,
	}

	select {
	case event.ResponseCh <- resp:
	default:
	}

	m.dlg.checkpoint.pending = nil
	m.state = stateChat
	return m, m.composer.input.Focus()
}

// --- Research suggestion handler ---

func (m *Model) handleResearchSuggestionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp, tea.KeyLeft:
		if m.dlg.researchSuggest.idx > 0 {
			m.dlg.researchSuggest.idx--
		}
		return m, nil
	case tea.KeyDown, tea.KeyRight:
		if m.dlg.researchSuggest.idx < 1 {
			m.dlg.researchSuggest.idx++
		}
		return m, nil
	case tea.KeyEnter:
		text := m.dlg.researchSuggest.text
		m.dlg.researchSuggest.text = ""
		if m.dlg.researchSuggest.idx == 0 {
			// 切换到研究模式 → show template selection
			m.dlg.template.options = []string{"empirical", "aris_empirical", "survey", "theoretical"}
			m.dlg.template.idx = 0
			m.dlg.template.folderName = ""
			m.dlg.template.picker = picker.New(8)
			m.dlg.template.fileIndex = nil
			m.dlg.template.pendingText = text
			m.state = stateTemplateSelection
			return m, nil
		}
		// 继续普通模式
		m.state = stateChat
		return m.sendToAgent(text)
	case tea.KeyEsc:
		// Escape = continue in normal mode
		text := m.dlg.researchSuggest.text
		m.dlg.researchSuggest.text = ""
		m.state = stateChat
		return m.sendToAgent(text)
	}
	switch msg.String() {
	case "1", "y":
		m.dlg.researchSuggest.idx = 0
		msg.Type = tea.KeyEnter
		return m.handleResearchSuggestionKey(msg)
	case "2", "n":
		m.dlg.researchSuggest.idx = 1
		msg.Type = tea.KeyEnter
		return m.handleResearchSuggestionKey(msg)
	}
	return m, nil
}

// --- Session browser handler ---

func (m *Model) handleSessionBrowserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.state = stateChat
		return m, m.composer.input.Focus()

	case tea.KeyUp:
		if m.dlg.sessionBrowser.listIdx > 0 {
			m.dlg.sessionBrowser.listIdx--
		}
		return m, nil

	case tea.KeyDown:
		if m.dlg.sessionBrowser.listIdx < len(m.dlg.sessionBrowser.list)-1 {
			m.dlg.sessionBrowser.listIdx++
		}
		return m, nil

	case tea.KeyEnter:
		if len(m.dlg.sessionBrowser.list) > 0 && m.dlg.sessionBrowser.listIdx < len(m.dlg.sessionBrowser.list) {
			sess := m.dlg.sessionBrowser.list[m.dlg.sessionBrowser.listIdx]
			m.state = stateChat
			return m, tea.Batch(m.composer.input.Focus(), m.loadSessionByIDCmd(sess.ID))
		}
		return m, nil
	}

	switch msg.String() {
	case "n":
		m.state = stateChat
		m.sessionID = ""
		m.resetSessionState()
		return m, tea.Batch(m.updateViewportContent(), m.composer.input.Focus())

	case "d":
		if len(m.dlg.sessionBrowser.list) > 0 && m.dlg.sessionBrowser.listIdx < len(m.dlg.sessionBrowser.list) {
			sess := m.dlg.sessionBrowser.list[m.dlg.sessionBrowser.listIdx]
			_ = m.app.Sessions.Delete(m.ctx, sess.ID)
			// If we deleted the current session, reset
			if sess.ID == m.sessionID {
				m.sessionID = ""
				m.resetSessionState()
			}
			// Re-fetch list
			return m, m.fetchSessionListCmd()
		}
	}

	return m, nil
}
