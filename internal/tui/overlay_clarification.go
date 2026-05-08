package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/tui/components"
)

// ClarificationChoice represents the user's clarification response.
type ClarificationChoice struct {
	Event    tools.ClarificationEvent
	Response tools.ClarificationResponse
}

// ClarificationOverlay implements Overlay for the AskUser clarification dialog.
type ClarificationOverlay struct {
	pending *tools.ClarificationEvent
	idx     int
	input   string
	focused bool // true = freeform input mode
}

// NewClarificationOverlay creates a new ClarificationOverlay for the given event.
func NewClarificationOverlay(event *tools.ClarificationEvent) *ClarificationOverlay {
	return &ClarificationOverlay{pending: event}
}

func (o *ClarificationOverlay) ID() string        { return "clarification" }
func (o *ClarificationOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *ClarificationOverlay) BlocksInput() bool { return true }

func (o *ClarificationOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	event := o.pending
	if event == nil {
		return o, nil, nil
	}

	if o.focused {
		// In freeform input mode
		switch keyMsg.Type {
		case tea.KeyTab:
			o.focused = false
			return o, nil, nil
		case tea.KeyEnter:
			if o.input != "" {
				return o.acceptFreeform()
			}
			return o, nil, nil
		case tea.KeyEsc:
			return o.skip()
		case tea.KeyBackspace:
			runes := []rune(o.input)
			if len(runes) > 0 {
				o.input = string(runes[:len(runes)-1])
			}
			return o, nil, nil
		default:
			if len(keyMsg.String()) == 1 || keyMsg.Type == tea.KeySpace {
				ch := keyMsg.String()
				if keyMsg.Type == tea.KeySpace {
					ch = " "
				}
				o.input += ch
			}
			return o, nil, nil
		}
	}

	// In option selection mode
	switch keyMsg.Type {
	case tea.KeyUp:
		if o.idx > 0 {
			o.idx--
		}
		return o, nil, nil
	case tea.KeyDown:
		if o.idx < len(event.Options)-1 {
			o.idx++
		}
		return o, nil, nil
	case tea.KeyTab:
		if event.AllowFreeform {
			o.focused = true
		}
		return o, nil, nil
	case tea.KeyEnter:
		return o.acceptSelected()
	case tea.KeyEsc:
		return o.skip()
	}

	// Number shortcuts 1-5
	switch keyMsg.String() {
	case "1", "2", "3", "4", "5":
		idx := int(keyMsg.String()[0]-'0') - 1
		if idx < len(event.Options) {
			o.idx = idx
			return o.acceptSelected()
		}
	}

	return o, nil, nil
}

func (o *ClarificationOverlay) acceptFreeform() (Overlay, *OverlayResult, tea.Cmd) {
	event := o.pending
	resp := tools.ClarificationResponse{
		SelectedIndex: -1,
		SelectedText:  o.input,
	}
	select {
	case event.ResponseCh <- resp:
	default:
	}
	return o, &OverlayResult{
		Action: "accept",
		Data:   ClarificationChoice{Event: *event, Response: resp},
	}, nil
}

func (o *ClarificationOverlay) acceptSelected() (Overlay, *OverlayResult, tea.Cmd) {
	event := o.pending
	resp := tools.ClarificationResponse{
		SelectedIndex: o.idx,
		SelectedText:  event.Options[o.idx],
	}
	select {
	case event.ResponseCh <- resp:
	default:
	}
	return o, &OverlayResult{
		Action: "accept",
		Data:   ClarificationChoice{Event: *event, Response: resp},
	}, nil
}

func (o *ClarificationOverlay) skip() (Overlay, *OverlayResult, tea.Cmd) {
	event := o.pending
	resp := tools.ClarificationResponse{Skipped: true}
	select {
	case event.ResponseCh <- resp:
	default:
	}
	return o, &OverlayResult{
		Action: "reject",
		Data:   ClarificationChoice{Event: *event, Response: resp},
	}, nil
}

func (o *ClarificationOverlay) View(width, height int) string {
	return components.RenderClarificationDialog(
		*o.pending, o.idx, o.input, o.focused, width-4,
	)
}
