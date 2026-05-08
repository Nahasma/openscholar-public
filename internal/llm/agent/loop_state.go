package agent

import "time"

// LoopState holds mutable runtime state for a single processGeneration() call.
// Updated after each LLM response and tool execution round.
type LoopState struct {
	Iteration         int
	TotalToolCalls    int
	TotalPromptTokens int64 // authoritative: input + cacheRead + cacheCreation
	TotalOutputTokens int64
	CostAccumulated   float64
	CostUnknown       bool
	StartedAt         time.Time
	LastDeltaTokens   int64 // delta between current and previous TotalPromptTokens (for diminishing returns)
	StreamRetries     int

	// Phase B: budget governance
	LastRoundToolSignatures []string // 本轮工具调用指纹列表（每轮覆盖）
	LastRoundToolOutcomes   []ToolOutcome
	ContinuationCount       int // BudgetController 累计 continuation 次数
	LastRoundProgress       RoundProgress
	SeenEvidence            map[string]int
	TargetCounts            map[string]int
	OutcomeCounts           map[string]int
	SearchToolCalls         int
	FailureCounts           map[string]int

	// Provider cooldown governance is scoped to this generation loop.
	ProviderCooldownConsecutive int
	ProviderCooldownLastScope   string
}

type ToolOutcome struct {
	ToolCallID      string
	ToolName        string
	Provider        string
	Source          string
	Action          string
	ProgressKind    string
	ErrorKind       string
	QueryKey        string
	IsError         bool
	Recoverable     bool
	DurableProgress bool
	ArtifactPaths   []string
	ResultBytes     int
	ContentEmpty    bool
	TargetKey       string
	CanonicalURL    string
	ContentClass    string
	PromptClass     string
	EvidenceKeys    []string
	OutcomeHash     string
	LowValueReason  string
	Signature       string
}

type RoundProgress struct {
	ToolCalls          int
	SuccessfulTools    int
	FailedTools        int
	DurableProgress    bool
	NewEvidenceCount   int
	ArtifactPaths      []string
	LowValueInspection bool
	ResultBytes        int
	OutputTokens       int64
}

type toolExecutionBudgetContextKey struct{}

var toolExecutionBudgetKey = toolExecutionBudgetContextKey{}

type toolExecutionBudget struct {
	RemainingToolCalls   int
	RemainingSearchCalls int
}

// NewLoopState creates a fresh LoopState with StartedAt set to now.
func NewLoopState() LoopState {
	return LoopState{
		StartedAt:     time.Now(),
		SeenEvidence:  map[string]int{},
		TargetCounts:  map[string]int{},
		OutcomeCounts: map[string]int{},
		FailureCounts: map[string]int{},
	}
}

// UpdateFromUsage updates the state from provider-returned token usage.
// promptEffective = inputTokens + cacheRead + cacheCreation (authoritative total input).
func (ls *LoopState) UpdateFromUsage(inputTokens, outputTokens, cacheRead, cacheCreation int64, cost float64, costKnown bool) {
	promptEffective := inputTokens + cacheRead + cacheCreation
	ls.LastDeltaTokens = promptEffective - ls.TotalPromptTokens
	ls.TotalPromptTokens = promptEffective
	ls.TotalOutputTokens += outputTokens
	ls.CostAccumulated += cost
	if !costKnown {
		ls.CostUnknown = true
	}
}

// PromptEffective returns the authoritative total input token count.
func (ls *LoopState) PromptEffective() int64 {
	return ls.TotalPromptTokens
}

// ElapsedTime returns how long the generation loop has been running.
func (ls *LoopState) ElapsedTime() time.Duration {
	return time.Since(ls.StartedAt)
}
