package tools

import (
	"context"
	"strings"
)

// research_shared.go — shared view types and helpers used across the three
// narrow research tools (research_pipeline, research_task, research_message).

// ResearchController is the interface for phase management operations.
// Implemented by an adapter in the app package to avoid circular imports.
type ResearchController interface {
	GetBySession(sessionID string) (ResearchPipelineView, error)
	GetPhasesView(pipelineID string) ([]ResearchPhaseView, error)
	AdvancePipeline(pipelineID string) error
	AdvancePipelineWithOptions(pipelineID string, opts ResearchAdvanceOptions) error
	PausePipeline(pipelineID string) error
	SetPipelineMode(pipelineID, mode string) error
}

type ResearchAdvanceOptions struct {
	SkipCheckpoint bool `json:"skip_checkpoint,omitempty"`
}

// ResearchPipelineView is a read-only projection of a research Pipeline
// surfaced to the tool layer without exposing domain internals.
type ResearchPipelineView struct {
	ID, Topic, Template, Status, WorkDir string
	Mode                                 string // "default" | "auto" | "strict"
	BudgetLimit, BudgetSpent             float64
}

// ResearchPhaseView is a read-only projection of a research Phase.
type ResearchPhaseView struct {
	ID, Name, Status string
	Order            int
	Checkpoint       bool
}

// CheckpointEvent is published when a phase advance requires user confirmation.
// The tool blocks on ResponseCh until the TUI responds.
type CheckpointEvent struct {
	ID           string
	PipelineID   string
	PhaseName    string
	PhaseOrder   int
	Summary      string
	Mode         string  // "auto" | "default" | "strict"
	ReviewScore  float64 // >0 if auto-reviewed
	ReviewReport string
	ResponseCh   chan CheckpointResponse
}

// CheckpointResponse carries the user's decision from the TUI dialog.
type CheckpointResponse struct {
	Approved bool
	Feedback string
}

type researchRootSessionContextKey string

// ResearchRootSessionContextKey stores the session id that owns a research
// pipeline when tools are running inside child task sessions.
const ResearchRootSessionContextKey researchRootSessionContextKey = "research_root_session_id"

// ResearchSessionID resolves the owning research session. In child task
// sessions, ResearchRootSessionContextKey takes precedence.
func ResearchSessionID(ctx context.Context) string {
	if ctx != nil {
		if root, ok := ctx.Value(ResearchRootSessionContextKey).(string); ok {
			root = strings.TrimSpace(root)
			if root != "" {
				return root
			}
		}
	}
	sessionID, _ := GetContextValues(ctx)
	return strings.TrimSpace(sessionID)
}
