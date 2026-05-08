package codeagent

import (
	"context"
	"time"
)

// CodeAgentProvider defines the interface for external AI coding CLI tools.
type CodeAgentProvider interface {
	// Name returns the provider identifier (e.g. "claude", "gemini", "codex").
	Name() string

	// Available reports whether the CLI tool is installed and callable.
	Available() bool

	// Execute runs a coding task and returns the result.
	Execute(ctx context.Context, req CodeRequest) (*CodeResponse, error)
}

// CodeRequest is the unified request sent to any code agent provider.
type CodeRequest struct {
	Prompt    string        // Task description
	WorkDir   string        // Working directory for code operations
	MaxTurns  int           // Max iteration rounds (0 = provider default)
	SessionID string        // Non-empty to resume a previous session
	Timeout   time.Duration // Execution timeout (0 = default 300s)

	// Claude-specific flags
	AppendSystemPrompt string // --append-system-prompt
	Bare               bool   // --bare (suppress interactive UI chrome)
	AllowedTools       string // --allowedTools (comma-separated)

	// Codex-specific flags
	Ephemeral  bool   // --ephemeral (stateless run)
	OutputFile string // -o <file>
}

// ProviderUsage records token consumption reported by the provider.
type ProviderUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// CodeResponse is the unified response from any code agent provider.
type CodeResponse struct {
	Result    string // Execution result text
	SessionID string // Session ID for subsequent --resume calls
	Success   bool   // Whether the task completed successfully

	StartedAt  time.Time      `json:"started_at"`
	EndedAt    time.Time      `json:"ended_at"`
	DurationMs int64          `json:"duration_ms"`
	Provider   string         `json:"provider"`
	ExitReason string         `json:"exit_reason"`
	Usage      *ProviderUsage `json:"usage,omitempty"`
}

// ProviderConfig holds per-provider configuration from config.json.
type ProviderConfig struct {
	AllowedTools string // Comma-separated tool whitelist (claude-specific)
	MaxTurns     int    // Default max turns for this provider
}
