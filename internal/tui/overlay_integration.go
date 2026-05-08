package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/config"
	initwizard "github.com/Nahasma/openscholar-public/internal/init"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/picker"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// processOverlayResult handles Model-level side effects when an overlay
// completes (returned a result via its Update method).
// It processes OverlayResult.Data to execute permission grants, session
// switches, wizard saves, etc.
func (m *Model) processOverlayResult(id string, result *OverlayResult) (tea.Model, tea.Cmd) {
	if result == nil {
		return m, nil
	}

	switch id {
	case "permission":
		if choice, ok := result.Data.(PermissionChoice); ok {
			return m.applyPermissionChoice(choice)
		}

	case "plan-approval":
		if choice, ok := result.Data.(PlanApprovalChoice); ok {
			m.applyPlanApprovalUIState(choice.Event.SessionID, choice.Response)
		}

	case "research-suggestion":
		text, _ := result.Data.(string)
		if result.Action == "accept" {
			// Switch to research mode → show template selection
			m.overlays.Push(NewTemplateOverlay(
				[]string{"empirical", "aris_empirical", "survey", "theoretical"}, text,
			))
			return m, nil
		}
		// Continue normal mode
		return m.sendToAgent(text)

	case "template":
		if choice, ok := result.Data.(TemplateChoice); ok {
			return m.applyTemplateChoice(choice)
		}

	case "model-select":
		if choice, ok := result.Data.(ModelSelectChoice); ok {
			if err := m.app.SetModelProvider(choice.Provider, string(choice.ModelID)); err != nil {
				m.composer.commandOutput = fmt.Sprintf("Error: %v", err)
			} else {
				mdl, _ := m.app.CoderAgent.Model(), ""
				m.composer.commandOutput = fmt.Sprintf("已切换为 %s (%s)", mdl.Name, mdl.ID)
				m.applyRuntimeConfigChange()
				return m, tea.Batch(m.updateViewportContent(), m.composer.input.Focus())
			}
		}

	case "session-browser":
		if choice, ok := result.Data.(SessionBrowserChoice); ok {
			return m.applySessionBrowserChoice(choice)
		}

	case "workspace":
		if choice, ok := result.Data.(WorkspaceChoice); ok {
			return m.applyWorkspaceChoice(choice)
		}

	case "init-required":
		if choice, ok := result.Data.(InitRequiredChoice); ok {
			if choice.StartInit {
				m.overlays.Push(NewInitWizardOverlay())
				return m, nil
			}
			// Skip — go to chat
		}

	case "init-wizard":
		if choice, ok := result.Data.(InitWizardChoice); ok {
			if choice.Wizard != nil {
				cwd, _ := os.Getwd()
				if err := choice.Wizard.Save(cwd); err != nil {
					m.composer.commandOutput = fmt.Sprintf("保存 profile 失败: %v", err)
				} else {
					m.composer.commandOutput = "个性化配置已保存到 .openscholar/profile.yaml"
				}
			}
		}

	case "config-wizard":
		if choice, ok := result.Data.(ConfigWizardChoice); ok {
			return m.applyConfigWizardChoice(choice)
		}

	case "copy-mode":
		if result.Action == "accept" {
			if text, ok := result.Data.(string); ok && text != "" {
				if errMsg := copyToClipboard(text); errMsg != "" {
					m.setNotice(components.NoticeError, errMsg)
				} else {
					m.setNotice(components.NoticeSuccess, "Copied code block ✓")
				}
				return m, clearStatusAfterDelay()
			}
		}

	case "memory-selector":
		if choice, ok := result.Data.(MemorySelectorChoice); ok {
			switch choice.Action {
			case "view":
				m.composer.commandOutput = fmt.Sprintf("Memory file: %s", choice.FilePath)
			case "delete":
				m.composer.commandOutput = fmt.Sprintf("Deleted memory file: %s", choice.FilePath)
				// TODO: actual file deletion via memory service
			}
		}
	}

	return m, m.composer.input.Focus()
}

// applyPermissionChoice executes the permission decision from a PermissionOverlay.
func (m *Model) applyPermissionChoice(choice PermissionChoice) (tea.Model, tea.Cmd) {
	// CC alignment: inline options order is Yes(0)=Allow, No(1)=Deny, Always(2)=AlwaysAllow
	switch choice.OptionIdx {
	case 0: // Yes — Allow (this session)
		m.app.Permissions.Grant(choice.Request)
	case 1: // No — Deny
		m.app.Permissions.Deny(choice.Request)
	case 2: // Always — Always Allow (persist to DB)
		m.app.Permissions.GrantAlways(choice.Request)
		m.transitionSessionMode(permission.ModeAuto)
	}
	return m, tea.Batch(m.updateViewportContent(), m.composer.input.Focus())
}

// applyTemplateChoice executes the template selection.
func (m *Model) applyTemplateChoice(choice TemplateChoice) (tea.Model, tea.Cmd) {
	topic := choice.PendingText
	runes := []rune(topic)
	if len(runes) > 40 {
		topic = string(runes[:40])
	}

	// Normalize work directory (same as legacy path in wizards.go:confirmTemplateSelection)
	workDir := strings.TrimSpace(choice.FolderName)
	workDir = strings.TrimPrefix(workDir, "@")
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		workDir = "."
	}

	if err := m.ensureSession(); err != nil {
		m.setNotice(components.NoticeError, fmt.Sprintf("Failed to create session: %v", err))
		return m, m.composer.input.Focus()
	}

	p, err := m.app.ResearchEngine.Create(m.sessionID, topic, choice.Template, "default", 8, workDir)
	if err != nil {
		m.setNotice(components.NoticeError, fmt.Sprintf("Failed to create pipeline: %v", err))
		return m, m.composer.input.Focus()
	}
	phases, _ := m.app.ResearchEngine.GetPhases(p.ID)
	if len(phases) > 0 {
		if err := m.app.ResearchEngine.StartPhase(p.ID, phases[0].ID); err != nil {
			m.setNotice(components.NoticeError, fmt.Sprintf("Failed to start phase 1: %v", err))
			m.composer.commandOutput = fmt.Sprintf("已创建研究流水线 [%s]，但第 1 阶段启动失败。", p.ID[:8])
			return m, m.composer.input.Focus()
		}
	}
	m.status.researchPipelineID = p.ID
	m.transitionSessionMode(permission.ModeResearch)

	m.composer.commandOutput = fmt.Sprintf("已创建研究流水线 [%s]，第 1 阶段已启动。后台 leader 任务将自动执行该阶段。", p.ID[:8])
	return m, m.composer.input.Focus()
}

// applySessionBrowserChoice executes the session browser action.
func (m *Model) applySessionBrowserChoice(choice SessionBrowserChoice) (tea.Model, tea.Cmd) {
	switch choice.Action {
	case "select":
		return m, tea.Batch(m.composer.input.Focus(), m.loadSessionByIDCmd(choice.SessionID))
	case "new":
		m.sessionID = ""
		m.resetSessionState()
		return m, tea.Batch(m.updateViewportContent(), m.composer.input.Focus())
	case "delete":
		_ = m.app.Sessions.Delete(m.ctx, choice.SessionID)
		if choice.SessionID == m.sessionID {
			m.sessionID = ""
			m.resetSessionState()
		}
		// Re-fetch list and push new overlay
		return m, m.fetchSessionListCmd()
	}
	return m, m.composer.input.Focus()
}

// applyWorkspaceChoice executes the workspace selection.
func (m *Model) applyWorkspaceChoice(choice WorkspaceChoice) (tea.Model, tea.Cmd) {
	switch choice.Action {
	case "confirm":
		m.dlg.workspace.confirmed = true
	case "change":
		config.SetWorkingDirectory(choice.Path)
		m.dlg.workspace.confirmed = true
	}

	// After workspace confirmed, check if init required
	cwd := config.WorkingDirectory()
	profile, _ := initwizard.LoadProfile(cwd)
	if profile == nil {
		m.overlays.Push(NewInitRequiredOverlay())
		return m, nil
	}
	return m, m.composer.input.Focus()
}

// applyConfigWizardChoice processes the config wizard completion.
func (m *Model) applyConfigWizardChoice(choice ConfigWizardChoice) (tea.Model, tea.Cmd) {
	summary := "Config wizard dismissed"
	if choice.Saved && choice.Wizard != nil && choice.Wizard.HasChanges() {
		newCfg := choice.Wizard.BuildConfig()
		m.composer.commandOutput, summary, _ = m.saveFullAndApplyRuntimeConfig(newCfg)
	} else if !choice.Saved {
		m.composer.commandOutput = "配置向导已取消"
	} else {
		m.composer.commandOutput = "未做任何更改"
	}
	if invocation := strings.TrimSpace(m.composer.pendingCommandActivity.Invocation); invocation != "" {
		m.appendCommandActivity(invocation, summary)
	}
	m.composer.pendingCommandActivity = CommandActivityState{}
	m.recalcLayout()
	return m, tea.Batch(m.updateViewportContent(), m.composer.input.Focus())
}

// pushOverlayForHelp opens help overlay.
func (m *Model) pushHelpOverlay() {
	m.overlays.Push(NewHelpOverlay())
	m.recalcLayout()
}

// pushCopyModeOverlay opens copy mode overlay.
func (m *Model) pushCopyModeOverlay(blocks []CodeBlock) {
	m.overlays.Push(NewCopyModeOverlay(blocks))
	m.recalcLayout()
}

// pushModelSelectOverlay opens model selection overlay.
func (m *Model) pushModelSelectOverlay() tea.Cmd {
	currentModel := m.app.CoderAgent.Model()
	overlay := NewModelSelectOverlay(currentModel.Provider, currentModel.ID)
	m.overlays.Push(overlay)
	m.recalcLayout()
	return overlay.discoverCurrentProviderCmd()
}

// pushResearchSuggestionOverlay opens research suggestion overlay.
func (m *Model) pushResearchSuggestionOverlay(text string) {
	m.overlays.Push(NewResearchSuggestionOverlay(text))
	m.recalcLayout()
}

// pushConfigWizardOverlay opens config wizard overlay.
func (m *Model) pushConfigWizardOverlay() {
	cfg := config.Get()
	m.overlays.Push(NewConfigWizardOverlay(cfg))
	m.recalcLayout()
}

// pushMemorySelectorOverlay opens the memory file browser overlay.
func (m *Model) pushMemorySelectorOverlay() {
	// Collect memory file entries from the session memory data directory
	entries := m.collectMemoryFileEntries()
	m.overlays.Push(NewMemorySelectorOverlay(entries))
	m.recalcLayout()
}

// collectMemoryFileEntries gathers memory files for the selector overlay.
// It scans session memory, project-level memory, and user-level memory directories.
func (m *Model) collectMemoryFileEntries() []MemoryFileEntry {
	var entries []MemoryFileEntry
	dataDir := config.DataDirectory()

	// 1. Session memory: .openscholar/session-memory/<sessionID>/notes.md
	if m.sessionID != "" {
		notesPath := filepath.Join(dataDir, "session-memory", m.sessionID, "notes.md")
		if info, err := os.Stat(notesPath); err == nil {
			entries = append(entries, MemoryFileEntry{
				RelPath:   "session-memory/" + m.sessionID + "/notes.md",
				Type:      "project",
				UpdatedAt: info.ModTime(),
				IsActive:  true,
			})
		}
	}

	// 2. Project-level memory: .openscholar/memory/*.md
	projectMemDir := filepath.Join(dataDir, "memory")
	entries = append(entries, scanMemoryDir(projectMemDir, "")...)

	// 3. User-level memory: ~/.openscholar/memory/*.md
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userMemDir := filepath.Join(homeDir, ".openscholar", "memory")
		if userMemDir != projectMemDir {
			entries = append(entries, scanMemoryDir(userMemDir, "~/.openscholar/memory/")...)
		}
	}

	return entries
}

// scanMemoryDir scans a directory for .md memory files and returns entries.
func scanMemoryDir(dir string, pathPrefix string) []MemoryFileEntry {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var entries []MemoryFileEntry
	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".md") || de.Name() == "MEMORY.md" {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		relPath := de.Name()
		if pathPrefix != "" {
			relPath = pathPrefix + de.Name()
		}
		// Detect type from frontmatter (simplified: check filename prefix)
		memType := inferMemoryType(de.Name())
		entries = append(entries, MemoryFileEntry{
			RelPath:   relPath,
			Type:      memType,
			UpdatedAt: info.ModTime(),
		})
	}
	return entries
}

// inferMemoryType guesses the memory type from the filename.
func inferMemoryType(name string) string {
	switch {
	case strings.HasPrefix(name, "user"):
		return "user"
	case strings.HasPrefix(name, "feedback"):
		return "feedback"
	case strings.HasPrefix(name, "project"):
		return "project"
	case strings.HasPrefix(name, "reference"):
		return "reference"
	default:
		return "project"
	}
}

// pushTemplateOverlay opens template selection overlay.
func (m *Model) pushTemplateOverlay(pendingText string) {
	m.overlays.Push(NewTemplateOverlay(
		[]string{"empirical", "aris_empirical", "survey", "theoretical"}, pendingText,
	))
	m.dlg.template.picker = picker.New(8)
	m.recalcLayout()
}
