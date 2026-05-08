package task

import "time"

// Kind identifies the category of a task.
type Kind string

const (
	KindToolAsync     Kind = "tool-async"
	KindAgent         Kind = "agent"
	KindResearchPhase Kind = "research-phase"
	KindHook          Kind = "hook"
	KindCron          Kind = "cron"
	KindDream         Kind = "dream"
)

// Status represents the lifecycle state of a task.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

// VerifyStatus tracks the verification gate for TaskV2 subtasks.
type VerifyStatus string

const (
	VerifyStatusSkipped VerifyStatus = "skipped"
	VerifyStatusPending VerifyStatus = "pending"
	VerifyStatusRunning VerifyStatus = "running"
	VerifyStatusPassed  VerifyStatus = "passed"
	VerifyStatusFailed  VerifyStatus = "failed"
)

// Meta holds the common metadata shared by all task kinds.
type Meta struct {
	ID             string     `json:"id"`
	Label          string     `json:"label"`
	SessionID      string     `json:"session_id"`
	Kind           Kind       `json:"kind"`
	Status         Status     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	IsBackgrounded bool       `json:"is_backgrounded"`
	Notified       bool       `json:"notified"`
	NotifyClaimed  bool       `json:"notify_claimed,omitempty"`
	ParentTaskID   string     `json:"parent_task_id,omitempty"`
}

// State is the interface all task state types must satisfy.
type State interface {
	TaskMeta() *Meta
	TaskKind() Kind
}

// BaseState is a general-purpose State implementation that can be used directly
// or embedded in kind-specific state structs.
type BaseState struct {
	M Meta
}

// TaskMeta returns a pointer to the embedded Meta.
func (s *BaseState) TaskMeta() *Meta { return &s.M }

// TaskKind returns the Kind stored in Meta.
func (s *BaseState) TaskKind() Kind { return s.M.Kind }

func cloneMeta(meta Meta) Meta {
	cloned := meta
	if meta.EndedAt != nil {
		endedAt := *meta.EndedAt
		cloned.EndedAt = &endedAt
	}
	return cloned
}

// CloneState returns a detached copy safe for read-only consumers.
func CloneState(state State) State {
	switch s := state.(type) {
	case *SubtaskState:
		cloned := *s
		cloned.BaseState = BaseState{M: cloneMeta(s.BaseState.M)}
		if s.WriteSet != nil {
			cloned.WriteSet = append([]string(nil), s.WriteSet...)
		}
		return &cloned
	case *BaseState:
		return &BaseState{M: cloneMeta(s.M)}
	default:
		return &BaseState{M: cloneMeta(*state.TaskMeta())}
	}
}
