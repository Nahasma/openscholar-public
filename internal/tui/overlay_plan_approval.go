package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

type PlanApprovalChoice struct {
	Event    tools.PlanApprovalEvent
	Response tools.PlanApprovalResponse
}

type PlanApprovalOverlay struct {
	pending   *tools.PlanApprovalEvent
	optionIdx int
	feedback  string
	inputMode bool
}

func NewPlanApprovalOverlay(event *tools.PlanApprovalEvent) *PlanApprovalOverlay {
	return &PlanApprovalOverlay{pending: event}
}

func (o *PlanApprovalOverlay) ID() string        { return "plan-approval" }
func (o *PlanApprovalOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *PlanApprovalOverlay) BlocksInput() bool { return true }

func (o *PlanApprovalOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok || o.pending == nil {
		return o, nil, nil
	}
	if o.inputMode {
		switch keyMsg.Type {
		case tea.KeyTab:
			o.inputMode = false
			return o, nil, nil
		case tea.KeyEnter:
			return o.submitChoice()
		case tea.KeyEsc:
			return o.reject()
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

	switch keyMsg.Type {
	case tea.KeyUp, tea.KeyLeft:
		if o.optionIdx > 0 {
			o.optionIdx--
		}
		return o, nil, nil
	case tea.KeyDown, tea.KeyRight:
		if o.optionIdx < 3 {
			o.optionIdx++
		}
		return o, nil, nil
	case tea.KeyTab:
		if o.optionIdx == 2 || o.optionIdx == 3 {
			o.inputMode = true
		}
		return o, nil, nil
	case tea.KeyEnter:
		if o.optionIdx == 2 || o.optionIdx == 3 {
			o.inputMode = true
			return o, nil, nil
		}
		return o.submitChoice()
	case tea.KeyEsc:
		return o.reject()
	}

	switch keyMsg.String() {
	case "1", "y", "Y":
		o.optionIdx = 0
		return o.submitChoice()
	case "2", "a", "A":
		o.optionIdx = 1
		return o.submitChoice()
	case "3", "n", "N":
		o.optionIdx = 2
		o.inputMode = true
		return o, nil, nil
	case "4", "e", "E":
		o.optionIdx = 3
		o.inputMode = true
		return o, nil, nil
	}
	return o, nil, nil
}

func (o *PlanApprovalOverlay) submitChoice() (Overlay, *OverlayResult, tea.Cmd) {
	resp := tools.PlanApprovalResponse{}
	switch o.optionIdx {
	case 0:
		resp.Approved = true
		resp.TargetMode = permission.ModeRestore
	case 1:
		resp.Approved = true
		resp.TargetMode = permission.ModeAuto
	case 2:
		resp.Feedback = o.feedback
	case 3:
		resp.Approved = true
		resp.TargetMode = permission.ModeRestore
		resp.EditedPlan = o.feedback
	}
	select {
	case o.pending.ResponseCh <- resp:
	default:
	}
	action := "reject"
	if resp.Approved {
		action = "accept"
	}
	return o, &OverlayResult{
		Action: action,
		Data:   PlanApprovalChoice{Event: *o.pending, Response: resp},
	}, nil
}

func (o *PlanApprovalOverlay) reject() (Overlay, *OverlayResult, tea.Cmd) {
	resp := tools.PlanApprovalResponse{Feedback: o.feedback}
	select {
	case o.pending.ResponseCh <- resp:
	default:
	}
	return o, &OverlayResult{
		Action: "reject",
		Data:   PlanApprovalChoice{Event: *o.pending, Response: resp},
	}, nil
}

func (o *PlanApprovalOverlay) View(width, height int) string {
	return components.RenderPlanApprovalDialog(*o.pending, o.optionIdx, o.feedback, o.inputMode, width-4)
}
