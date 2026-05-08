package cron

import "time"

// Task represents a scheduled task that fires at specified cron intervals.
type Task struct {
	ID        string     `json:"id"`
	Spec      string     `json:"spec"`              // cron expression
	Prompt    string     `json:"prompt"`             // prompt to send when fired
	SessionID string     `json:"sessionId,omitempty"` // owning session for task registry isolation
	CreatedAt time.Time  `json:"createdAt"`
	NextRunAt time.Time  `json:"nextRunAt"`          // persisted to avoid recomputation on restart
	LastRunAt *time.Time `json:"lastRunAt,omitempty"`
	Recurring bool       `json:"recurring"`          // true=repeat, false=one-shot
	Durable   bool       `json:"durable"`            // true=persist to disk
}

// OnFireFunc is the callback invoked when a task fires.
type OnFireFunc func(task Task)
