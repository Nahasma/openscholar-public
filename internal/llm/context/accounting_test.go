package context

import (
	"testing"

	"github.com/openscholar/openscholar/internal/message"
)

func TestUsageMath(t *testing.T) {
	usage := message.Usage{
		InputTokens:              100,
		OutputTokens:             50,
		CacheCreationInputTokens: 30,
		CacheReadInputTokens:     20,
	}
	if got := CurrentUsageTokens(usage); got != 150 {
		t.Fatalf("CurrentUsageTokens() = %d, want 150", got)
	}
	if got := FullResponseTokenCount(usage); got != 200 {
		t.Fatalf("FullResponseTokenCount() = %d, want 200", got)
	}
	if got := CurrentUsagePercentage(usage, 300); got != 50 {
		t.Fatalf("CurrentUsagePercentage() = %f, want 50", got)
	}
}

func TestTokenCountWithEstimation_UsesLastRealUsage(t *testing.T) {
	history := []message.Message{
		{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: "older context"}},
		},
		{
			Role: message.Assistant,
			Usage: message.Usage{
				InputTokens:              1000,
				OutputTokens:             200,
				CacheCreationInputTokens: 300,
				CacheReadInputTokens:     100,
			},
		},
		{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: "new message after usage snapshot"}},
		},
	}

	got := TokenCountWithEstimation(history)
	if !got.HasRealUsage {
		t.Fatal("expected HasRealUsage=true")
	}
	if got.LastRealUsageIdx != 1 {
		t.Fatalf("LastRealUsageIdx = %d, want 1", got.LastRealUsageIdx)
	}
	if got.TokenCount <= 1400 {
		t.Fatalf("TokenCount = %d, want > 1400", got.TokenCount)
	}
	if got.EstimatedDelta <= 0 {
		t.Fatalf("EstimatedDelta = %d, want > 0", got.EstimatedDelta)
	}
}

func TestTokenCountWithEstimation_AllEstimated(t *testing.T) {
	history := []message.Message{
		{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: "hello world"}},
		},
	}

	got := TokenCountWithEstimation(history)
	if got.HasRealUsage {
		t.Fatal("expected HasRealUsage=false")
	}
	if got.LastRealUsageIdx != -1 {
		t.Fatalf("LastRealUsageIdx = %d, want -1", got.LastRealUsageIdx)
	}
	if got.TokenCount <= 0 {
		t.Fatalf("TokenCount = %d, want > 0", got.TokenCount)
	}
}

func TestBuildBudgetSnapshot_UsesHardStopFallbackAndEffectiveTokens(t *testing.T) {
	s := BuildBudgetSnapshot(BudgetSnapshotInput{
		ContextWindow:   1000,
		ReserveTokens:   200,
		PromptTokens:    500,
		LastUsage:       message.Usage{CacheReadInputTokens: 100, CacheCreationInputTokens: 50},
		Estimate:        TokenCountEstimate{TokenCount: 500},
		WarningAt:       600,
		AutoCompactAt:   700,
		HardStopAt:      0,
		MaxOutputTokens: 300,
	})
	if s.HardStopAt != 800 {
		t.Fatalf("HardStopAt = %d, want 800", s.HardStopAt)
	}
	if s.EffectiveTokens != 650 {
		t.Fatalf("EffectiveTokens = %d, want 650", s.EffectiveTokens)
	}
	if s.AvailableTokens != 150 {
		t.Fatalf("AvailableTokens = %d, want 150", s.AvailableTokens)
	}
	if s.PromptTokens != 500 {
		t.Fatalf("PromptTokens = %d, want 500", s.PromptTokens)
	}
}

func TestBuildBudgetSnapshot_AvailableNeverNegative(t *testing.T) {
	s := BuildBudgetSnapshot(BudgetSnapshotInput{
		ContextWindow: 800,
		ReserveTokens: 100,
		PromptTokens:  790,
		HardStopAt:    700,
	})
	if s.AvailableTokens != 0 {
		t.Fatalf("AvailableTokens = %d, want 0", s.AvailableTokens)
	}
}
