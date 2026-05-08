package context

import "github.com/openscholar/openscholar/internal/message"

// BudgetSnapshot captures request-time context budget state before a provider call.
type BudgetSnapshot struct {
	ContextWindow    int64
	MaxOutputTokens  int64
	ReserveTokens    int64
	PromptTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	EffectiveTokens  int64
	AvailableTokens  int64
	WarningAt        int64
	AutoCompactAt    int64
	HardStopAt       int64
	Estimate         TokenCountEstimate
}

type BudgetSnapshotInput struct {
	ContextWindow   int64
	MaxOutputTokens int64
	ReserveTokens   int64
	PromptTokens    int64
	Estimate        TokenCountEstimate
	LastUsage       message.Usage
	WarningAt       int64
	AutoCompactAt   int64
	HardStopAt      int64
}

func BuildBudgetSnapshot(in BudgetSnapshotInput) BudgetSnapshot {
	cacheRead := nonNegative(in.LastUsage.CacheReadInputTokens)
	cacheWrite := nonNegative(in.LastUsage.CacheCreationInputTokens)
	effective := nonNegative(in.PromptTokens + cacheRead + cacheWrite)
	hardStop := in.HardStopAt
	if hardStop <= 0 {
		hardStop = max64(nonNegative(in.ContextWindow)-nonNegative(in.ReserveTokens), 0)
	}
	available := hardStop - effective
	if available < 0 {
		available = 0
	}
	return BudgetSnapshot{
		ContextWindow:    nonNegative(in.ContextWindow),
		MaxOutputTokens:  nonNegative(in.MaxOutputTokens),
		ReserveTokens:    nonNegative(in.ReserveTokens),
		PromptTokens:     nonNegative(in.PromptTokens),
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
		EffectiveTokens:  effective,
		AvailableTokens:  available,
		WarningAt:        nonNegative(in.WarningAt),
		AutoCompactAt:    nonNegative(in.AutoCompactAt),
		HardStopAt:       nonNegative(hardStop),
		Estimate:         in.Estimate,
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
