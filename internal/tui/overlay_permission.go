package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/tui/components"
)

// PermissionChoice represents the user's permission decision.
type PermissionChoice struct {
	Request   permission.PermissionRequest
	OptionIdx int // 0=Yes, 1=No, 2=Always
}

// PermissionOverlay implements Overlay for the permission confirmation dialog.
type PermissionOverlay struct {
	pending   *permission.PermissionRequest
	optionIdx int
}

// NewPermissionOverlay creates a new PermissionOverlay for the given request.
func NewPermissionOverlay(req *permission.PermissionRequest) *PermissionOverlay {
	return &PermissionOverlay{pending: req, optionIdx: 0}
}

func (o *PermissionOverlay) ID() string        { return "permission" }
func (o *PermissionOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *PermissionOverlay) BlocksInput() bool { return true }

func (o *PermissionOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	switch keyMsg.Type {
	case tea.KeyLeft, tea.KeyUp:
		if o.optionIdx > 0 {
			o.optionIdx--
		}
	case tea.KeyRight, tea.KeyDown:
		if o.optionIdx < 2 {
			o.optionIdx++
		}
	case tea.KeyEnter:
		return o, &OverlayResult{
			Action: "accept",
			Data:   PermissionChoice{Request: *o.pending, OptionIdx: o.optionIdx},
		}, nil
	case tea.KeyShiftTab:
		// Quick accept + auto mode (Always = index 2)
		o.optionIdx = 2
		return o, &OverlayResult{
			Action: "accept",
			Data:   PermissionChoice{Request: *o.pending, OptionIdx: 2},
		}, nil
	default:
		// CC alignment: y=Yes(0), n=No(1), a=Always(2)
		switch keyMsg.String() {
		case "y", "Y":
			o.optionIdx = 0
			return o, &OverlayResult{Action: "accept", Data: PermissionChoice{Request: *o.pending, OptionIdx: 0}}, nil
		case "n", "N":
			o.optionIdx = 1
			return o, &OverlayResult{Action: "accept", Data: PermissionChoice{Request: *o.pending, OptionIdx: 1}}, nil
		case "a", "A":
			o.optionIdx = 2
			return o, &OverlayResult{Action: "accept", Data: PermissionChoice{Request: *o.pending, OptionIdx: 2}}, nil
		}
	}
	return o, nil, nil
}

func (o *PermissionOverlay) View(width, height int) string {
	maxPreviewLines := height - 8
	if maxPreviewLines < 0 {
		maxPreviewLines = 0
	}
	return clampRenderedHeight(
		components.RenderPermissionRequestDialog(*o.pending, o.optionIdx, width-4, maxPreviewLines),
		height,
	)
}
