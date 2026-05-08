package permission

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DenialRecord captures a denied permission request for downgrade detection.
type DenialRecord struct {
	Tool      string
	Action    string         // command string for Bash, or description
	Target    string         // file path or command target
	SessionID string
	DeniedAt  time.Time
	Source    DecisionSource // 拒绝来源（mode/rule/user/tracker）
	Reason    DenialReason   // 拒绝原因
}

// DenialTracker tracks recent permission denials to prevent downgrade bypass.
// When a user denies "Write /a/b", the model should not be able to bypass it
// by using "Edit /a/b" or "Bash echo > /a/b".
type DenialTracker struct {
	mu                          sync.Mutex
	records                     []DenialRecord
	consecutiveDenialsBySession map[string]int // per-session consecutive denial count
	fallbackSessions            map[string]bool // sessions that have been downgraded to prompting
	maxAge                      time.Duration
	maxSize                     int
}

// NewDenialTracker creates a new tracker with default settings.
func NewDenialTracker() *DenialTracker {
	return &DenialTracker{
		consecutiveDenialsBySession: make(map[string]int),
		fallbackSessions:            make(map[string]bool),
		maxAge:                      10 * time.Minute,
		maxSize:                     50,
	}
}

// RecordDenial adds a denial record. Only non-mode denials increment the
// consecutive counter and contribute to the fallback threshold. Mode denials
// are still stored for audit purposes but do not affect fallback detection.
func (t *DenialTracker) RecordDenial(toolName, description, target string, sessionID string, source DecisionSource, reason DenialReason) {
	t.mu.Lock()
	defer t.mu.Unlock()

	sid := sessionID

	// Only increment consecutive counter for non-mode denials
	if source != SourceMode {
		t.consecutiveDenialsBySession[sid]++
	}

	t.records = append(t.records, DenialRecord{
		Tool:      toolName,
		Action:    description,
		Target:    normalizeTarget(target),
		SessionID: sid,
		DeniedAt:  time.Now(),
		Source:    source,
		Reason:    reason,
	})

	// Check if session should be permanently downgraded (only for non-mode denials)
	if source != SourceMode {
		t.cleanup()
		sessionTotal := 0
		for _, rec := range t.records {
			if rec.Source == SourceMode {
				continue // mode deny 不计入
			}
			if sid == "" || rec.SessionID == "" || rec.SessionID == sid {
				sessionTotal++
			}
		}
		if t.consecutiveDenialsBySession[sid] >= DefaultDenialLimits.MaxConsecutive ||
			sessionTotal >= DefaultDenialLimits.MaxTotal {
			t.fallbackSessions[sid] = true
		}
	}
}

// RecordDenialFromResult is a convenience method that records a denial from
// a DecisionResult, extracting Source and Reason automatically.
func (t *DenialTracker) RecordDenialFromResult(toolName, description, target, sessionID string, result DecisionResult) {
	t.RecordDenial(toolName, description, target, sessionID, result.Source, result.Reason)
}

// RecordGrant resets the consecutive denial counter for the active session.
// Does NOT clear fallbackSessions — once a session is downgraded, it stays downgraded.
func (t *DenialTracker) RecordGrant(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.consecutiveDenialsBySession[sessionID] = 0
}

// IsDowngrade checks if a new permission request represents a downgrade attempt
// that should be automatically denied. Returns (isDowngrade, reason).
//
// For Bash commands, the target parameter should be the command string itself
// (not a file path), since Bash requests don't carry a Path field.
func (t *DenialTracker) IsDowngrade(toolName, action, target string, sessionID ...string) (bool, string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cleanup()

	sid := ""
	if len(sessionID) > 0 {
		sid = sessionID[0]
	}

	normalizedTarget := normalizeTarget(target)
	now := time.Now()

	for _, rec := range t.records {
		if now.Sub(rec.DeniedAt) > t.maxAge {
			continue
		}

		// Only check records from the same session (or all if no session specified)
		if sid != "" && rec.SessionID != "" && rec.SessionID != sid {
			continue
		}

		// Check if this is a downgrade of a previously denied write operation
		if matchesDowngrade(rec, toolName, action, normalizedTarget) {
			return true, formatDowngradeReason(rec, toolName)
		}
	}

	return false, ""
}

// ConsecutiveDenials returns the total count of consecutive permission denials
// across all sessions since the last grant. Resets to 0 when RecordGrant is called.
// Mode denials are not counted.
func (t *DenialTracker) ConsecutiveDenials() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	total := 0
	for _, c := range t.consecutiveDenialsBySession {
		total += c
	}
	return total
}

// Reset clears all denial records and resets all counters (e.g., on mode switch).
func (t *DenialTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = t.records[:0]
	t.consecutiveDenialsBySession = make(map[string]int)
	t.fallbackSessions = make(map[string]bool)
}

// cleanup removes expired records and enforces max size.
func (t *DenialTracker) cleanup() {
	now := time.Now()
	kept := t.records[:0]
	for _, rec := range t.records {
		if now.Sub(rec.DeniedAt) <= t.maxAge {
			kept = append(kept, rec)
		}
	}
	// Enforce max size (keep most recent)
	if len(kept) > t.maxSize {
		kept = kept[len(kept)-t.maxSize:]
	}
	t.records = kept
}

// matchesDowngrade checks if a new request is a downgrade of a denied request.
func matchesDowngrade(denied DenialRecord, newTool, newAction, newTarget string) bool {
	// Only check write→write downgrades
	if !isWriteFamily(denied.Tool) {
		return false
	}

	// Case 1: Write/Edit → different Write/Edit tool on same target
	if isWriteFamily(newTool) && denied.Tool != newTool && targetsOverlap(denied.Target, newTarget) {
		return true
	}

	// Case 2: Write/Edit → Bash redirect targeting same file
	// For Bash, newTarget is the command string (Bash requests don't carry Path)
	if newTool == "Bash" {
		// Check if the bash command writes to the denied file
		if bashTargetsFile(newAction, denied.Target) {
			return true
		}
		// Also check newTarget if it's a command string
		if newTarget != "" && bashTargetsFile(newTarget, denied.Target) {
			return true
		}
	}

	return false
}

// isWriteFamily returns true for tools that modify files.
func isWriteFamily(tool string) bool {
	switch tool {
	case "Edit", "Write", "NotebookEdit":
		return true
	}
	return false
}

// targetsOverlap checks if two targets refer to the same or overlapping resource.
func targetsOverlap(target1, target2 string) bool {
	if target1 == "" || target2 == "" {
		return false
	}
	return target1 == target2
}

// bashTargetsFile checks if a bash command writes to a specific file path.
func bashTargetsFile(command, filePath string) bool {
	if filePath == "" || command == "" {
		return false
	}

	// Check for common bash write patterns targeting the file
	// echo ... > /path, cat ... > /path, tee /path
	normalizedPath := filepath.Base(filePath)

	writePatterns := []string{
		"> " + filePath,
		">> " + filePath,
		"> " + normalizedPath,
		">> " + normalizedPath,
		"tee " + filePath,
		"tee " + normalizedPath,
	}

	for _, pattern := range writePatterns {
		if strings.Contains(command, pattern) {
			return true
		}
	}

	return false
}

// normalizeTarget cleans a file path for comparison.
func normalizeTarget(target string) string {
	if target == "" {
		return ""
	}
	return filepath.Clean(target)
}

// formatDowngradeReason creates a human-readable reason for the downgrade block.
func formatDowngradeReason(denied DenialRecord, newTool string) string {
	return "blocked: " + newTool + " targets same resource as previously denied " +
		denied.Tool + " on " + denied.Target
}

// DenialLimits 定义拒绝降级阈值。
type DenialLimits struct {
	MaxConsecutive int
	MaxTotal       int
}

// DefaultDenialLimits 默认阈值：连续 3 次或累计 20 次。
var DefaultDenialLimits = DenialLimits{MaxConsecutive: 3, MaxTotal: 20}

// ShouldFallbackToPrompting 检查指定会话是否已被降级到手动确认模式。
// 一旦 session 被标记为 fallback，Grant 不会自动恢复，需要显式 ClearFallback。
func (t *DenialTracker) ShouldFallbackToPrompting(sessionID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.fallbackSessions[sessionID]
}

// ClearFallback 显式清除某个 session 的 fallback 状态（用户主动重新开启 auto 模式时调用）。
func (t *DenialTracker) ClearFallback(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fallbackSessions, sessionID)
	t.consecutiveDenialsBySession[sessionID] = 0
}
