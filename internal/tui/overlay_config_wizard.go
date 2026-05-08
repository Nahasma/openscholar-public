package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/tui/components"
)

// ConfigWizardChoice represents the config wizard completion.
type ConfigWizardChoice struct {
	Wizard *config.ConfigWizard
	Saved  bool
}

// ConfigWizardOverlay implements Overlay for the multi-step config wizard.
type ConfigWizardOverlay struct {
	wizard  *config.ConfigWizard
	idx     int
	input   string
	cursor  int
	menuIdx int
}

// NewConfigWizardOverlay creates a new config wizard overlay.
func NewConfigWizardOverlay(cfg *config.Config) *ConfigWizardOverlay {
	return &ConfigWizardOverlay{
		wizard: config.NewConfigWizard(cfg),
	}
}

func (o *ConfigWizardOverlay) ID() string        { return "config-wizard" }
func (o *ConfigWizardOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *ConfigWizardOverlay) BlocksInput() bool { return true }

func (o *ConfigWizardOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	if o.wizard == nil {
		return o, &OverlayResult{Action: "dismiss", Data: ConfigWizardChoice{Saved: false}}, nil
	}

	// Ctrl+C cancels entirely
	if keyMsg.Type == tea.KeyCtrlC {
		return o, &OverlayResult{Action: "dismiss", Data: ConfigWizardChoice{Wizard: o.wizard, Saved: false}}, nil
	}

	step := o.wizard.CurrentStep()
	if step == nil {
		if o.wizard.IsDone() {
			return o, &OverlayResult{
				Action: "accept",
				Data:   ConfigWizardChoice{Wizard: o.wizard, Saved: true},
			}, nil
		}
		return o, &OverlayResult{Action: "dismiss", Data: ConfigWizardChoice{Wizard: o.wizard, Saved: false}}, nil
	}

	switch step.Type {
	case config.CWStepProviderMenu, config.CWStepAgentMenu:
		return o.handleMenuSelect(keyMsg)
	case config.CWStepProviderKey, config.CWStepProviderBaseURL:
		return o.handleTextInput(keyMsg)
	case config.CWStepDefaultProvider:
		return o.handleSelectSkipOnEsc(keyMsg)
	case config.CWStepAgentProvider, config.CWStepAgentModel:
		return o.handleSelectBackOnEsc(keyMsg)
	case config.CWStepSummary:
		switch keyMsg.Type {
		case tea.KeyEnter:
			if o.wizard.HasChanges() {
				o.wizard.Apply("save")
				return o, &OverlayResult{
					Action: "accept",
					Data:   ConfigWizardChoice{Wizard: o.wizard, Saved: true},
				}, nil
			}
			return o, &OverlayResult{Action: "dismiss", Data: ConfigWizardChoice{Wizard: o.wizard, Saved: false}}, nil
		case tea.KeyEsc:
			return o, &OverlayResult{Action: "dismiss", Data: ConfigWizardChoice{Wizard: o.wizard, Saved: false}}, nil
		}
	}

	return o, nil, nil
}

func (o *ConfigWizardOverlay) handleMenuSelect(keyMsg tea.KeyMsg) (Overlay, *OverlayResult, tea.Cmd) {
	step := o.wizard.CurrentStep()
	if step == nil {
		return o, nil, nil
	}

	switch keyMsg.Type {
	case tea.KeyUp:
		if o.idx > 0 {
			o.idx--
		}
		return o, nil, nil
	case tea.KeyDown:
		if o.idx < len(step.Options)-1 {
			o.idx++
		}
		return o, nil, nil
	case tea.KeyEnter:
		if o.idx < len(step.Options) {
			opt := step.Options[o.idx]
			if opt.Value == "continue" {
				o.wizard.Apply("continue")
				o.idx = 0
			} else {
				o.menuIdx = o.idx
				o.wizard.Apply(opt.Value)
				o.idx = 0
				o.input = ""
				o.cursor = 0
			}
			if o.wizard.IsDone() || o.wizard.IsCancelled() {
				saved := o.wizard.IsDone()
				return o, &OverlayResult{
					Action: "accept",
					Data:   ConfigWizardChoice{Wizard: o.wizard, Saved: saved},
				}, nil
			}
		}
		return o, nil, nil
	case tea.KeyEsc:
		return o, &OverlayResult{Action: "dismiss", Data: ConfigWizardChoice{Wizard: o.wizard, Saved: false}}, nil
	}

	return o, nil, nil
}

func (o *ConfigWizardOverlay) handleTextInput(keyMsg tea.KeyMsg) (Overlay, *OverlayResult, tea.Cmd) {
	runes := []rune(o.input)
	cur := o.cursor
	if cur > len(runes) {
		cur = len(runes)
	}

	switch keyMsg.Type {
	case tea.KeyEnter:
		o.wizard.Apply(o.input)
		o.input = ""
		o.cursor = 0
		o.idx = o.menuIdx
		if o.wizard.IsDone() || o.wizard.IsCancelled() {
			saved := o.wizard.IsDone()
			return o, &OverlayResult{
				Action: "accept",
				Data:   ConfigWizardChoice{Wizard: o.wizard, Saved: saved},
			}, nil
		}
		return o, nil, nil
	case tea.KeyEsc:
		o.wizard.Back()
		o.input = ""
		o.cursor = 0
		o.idx = o.menuIdx
		if o.wizard.IsDone() || o.wizard.IsCancelled() {
			saved := o.wizard.IsDone()
			return o, &OverlayResult{
				Action: "accept",
				Data:   ConfigWizardChoice{Wizard: o.wizard, Saved: saved},
			}, nil
		}
		return o, nil, nil
	case tea.KeyLeft:
		if cur > 0 {
			o.cursor = cur - 1
		}
		return o, nil, nil
	case tea.KeyRight:
		if cur < len(runes) {
			o.cursor = cur + 1
		}
		return o, nil, nil
	case tea.KeyHome, tea.KeyCtrlA:
		o.cursor = 0
		return o, nil, nil
	case tea.KeyEnd, tea.KeyCtrlE:
		o.cursor = len(runes)
		return o, nil, nil
	case tea.KeyBackspace:
		if cur > 0 {
			o.input = string(runes[:cur-1]) + string(runes[cur:])
			o.cursor = cur - 1
		}
		return o, nil, nil
	case tea.KeyDelete:
		if cur < len(runes) {
			o.input = string(runes[:cur]) + string(runes[cur+1:])
		}
		return o, nil, nil
	default:
		var insert string
		if keyMsg.Type == tea.KeyRunes {
			insert = string(keyMsg.Runes)
		} else if keyMsg.Type == tea.KeySpace {
			insert = " "
		}
		if insert != "" {
			o.input = string(runes[:cur]) + insert + string(runes[cur:])
			o.cursor = cur + len([]rune(insert))
		}
		return o, nil, nil
	}
}

func (o *ConfigWizardOverlay) handleSelectSkipOnEsc(keyMsg tea.KeyMsg) (Overlay, *OverlayResult, tea.Cmd) {
	step := o.wizard.CurrentStep()
	if step == nil {
		return o, nil, nil
	}

	switch keyMsg.Type {
	case tea.KeyUp:
		if o.idx > 0 {
			o.idx--
		}
		return o, nil, nil
	case tea.KeyDown:
		if o.idx < len(step.Options)-1 {
			o.idx++
		}
		return o, nil, nil
	case tea.KeyEnter:
		if o.idx < len(step.Options) {
			o.wizard.Apply(step.Options[o.idx].Value)
			o.idx = 0
			if o.wizard.IsDone() || o.wizard.IsCancelled() {
				saved := o.wizard.IsDone()
				return o, &OverlayResult{
					Action: "accept",
					Data:   ConfigWizardChoice{Wizard: o.wizard, Saved: saved},
				}, nil
			}
		}
		return o, nil, nil
	case tea.KeyEsc:
		o.wizard.Skip()
		o.idx = 0
		if o.wizard.IsDone() || o.wizard.IsCancelled() {
			saved := o.wizard.IsDone()
			return o, &OverlayResult{
				Action: "accept",
				Data:   ConfigWizardChoice{Wizard: o.wizard, Saved: saved},
			}, nil
		}
		return o, nil, nil
	}

	return o, nil, nil
}

func (o *ConfigWizardOverlay) handleSelectBackOnEsc(keyMsg tea.KeyMsg) (Overlay, *OverlayResult, tea.Cmd) {
	step := o.wizard.CurrentStep()
	if step == nil {
		return o, nil, nil
	}

	switch keyMsg.Type {
	case tea.KeyUp:
		if o.idx > 0 {
			o.idx--
		}
		return o, nil, nil
	case tea.KeyDown:
		if o.idx < len(step.Options)-1 {
			o.idx++
		}
		return o, nil, nil
	case tea.KeyEnter:
		if o.idx < len(step.Options) {
			o.wizard.Apply(step.Options[o.idx].Value)
			nextStep := o.wizard.CurrentStep()
			if nextStep != nil && (nextStep.Type == config.CWStepProviderMenu || nextStep.Type == config.CWStepAgentMenu) {
				o.idx = o.menuIdx
			} else {
				o.idx = 0
			}
			if o.wizard.IsDone() || o.wizard.IsCancelled() {
				saved := o.wizard.IsDone()
				return o, &OverlayResult{
					Action: "accept",
					Data:   ConfigWizardChoice{Wizard: o.wizard, Saved: saved},
				}, nil
			}
		}
		return o, nil, nil
	case tea.KeyEsc:
		o.wizard.Back()
		nextStep := o.wizard.CurrentStep()
		if nextStep != nil && (nextStep.Type == config.CWStepProviderMenu || nextStep.Type == config.CWStepAgentMenu) {
			o.idx = o.menuIdx
		} else {
			o.idx = 0
		}
		if o.wizard.IsDone() || o.wizard.IsCancelled() {
			saved := o.wizard.IsDone()
			return o, &OverlayResult{
				Action: "accept",
				Data:   ConfigWizardChoice{Wizard: o.wizard, Saved: saved},
			}, nil
		}
		return o, nil, nil
	}

	return o, nil, nil
}

func (o *ConfigWizardOverlay) View(width, height int) string {
	if o.wizard == nil {
		return ""
	}
	step := o.wizard.CurrentStep()
	if step == nil {
		return ""
	}

	contentWidth := width - 4
	if contentWidth < 0 {
		contentWidth = 0
	}
	cur, total, phaseLabel := o.wizard.PhaseProgress()
	return components.RenderConfigWizardDialog(
		step, cur, total, phaseLabel,
		o.idx, o.input, o.cursor,
		o.wizard.Changes(), contentWidth,
	)
}
