package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type ScreenMode string

const (
	ScreenModeFullscreen ScreenMode = "fullscreen"
	ScreenModeMain       ScreenMode = "main"
)

type TerminalState struct {
	AltScreen bool
	Mouse     bool
}

type screenModeAppliedMsg struct {
	target    ScreenMode
	prevWidth int
}

func sequenceCmd(cmds ...tea.Cmd) tea.Cmd {
	var valid []tea.Cmd
	for _, cmd := range cmds {
		if cmd != nil {
			valid = append(valid, cmd)
		}
	}
	switch len(valid) {
	case 0:
		return nil
	case 1:
		return valid[0]
	default:
		return tea.Sequence(valid...)
	}
}

func ScreenModeFromEnv() ScreenMode {
	if FullscreenEnabled() {
		return ScreenModeFullscreen
	}
	return ScreenModeMain
}

func (m ScreenMode) IsFullscreen() bool {
	return m == ScreenModeFullscreen
}

func ParseScreenMode(raw string) (ScreenMode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "fullscreen", "full":
		return ScreenModeFullscreen, true
	case "main", "main-screen", "mainscreen":
		return ScreenModeMain, true
	default:
		return "", false
	}
}

func (m *Model) setScreenMode(mode ScreenMode) {
	m.screenMode = mode
	m.terminalState = TerminalState{
		AltScreen: mode.IsFullscreen(),
		Mouse:     mode.IsFullscreen(),
	}
	if mode.IsFullscreen() {
		m.markFullscreenFrameDirty()
	}
}

func (m Model) isFullscreenMode() bool {
	return m.terminalState.AltScreen
}

func (m Model) terminalMatchesMode() bool {
	wantAlt := m.screenMode.IsFullscreen()
	return m.terminalState.AltScreen == wantAlt && m.terminalState.Mouse == wantAlt
}

func (m Model) toggleScreenMode() ScreenMode {
	if m.isFullscreenMode() {
		return ScreenModeMain
	}
	return ScreenModeFullscreen
}

func (m *Model) clearScreenModeTransientState() {
	m.chat.selection = TextSelection{}
	m.resetMainScreenIntroCache()
	m.chat.mainScreenViewportOwned = false
	m.chat.mainScreenOwnedStart = nil
	m.chat.mainScreenOwnedStream = nil
	m.chat.mainScreenFrame = nil
}

func (m *Model) switchScreenMode(target ScreenMode) (bool, tea.Cmd) {
	if target == "" {
		return false, nil
	}
	if target == m.screenMode && m.terminalMatchesMode() {
		return false, nil
	}

	prevWidth := m.width
	m.screenMode = target
	m.clearScreenModeTransientState()

	var cmds []tea.Cmd
	if target.IsFullscreen() {
		m.markFullscreenFrameDirty()
		if !m.terminalState.AltScreen {
			cmds = append(cmds, tea.EnterAltScreen)
		}
		if !m.terminalState.Mouse {
			cmds = append(cmds, tea.EnableMouseCellMotion)
		}
		cmds = append(cmds, func() tea.Msg {
			return screenModeAppliedMsg{target: target, prevWidth: prevWidth}
		})
		return true, sequenceCmd(cmds...)
	}

	if m.terminalState.Mouse {
		cmds = append(cmds, tea.DisableMouse)
	}
	if m.terminalState.AltScreen {
		cmds = append(cmds, tea.ExitAltScreen)
	}
	cmds = append(cmds, func() tea.Msg {
		return screenModeAppliedMsg{target: target, prevWidth: prevWidth}
	})
	return true, sequenceCmd(cmds...)
}
