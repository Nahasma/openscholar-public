package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/config"
	initwizard "github.com/openscholar/openscholar/internal/init"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/picker"
	"github.com/openscholar/openscholar/internal/research"
	"github.com/openscholar/openscholar/internal/tui/components"
)

// --- Template selection handlers ---

func (m *Model) handleTemplateSelectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pk := m.dlg.template.picker

	// @ picker active: intercept navigation keys
	if pk != nil && pk.Active && len(pk.Items) > 0 {
		switch msg.Type {
		case tea.KeyUp:
			pk.MoveUp()
			return m, nil
		case tea.KeyDown:
			pk.MoveDown()
			return m, nil
		case tea.KeyEnter, tea.KeyTab:
			entry, isDir := pk.Select()
			// Replace @query with the selected path
			atIdx := strings.LastIndex(m.dlg.template.folderName, "@")
			if atIdx >= 0 {
				suffix := entry.RelPath
				if isDir {
					suffix += "/"
				}
				m.dlg.template.folderName = m.dlg.template.folderName[:atIdx] + suffix
			}
			if isDir {
				// Continue searching inside selected dir
				m.templatePickerUpdateQuery()
			} else {
				pk.Reset()
			}
			return m, nil
		case tea.KeyEsc:
			pk.Reset()
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyEnter:
		return m.confirmTemplateSelection()

	case tea.KeyEsc:
		m.state = stateChat
		m.dlg.template.pendingText = ""
		return m, m.composer.input.Focus()

	case tea.KeyUp:
		if m.dlg.template.idx > 0 {
			m.dlg.template.idx--
		}
		return m, nil

	case tea.KeyDown:
		if m.dlg.template.idx < len(m.dlg.template.options)-1 {
			m.dlg.template.idx++
		}
		return m, nil

	case tea.KeyBackspace:
		runes := []rune(m.dlg.template.folderName)
		if len(runes) > 0 {
			m.dlg.template.folderName = string(runes[:len(runes)-1])
		}
		m.templatePickerUpdateQuery()
		return m, nil

	default:
		// Text input including paste (KeyRunes can carry multiple chars)
		ch := msg.String()
		if msg.Type == tea.KeySpace {
			ch = " "
		}
		if ch != "" {
			m.dlg.template.folderName += ch
			// Build file index on first @ trigger
			if strings.Contains(ch, "@") && m.dlg.template.fileIndex == nil {
				cwd, _ := os.Getwd()
				m.dlg.template.fileIndex = picker.IndexFiles(cwd, picker.DefaultIndexConfig())
			}
			m.templatePickerUpdateQuery()
		}
		return m, nil
	}
}

// templatePickerUpdateQuery extracts the @query from templateFolderName and updates the picker.
func (m *Model) templatePickerUpdateQuery() {
	if m.dlg.template.picker == nil {
		m.dlg.template.picker = picker.New(8)
	}
	atIdx := strings.LastIndex(m.dlg.template.folderName, "@")
	if atIdx < 0 {
		m.dlg.template.picker.Reset()
		return
	}
	query := m.dlg.template.folderName[atIdx+1:]
	if m.dlg.template.fileIndex == nil {
		cwd, _ := os.Getwd()
		m.dlg.template.fileIndex = picker.IndexFiles(cwd, picker.DefaultIndexConfig())
	}
	m.dlg.template.picker.UpdateQuery(query, m.dlg.template.fileIndex)
}

func (m *Model) confirmTemplateSelection() (tea.Model, tea.Cmd) {
	template := m.dlg.template.options[m.dlg.template.idx]
	topic := m.dlg.template.pendingText
	runes := []rune(topic)
	if len(runes) > 40 {
		topic = string(runes[:40])
	}

	// Work directory: use user input, or default to current directory.
	// Strip leading @ if present (from @ autocomplete).
	workDir := strings.TrimSpace(m.dlg.template.folderName)
	workDir = strings.TrimPrefix(workDir, "@")
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		workDir = "."
	}

	if err := m.ensureSession(); err != nil {
		m.setNotice(components.NoticeError, fmt.Sprintf("Failed to create session: %v", err))
		m.state = stateChat
		m.dlg.template.pendingText = ""
		return m, m.composer.input.Focus()
	}

	p, err := m.app.ResearchEngine.Create(m.sessionID, topic, template, research.ModeDefault, 8, workDir)
	if err != nil {
		m.setNotice(components.NoticeError, fmt.Sprintf("Failed to create pipeline: %v", err))
		m.state = stateChat
		m.dlg.template.pendingText = ""
		return m, m.composer.input.Focus()
	}
	phases, _ := m.app.ResearchEngine.GetPhases(p.ID)
	if len(phases) > 0 {
		if err := m.app.ResearchEngine.StartPhase(p.ID, phases[0].ID); err != nil {
			m.setNotice(components.NoticeError, fmt.Sprintf("Failed to start phase 1: %v", err))
			m.composer.commandOutput = fmt.Sprintf("已创建研究流水线 [%s]，但第 1 阶段启动失败。", p.ID[:8])
			m.dlg.template.pendingText = ""
			m.state = stateChat
			return m, m.composer.input.Focus()
		}
	}
	m.status.researchPipelineID = p.ID
	m.transitionSessionMode(permission.ModeResearch)

	m.composer.commandOutput = fmt.Sprintf("已创建研究流水线 [%s]，第 1 阶段已启动。后台 leader 任务将自动执行该阶段。", p.ID[:8])
	m.dlg.template.pendingText = ""
	m.state = stateChat
	return m, m.composer.input.Focus()
}

// --- Workspace selection handlers ---

func (m *Model) handleWorkspaceSelectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.dlg.workspace.inputMode {
		// Text input mode for custom path
		switch msg.Type {
		case tea.KeyEsc:
			m.dlg.workspace.inputMode = false
			m.dlg.workspace.input = ""
			return m, nil
		case tea.KeyEnter:
			dir := strings.TrimSpace(m.dlg.workspace.input)
			if dir != "" {
				absDir, err := filepath.Abs(dir)
				if err == nil {
					if info, statErr := os.Stat(absDir); statErr == nil && info.IsDir() {
						config.SetWorkingDirectory(absDir)
						m.dlg.workspace.confirmed = true
						m.dlg.workspace.inputMode = false
						return m.finishWorkspaceSelection()
					}
				}
				// Invalid path, stay in input mode
				m.dlg.workspace.input = ""
			}
			return m, nil
		case tea.KeyBackspace:
			if len(m.dlg.workspace.input) > 0 {
				runes := []rune(m.dlg.workspace.input)
				m.dlg.workspace.input = string(runes[:len(runes)-1])
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
				m.dlg.workspace.input += msg.String()
			}
			return m, nil
		}
	}

	// Selection mode
	switch msg.Type {
	case tea.KeyUp, tea.KeyLeft:
		if m.dlg.workspace.idx > 0 {
			m.dlg.workspace.idx--
		}
		return m, nil
	case tea.KeyDown, tea.KeyRight:
		if m.dlg.workspace.idx < 1 {
			m.dlg.workspace.idx++
		}
		return m, nil
	case tea.KeyEnter:
		return m.executeWorkspaceChoice()
	}
	switch msg.String() {
	case "1":
		m.dlg.workspace.idx = 0
		return m.executeWorkspaceChoice()
	case "2":
		m.dlg.workspace.idx = 1
		return m.executeWorkspaceChoice()
	}
	return m, nil
}

func (m *Model) executeWorkspaceChoice() (tea.Model, tea.Cmd) {
	switch m.dlg.workspace.idx {
	case 0: // 使用当前目录
		m.dlg.workspace.confirmed = true
		return m.finishWorkspaceSelection()
	case 1: // 更换目录
		m.dlg.workspace.inputMode = true
		m.dlg.workspace.input = ""
		return m, nil
	}
	return m, nil
}

func (m *Model) finishWorkspaceSelection() (tea.Model, tea.Cmd) {
	// After workspace confirmed, check if init required
	cwd := config.WorkingDirectory()
	profile, _ := initwizard.LoadProfile(cwd)
	if profile == nil {
		m.state = stateInitRequired
		m.dlg.initWizard.choiceIdx = 0
		return m, nil
	}
	m.state = stateChat
	return m, m.composer.input.Focus()
}

// --- Init wizard handlers ---

func (m *Model) executeInitChoice() (tea.Model, tea.Cmd) {
	switch m.dlg.initWizard.choiceIdx {
	case 0: // 开始初始化 → enter init wizard
		m.startInitWizard()
		return m, nil
	case 1: // 跳过
		m.state = stateChat
		return m, m.composer.input.Focus()
	}
	m.state = stateChat
	return m, m.composer.input.Focus()
}

func (m *Model) startInitWizard() {
	m.state = stateInitWizard
	m.dlg.initWizard.wizard = initwizard.NewWizard()
	m.dlg.initWizard.idx = 0
	m.dlg.initWizard.input = ""
}

// wizardStepToDialog converts a Wizard engine step to the UI rendering descriptor.
// For StepSelectWithOther, appends "Other (自定义)" to the option list.
func wizardStepToDialog(step *initwizard.WizardStep) components.InitWizardStep {
	if step.Type == initwizard.StepTextInput {
		return components.InitWizardStep{
			Question:    step.Question,
			IsTextInput: true,
		}
	}
	labels := make([]string, len(step.Options))
	for i, o := range step.Options {
		labels[i] = o.Label
	}
	if step.Type == initwizard.StepSelectWithOther {
		labels = append(labels, "Other (自定义)")
	}
	return components.InitWizardStep{
		Question: step.Question,
		Options:  labels,
	}
}

func (m *Model) handleInitWizardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.dlg.initWizard.wizard == nil || m.dlg.initWizard.wizard.IsDone() {
		return m.finishInitWizard()
	}

	step := m.dlg.initWizard.wizard.CurrentStep()
	if step == nil {
		return m.finishInitWizard()
	}

	// Text input mode: native StepTextInput or "Other" selected in StepSelectWithOther
	if step.Type == initwizard.StepTextInput || m.dlg.initWizard.otherMode {
		switch msg.Type {
		case tea.KeyEnter:
			if m.dlg.initWizard.input != "" {
				m.dlg.initWizard.wizard.Apply(m.dlg.initWizard.input)
				m.dlg.initWizard.input = ""
				m.dlg.initWizard.idx = 0
				m.dlg.initWizard.otherMode = false
				if m.dlg.initWizard.wizard.IsDone() {
					return m.finishInitWizard()
				}
			}
			return m, nil
		case tea.KeyEsc:
			if m.dlg.initWizard.otherMode {
				// Return to option selection
				m.dlg.initWizard.otherMode = false
				m.dlg.initWizard.input = ""
				return m, nil
			}
			m.dlg.initWizard.wizard.Skip()
			m.dlg.initWizard.input = ""
			m.dlg.initWizard.idx = 0
			if m.dlg.initWizard.wizard.IsDone() {
				return m.finishInitWizard()
			}
			return m, nil
		case tea.KeyBackspace:
			runes := []rune(m.dlg.initWizard.input)
			if len(runes) > 0 {
				m.dlg.initWizard.input = string(runes[:len(runes)-1])
			}
			return m, nil
		default:
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
				ch := msg.String()
				if msg.Type == tea.KeySpace {
					ch = " "
				}
				m.dlg.initWizard.input += ch
			}
			return m, nil
		}
	}

	// Option selection mode
	// For StepSelectWithOther, UI shows len(Options)+1 items (last is "Other")
	optCount := len(step.Options)
	uiOptCount := optCount
	if step.Type == initwizard.StepSelectWithOther {
		uiOptCount = optCount + 1 // +1 for "Other"
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.dlg.initWizard.idx > 0 {
			m.dlg.initWizard.idx--
		}
		return m, nil
	case tea.KeyDown:
		if m.dlg.initWizard.idx < uiOptCount-1 {
			m.dlg.initWizard.idx++
		}
		return m, nil
	case tea.KeyEnter:
		return m.applyInitWizardSelection(step, m.dlg.initWizard.idx)
	case tea.KeyEsc:
		m.dlg.initWizard.wizard.Skip()
		m.dlg.initWizard.idx = 0
		if m.dlg.initWizard.wizard.IsDone() {
			return m.finishInitWizard()
		}
		return m, nil
	}

	// Number shortcuts
	s := msg.String()
	if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		idx := int(s[0]-'0') - 1
		if idx < uiOptCount {
			return m.applyInitWizardSelection(step, idx)
		}
	}

	return m, nil
}

// applyInitWizardSelection handles selecting an option in the wizard.
// For StepSelectWithOther, selecting the last index enters text input mode.
func (m *Model) applyInitWizardSelection(step *initwizard.WizardStep, idx int) (tea.Model, tea.Cmd) {
	if step.Type == initwizard.StepSelectWithOther && idx >= len(step.Options) {
		// "Other" selected → switch to text input
		m.dlg.initWizard.otherMode = true
		m.dlg.initWizard.input = ""
		return m, nil
	}

	if idx < len(step.Options) {
		m.dlg.initWizard.wizard.Apply(step.Options[idx].Value)
		m.dlg.initWizard.idx = 0
		if m.dlg.initWizard.wizard.IsDone() {
			return m.finishInitWizard()
		}
	}

	return m, nil
}

func (m *Model) finishInitWizard() (tea.Model, tea.Cmd) {
	if m.dlg.initWizard.wizard != nil {
		cwd, _ := os.Getwd()
		if err := m.dlg.initWizard.wizard.Save(cwd); err != nil {
			m.composer.commandOutput = fmt.Sprintf("保存 profile 失败: %v", err)
		} else {
			m.composer.commandOutput = "个性化配置已保存到 .openscholar/profile.yaml"
		}
		m.dlg.initWizard.wizard = nil
	}
	m.state = stateChat
	return m, m.composer.input.Focus()
}
