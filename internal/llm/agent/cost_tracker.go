package agent

import (
	"sync"
	"time"
)

// UsageEntry records a single API call's token usage and cost.
type UsageEntry struct {
	Timestamp    time.Time
	Model        string
	Source       string // "main" / "compact" / "summarizer" / "sub-agent"
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CacheWrite   int64
	CostUSD      float64
}

// PromptEffective returns the authoritative total input token count:
// input + cacheRead + cacheWrite.
func (e UsageEntry) PromptEffective() int64 {
	return e.InputTokens + e.CacheRead + e.CacheWrite
}

// UsageTotals holds aggregated token and cost statistics.
type UsageTotals struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	PromptEffective  int64 // InputTokens + CacheReadTokens + CacheWriteTokens
	TotalTokens      int64 // PromptEffective + OutputTokens
	CostUSD          float64
}

// ModelUsageV2 holds per-model aggregated usage (V2 suffix avoids conflict with ModelUsage).
type ModelUsageV2 struct {
	ModelID string
	UsageTotals
}

// SessionCostSnapshotV2 is a point-in-time session cost snapshot (V2 avoids conflict).
type SessionCostSnapshotV2 struct {
	SessionID string
	UpdatedAt time.Time
	Totals    UsageTotals
	ByModel   map[string]*ModelUsageV2
}

// CostTrackerV2 is a three-layer cost tracker:
//
//	Layer 1 – UsageLedger  : raw append-only []UsageEntry
//	Layer 2 – Aggregates   : live totals and per-model rollups
//	Layer 3 – Snapshot API : deep-copy exports for consumers
type CostTrackerV2 struct {
	mu      sync.RWMutex
	entries []UsageEntry
	totals  UsageTotals
	byModel map[string]*ModelUsageV2
}

// NewCostTrackerV2 creates an empty CostTrackerV2.
func NewCostTrackerV2() *CostTrackerV2 {
	return &CostTrackerV2{
		byModel: make(map[string]*ModelUsageV2),
	}
}

// Record appends a usage event and updates all aggregates atomically.
func (ct *CostTrackerV2) Record(model, source string, inputTokens, outputTokens, cacheRead, cacheWrite int64, costUSD float64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	entry := UsageEntry{
		Timestamp:    time.Now(),
		Model:        model,
		Source:       source,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CacheRead:    cacheRead,
		CacheWrite:   cacheWrite,
		CostUSD:      costUSD,
	}
	ct.entries = append(ct.entries, entry)

	// Update session-level totals.
	effective := inputTokens + cacheRead + cacheWrite
	ct.totals.InputTokens += inputTokens
	ct.totals.OutputTokens += outputTokens
	ct.totals.CacheReadTokens += cacheRead
	ct.totals.CacheWriteTokens += cacheWrite
	ct.totals.PromptEffective += effective
	ct.totals.TotalTokens += effective + outputTokens
	ct.totals.CostUSD += costUSD

	// Update per-model totals.
	mv, ok := ct.byModel[model]
	if !ok {
		mv = &ModelUsageV2{ModelID: model}
		ct.byModel[model] = mv
	}
	mv.InputTokens += inputTokens
	mv.OutputTokens += outputTokens
	mv.CacheReadTokens += cacheRead
	mv.CacheWriteTokens += cacheWrite
	mv.PromptEffective += effective
	mv.TotalTokens += effective + outputTokens
	mv.CostUSD += costUSD
}

// TotalCost returns the total session cost in USD.
func (ct *CostTrackerV2) TotalCost() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totals.CostUSD
}

// TotalPromptTokens returns the authoritative total input token count
// (InputTokens + CacheReadTokens + CacheWriteTokens).
func (ct *CostTrackerV2) TotalPromptTokens() int64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totals.PromptEffective
}

// Snapshot returns a deep copy of the current session cost as SessionCostSnapshotV2.
func (ct *CostTrackerV2) Snapshot(sessionID string) SessionCostSnapshotV2 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	snap := SessionCostSnapshotV2{
		SessionID: sessionID,
		UpdatedAt: time.Now(),
		Totals:    ct.totals,
		ByModel:   make(map[string]*ModelUsageV2, len(ct.byModel)),
	}
	for k, v := range ct.byModel {
		cp := *v
		snap.ByModel[k] = &cp
	}
	return snap
}

// LegacySnapshot converts the V2 aggregates into the existing SessionCostState
// type so callers that depend on the old shape can work without modification.
func (ct *CostTrackerV2) LegacySnapshot() SessionCostState {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	snap := SessionCostState{
		ByModel:   make(map[string]*ModelUsage, len(ct.byModel)),
		Total:     ct.totals.CostUSD,
		CostKnown: true,
	}
	for k, v := range ct.byModel {
		snap.ByModel[k] = &ModelUsage{
			ModelID:          v.ModelID,
			InputTokens:      v.InputTokens,
			OutputTokens:     v.OutputTokens,
			CacheReadTokens:  v.CacheReadTokens,
			CacheWriteTokens: v.CacheWriteTokens,
			Cost:             v.CostUSD,
			CostKnown:        true,
		}
	}
	return snap
}

// EntryCount returns the number of recorded usage events.
func (ct *CostTrackerV2) EntryCount() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return len(ct.entries)
}

// Entries returns a copy of all recorded usage events.
func (ct *CostTrackerV2) Entries() []UsageEntry {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	result := make([]UsageEntry, len(ct.entries))
	copy(result, ct.entries)
	return result
}
