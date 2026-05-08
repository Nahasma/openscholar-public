package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/session"
)

func (m *Model) applyCommandActions(result command.Result, invocation string) (handled bool, cmd tea.Cmd) {
	for _, action := range result.NormalizedActions() {
		switch action.Kind {
		case command.CommandActionResearchStart:
			if m.sessionID != "" && m.app.ResearchEngine != nil {
				if p, err := m.app.ResearchEngine.GetBySession(m.sessionID); err == nil {
					m.status.mode = "research"
					m.status.researchPipelineID = p.ID
					m.applyCurrentModeToSession(m.sessionID)
				}
			}

		case command.CommandActionPlanEnter:
			if m.sessionID == "" {
				if err := m.ensureSession(); err != nil {
					m.composer.commandOutput = fmt.Sprintf("Error: %v", err)
					return true, nil
				}
			}
			m.transitionSessionMode(permission.ModePlan)
			if result.Output != "" {
				m.composer.commandOutput = result.Output
			}
			if result.Prompt != "" {
				next, nextCmd := m.sendToAgentWithRuntime(result.Prompt, result.Runtime)
				if updated, ok := next.(Model); ok {
					*m = updated
				}
				return true, nextCmd
			}
			return true, nil

		case command.CommandActionModelSelect:
			if m.features.OverlayStack {
				return true, m.pushModelSelectOverlay()
			} else {
				cmd := m.initModelSelection()
				m.state = stateModelSelection
				return true, cmd
			}

		case command.CommandActionSessionResume:
			selector := strings.TrimSpace(action.Target)
			if selector == "" {
				return true, m.fetchSessionListCmd()
			}
			mode := session.ResolveQuery
			if selector == "__latest__" {
				mode = session.ResolveLatest
				selector = ""
			}
			resolved, err := m.app.Resume.Resolve(m.ctx, session.ResumeRequest{
				Mode:        mode,
				Selector:    selector,
				ProjectPath: config.WorkingDirectory(),
				Limit:       20,
			})
			if err != nil {
				m.composer.commandOutput = fmt.Sprintf("Error: %v", err)
				return true, nil
			}
			if resolved.Found {
				return true, m.loadSessionByIDCmd(resolved.Session.ID)
			}
			if len(resolved.Ambiguous) > 0 {
				if m.features.OverlayStack {
					m.overlays.Push(NewSessionBrowserOverlay(resolved.Ambiguous))
					return true, nil
				}
				m.dlg.sessionBrowser.list = resolved.Ambiguous
				m.dlg.sessionBrowser.listIdx = 0
				m.dlg.sessionBrowser.listScroll = 0
				m.state = stateSessionBrowser
				return true, nil
			}
			m.composer.commandOutput = "No matching session found."
			return true, nil

		case command.CommandActionCD:
			m.composer.fileIndex = nil
			m.composer.fileIndexRoot = ""
			m.composer.fileIndexRefreshing = false
			if m.composer.filePicker != nil {
				m.composer.filePicker.Reset()
			}

		case command.CommandActionInitWizard:
			if m.features.OverlayStack {
				m.overlays.Push(NewInitWizardOverlay())
				m.recalcLayout()
			} else {
				m.startInitWizard()
			}
			return true, nil

		case command.CommandActionMemorySelector:
			if m.features.OverlayStack && m.features.MemoryUI {
				m.pushMemorySelectorOverlay()
			} else {
				entries := m.collectMemoryFileEntries()
				if len(entries) == 0 {
					m.composer.commandOutput = "No memory files found."
				} else {
					var sb strings.Builder
					sb.WriteString("Memory Files:\n")
					for _, e := range entries {
						sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", e.RelPath, e.Type, e.UpdatedAt.Format("2006-01-02 15:04")))
					}
					m.composer.commandOutput = sb.String()
				}
			}
			return true, nil

		case command.CommandActionConfigWizard:
			m.composer.pendingCommandActivity = CommandActivityState{Invocation: strings.TrimSpace(invocation)}
			if m.features.OverlayStack {
				m.pushConfigWizardOverlay()
			} else {
				m.startConfigWizard()
			}
			return true, nil

		case command.CommandActionCompact:
			next, nextCmd := m.startManualCompact("")
			if updated, ok := next.(Model); ok {
				*m = updated
			}
			return true, nextCmd

		case command.CommandActionScreen:
			target := action.Target
			var mode ScreenMode
			switch target {
			case "toggle":
				mode = m.toggleScreenMode()
			default:
				parsed, ok := ParseScreenMode(target)
				if !ok {
					m.composer.commandOutput = fmt.Sprintf("Unknown screen action: screen:%s", target)
					return true, nil
				}
				mode = parsed
			}
			changed, switchCmd := m.switchScreenMode(mode)
			if !changed {
				m.composer.commandOutput = fmt.Sprintf("Screen mode already %s.", mode)
				return true, nil
			}
			m.composer.commandOutput = fmt.Sprintf("Switched screen mode to %s.", mode)
			return true, switchCmd
		}
	}
	return false, nil
}
