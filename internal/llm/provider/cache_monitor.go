package provider

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
)

// CacheBreakReason classifies why a cache break occurred.
type CacheBreakReason string

const (
	BreakNone            CacheBreakReason = ""
	BreakExpectedCompact CacheBreakReason = "expected_compact"
	BreakExpectedModel   CacheBreakReason = "expected_model_change"
	BreakExpectedToolset CacheBreakReason = "expected_toolset_change"
	BreakBaselineReset   CacheBreakReason = "baseline_reset"
	BreakUnexpected      CacheBreakReason = "unexpected"
)

// CacheMonitor tracks prompt cache hit patterns and detects cache breaks.
type CacheMonitor struct {
	lastSystemHash string
	lastToolsHash  string
	lastModel      string
	lastCacheRead  int
	callCount      int
	provider       string
	model          string
	cacheReadKnown bool

	// Governance event tracking
	pendingEvent  CacheBreakReason // set by Notify* methods, consumed by PostCall
	baselineValid bool             // false = next PostCall establishes new baseline
}

// NewCacheMonitor creates a new CacheMonitor.
func NewCacheMonitor() *CacheMonitor {
	return &CacheMonitor{cacheReadKnown: true}
}

// NewCacheMonitorWithProvider creates a cache monitor with provider capability metadata.
func NewCacheMonitorWithProvider(providerName, modelName string, cacheReadKnown bool) *CacheMonitor {
	return &CacheMonitor{
		provider:       providerName,
		model:          modelName,
		cacheReadKnown: cacheReadKnown,
	}
}

// NotifyCompaction marks that a compaction just happened.
// The next PostCall will treat any cache drop as expected.
func (cm *CacheMonitor) NotifyCompaction() {
	cm.pendingEvent = BreakExpectedCompact
}

// NotifyModelChange marks that the model was changed.
func (cm *CacheMonitor) NotifyModelChange() {
	cm.pendingEvent = BreakExpectedModel
}

// NotifyToolsetChange marks that the active toolset changed.
func (cm *CacheMonitor) NotifyToolsetChange() {
	cm.pendingEvent = BreakExpectedToolset
}

// ResetBaseline forces the next PostCall to establish a new baseline
// instead of comparing against the previous value.
func (cm *CacheMonitor) ResetBaseline() {
	cm.baselineValid = false
	cm.lastCacheRead = 0
}

// PreCall records the current prompt state before an API call.
func (cm *CacheMonitor) PreCall(systemBlocks []string, toolNames []string, model string) {
	cm.lastSystemHash = hashStrings(systemBlocks)
	cm.lastToolsHash = hashStrings(toolNames)
	cm.lastModel = model
	if cm.model == "" {
		cm.model = model
	}
}

// PostCall analyzes cache performance after an API call completes.
func (cm *CacheMonitor) PostCall(cacheReadTokens int) {
	if !cm.cacheReadKnown {
		slog.Debug("[CACHE] cache read tokens unavailable",
			"provider", cm.provider,
			"model", cm.model,
			"cache_read_known", false)
		cm.pendingEvent = BreakNone
		cm.baselineValid = false
		return
	}

	cm.callCount++

	// First call or after baseline reset: establish baseline
	if cm.callCount <= 1 || !cm.baselineValid {
		cm.lastCacheRead = cacheReadTokens
		cm.baselineValid = true
		cm.pendingEvent = BreakNone
		return
	}

	// Governance event → expected break, debug-level only
	if cm.pendingEvent != BreakNone {
		slog.Debug("[CACHE] expected break after governance event",
			"event", cm.pendingEvent,
			"prev", cm.lastCacheRead,
			"curr", cacheReadTokens)
		cm.lastCacheRead = cacheReadTokens
		cm.pendingEvent = BreakNone
		return
	}

	// No governance event + stable hashes → detect real anomaly
	if cm.lastCacheRead > 0 && cacheReadTokens >= 0 {
		drop := cm.lastCacheRead - cacheReadTokens
		dropPct := float64(drop) / float64(cm.lastCacheRead) * 100
		if dropPct > 5 && drop > 2000 {
			slog.Warn("[CACHE BREAK] unexpected drop",
				"prev", cm.lastCacheRead,
				"curr", cacheReadTokens,
				"drop_pct", fmt.Sprintf("%.1f%%", dropPct),
				"system_hash", shortHash(cm.lastSystemHash),
				"tools_hash", shortHash(cm.lastToolsHash),
				"provider", cm.provider,
				"model", cm.model,
				"cache_read_known", true,
			)
		}
	}

	cm.lastCacheRead = cacheReadTokens
}

// hashStrings computes a SHA-256 digest of the concatenated strings (NUL-separated).
func hashStrings(ss []string) string {
	h := sha256.New()
	for _, s := range ss {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// shortHash returns the first 8 characters of a hex hash string.
func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}
