package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/tui/components"
)

// InitRequiredChoice represents the init required dialog result.
type InitRequiredChoice struct {
	StartInit bool // true = start wizard, false = skip
}

// InitRequiredOverlay implements Overlay for the init required confirmation.
type InitRequiredOverlay struct {
	choiceIdx int // 0=开始初始化, 1=跳过
}

// NewInitRequiredOverlay creates a new init required overlay.
func NewInitRequiredOverlay() *InitRequiredOverlay {
	return &InitRequiredOverlay{}
}

func (o *InitRequiredOverlay) ID() string        { return "init-required" }
func (o *InitRequiredOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *InitRequiredOverlay) BlocksInput() bool { return true }

func (o *InitRequiredOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	switch keyMsg.Type {
	case tea.KeyUp, tea.KeyLeft:
		if o.choiceIdx > 0 {
			o.choiceIdx--
		}
		return o, nil, nil
	case tea.KeyDown, tea.KeyRight:
		if o.choiceIdx < 1 {
			o.choiceIdx++
		}
		return o, nil, nil
	case tea.KeyEnter:
		return o, &OverlayResult{
			Action: "accept",
			Data:   InitRequiredChoice{StartInit: o.choiceIdx == 0},
		}, nil
	case tea.KeyEsc:
		// Escape = skip
		return o, &OverlayResult{
			Action: "accept",
			Data:   InitRequiredChoice{StartInit: false},
		}, nil
	}

	switch keyMsg.String() {
	case "1":
		return o, &OverlayResult{
			Action: "accept",
			Data:   InitRequiredChoice{StartInit: true},
		}, nil
	case "2":
		return o, &OverlayResult{
			Action: "accept",
			Data:   InitRequiredChoice{StartInit: false},
		}, nil
	}

	return o, nil, nil
}

func (o *InitRequiredOverlay) View(width, height int) string {
	return components.RenderInitRequiredDialog(o.choiceIdx, width-4)
}
