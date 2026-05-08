package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/config"
)

// --- Config Wizard ---

func (m *Model) startConfigWizard() {
	cfg := config.Get()
	m.dlg.configWizard.wizard = config.NewConfigWizard(cfg)
	m.dlg.configWizard.idx = 0
	m.dlg.configWizard.menuIdx = 0
	m.dlg.configWizard.input = ""
	m.dlg.configWizard.cursor = 0
	m.composer.commandOutput = ""
	m.composer.commandDialog = CommandDialogState{}
	m.composer.commandPickerActive = false
	m.composer.commandPickerIdx = 0
	m.composer.filteredCompletions = nil
	if m.composer.filePicker != nil {
		m.composer.filePicker.Reset()
	}
	m.state = stateConfigWizard
}

func (m *Model) handleConfigWizardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.dlg.configWizard.wizard == nil {
		return m.finishConfigWizard(false)
	}

	// Ctrl+C cancels entirely
	if msg.Type == tea.KeyCtrlC {
		return m.finishConfigWizard(false)
	}

	step := m.dlg.configWizard.wizard.CurrentStep()
	if step == nil {
		if m.dlg.configWizard.wizard.IsDone() {
			return m.finishConfigWizard(true)
		}
		return m.finishConfigWizard(false)
	}

	switch step.Type {
	case config.CWStepProviderMenu, config.CWStepAgentMenu:
		return m.handleConfigMenuSelect(msg)

	case config.CWStepProviderKey, config.CWStepProviderBaseURL:
		return m.handleConfigTextInput(msg)

	case config.CWStepDefaultProvider:
		return m.handleConfigSelectSkipOnEsc(msg)

	case config.CWStepAgentProvider, config.CWStepAgentModel:
		return m.handleConfigSelectBackOnEsc(msg)

	case config.CWStepSummary:
		switch msg.Type {
		case tea.KeyEnter:
			if m.dlg.configWizard.wizard.HasChanges() {
				m.dlg.configWizard.wizard.Apply("save")
				return m.finishConfigWizard(true)
			}
			return m.finishConfigWizard(false)
		case tea.KeyEsc:
			return m.finishConfigWizard(false)
		}
	}

	return m, nil
}

func (m *Model) handleConfigMenuSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	step := m.dlg.configWizard.wizard.CurrentStep()
	if step == nil {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.dlg.configWizard.idx > 0 {
			m.dlg.configWizard.idx--
		}
		return m, nil
	case tea.KeyDown:
		if m.dlg.configWizard.idx < len(step.Options)-1 {
			m.dlg.configWizard.idx++
		}
		return m, nil
	case tea.KeyEnter:
		if m.dlg.configWizard.idx < len(step.Options) {
			opt := step.Options[m.dlg.configWizard.idx]
			if opt.Value == "continue" {
				m.dlg.configWizard.wizard.Apply("continue")
				m.dlg.configWizard.idx = 0
			} else {
				// Drill into item — save menu position, reset idx for sub-view
				m.dlg.configWizard.menuIdx = m.dlg.configWizard.idx
				m.dlg.configWizard.wizard.Apply(opt.Value)
				m.dlg.configWizard.idx = 0
				m.dlg.configWizard.input = ""
				m.dlg.configWizard.cursor = 0
			}
			if m.dlg.configWizard.wizard.IsDone() || m.dlg.configWizard.wizard.IsCancelled() {
				return m.finishConfigWizard(m.dlg.configWizard.wizard.IsDone())
			}
		}
		return m, nil
	case tea.KeyEsc:
		return m.finishConfigWizard(false)
	}

	return m, nil
}

func (m *Model) handleConfigTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	runes := []rune(m.dlg.configWizard.input)
	cur := m.dlg.configWizard.cursor
	if cur > len(runes) {
		cur = len(runes)
	}

	switch msg.Type {
	case tea.KeyEnter:
		m.dlg.configWizard.wizard.Apply(m.dlg.configWizard.input)
		m.dlg.configWizard.input = ""
		m.dlg.configWizard.cursor = 0
		m.dlg.configWizard.idx = m.dlg.configWizard.menuIdx
		if m.dlg.configWizard.wizard.IsDone() || m.dlg.configWizard.wizard.IsCancelled() {
			return m.finishConfigWizard(m.dlg.configWizard.wizard.IsDone())
		}
		return m, nil
	case tea.KeyEsc:
		m.dlg.configWizard.wizard.Back()
		m.dlg.configWizard.input = ""
		m.dlg.configWizard.cursor = 0
		m.dlg.configWizard.idx = m.dlg.configWizard.menuIdx
		if m.dlg.configWizard.wizard.IsDone() || m.dlg.configWizard.wizard.IsCancelled() {
			return m.finishConfigWizard(m.dlg.configWizard.wizard.IsDone())
		}
		return m, nil
	case tea.KeyLeft:
		if cur > 0 {
			m.dlg.configWizard.cursor = cur - 1
		}
		return m, nil
	case tea.KeyRight:
		if cur < len(runes) {
			m.dlg.configWizard.cursor = cur + 1
		}
		return m, nil
	case tea.KeyHome, tea.KeyCtrlA:
		m.dlg.configWizard.cursor = 0
		return m, nil
	case tea.KeyEnd, tea.KeyCtrlE:
		m.dlg.configWizard.cursor = len(runes)
		return m, nil
	case tea.KeyBackspace:
		if cur > 0 {
			m.dlg.configWizard.input = string(runes[:cur-1]) + string(runes[cur:])
			m.dlg.configWizard.cursor = cur - 1
		}
		return m, nil
	case tea.KeyDelete:
		if cur < len(runes) {
			m.dlg.configWizard.input = string(runes[:cur]) + string(runes[cur+1:])
		}
		return m, nil
	default:
		var insert string
		if msg.Type == tea.KeyRunes {
			insert = string(msg.Runes)
		} else if msg.Type == tea.KeySpace {
			insert = " "
		}
		if insert != "" {
			m.dlg.configWizard.input = string(runes[:cur]) + insert + string(runes[cur:])
			m.dlg.configWizard.cursor = cur + len([]rune(insert))
		}
		return m, nil
	}
}

// handleConfigSelectSkipOnEsc handles select steps where Esc skips (keeps existing value).
// Used for DefaultProvider selection.
func (m *Model) handleConfigSelectSkipOnEsc(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	step := m.dlg.configWizard.wizard.CurrentStep()
	if step == nil {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.dlg.configWizard.idx > 0 {
			m.dlg.configWizard.idx--
		}
		return m, nil
	case tea.KeyDown:
		if m.dlg.configWizard.idx < len(step.Options)-1 {
			m.dlg.configWizard.idx++
		}
		return m, nil
	case tea.KeyEnter:
		if m.dlg.configWizard.idx < len(step.Options) {
			m.dlg.configWizard.wizard.Apply(step.Options[m.dlg.configWizard.idx].Value)
			m.dlg.configWizard.idx = 0
			if m.dlg.configWizard.wizard.IsDone() || m.dlg.configWizard.wizard.IsCancelled() {
				return m.finishConfigWizard(m.dlg.configWizard.wizard.IsDone())
			}
		}
		return m, nil
	case tea.KeyEsc:
		m.dlg.configWizard.wizard.Skip()
		m.dlg.configWizard.idx = 0
		if m.dlg.configWizard.wizard.IsDone() || m.dlg.configWizard.wizard.IsCancelled() {
			return m.finishConfigWizard(m.dlg.configWizard.wizard.IsDone())
		}
		return m, nil
	}

	return m, nil
}

// handleConfigSelectBackOnEsc handles select steps where Esc goes back to parent.
// Used for AgentProvider and AgentModel selection.
func (m *Model) handleConfigSelectBackOnEsc(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	step := m.dlg.configWizard.wizard.CurrentStep()
	if step == nil {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.dlg.configWizard.idx > 0 {
			m.dlg.configWizard.idx--
		}
		return m, nil
	case tea.KeyDown:
		if m.dlg.configWizard.idx < len(step.Options)-1 {
			m.dlg.configWizard.idx++
		}
		return m, nil
	case tea.KeyEnter:
		if m.dlg.configWizard.idx < len(step.Options) {
			m.dlg.configWizard.wizard.Apply(step.Options[m.dlg.configWizard.idx].Value)
			// Check if we returned to a menu after this apply
			nextStep := m.dlg.configWizard.wizard.CurrentStep()
			if nextStep != nil && (nextStep.Type == config.CWStepProviderMenu || nextStep.Type == config.CWStepAgentMenu) {
				m.dlg.configWizard.idx = m.dlg.configWizard.menuIdx
			} else {
				m.dlg.configWizard.idx = 0
			}
			if m.dlg.configWizard.wizard.IsDone() || m.dlg.configWizard.wizard.IsCancelled() {
				return m.finishConfigWizard(m.dlg.configWizard.wizard.IsDone())
			}
		}
		return m, nil
	case tea.KeyEsc:
		m.dlg.configWizard.wizard.Back()
		// Restore menu position if returning to menu
		nextStep := m.dlg.configWizard.wizard.CurrentStep()
		if nextStep != nil && (nextStep.Type == config.CWStepProviderMenu || nextStep.Type == config.CWStepAgentMenu) {
			m.dlg.configWizard.idx = m.dlg.configWizard.menuIdx
		} else {
			m.dlg.configWizard.idx = 0
		}
		if m.dlg.configWizard.wizard.IsDone() || m.dlg.configWizard.wizard.IsCancelled() {
			return m.finishConfigWizard(m.dlg.configWizard.wizard.IsDone())
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) finishConfigWizard(save bool) (tea.Model, tea.Cmd) {
	summary := "Config wizard dismissed"
	if save && m.dlg.configWizard.wizard != nil && m.dlg.configWizard.wizard.HasChanges() {
		newCfg := m.dlg.configWizard.wizard.BuildConfig()
		m.composer.commandOutput, summary, _ = m.saveFullAndApplyRuntimeConfig(newCfg)
	} else if !save {
		m.composer.commandOutput = "配置向导已取消"
	} else {
		m.composer.commandOutput = "未做任何更改"
	}
	if invocation := strings.TrimSpace(m.composer.pendingCommandActivity.Invocation); invocation != "" {
		m.appendCommandActivity(invocation, summary)
	}
	m.composer.pendingCommandActivity = CommandActivityState{}

	m.dlg.configWizard.wizard = nil
	m.state = stateChat
	m.recalcLayout()
	return m, tea.Batch(m.updateViewportContent(), m.composer.input.Focus())
}
