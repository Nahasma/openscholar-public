package tui

import (
	"context"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/debug"
	initwizard "github.com/openscholar/openscholar/internal/init"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/session"
	"golang.org/x/term"
)

// ScrollMode controls auto-scroll behavior.
type ScrollMode int

const (
	// ScrollAutoFollow scrolls to bottom on new content.
	ScrollAutoFollow ScrollMode = iota
	// ScrollManualLocked keeps viewport at current position when user is browsing history.
	ScrollManualLocked
)

type state int

const (
	stateChat state = iota
	statePermission
	stateHelp
	stateSessionBrowser
	statePlanApproval
	stateClarification
	stateModelSelection
	stateInitRequired
	stateInitWizard
	stateConfigWizard
	stateCheckpoint
	stateTemplateSelection
	stateWorkspaceSelection
	stateResearchSuggestion
	stateCopyMode
)

// TextSelection tracks mouse text selection state.
type TextSelection struct {
	Active     bool // drag in progress
	HasRange   bool // selection completed (show highlight until cleared)
	StartRow   int  // cached content line index (start)
	StartCol   int  // visual column (start)
	EndRow     int  // cached content line index (end)
	EndCol     int  // visual column (end)
	StartCoord TranscriptCoord
	EndCoord   TranscriptCoord
	PlainText  string
}

// TranscriptCoord is a semantic transcript position. ContentRow remains local
// to the currently mounted contentLines slice; AbsLine is the global transcript
// display line used by virtual scrolling and block anchors.
type TranscriptCoord struct {
	Valid          bool
	ContentRow     int
	AbsLine        int
	Column         int
	PlainOffset    int
	LineStart      bool
	BlockID        string
	MsgID          string
	BlockIdx       int
	LineInBlock    int
	BoundaryBefore bool
}

// tickMsg drives the spinner animation.
type tickMsg time.Time

// ctrlCResetMsg resets the Ctrl+C confirmation state after timeout.
type ctrlCResetMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// loadSessionMsg is sent when a session is loaded (resume or switch).
type loadSessionMsg struct {
	session      session.Session
	messages     []message.Message
	toolMessages map[string]message.Message
}

// sessionListMsg is sent when session list is fetched.
type sessionListMsg []session.Session

type Model struct {
	// Core
	ctx             context.Context
	app             *app.App
	dispatcher      *command.Dispatcher
	state           state
	features        TUIFeatures
	screenMode      ScreenMode
	terminalState   TerminalState
	width           int
	height          int
	geometry        GeometryState
	resizeEpoch     int
	fullscreenFrame *fullscreenFrameState
	sessionID       string
	// sessionSummaryMessageID tracks the compact boundary for status-bar token
	// estimates. The transcript can still show full history, but requests only
	// include messages from this boundary forward.
	sessionSummaryMessageID string
	debugTrace              *debug.TUITrace
	mainOutput              *MainScreenOutputController
	mainResetMode           MainScreenResetMode

	// Session resume
	resumeSession bool
	resumeLatest  bool
	resumeQuery   string

	// Feature sub-models
	chat     ChatFeature
	status   StatusFeature
	search   SearchFeature
	composer ComposerFeature

	// Dialog states container
	dlg DialogStates

	// Overlay stack (Phase 4: replaces state switch when features.OverlayStack is true)
	overlays *OverlayManager

	// Vim mode (Phase 6: optional vim normal mode for chat viewport navigation)
	vim *VimMode
}

func New(ctx context.Context, a *app.App, resume bool, dispatcher *command.Dispatcher, opts ...Option) Model {
	m := Model{
		ctx:           ctx,
		app:           a,
		dispatcher:    dispatcher,
		state:         stateChat,
		features:      FeaturesFromEnv().Sanitize(),
		screenMode:    ScreenModeFromEnv(),
		resumeSession: resume,
		chat:          NewChatFeature(),
		status:        StatusFeature{},
		search:        NewSearchFeature(),
		composer:      NewComposerFeature(),
		overlays:      NewOverlayManager(),
		vim:           NewVimMode(),
		mainResetMode: MainScreenResetModeFromEnv(),
	}
	m.terminalState = TerminalState{
		AltScreen: m.screenMode.IsFullscreen(),
		Mouse:     m.screenMode.IsFullscreen(),
	}

	// Enable vim mode based on feature flag
	if m.features.VimMode {
		m.vim.SetEnabled(true)
	}

	for _, opt := range opts {
		opt(&m)
	}
	if m.screenMode.IsFullscreen() {
		m.markFullscreenFrameDirty()
	}

	return m
}

// Option configures the TUI model.
type Option func(*Model)

// WithCWDExplicit marks that --cwd was explicitly provided, skipping workspace dialog.
func WithCWDExplicit() Option {
	return func(m *Model) {
		m.dlg.workspace.confirmed = true
	}
}

// WithFullscreen overrides the fullscreen/alt-screen rendering mode.
func WithFullscreen(enabled bool) Option {
	return func(m *Model) {
		if enabled {
			m.setScreenMode(ScreenModeFullscreen)
			return
		}
		m.setScreenMode(ScreenModeMain)
	}
}

// WithScreenMode overrides the initial screen mode.
func WithScreenMode(mode ScreenMode) Option {
	return func(m *Model) {
		if mode == "" {
			return
		}
		m.setScreenMode(mode)
	}
}

// WithDebugTrace wires optional TUI trace logging.
func WithDebugTrace(trace *debug.TUITrace) Option {
	return func(m *Model) {
		m.debugTrace = trace
	}
}

func WithMainScreenRenderer(controller *MainScreenOutputController) Option {
	return func(m *Model) {
		m.mainOutput = controller
	}
}

func WithResumeFlags(latest bool, query string) Option {
	return func(m *Model) {
		m.resumeLatest = latest
		m.resumeQuery = strings.TrimSpace(query)
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.composer.input.Focus(), initialWindowSizeCmd()}
	if m.resumeSession {
		cmds = append(cmds, m.loadLastSessionCmd())
	}
	// Show dependency warnings on first launch (dismisses on any keypress)
	if len(m.app.DependencyWarnings) > 0 {
		var sb strings.Builder
		sb.WriteString("Environment Notice\n")
		for _, w := range m.app.DependencyWarnings {
			sb.WriteString("  " + w + "\n")
		}
		sb.WriteString("\nPress any key to dismiss.")
		m.composer.commandOutput = sb.String()
	}

	// Show workspace selection dialog if not explicitly set via --cwd
	if !m.resumeSession && !m.dlg.workspace.confirmed {
		if m.features.OverlayStack {
			m.overlays.Push(NewWorkspaceOverlay(config.WorkingDirectory()))
		} else {
			m.state = stateWorkspaceSelection
			m.dlg.workspace.idx = 0
		}
	} else if !m.resumeSession {
		// Check if user profile exists; if not, show init required dialog
		cwd, _ := os.Getwd()
		profile, _ := initwizard.LoadProfile(cwd)
		if profile == nil {
			if m.features.OverlayStack {
				m.overlays.Push(NewInitRequiredOverlay())
			} else {
				m.state = stateInitRequired
				m.dlg.initWizard.choiceIdx = 0
			}
		}
	}

	return tea.Batch(cmds...)
}

func initialWindowSizeCmd() tea.Cmd {
	return func() tea.Msg {
		source := geometrySourceBootstrap
		w, h, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || w <= 0 || h <= 0 {
			w, h = defaultWindowWidth, defaultWindowHeight
			source = geometrySourceFallback
		}
		return initialWindowSizeMsg{Width: w, Height: h, Source: source}
	}
}

func (m Model) loadLastSessionCmd() tea.Cmd {
	return func() tea.Msg {
		if m.app == nil || m.app.Resume == nil {
			return nil
		}
		mode := session.ResolveQuery
		selector := m.resumeQuery
		if m.resumeLatest {
			mode = session.ResolveLatest
			selector = ""
		} else if selector == "" || selector == "__PICKER__" {
			sessions, err := m.app.Resume.List(m.ctx, config.WorkingDirectory(), 50)
			if err != nil {
				return nil
			}
			return sessionListMsg(sessions)
		}
		resolved, err := m.app.Resume.Resolve(m.ctx, session.ResumeRequest{
			Mode:        mode,
			Selector:    selector,
			ProjectPath: config.WorkingDirectory(),
			Limit:       20,
		})
		if err != nil {
			return nil
		}
		if resolved.NeedPicker || len(resolved.Ambiguous) > 1 {
			return sessionListMsg(resolved.Ambiguous)
		}
		if !resolved.Found {
			return nil
		}
		loaded, err := m.app.Resume.Load(m.ctx, resolved.Session.ID)
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
