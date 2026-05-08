package agent

import (
	"context"
	"testing"
	"time"
)

// --- helpers ---

type alwaysContinue struct{}

func (a *alwaysContinue) Name() string { return "always_continue" }
func (a *alwaysContinue) Check(_ context.Context, _ *LoopState) StopDecision {
	return StopDecision{Action: ActionContinue, Code: StopCodeNone}
}

type alwaysTerminate struct {
	code StopCode
}

func (t *alwaysTerminate) Name() string { return "always_terminate" }
func (t *alwaysTerminate) Check(_ context.Context, _ *LoopState) StopDecision {
	return StopDecision{
		Action: ActionTerminate,
		Reason: "forced",
		Code:   t.code,
	}
}

func defaultState() *LoopState {
	return &LoopState{
		Iteration: 0,
		StartedAt: time.Now(),
	}
}

// --- StopController tests ---

func TestStopController_NoPolicies_Continue(t *testing.T) {
	sc := NewStopController()
	d := sc.Run(context.Background(), defaultState())
	if d.Action != ActionContinue {
		t.Fatalf("expected ActionContinue, got %v", d.Action)
	}
}

func TestStopController_AllContinue(t *testing.T) {
	sc := NewStopController(&alwaysContinue{}, &alwaysContinue{})
	d := sc.Run(context.Background(), defaultState())
	if d.Action != ActionContinue {
		t.Fatalf("expected ActionContinue, got %v", d.Action)
	}
}

func TestStopController_TerminateOnFirst(t *testing.T) {
	sc := NewStopController(
		&alwaysTerminate{code: StopCodeHook},
		&alwaysContinue{},
	)
	d := sc.Run(context.Background(), defaultState())
	if d.Action != ActionTerminate {
		t.Fatalf("expected ActionTerminate, got %v", d.Action)
	}
	if d.Code != StopCodeHook {
		t.Fatalf("expected StopCodeHook, got %v", d.Code)
	}
}

func TestStopController_TerminateOnSecond(t *testing.T) {
	sc := NewStopController(
		&alwaysContinue{},
		&alwaysTerminate{code: StopCodeMaxIterations},
	)
	d := sc.Run(context.Background(), defaultState())
	if d.Action != ActionTerminate {
		t.Fatalf("expected ActionTerminate, got %v", d.Action)
	}
	if d.Code != StopCodeMaxIterations {
		t.Fatalf("expected StopCodeMaxIterations, got %v", d.Code)
	}
}

func TestStopController_AddPolicy(t *testing.T) {
	sc := NewStopController()
	sc.AddPolicy(&alwaysTerminate{code: StopCodeCostLimit})
	d := sc.Run(context.Background(), defaultState())
	if d.Action != ActionTerminate {
		t.Fatalf("expected ActionTerminate after AddPolicy, got %v", d.Action)
	}
	if d.Code != StopCodeCostLimit {
		t.Fatalf("expected StopCodeCostLimit, got %v", d.Code)
	}
}

// --- MaxIterationHook tests ---

func TestMaxIterationHook_Name(t *testing.T) {
	h := NewMaxIterationHook(10)
	if h.Name() != "max_iteration" {
		t.Fatalf("unexpected name: %s", h.Name())
	}
}

func TestMaxIterationHook_BelowLimit_Continue(t *testing.T) {
	h := NewMaxIterationHook(5)
	state := &LoopState{Iteration: 3}
	d := h.Check(context.Background(), state)
	if d.Action != ActionContinue {
		t.Fatalf("expected ActionContinue at iteration 3/5, got %v", d.Action)
	}
}

func TestMaxIterationHook_AtLimit_Terminate(t *testing.T) {
	h := NewMaxIterationHook(5)
	state := &LoopState{Iteration: 5}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate {
		t.Fatalf("expected ActionTerminate at iteration 5/5, got %v", d.Action)
	}
	if d.Code != StopCodeMaxIterations {
		t.Fatalf("expected StopCodeMaxIterations, got %v", d.Code)
	}
}

func TestMaxIterationHook_AboveLimit_Terminate(t *testing.T) {
	h := NewMaxIterationHook(5)
	state := &LoopState{Iteration: 99}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate {
		t.Fatalf("expected ActionTerminate at iteration 99/5, got %v", d.Action)
	}
}

func TestMaxIterationHook_ZeroLimit_AlwaysTerminate(t *testing.T) {
	h := NewMaxIterationHook(0)
	state := &LoopState{Iteration: 0}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate {
		t.Fatalf("expected ActionTerminate for zero limit, got %v", d.Action)
	}
}

func TestMaxIterationHook_Diagnostics(t *testing.T) {
	h := NewMaxIterationHook(10)
	state := &LoopState{Iteration: 7}
	d := h.Check(context.Background(), state)

	iter, ok := d.Diagnostics["iteration"]
	if !ok {
		t.Fatal("diagnostics missing 'iteration' key")
	}
	if iter != 7 {
		t.Fatalf("expected iteration=7, got %v", iter)
	}

	max, ok := d.Diagnostics["max"]
	if !ok {
		t.Fatal("diagnostics missing 'max' key")
	}
	if max != 10 {
		t.Fatalf("expected max=10, got %v", max)
	}
}

func TestMaxIterationHook_IntegratedWithController(t *testing.T) {
	sc := NewStopController(NewMaxIterationHook(3))

	// iterations 1 and 2 should continue
	for i := 1; i < 3; i++ {
		state := &LoopState{Iteration: i}
		d := sc.Run(context.Background(), state)
		if d.Action != ActionContinue {
			t.Fatalf("iteration %d: expected ActionContinue, got %v", i, d.Action)
		}
	}

	// iteration 3 should terminate
	state := &LoopState{Iteration: 3}
	d := sc.Run(context.Background(), state)
	if d.Action != ActionTerminate {
		t.Fatalf("iteration 3: expected ActionTerminate, got %v", d.Action)
	}
	if d.Code != StopCodeMaxIterations {
		t.Fatalf("expected StopCodeMaxIterations, got %v", d.Code)
	}
}

// --- RepeatedToolPatternHook tests ---

func TestRepeatedToolPatternHook_Name(t *testing.T) {
	h := NewRepeatedToolPatternHook()
	if h.Name() != "repeated_tool_pattern" {
		t.Fatalf("unexpected name: %s", h.Name())
	}
}

func TestRepeatedToolPatternHook_NoSignatures_Continue(t *testing.T) {
	h := NewRepeatedToolPatternHook()
	state := &LoopState{}
	d := h.Check(context.Background(), state)
	if d.Action != ActionContinue {
		t.Fatalf("expected ActionContinue for empty signatures, got %v", d.Action)
	}
}

func TestRepeatedToolPatternHook_Repeated_Terminate(t *testing.T) {
	h := NewRepeatedToolPatternHook()
	sig := "Read:abc123"

	// Feed the same signature across multiple rounds until it triggers.
	// window=5, threshold=3: needs at least 5 entries with 3+ repeats.
	for i := 0; i < 6; i++ {
		state := &LoopState{LastRoundToolSignatures: []string{sig}}
		d := h.Check(context.Background(), state)
		if d.Action == ActionTerminate {
			if d.Code != StopCodeRepeatTool {
				t.Fatalf("expected StopCodeRepeatTool, got %v", d.Code)
			}
			return // success: detected repetition
		}
	}
	t.Fatal("expected ActionTerminate after repeated signatures, but never triggered")
}

func TestRepeatedToolPatternHook_Varied_Continue(t *testing.T) {
	h := NewRepeatedToolPatternHook()
	sigs := []string{"Read:a", "Write:b", "Edit:c", "Grep:d", "Glob:e"}
	for _, sig := range sigs {
		state := &LoopState{LastRoundToolSignatures: []string{sig}}
		d := h.Check(context.Background(), state)
		if d.Action != ActionContinue {
			t.Fatalf("expected ActionContinue for varied signatures, got %v at sig %s", d.Action, sig)
		}
	}
}

// --- NoProgressHook tests ---

func TestNoProgressHook_Name(t *testing.T) {
	h := NewNoProgressHook(4)
	if h.Name() != "no_progress" {
		t.Fatalf("unexpected name: %s", h.Name())
	}
}

func TestNoProgressHook_Progress_Continue(t *testing.T) {
	h := NewNoProgressHook(4)
	state := &LoopState{LastRoundProgress: RoundProgress{ToolCalls: 1, DurableProgress: true}}
	d := h.Check(context.Background(), state)
	if d.Action != ActionContinue {
		t.Fatalf("expected ActionContinue with progress, got %v", d.Action)
	}
}

func TestNoProgressHook_NoProgress_Terminate(t *testing.T) {
	h := NewNoProgressHook(4)
	for i := 0; i < 4; i++ {
		state := &LoopState{LastRoundProgress: RoundProgress{ToolCalls: 1, FailedTools: 1}}
		d := h.Check(context.Background(), state)
		if i < 3 {
			if d.Action != ActionContinue {
				t.Fatalf("round %d: expected ActionContinue, got %v", i, d.Action)
			}
		} else {
			if d.Action != ActionTerminate {
				t.Fatalf("round %d: expected ActionTerminate, got %v", i, d.Action)
			}
			if d.Code != StopCodeNoProgress {
				t.Fatalf("expected StopCodeNoProgress, got %v", d.Code)
			}
		}
	}
}

func TestNoProgressHook_ResetOnProgress(t *testing.T) {
	h := NewNoProgressHook(4)
	// 3 rounds no progress
	for range 3 {
		state := &LoopState{LastRoundProgress: RoundProgress{ToolCalls: 1, LowValueInspection: true}}
		h.Check(context.Background(), state)
	}
	// 1 round with progress resets counter
	state := &LoopState{LastRoundProgress: RoundProgress{ToolCalls: 1, DurableProgress: true}}
	h.Check(context.Background(), state)
	// then 3 more no progress should not trigger
	for range 3 {
		state := &LoopState{LastRoundProgress: RoundProgress{ToolCalls: 1, LowValueInspection: true}}
		d := h.Check(context.Background(), state)
		if d.Action == ActionTerminate {
			t.Fatal("should not terminate after reset")
		}
	}
}

func TestNoProgressHook_SearchPageDoesNotResetCounter(t *testing.T) {
	h := NewNoProgressHook(2)
	state := &LoopState{LastRoundProgress: classifyRoundProgress(&LoopState{SeenEvidence: map[string]int{}, TargetCounts: map[string]int{}, OutcomeCounts: map[string]int{}, FailureCounts: map[string]int{}}, []ToolOutcome{
		{ToolName: "WebSearch", ProgressKind: "search_page", ResultBytes: 512},
	})}
	if d := h.Check(context.Background(), state); d.Action != ActionContinue {
		t.Fatalf("first search_page round should continue, got %v", d.Action)
	}
	if d := h.Check(context.Background(), state); d.Action != ActionTerminate || d.Code != StopCodeNoProgress {
		t.Fatalf("second search_page round should terminate as no progress, got %#v", d)
	}
}

func TestStopCodeToTerminalReason_DoesNotCollapseToMaxTurns(t *testing.T) {
	cases := []struct {
		code StopCode
		want TerminalReason
	}{
		{StopCodeMaxIterations, ReasonMaxTurnsReached},
		{StopCodeNoProgress, ReasonNoProgress},
		{StopCodeRepeatTool, ReasonRepeatedToolPattern},
		{StopCodeTokenBudget, ReasonTokenBudgetExceeded},
		{StopCodeCostLimit, ReasonCostBudgetExceeded},
		{StopCodeCompactFuse, ReasonCompactExhausted},
		{StopCodeProviderCooldown, ReasonProviderCooldown},
		{StopCodeHook, ReasonLoopHookStopped},
	}
	for _, tc := range cases {
		if got := stopCodeToTerminalReason(tc.code); got != tc.want {
			t.Fatalf("code %v: got %s, want %s", tc.code, got, tc.want)
		}
	}
}

// --- BudgetExceededHook tests ---

func TestBudgetExceededHook_Name(t *testing.T) {
	h := NewBudgetExceededHook(BudgetLimits{})
	if h.Name() != "budget_exceeded" {
		t.Fatalf("unexpected name: %s", h.Name())
	}
}

func TestBudgetExceededHook_CostExceeded(t *testing.T) {
	h := NewBudgetExceededHook(BudgetLimits{CostBudgetUSD: 1.0})
	state := &LoopState{CostAccumulated: 1.5}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate {
		t.Fatalf("expected ActionTerminate for cost exceeded, got %v", d.Action)
	}
	if d.Code != StopCodeCostLimit {
		t.Fatalf("expected StopCodeCostLimit, got %v", d.Code)
	}
}

func TestBudgetExceededHook_UnknownCostWithBudgetTerminates(t *testing.T) {
	h := NewBudgetExceededHook(BudgetLimits{CostBudgetUSD: 1.0})
	state := &LoopState{CostUnknown: true}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate {
		t.Fatalf("expected ActionTerminate for unknown cost, got %v", d.Action)
	}
	if d.Code != StopCodeCostLimit {
		t.Fatalf("expected StopCodeCostLimit, got %v", d.Code)
	}
}

func TestBudgetExceededHook_TokenExceeded(t *testing.T) {
	h := NewBudgetExceededHook(BudgetLimits{TokenBudget: 100000})
	state := &LoopState{TotalPromptTokens: 100000}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate {
		t.Fatalf("expected ActionTerminate for token exceeded, got %v", d.Action)
	}
	if d.Code != StopCodeTokenBudget {
		t.Fatalf("expected StopCodeTokenBudget, got %v", d.Code)
	}
}

func TestBudgetExceededHook_WithinBudget_Continue(t *testing.T) {
	h := NewBudgetExceededHook(BudgetLimits{TokenBudget: 100000, CostBudgetUSD: 5.0})
	state := &LoopState{TotalPromptTokens: 50000, CostAccumulated: 1.0}
	d := h.Check(context.Background(), state)
	if d.Action != ActionContinue {
		t.Fatalf("expected ActionContinue within budget, got %v", d.Action)
	}
}

func TestBudgetExceededHook_NoBudget_Continue(t *testing.T) {
	h := NewBudgetExceededHook(BudgetLimits{})
	state := &LoopState{TotalPromptTokens: 999999, CostAccumulated: 999.0}
	d := h.Check(context.Background(), state)
	if d.Action != ActionContinue {
		t.Fatalf("expected ActionContinue with no budget limits, got %v", d.Action)
	}
}

func TestToolCallLimitHook_Terminates(t *testing.T) {
	h := NewToolCallLimitHook(2)
	state := &LoopState{TotalToolCalls: 2}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate || d.Code != StopCodeMaxToolCalls {
		t.Fatalf("expected max tool call stop, got %#v", d)
	}
}

func TestSearchConvergenceBudgetHook_SoftBudgetContinuesWithNewEvidence(t *testing.T) {
	h := NewSearchConvergenceBudgetHook()
	state := &LoopState{
		SearchToolCalls: h.MaxSearchToolCalls,
		LastRoundProgress: RoundProgress{
			ToolCalls:        1,
			DurableProgress:  true,
			NewEvidenceCount: 3,
		},
		LastRoundToolOutcomes: []ToolOutcome{{
			ToolName:     "ScholarSearch",
			ProgressKind: "search_page",
			EvidenceKeys: []string{"doi:10.1/a", "doi:10.1/b", "doi:10.1/c"},
		}},
	}
	d := h.Check(context.Background(), state)
	if d.Action == ActionTerminate {
		t.Fatalf("soft search budget should continue while new evidence arrives, got %#v", d)
	}
}

func TestSearchConvergenceBudgetHook_SoftBudgetStopsWithoutNewEvidence(t *testing.T) {
	h := NewSearchConvergenceBudgetHook()
	state := &LoopState{
		SearchToolCalls: h.MaxSearchToolCalls,
		LastRoundProgress: RoundProgress{
			ToolCalls:          1,
			LowValueInspection: true,
		},
		LastRoundToolOutcomes: []ToolOutcome{{
			ToolName:     "ScholarSearch",
			ProgressKind: "search_page",
		}},
	}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate || d.Reason != "search soft budget reached without new evidence" {
		t.Fatalf("expected soft budget stop without new evidence, got %#v", d)
	}
}

func TestSearchConvergenceBudgetHook_HardBudgetStopsEvenWithEvidence(t *testing.T) {
	h := NewSearchConvergenceBudgetHook()
	state := &LoopState{
		SearchToolCalls: h.HardMaxSearchToolCalls,
		LastRoundProgress: RoundProgress{
			ToolCalls:        1,
			DurableProgress:  true,
			NewEvidenceCount: 1,
		},
		LastRoundToolOutcomes: []ToolOutcome{{
			ToolName:     "ScholarSearch",
			ProgressKind: "search_page",
			EvidenceKeys: []string{"doi:10.1/a"},
		}},
	}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate || d.Reason != "search hard budget exhausted" {
		t.Fatalf("expected hard budget stop, got %#v", d)
	}
}

func TestSearchConvergenceBudgetHook_RepeatedTargetStops(t *testing.T) {
	h := NewSearchConvergenceBudgetHook()
	state := &LoopState{
		TargetCounts: map[string]int{"webfetch:https://papers.nips.cc/paper_files/paper/2024": 3},
		LastRoundToolOutcomes: []ToolOutcome{{
			ToolName:       "WebFetch",
			TargetKey:      "webfetch:https://papers.nips.cc/paper_files/paper/2024",
			ContentClass:   "list_page",
			ProgressKind:   "fetched_page",
			LowValueReason: "repeated_target",
		}},
	}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate {
		t.Fatalf("expected terminate for repeated target, got %#v", d)
	}
}

func TestSearchConvergenceBudgetHook_RepeatedDurableTargetContinues(t *testing.T) {
	h := NewSearchConvergenceBudgetHook()
	state := &LoopState{
		TargetCounts: map[string]int{"webfetch:https://example.com/paper": 3},
		LastRoundToolOutcomes: []ToolOutcome{{
			ToolName:        "WebFetch",
			TargetKey:       "webfetch:https://example.com/paper",
			ContentClass:    "paper_page",
			ProgressKind:    "fetched_page",
			DurableProgress: true,
			EvidenceKeys:    []string{"url:https://example.com/paper"},
			OutcomeHash:     "sha256:new",
			CanonicalURL:    "https://example.com/paper",
			ResultBytes:     512,
			PromptClass:     "extract",
			Provider:        "http",
			Source:          "https://example.com/paper",
			Signature:       "WebFetch:webfetch:https://example.com/paper",
		}},
	}
	d := h.Check(context.Background(), state)
	if d.Action == ActionTerminate {
		t.Fatalf("durable repeated paper page should not trigger same-target stop, got %#v", d)
	}
}

func TestSearchConvergenceBudgetHook_RepeatedTargetOutcomeStops(t *testing.T) {
	h := NewSearchConvergenceBudgetHook()
	state := &LoopState{
		OutcomeCounts: map[string]int{"webfetch:https://example.com/list|sha256:abc": 3},
		LastRoundToolOutcomes: []ToolOutcome{{
			ToolName:    "WebFetch",
			TargetKey:   "webfetch:https://example.com/list",
			OutcomeHash: "sha256:abc",
		}},
	}
	d := h.Check(context.Background(), state)
	if d.Action != ActionTerminate {
		t.Fatalf("expected terminate for repeated target+outcome, got %#v", d)
	}
}

func TestSearchConvergenceBudgetHook_NoProgressIgnoresNonSearchRounds(t *testing.T) {
	h := NewSearchConvergenceBudgetHook()
	state := &LoopState{
		LastRoundProgress: RoundProgress{ToolCalls: 1, LowValueInspection: true},
		LastRoundToolOutcomes: []ToolOutcome{{
			ToolName: "Bash",
			QueryKey: "--help",
		}},
	}
	for range h.MaxSearchNoProgressTurn {
		if d := h.Check(context.Background(), state); d.Action == ActionTerminate {
			t.Fatalf("non-search low-value rounds should not trigger search convergence, got %#v", d)
		}
	}
}
