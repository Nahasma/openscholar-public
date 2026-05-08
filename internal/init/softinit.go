package initwizard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Soft Init step state machine
type softStep int

const (
	softStepDomain    softStep = iota // Select research domain
	softStepSubdomain                 // Input specific area
	softStepLanguage                  // Select writing language
	softStepRole                      // Select academic role
	softStepPubLevel                  // Select publication level
	softStepAskDetail                 // Ask if user wants detailed questions
	// Detailed mode steps
	softStepResearchType
	softStepConcurrentProjects
	softStepWeaknesses
	softStepLaTeX
	softStepCollaboration
	softStepFeedback
	softStepSpecialNeeds
	softStepDone
)

// Options for each select step
var (
	domainOptions = []selectOption{
		{"Computer Science", "Computer Science"},
		{"Physics", "Physics"},
		{"Mathematics", "Mathematics"},
		{"Chemistry", "Chemistry"},
		{"Biology", "Biology"},
		{"Medicine", "Medicine"},
		{"Engineering", "Engineering"},
		{"Earth & Environmental", "Earth & Environmental"},
		{"Economics", "Economics"},
		{"Social Sciences", "Social Sciences"},
		{"Humanities", "Humanities"},
		{"Other", "Other"},
	}

	languageOptions = []selectOption{
		{"English", "English"},
		{"中文", "Chinese"},
		{"Bilingual (EN+CN)", "Bilingual"},
		{"日本語", "Japanese"},
		{"한국어", "Korean"},
		{"Other", "Other"},
	}

	roleOptions = []selectOption{
		{"Undergraduate", "Undergraduate"},
		{"Master", "Master"},
		{"PhD", "PhD"},
		{"Postdoc", "Postdoc"},
		{"Faculty", "Faculty"},
		{"Industry Researcher", "Industry"},
	}

	pubLevelOptions = []selectOption{
		{"Top-tier (Nature, NeurIPS, Cell, PRL...)", "top"},
		{"Well-known (AAAI, PRB, JACS...)", "known"},
		{"Regular journals", "regular"},
		{"Preprint (arXiv, etc.)", "preprint"},
		{"Thesis / Dissertation", "thesis"},
		{"No specific target yet", "undecided"},
	}

	researchTypeOptions = []selectOption{
		{"Primarily theoretical", "theory"},
		{"Primarily experimental", "experiment"},
		{"Theory + Experiment", "theory+experiment"},
		{"Engineering & Systems", "engineering"},
		{"Survey & Analysis", "survey"},
	}

	concurrentOptions = []selectOption{
		{"1 (focused)", "1"},
		{"2-3", "2-3"},
		{"4+", "4+"},
	}

	weaknessOptions = []selectOption{
		{"Literature review", "literature_review"},
		{"Method description clarity", "method_clarity"},
		{"Results analysis", "results_analysis"},
		{"Academic English", "academic_english"},
		{"Figure/table design", "figure_design"},
		{"Submission formatting", "submission_format"},
		{"Logical argumentation", "logical_structure"},
	}

	latexOptions = []selectOption{
		{"Beginner (need syntax guidance)", "beginner"},
		{"Intermediate (can write, occasional issues)", "intermediate"},
		{"Expert (no help needed)", "expert"},
	}

	collaborationOptions = []selectOption{
		{"Solo researcher", "solo"},
		{"Small team (2-5 people)", "small_team"},
		{"Large team (cross-group/cross-institution)", "large_team"},
	}

	feedbackOptions = []selectOption{
		{"Direct criticism (point out all issues)", "direct"},
		{"Gentle suggestions", "gentle"},
		{"Only comment when asked", "passive"},
	}
)

type selectOption struct {
	Label string
	Value string
}

type softInitModel struct {
	step       softStep
	workingDir string

	// Collected profile data
	profile UserProfile

	// UI state
	selectIdx  int
	textInput  textinput.Model
	multiSel   []bool // for weaknesses multi-select
	wantDetail bool

	// Output
	err  error
	done bool
}

func newSoftInitModel(workingDir string) softInitModel {
	ti := textinput.New()
	ti.Placeholder = "e.g., NLP, quantum computing, organic chemistry"
	ti.Focus()
	ti.CharLimit = 256

	return softInitModel{
		step:       softStepDomain,
		workingDir: workingDir,
		textInput:  ti,
		multiSel:   make([]bool, len(weaknessOptions)),
	}
}

// RunSoftInit launches the Soft Init questionnaire and saves the profile.
func RunSoftInit(workingDir string) error {
	m := newSoftInitModel(workingDir)

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("soft init error: %w", err)
	}

	soft, ok := finalModel.(softInitModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}
	if soft.err != nil {
		return soft.err
	}

	return SaveProfile(workingDir, &soft.profile)
}

func (m softInitModel) Init() tea.Cmd {
	return nil
}

func (m softInitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.err = fmt.Errorf("cancelled by user")
			m.done = true
			return m, tea.Quit
		}
	}

	switch m.step {
	case softStepDomain:
		return m.updateSelect(msg, domainOptions, func(v string) {
			m.profile.Domain = v
		}, softStepSubdomain)

	case softStepSubdomain:
		return m.updateTextInput(msg, softStepLanguage, func(v string) {
			m.profile.Subdomain = v
		})

	case softStepLanguage:
		return m.updateSelect(msg, languageOptions, func(v string) {
			m.profile.Language = v
		}, softStepRole)

	case softStepRole:
		return m.updateSelect(msg, roleOptions, func(v string) {
			m.profile.Role = v
		}, softStepPubLevel)

	case softStepPubLevel:
		return m.updateSelect(msg, pubLevelOptions, func(v string) {
			m.profile.PubLevel = v
		}, softStepAskDetail)

	case softStepAskDetail:
		return m.updateAskDetail(msg)

	case softStepResearchType:
		return m.updateSelect(msg, researchTypeOptions, func(v string) {
			m.profile.ResearchType = v
		}, softStepConcurrentProjects)

	case softStepConcurrentProjects:
		return m.updateSelect(msg, concurrentOptions, func(v string) {
			m.profile.ConcurrentProjects = v
		}, softStepWeaknesses)

	case softStepWeaknesses:
		return m.updateMultiSelect(msg)

	case softStepLaTeX:
		return m.updateSelect(msg, latexOptions, func(v string) {
			m.profile.LaTeXLevel = v
		}, softStepCollaboration)

	case softStepCollaboration:
		return m.updateSelect(msg, collaborationOptions, func(v string) {
			m.profile.Collaboration = v
		}, softStepFeedback)

	case softStepFeedback:
		return m.updateSelect(msg, feedbackOptions, func(v string) {
			m.profile.FeedbackStyle = v
		}, softStepSpecialNeeds)

	case softStepSpecialNeeds:
		return m.updateTextInput(msg, softStepDone, func(v string) {
			m.profile.SpecialNeeds = v
		})

	case softStepDone:
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// updateSelect handles a list selection step.
func (m softInitModel) updateSelect(msg tea.Msg, options []selectOption, setter func(string), next softStep) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if m.selectIdx > 0 {
				m.selectIdx--
			}
		case "down", "j":
			if m.selectIdx < len(options)-1 {
				m.selectIdx++
			}
		case "enter":
			setter(options[m.selectIdx].Value)
			m.selectIdx = 0
			m.step = next
			if next == softStepSubdomain || next == softStepSpecialNeeds {
				m.textInput.SetValue("")
				if next == softStepSubdomain {
					m.textInput.Placeholder = "e.g., NLP, quantum computing, organic chemistry"
				} else {
					m.textInput.Placeholder = "e.g., need bilingual abstracts, prefer Word over LaTeX"
				}
				m.textInput.Focus()
				return m, textinput.Blink
			}
			return m, nil
		}
	}
	return m, nil
}

// updateTextInput handles a text input step.
func (m softInitModel) updateTextInput(msg tea.Msg, next softStep, setter func(string)) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "enter" {
		setter(strings.TrimSpace(m.textInput.Value()))
		m.selectIdx = 0
		m.step = next
		if next == softStepDone {
			m.done = true
			return m, tea.Quit
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// updateAskDetail handles the "want detailed questions?" prompt.
func (m softInitModel) updateAskDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "y", "Y", "enter":
			m.wantDetail = true
			m.step = softStepResearchType
			m.selectIdx = 0
			return m, nil
		case "n", "N", "esc":
			m.wantDetail = false
			m.step = softStepDone
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// updateMultiSelect handles the weaknesses multi-select step.
func (m softInitModel) updateMultiSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if m.selectIdx > 0 {
				m.selectIdx--
			}
		case "down", "j":
			if m.selectIdx < len(weaknessOptions)-1 {
				m.selectIdx++
			}
		case " ":
			m.multiSel[m.selectIdx] = !m.multiSel[m.selectIdx]
		case "enter":
			var selected []string
			for i, opt := range weaknessOptions {
				if m.multiSel[i] {
					selected = append(selected, opt.Value)
				}
			}
			m.profile.Weaknesses = selected
			m.selectIdx = 0
			m.step = softStepLaTeX
			return m, nil
		}
	}
	return m, nil
}

// View renders the soft init UI.
func (m softInitModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	switch m.step {
	case softStepDomain:
		return m.viewSelect(titleStyle, dimStyle, "Research Domain", domainOptions, "")
	case softStepSubdomain:
		return m.viewTextInput(titleStyle, dimStyle, "Specific Research Area")
	case softStepLanguage:
		return m.viewSelect(titleStyle, dimStyle, "Writing Language", languageOptions, "")
	case softStepRole:
		return m.viewSelect(titleStyle, dimStyle, "Academic Role", roleOptions, "")
	case softStepPubLevel:
		return m.viewSelect(titleStyle, dimStyle, "Target Publication Level", pubLevelOptions,
			"This determines writing standards and review rigor")
	case softStepAskDetail:
		return m.viewAskDetail(titleStyle, dimStyle)
	case softStepResearchType:
		return m.viewSelect(titleStyle, dimStyle, "Research Type", researchTypeOptions, "")
	case softStepConcurrentProjects:
		return m.viewSelect(titleStyle, dimStyle, "Concurrent Projects", concurrentOptions, "")
	case softStepWeaknesses:
		return m.viewMultiSelect(titleStyle, dimStyle)
	case softStepLaTeX:
		return m.viewSelect(titleStyle, dimStyle, "LaTeX / Typesetting Experience", latexOptions, "")
	case softStepCollaboration:
		return m.viewSelect(titleStyle, dimStyle, "Collaboration Style", collaborationOptions, "")
	case softStepFeedback:
		return m.viewSelect(titleStyle, dimStyle, "Preferred Feedback Style", feedbackOptions, "")
	case softStepSpecialNeeds:
		return m.viewTextInput(titleStyle, dimStyle, "Anything else we should know?")
	case softStepDone:
		return "\n  ✓ Profile saved!\n"
	}
	return ""
}

func (m softInitModel) viewSelect(titleStyle, dimStyle lipgloss.Style, title string, options []selectOption, hint string) string {
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s\n", titleStyle.Render("  "+title)))
	if hint != "" {
		sb.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render(hint)))
	}
	sb.WriteString("\n")

	for i, opt := range options {
		marker := "  "
		label := opt.Label
		if i == m.selectIdx {
			marker = "→ "
			label = selectedStyle.Render(label)
		}
		sb.WriteString(fmt.Sprintf("  %s%s\n", marker, label))
	}

	sb.WriteString(fmt.Sprintf("\n%s\n", dimStyle.Render("  ↑↓ select  |  Enter confirm")))
	return sb.String()
}

func (m softInitModel) viewTextInput(titleStyle, dimStyle lipgloss.Style, title string) string {
	return fmt.Sprintf(`
%s

  %s

%s`,
		titleStyle.Render("  "+title),
		m.textInput.View(),
		dimStyle.Render("  Enter confirm"),
	)
}

func (m softInitModel) viewAskDetail(titleStyle, dimStyle lipgloss.Style) string {
	return fmt.Sprintf(`
%s

  Basic profile collected! Would you like to answer more questions
  for deeper personalization? (7 additional questions)

%s`,
		titleStyle.Render("  Personalization"),
		dimStyle.Render("  Y/Enter = Yes  |  N/Esc = Skip"),
	)
}

func (m softInitModel) viewMultiSelect(titleStyle, dimStyle lipgloss.Style) string {
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s\n", titleStyle.Render("  Writing areas you'd like more help with")))
	sb.WriteString(fmt.Sprintf("  %s\n\n", dimStyle.Render("Select all that apply")))

	for i, opt := range weaknessOptions {
		marker := "  "
		check := "[ ]"
		label := opt.Label
		if m.multiSel[i] {
			check = "[✓]"
		}
		if i == m.selectIdx {
			marker = "→ "
			label = selectedStyle.Render(label)
		}
		sb.WriteString(fmt.Sprintf("  %s%s %s\n", marker, check, label))
	}

	sb.WriteString(fmt.Sprintf("\n%s\n", dimStyle.Render("  ↑↓ navigate  |  Space toggle  |  Enter confirm")))
	return sb.String()
}
