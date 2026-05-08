package agent

import "testing"

func TestBudgetController_CostLimitHardStop(t *testing.T) {
	bc := NewBudgetController(BudgetLimits{
		TokenBudget:   100000,
		CostBudgetUSD: 1.0,
	})
	state := &LoopState{CostAccumulated: 1.5}
	d := bc.ShouldContinue(state)
	if d.Action != ContinuationHardStop {
		t.Fatalf("expected ContinuationHardStop, got %v", d.Action)
	}
}

func TestBudgetController_UnknownCostHardStopWhenBudgetConfigured(t *testing.T) {
	bc := NewBudgetController(BudgetLimits{
		TokenBudget:   100000,
		CostBudgetUSD: 1.0,
	})
	state := &LoopState{CostUnknown: true}
	d := bc.ShouldContinue(state)
	if d.Action != ContinuationHardStop {
		t.Fatalf("expected ContinuationHardStop, got %v", d.Action)
	}
}

func TestBudgetController_CostBudgetZero_NoHardStop(t *testing.T) {
	bc := NewBudgetController(BudgetLimits{
		TokenBudget:   100000,
		CostBudgetUSD: 0, // 不限
	})
	state := &LoopState{CostAccumulated: 999.0, TotalPromptTokens: 50000}
	d := bc.ShouldContinue(state)
	if d.Action == ContinuationHardStop {
		t.Fatal("cost budget=0 should not trigger hard stop")
	}
}

func TestBudgetController_NoBudget_Deny(t *testing.T) {
	bc := NewBudgetController(BudgetLimits{TokenBudget: 0})
	state := &LoopState{}
	d := bc.ShouldContinue(state)
	if d.Action != ContinuationDeny {
		t.Fatalf("expected ContinuationDeny for no budget, got %v", d.Action)
	}
}

func TestBudgetController_ContinuationLimit_Deny(t *testing.T) {
	bc := NewBudgetController(BudgetLimits{
		TokenBudget:      100000,
		MaxContinuations: 3,
	})
	state := &LoopState{ContinuationCount: 3}
	d := bc.ShouldContinue(state)
	if d.Action != ContinuationDeny {
		t.Fatalf("expected ContinuationDeny at continuation limit, got %v", d.Action)
	}
}

func TestBudgetController_DiminishingReturns_Deny(t *testing.T) {
	bc := NewBudgetController(BudgetLimits{
		TokenBudget:              100000,
		DiminishingMinDeltaToken: 500,
		MaxContinuations:         10,
	})
	state := &LoopState{
		TotalPromptTokens: 50000,
		LastDeltaTokens:   100, // below threshold
	}
	d := bc.ShouldContinue(state)
	if d.Action != ContinuationDeny {
		t.Fatalf("expected ContinuationDeny for diminishing returns, got %v", d.Action)
	}
}

func TestBudgetController_TokenBudgetExhausted_Deny(t *testing.T) {
	bc := NewBudgetController(BudgetLimits{
		TokenBudget:      100000,
		MaxContinuations: 10,
	})
	state := &LoopState{TotalPromptTokens: 100000}
	d := bc.ShouldContinue(state)
	if d.Action != ContinuationDeny {
		t.Fatalf("expected ContinuationDeny when budget exhausted, got %v", d.Action)
	}
}

func TestBudgetController_ApproachingThreshold_AllowWithNudge(t *testing.T) {
	bc := NewBudgetController(BudgetLimits{
		TokenBudget:         100000,
		CompletionThreshold: 0.90,
		MaxContinuations:    10,
	})
	state := &LoopState{TotalPromptTokens: 95000} // ratio=0.95 >= 0.90
	d := bc.ShouldContinue(state)
	if d.Action != ContinuationAllow {
		t.Fatalf("expected ContinuationAllow near threshold, got %v", d.Action)
	}
	if d.NudgeText == "" {
		t.Fatal("expected non-empty NudgeText near threshold")
	}
}

func TestBudgetController_Normal_AllowNoNudge(t *testing.T) {
	bc := NewBudgetController(BudgetLimits{
		TokenBudget:         100000,
		CompletionThreshold: 0.90,
		MaxContinuations:    10,
	})
	state := &LoopState{TotalPromptTokens: 50000} // ratio=0.50, well within budget
	d := bc.ShouldContinue(state)
	if d.Action != ContinuationAllow {
		t.Fatalf("expected ContinuationAllow, got %v", d.Action)
	}
	if d.NudgeText != "" {
		t.Fatalf("expected empty NudgeText, got %q", d.NudgeText)
	}
}
