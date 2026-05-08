package initwizard

// StepType defines the type of a wizard step.
type StepType int

const (
	StepSelect          StepType = iota // Single selection from options
	StepTextInput                       // Free text input
	StepSelectWithOther                 // Select from options, with "Other" triggering text input
)

// WizardStep defines a single step in the init wizard (UI-agnostic).
type WizardStep struct {
	ID          string
	Question    string
	Type        StepType
	Options     []SelectOption // Used when Type == StepSelect
	Placeholder string         // Used when Type == StepTextInput
}

// Wizard is a UI-agnostic init wizard engine.
// Any frontend (TUI, Web GUI, CLI) can drive it via CurrentStep/Apply/Skip.
type Wizard struct {
	coreSteps   []WizardStep
	detailSteps []WizardStep
	currentIdx  int
	inDetail    bool // whether user chose to continue with detail steps
	profile     UserProfile
	done        bool
}

// NewWizard creates a new wizard with all steps registered.
func NewWizard() *Wizard {
	return &Wizard{
		coreSteps: []WizardStep{
			{ID: "domain", Question: "你的研究领域是？", Type: StepSelectWithOther, Options: DomainOptions},
			{ID: "subdomain", Question: "你的具体研究方向？", Type: StepTextInput, Placeholder: "如 NLP, quantum computing, organic chemistry"},
			{ID: "language", Question: "论文写作语言？", Type: StepSelect, Options: LanguageOptions},
			{ID: "role", Question: "你的学术角色？", Type: StepSelect, Options: RoleOptions},
			{ID: "pub_level", Question: "目标发表级别？", Type: StepSelect, Options: PubLevelOptions},
			{ID: "ask_detail", Question: "是否继续填写详细配置？（可以提升个性化体验）", Type: StepSelect, Options: []SelectOption{
				{"继续详细配置", "yes"},
				{"跳过，开始使用", "no"},
			}},
		},
		detailSteps: []WizardStep{
			{ID: "research_type", Question: "你的研究类型？", Type: StepSelect, Options: ResearchTypeOptions},
			{ID: "concurrent", Question: "同时进行的研究项目数？", Type: StepSelect, Options: ConcurrentOptions},
			{ID: "latex", Question: "LaTeX 熟练程度？", Type: StepSelect, Options: LatexOptions},
			{ID: "collaboration", Question: "你的协作模式？", Type: StepSelect, Options: CollaborationOptions},
			{ID: "feedback", Question: "你偏好的反馈风格？", Type: StepSelect, Options: FeedbackOptions},
		},
	}
}

// CurrentStep returns the current step descriptor, or nil if done.
func (w *Wizard) CurrentStep() *WizardStep {
	if w.done {
		return nil
	}
	steps := w.activeSteps()
	if w.currentIdx >= len(steps) {
		return nil
	}
	s := steps[w.currentIdx]
	return &s
}

// StepNumber returns (1-based current, total) for progress display.
func (w *Wizard) StepNumber() (int, int) {
	steps := w.activeSteps()
	return w.currentIdx + 1, len(steps)
}

// Apply commits the answer for the current step and advances.
// For StepSelect, value should be the SelectOption.Value string.
// For StepTextInput, value is the entered text.
func (w *Wizard) Apply(value string) {
	step := w.CurrentStep()
	if step == nil {
		return
	}

	// Set profile field
	if setter, ok := fieldSetters[step.ID]; ok {
		setter(&w.profile, value)
	}

	// Handle branching for ask_detail
	if step.ID == "ask_detail" {
		if value == "no" {
			w.done = true
			return
		}
		// "yes" → enter detail mode
		w.inDetail = true
		w.currentIdx++ // move past ask_detail
		return
	}

	w.currentIdx++
	if w.currentIdx >= len(w.activeSteps()) {
		w.done = true
	}
}

// Skip skips the current step (leaves field empty) and advances.
func (w *Wizard) Skip() {
	step := w.CurrentStep()
	if step == nil {
		return
	}

	// Skipping ask_detail means no detail
	if step.ID == "ask_detail" {
		w.done = true
		return
	}

	w.currentIdx++
	if w.currentIdx >= len(w.activeSteps()) {
		w.done = true
	}
}

// IsDone returns true when all steps are completed or skipped.
func (w *Wizard) IsDone() bool {
	return w.done
}

// Profile returns the collected profile.
func (w *Wizard) Profile() *UserProfile {
	return &w.profile
}

// Save persists the profile to .openscholar/profile.yaml.
func (w *Wizard) Save(workingDir string) error {
	return SaveProfile(workingDir, &w.profile)
}

// activeSteps returns the current step list based on detail mode.
func (w *Wizard) activeSteps() []WizardStep {
	if w.inDetail {
		// Already past core steps; return combined for index consistency
		return append(w.coreSteps, w.detailSteps...)
	}
	return w.coreSteps
}

// fieldSetters maps step ID to profile field setter.
var fieldSetters = map[string]func(p *UserProfile, v string){
	"domain":        func(p *UserProfile, v string) { p.Domain = v },
	"subdomain":     func(p *UserProfile, v string) { p.Subdomain = v },
	"language":      func(p *UserProfile, v string) { p.Language = v },
	"role":          func(p *UserProfile, v string) { p.Role = v },
	"pub_level":     func(p *UserProfile, v string) { p.PubLevel = v },
	"research_type": func(p *UserProfile, v string) { p.ResearchType = v },
	"concurrent":    func(p *UserProfile, v string) { p.ConcurrentProjects = v },
	"latex":         func(p *UserProfile, v string) { p.LaTeXLevel = v },
	"collaboration": func(p *UserProfile, v string) { p.Collaboration = v },
	"feedback":      func(p *UserProfile, v string) { p.FeedbackStyle = v },
}
