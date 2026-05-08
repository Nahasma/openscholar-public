package tui

import (
	"github.com/openscholar/openscholar/internal/config"
	initwizard "github.com/openscholar/openscholar/internal/init"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/picker"
	"github.com/openscholar/openscholar/internal/session"
)

// DialogStates is the container for all dialog sub-states.
type DialogStates struct {
	perm            PermissionState
	planApproval    PlanApprovalState
	clarification   ClarificationState
	checkpoint      CheckpointState
	template        TemplateState
	modelSelect     ModelSelectState
	sessionBrowser  SessionBrowserState
	initWizard      InitWizardState
	configWizard    ConfigWizardState
	workspace       WorkspaceState
	researchSuggest ResearchSuggestionState
}

// PermissionState holds pending permission dialog state.
type PermissionState struct {
	pending   *permission.PermissionRequest
	optionIdx int
}

func (s *PermissionState) Reset() { *s = PermissionState{} }

// PlanApprovalState holds the dedicated ExitPlanMode approval dialog state.
type PlanApprovalState struct {
	pending   *tools.PlanApprovalEvent
	optionIdx int
	feedback  string
	inputMode bool
}

func (s *PlanApprovalState) Reset() { *s = PlanApprovalState{} }

// ClarificationState holds AskUser clarification dialog state.
type ClarificationState struct {
	pending *tools.ClarificationEvent
	idx     int
	input   string
	focused bool // true = focus on freeform input
}

func (s *ClarificationState) Reset() { *s = ClarificationState{} }

// CheckpointState holds checkpoint confirmation dialog state.
type CheckpointState struct {
	pending   *tools.CheckpointEvent
	optionIdx int
	feedback  string
	inputMode bool // true = typing rejection feedback
}

func (s *CheckpointState) Reset() { *s = CheckpointState{} }

// TemplateState holds template selection dialog state.
type TemplateState struct {
	options     []string // ["empirical", "survey", "theoretical"]
	idx         int
	folderName  string // user-specified workspace directory path
	pendingText string // stash user input while template dialog is shown
	picker      *picker.State
	fileIndex   []picker.FileEntry
}

func (s *TemplateState) Reset() { *s = TemplateState{} }

// ModelSelectState holds model selection dialog state.
type ModelSelectState struct {
	providers       []models.ModelProvider
	providerIdx     int
	list            []models.Model
	listIdx         int
	listScroll      int
	selectedModelID models.ModelID
	discovered      map[models.ModelProvider][]models.Model
	loadingProvider models.ModelProvider
	listWarning     string
}

func (s *ModelSelectState) Reset() { *s = ModelSelectState{} }

// SessionBrowserState holds session browser dialog state.
type SessionBrowserState struct {
	list       []session.Session
	listIdx    int
	listScroll int
}

func (s *SessionBrowserState) Reset() { *s = SessionBrowserState{} }

// InitWizardState holds init wizard dialog state.
type InitWizardState struct {
	wizard    *initwizard.Wizard
	idx       int
	input     string
	otherMode bool
	choiceIdx int // 0=开始初始化, 1=跳过 (for stateInitRequired)
}

func (s *InitWizardState) Reset() { *s = InitWizardState{} }

// ConfigWizardState holds config wizard dialog state.
type ConfigWizardState struct {
	wizard  *config.ConfigWizard
	idx     int
	input   string
	cursor  int
	menuIdx int
}

func (s *ConfigWizardState) Reset() { *s = ConfigWizardState{} }

// WorkspaceState holds workspace selection dialog state.
type WorkspaceState struct {
	confirmed bool
	idx       int
	inputMode bool
	input     string
}

func (s *WorkspaceState) Reset() { *s = WorkspaceState{} }

// ResearchSuggestionState holds research suggestion dialog state.
type ResearchSuggestionState struct {
	idx  int
	text string
}

func (s *ResearchSuggestionState) Reset() { *s = ResearchSuggestionState{} }
