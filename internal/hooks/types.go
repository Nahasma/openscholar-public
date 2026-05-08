package hooks

import "time"

// Event represents a lifecycle hook event type.
type Event string

const (
	PreToolUse           Event = "pre_tool_use"
	PostToolUse          Event = "post_tool_use"
	PostToolUseFailure   Event = "post_tool_use_failure"
	ProviderCooldown     Event = "provider_cooldown"
	ArtifactCreated      Event = "artifact_created"
	BudgetExhausted      Event = "budget_exhausted"
	UserPromptSubmit     Event = "user_prompt_submit"
	SessionStart         Event = "session_start"
	SessionEnd           Event = "session_end"
	SubagentStart        Event = "subagent_start"
	SubagentStop         Event = "subagent_stop"
	PreCompact           Event = "pre_compact"
	PostCompact          Event = "post_compact"
	PermissionRequest    Event = "permission_request"
	PermissionDenied     Event = "permission_denied"
	TaskCreated          Event = "task_created"
	TaskCompleted        Event = "task_completed"
	CronTriggered        Event = "cron_triggered"
	AwaySummaryGenerated Event = "away_summary_generated"
)

// ValidEvent reports whether event is a lifecycle hook event understood by the
// runtime.
func ValidEvent(event Event) bool {
	switch event {
	case PreToolUse,
		PostToolUse,
		PostToolUseFailure,
		ProviderCooldown,
		ArtifactCreated,
		BudgetExhausted,
		UserPromptSubmit,
		SessionStart,
		SessionEnd,
		SubagentStart,
		SubagentStop,
		PreCompact,
		PostCompact,
		PermissionRequest,
		PermissionDenied,
		TaskCreated,
		TaskCompleted,
		CronTriggered,
		AwaySummaryGenerated:
		return true
	default:
		return false
	}
}

// toolHookEvents are events that carry tool information and are subject to
// hookDepth anti-recursion checks.
var toolHookEvents = map[Event]bool{
	PreToolUse:         true,
	PostToolUse:        true,
	PostToolUseFailure: true,
	ProviderCooldown:   true,
	ArtifactCreated:    true,
}

// depthAllowlistEvents are events always allowed regardless of hookDepth.
var depthAllowlistEvents = map[Event]bool{
	TaskCreated:   true,
	TaskCompleted: true,
}

// Input is the data payload passed to hook commands via stdin as JSON.
type Input struct {
	SessionID     string         `json:"session_id"`
	ToolName      string         `json:"tool_name,omitempty"`
	ToolInput     map[string]any `json:"tool_input,omitempty"`
	ToolResult    string         `json:"tool_result,omitempty"`
	IsError       bool           `json:"is_error,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	Source        string         `json:"source,omitempty"`
	ErrorKind     string         `json:"error_kind,omitempty"`
	ProgressKind  string         `json:"progress_kind,omitempty"`
	GoalType      string         `json:"goal_type,omitempty"`
	ArtifactPaths []string       `json:"artifact_paths,omitempty"`
	UserPrompt    string         `json:"user_prompt,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
}

// HookConfig represents a single hook definition loaded from config.json.
type HookConfig struct {
	Event   Event     `json:"event"`
	Command string    `json:"command"`           // shell command to exec
	Timeout int       `json:"timeout,omitempty"` // seconds, default 10
	If      *IfConfig `json:"if,omitempty"`      // optional condition
	Async   bool      `json:"async,omitempty"`
	Once    bool      `json:"once,omitempty"`
}

// IfConfig holds the optional condition fields for a HookConfig.
type IfConfig struct {
	Tool         string `json:"tool,omitempty"`          // tool name glob: "Bash", "Edit", "*"
	Path         string `json:"path,omitempty"`          // file path glob: "*.go", "internal/**"
	Provider     string `json:"provider,omitempty"`      // provider/source metadata glob
	Source       string `json:"source,omitempty"`        // provider source metadata glob
	ErrorKind    string `json:"error_kind,omitempty"`    // error taxonomy glob
	ProgressKind string `json:"progress_kind,omitempty"` // progress kind glob
	GoalType     string `json:"goal_type,omitempty"`     // goal type glob
	Artifact     string `json:"artifact,omitempty"`      // artifact path glob
}

// hookDepthKey is the context key used to track hook call depth.
type hookDepthKey struct{}
