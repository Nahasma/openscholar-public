package research

// ResearchEvent 研究流水线事件，通过 PubSub 广播
type ResearchEvent struct {
	PipelineID string
	PhaseID    string
	Type       ResearchEventType
	Data       any
}

type ResearchEventType string

const (
	EventPhaseStarted   ResearchEventType = "phase_started"
	EventPhaseCompleted ResearchEventType = "phase_completed"
	EventPhaseFailed    ResearchEventType = "phase_failed"
	EventCheckpoint     ResearchEventType = "checkpoint"
	EventBudgetWarning  ResearchEventType = "budget_warning"
	EventBudgetExceeded ResearchEventType = "budget_exceeded"
)

type PhaseFailureReason string

const (
	FailureReasonExecution     PhaseFailureReason = "execution_failed"
	FailureReasonLeaderFailed  PhaseFailureReason = "leader_failed"
	FailureReasonLeaderCanceled PhaseFailureReason = "leader_canceled"
	FailureReasonLeaderStalled PhaseFailureReason = "leader_stalled"
)

type PhaseFailureData struct {
	Reason      PhaseFailureReason `json:"reason"`
	Error       string             `json:"error,omitempty"`
	Recoverable bool               `json:"recoverable,omitempty"`
}
