package agent

import (
	"sync"
)

// ModelUsage tracks token usage and cost for a single model.
type ModelUsage struct {
	ModelID          string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	Cost             float64
	CostKnown        bool
}

// SessionCostState tracks per-model cost across a session.
type SessionCostState struct {
	ByModel   map[string]*ModelUsage
	Total     float64
	CostKnown bool
}

// CostStateReader provides read-only access to cost state.
type CostStateReader interface {
	CostState() SessionCostState
}

// costState is the internal mutable cost tracker.
type costState struct {
	mu      sync.RWMutex
	byModel map[string]*ModelUsage
	total   float64
}

func newCostState() *costState {
	return &costState{
		byModel: make(map[string]*ModelUsage),
		total:   0,
	}
}

// Record adds usage for a model.
func (cs *costState) Record(modelID string, inputTokens, outputTokens, cacheRead, cacheWrite int64, cost float64) {
	cs.RecordWithCostKnown(modelID, inputTokens, outputTokens, cacheRead, cacheWrite, cost, true)
}

func (cs *costState) RecordWithCostKnown(modelID string, inputTokens, outputTokens, cacheRead, cacheWrite int64, cost float64, costKnown bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	mu, ok := cs.byModel[modelID]
	if !ok {
		mu = &ModelUsage{ModelID: modelID, CostKnown: true}
		cs.byModel[modelID] = mu
	}
	mu.InputTokens += inputTokens
	mu.OutputTokens += outputTokens
	mu.CacheReadTokens += cacheRead
	mu.CacheWriteTokens += cacheWrite
	mu.Cost += cost
	mu.CostKnown = mu.CostKnown && costKnown
	cs.total += cost
}

// Snapshot returns a deep copy of the current state.
func (cs *costState) Snapshot() SessionCostState {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	snap := SessionCostState{
		ByModel:   make(map[string]*ModelUsage, len(cs.byModel)),
		Total:     cs.total,
		CostKnown: true,
	}
	for k, v := range cs.byModel {
		cp := *v
		snap.ByModel[k] = &cp
		snap.CostKnown = snap.CostKnown && cp.CostKnown
	}
	return snap
}
