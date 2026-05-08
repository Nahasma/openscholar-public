package tui

import (
	"math/rand"
	"time"

	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// ProcessingPhase represents the current phase of agent processing.
type ProcessingPhase uint8

const (
	PhaseIdle        ProcessingPhase = iota // No processing
	PhaseThinking                           // Waiting for first token (0-1s: silent, >1s: active)
	PhaseToolQueued                         // Tools known but none started yet
	PhaseToolRunning                        // Tool execution in progress
	PhaseStreaming                          // Receiving assistant content tokens
	PhaseCompacting                         // Context compaction in progress
)

// ProcessingState is the unified processing state driven by agent events.
// Both the chat area and status bar read from this single source of truth.
type ProcessingState struct {
	Phase        ProcessingPhase
	Label        string    // "reasoning", "running Bash", etc.
	ActiveTool   string    // Current tool name (Phase=ToolRunning)
	StartedAt    time.Time // When this phase started
	SpinnerFrame int       // Mirrors StatusFeature.spinnerFrame for convenience
	QueuedCount  int       // Number of tools in queued state
	TotalTools   int       // Total tools in current batch
}

// ElapsedSeconds returns seconds since the current phase started.
func (ps ProcessingState) ElapsedSeconds() int {
	if ps.Phase == PhaseIdle {
		return 0
	}
	return int(time.Since(ps.StartedAt).Seconds())
}

// IsActiveWaiting returns true when thinking phase has lasted >1s.
func (ps ProcessingState) IsActiveWaiting() bool {
	return ps.Phase == PhaseThinking && time.Since(ps.StartedAt) > time.Second
}

// StatusFeature groups processing state, mode, and spinner.
type StatusFeature struct {
	mode               string // "" | "auto" | "research" | "plan"
	isProcessing       bool
	isCompacting       bool
	spinnerFrame       int
	ticking            bool // whether spinner tick is active
	notice             components.TransientNotice
	researchPipelineID string // active research pipeline ID
	ctrlCPending       bool   // waiting for second Ctrl+C to quit

	// Wave 2: Unified processing state
	processing ProcessingState

	// StatusV2 fields
	processingVerb    string // "reasoning", "writing", etc.
	processingElapsed int    // seconds since processing started
	overlayName       string // active overlay name (e.g., "Help", "Model")
	bgTaskCount       int    // number of background tasks
	memoryCount       int    // memory pill count

	// Wave 3: Memory UI
	memoryToast components.MemoryToastManager

	// Wave 4: Research pipeline progress
	pipelineProgress components.PipelineProgressData

	// Wave 4: Progress presentation layer
	telemetry   ProgressTelemetry
	sessionVerb string // random verb chosen per agent turn

	// Runtime tool lifecycle cache (event-driven progress feedback).
	toolRuntime map[string]ToolRuntimeState
	runtimeErr  RuntimeErrorState
}

type RuntimeErrorState struct {
	Summary  string
	Detail   string
	Expanded bool
}

// ToolRuntimeState is the latest observed lifecycle state for a tool call.
type ToolRuntimeState struct {
	ToolName      string
	BatchID       string
	Input         string
	State         message.ToolCallState
	Order         int
	Total         int
	DurationMs    int64
	ResultSummary string
}

// ProgressTelemetry tracks timing and count facts for the progress presentation layer.
type ProgressTelemetry struct {
	LastTokenAt         time.Time // last time a streaming token arrived
	LastTokenCount      int       // actual token/char count from streaming
	DisplayedTokenCount int       // smoothly animated display value
	ActiveToolStartedAt time.Time // when the current tool started
	StalledIntensity    float64   // 0.0-1.0, smoothed stalled severity
}

// progressSessionVerbs are academic-flavored verbs shown during processing.
var progressSessionVerbs = []string{
	"Thinking", "Reasoning", "Analyzing", "Composing", "Synthesizing",
	"Researching", "Formulating", "Computing", "Architecting", "Crafting",
	"Evaluating", "Processing", "Generating", "Structuring", "Pondering",
	"Deliberating", "Examining", "Investigating", "Constructing", "Refining",
	"Considering", "Reviewing", "Preparing", "Developing", "Designing",
	"Organizing", "Outlining", "Drafting", "Conceptualizing", "Contemplating",
}

// pickSessionVerb picks a random verb for this processing turn.
func pickSessionVerb() string {
	return progressSessionVerbs[rand.Intn(len(progressSessionVerbs))]
}
