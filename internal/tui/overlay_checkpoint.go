package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/tui/components"
)

// CheckpointChoice represents the user's checkpoint decision.
type CheckpointChoice struct {
	Event    tools.CheckpointEvent
	Response tools.CheckpointResponse
}

// CheckpointOverlay implements Overlay for the research pipeline checkpoint dialog.
type CheckpointOverlay struct {
	pending   *tools.CheckpointEvent
	optionIdx int  // 0=Approve, 1=Reject
	feedback  string
	inputMode bool // true = typing rejection feedback
}

// NewCheckpointOverlay creates a new checkpoint overlay.
func NewCheckpointOverlay(event *tools.CheckpointEvent) *CheckpointOverlay {
	return &CheckpointOverlay{pending: event}
}

func (o *CheckpointOverlay) ID() string        { return "checkpoint" }
func (o *CheckpointOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *CheckpointOverlay) BlocksInput() bool { return true }

func (o *CheckpointOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}
	if o.pending == nil {
		return o, nil, nil
	}

	if o.inputMode {
		switch keyMsg.Type {
		case tea.KeyTab:
			o.inputMode = false
			return o, nil, nil
		case tea.KeyEnter:
			if o.feedback != "" {
				return o.submitChoice(false)
			}
			return o, nil, nil
		case tea.KeyEsc:
			return o.submitChoice(false)
		case tea.KeyBackspace:
			runes := []rune(o.feedback)
			if len(runes) > 0 {
				o.feedback = string(runes[:len(runes)-1])
			}
			return o, nil, nil
		default:
			if len(keyMsg.String()) == 1 || keyMsg.Type == tea.KeySpace {
				ch := keyMsg.String()
				if keyMsg.Type == tea.KeySpace {
					ch = " "
				}
				o.feedback += ch
			}
			return o, nil, nil
		}
	}

	// Option selection mode
	switch keyMsg.Type {
	case tea.KeyUp, tea.KeyDown:
		o.optionIdx = 1 - o.optionIdx // toggle 0↔1
		return o, nil, nil
	case tea.KeyTab:
		if o.optionIdx == 1 { // Reject selected
			o.inputMode = true
		}
		return o, nil, nil
	case tea.KeyEnter:
		return o.submitChoice(o.optionIdx == 0)
	case tea.KeyEsc:
		return o.submitChoice(false) // reject
	}

	// Shortcut keys
	switch keyMsg.String() {
	case "y", "Y":
		return o.submitChoice(true)
	case "n", "N":
		o.optionIdx = 1
		o.inputMode = true
		return o, nil, nil
	}

	return o, nil, nil
}

func (o *CheckpointOverlay) submitChoice(approved bool) (Overlay, *OverlayResult, tea.Cmd) {
	resp := tools.CheckpointResponse{
		Approved: approved,
		Feedback: o.feedback,
	}

	select {
	case o.pending.ResponseCh <- resp:
	default:
	}

	action := "accept"
	if !approved {
		action = "reject"
	}
	return o, &OverlayResult{
		Action: action,
		Data:   CheckpointChoice{Event: *o.pending, Response: resp},
	}, nil
}

func (o *CheckpointOverlay) View(width, height int) string {
	return components.RenderCheckpointDialog(
		*o.pending, o.optionIdx, o.feedback, o.inputMode, width-4,
	)
}
