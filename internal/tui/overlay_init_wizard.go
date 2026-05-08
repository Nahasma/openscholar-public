package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	initwizard "github.com/openscholar/openscholar/internal/init"
	"github.com/openscholar/openscholar/internal/tui/components"
)

// InitWizardChoice represents the init wizard completion.
type InitWizardChoice struct {
	Wizard *initwizard.Wizard // carries all answers; caller should Save()
}

// InitWizardOverlay implements Overlay for the multi-step init wizard.
type InitWizardOverlay struct {
	wizard    *initwizard.Wizard
	idx       int
	input     string
	otherMode bool
}

// NewInitWizardOverlay creates a new init wizard overlay.
func NewInitWizardOverlay() *InitWizardOverlay {
	return &InitWizardOverlay{wizard: initwizard.NewWizard()}
}

func (o *InitWizardOverlay) ID() string        { return "init-wizard" }
func (o *InitWizardOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *InitWizardOverlay) BlocksInput() bool { return true }

func (o *InitWizardOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	if o.wizard == nil || o.wizard.IsDone() {
		return o, &OverlayResult{
			Action: "accept",
			Data:   InitWizardChoice{Wizard: o.wizard},
		}, nil
	}

	step := o.wizard.CurrentStep()
	if step == nil {
		return o, &OverlayResult{
			Action: "accept",
			Data:   InitWizardChoice{Wizard: o.wizard},
		}, nil
	}

	// Text input mode: native StepTextInput or "Other" selected
	if step.Type == initwizard.StepTextInput || o.otherMode {
		switch keyMsg.Type {
		case tea.KeyEnter:
			if o.input != "" {
				o.wizard.Apply(o.input)
				o.input = ""
				o.idx = 0
				o.otherMode = false
				if o.wizard.IsDone() {
					return o, &OverlayResult{
						Action: "accept",
						Data:   InitWizardChoice{Wizard: o.wizard},
					}, nil
				}
			}
			return o, nil, nil
		case tea.KeyEsc:
			if o.otherMode {
				o.otherMode = false
				o.input = ""
				return o, nil, nil
			}
			o.wizard.Skip()
			o.input = ""
			o.idx = 0
			if o.wizard.IsDone() {
				return o, &OverlayResult{
					Action: "accept",
					Data:   InitWizardChoice{Wizard: o.wizard},
				}, nil
			}
			return o, nil, nil
		case tea.KeyBackspace:
			runes := []rune(o.input)
			if len(runes) > 0 {
				o.input = string(runes[:len(runes)-1])
			}
			return o, nil, nil
		default:
			if keyMsg.Type == tea.KeyRunes || keyMsg.Type == tea.KeySpace {
				ch := keyMsg.String()
				if keyMsg.Type == tea.KeySpace {
					ch = " "
				}
				o.input += ch
			}
			return o, nil, nil
		}
	}

	// Option selection mode
	optCount := len(step.Options)
	uiOptCount := optCount
	if step.Type == initwizard.StepSelectWithOther {
		uiOptCount = optCount + 1 // +1 for "Other"
	}

	switch keyMsg.Type {
	case tea.KeyUp:
		if o.idx > 0 {
			o.idx--
		}
		return o, nil, nil
	case tea.KeyDown:
		if o.idx < uiOptCount-1 {
			o.idx++
		}
		return o, nil, nil
	case tea.KeyEnter:
		return o.applySelection(step, o.idx)
	case tea.KeyEsc:
		o.wizard.Skip()
		o.idx = 0
		if o.wizard.IsDone() {
			return o, &OverlayResult{
				Action: "accept",
				Data:   InitWizardChoice{Wizard: o.wizard},
			}, nil
		}
		return o, nil, nil
	}

	// Number shortcuts
	s := keyMsg.String()
	if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		idx := int(s[0]-'0') - 1
		if idx < uiOptCount {
			return o.applySelection(step, idx)
		}
	}

	return o, nil, nil
}

func (o *InitWizardOverlay) applySelection(step *initwizard.WizardStep, idx int) (Overlay, *OverlayResult, tea.Cmd) {
	if step.Type == initwizard.StepSelectWithOther && idx >= len(step.Options) {
		// "Other" selected → switch to text input
		o.otherMode = true
		o.input = ""
		return o, nil, nil
	}

	if idx < len(step.Options) {
		o.wizard.Apply(step.Options[idx].Value)
		o.idx = 0
		if o.wizard.IsDone() {
			return o, &OverlayResult{
				Action: "accept",
				Data:   InitWizardChoice{Wizard: o.wizard},
			}, nil
		}
	}

	return o, nil, nil
}

func (o *InitWizardOverlay) View(width, height int) string {
	if o.wizard == nil {
		return ""
	}
	step := o.wizard.CurrentStep()
	if step == nil {
		return ""
	}

	stepUI := wizardStepToDialog(step)
	cur, total := o.wizard.StepNumber()
	isTextMode := step.Type == initwizard.StepTextInput || o.otherMode
	return components.RenderInitWizardDialog(
		stepUI, cur, total,
		o.idx, o.input, isTextMode, width-4,
	)
}
