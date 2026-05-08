package research

// Pipeline 表示一个科研项目的完整流水线
type Pipeline struct {
	ID        string
	SessionID string
	Topic     string
	Template  string         // "empirical" | "survey" | "theoretical"
	Mode      AutomationMode // "default" | "auto" | "strict"
	Status    PipelineStatus
	Budget    Budget
	WorkDir   string
	CreatedAt int64
	UpdatedAt int64
}

type PipelineStatus string

const (
	StatusPlanning  PipelineStatus = "planning"
	StatusRunning   PipelineStatus = "running"
	StatusPaused    PipelineStatus = "paused"
	StatusCompleted PipelineStatus = "completed"
	StatusFailed    PipelineStatus = "failed"
)

type AutomationMode string

const (
	ModeDefault AutomationMode = "default"
	ModeAuto    AutomationMode = "auto"
	ModeStrict  AutomationMode = "strict"
)

type Budget struct {
	Limit float64 // 总预算（美元），0 = 不限
	Spent float64
}

// Phase 表示流水线中的一个阶段
type Phase struct {
	ID          string
	PipelineID  string
	Name        string
	Order       int
	Status      PhaseStatus
	Checkpoint  bool
	MaxWorkers  int
	CreatedAt   int64
	CompletedAt int64
}

type PhaseStatus string

const (
	PhasePending   PhaseStatus = "pending"
	PhaseRunning   PhaseStatus = "running"
	PhaseCompleted PhaseStatus = "completed"
	PhaseFailed    PhaseStatus = "failed"
	PhasePaused    PhaseStatus = "paused"
)

// NeedsCheckpoint returns true if the pipeline should pause for user review
// after the given phase completes.
//
// Rules (in priority order):
//  1. If the phase has an explicit Checkpoint field set, respect it.
//  2. In "strict" mode every phase is a checkpoint.
//  3. In "auto" mode no phase is a checkpoint (reviewer agent decides).
//  4. In "default" mode only phases with Checkpoint==true pause.
func (p *Pipeline) NeedsCheckpoint(phase *Phase) bool {
	switch p.Mode {
	case ModeStrict:
		return true
	case ModeAuto:
		return false
	default: // ModeDefault and anything unrecognised
		return phase.Checkpoint
	}
}
