package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// HelpOverlay displays keyboard shortcuts. Any key press dismisses it.
type HelpOverlay struct{}

// NewHelpOverlay creates a new HelpOverlay.
func NewHelpOverlay() *HelpOverlay {
	return &HelpOverlay{}
}

func (o *HelpOverlay) ID() string        { return "help" }
func (o *HelpOverlay) Kind() OverlayKind { return OverlayNonBlocking }
func (o *HelpOverlay) BlocksInput() bool { return true }

func (o *HelpOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return o, &OverlayResult{Action: "dismiss"}, nil
	}
	return o, nil, nil
}

func (o *HelpOverlay) View(width, height int) string {
	return components.RenderHelpOverlay(width)
}
