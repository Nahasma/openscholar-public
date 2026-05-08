package permission

import "fmt"

// DecisionSource identifies where a permission decision originated.
type DecisionSource string

const (
	SourceMode      DecisionSource = "mode"           // Mode-level constraint (e.g., plan mode)
	SourceRule      DecisionSource = "rule"            // Rule evaluation (deny/ask/allow rules)
	SourceUser      DecisionSource = "user"            // User clicked deny/allow in TUI
	SourceTracker   DecisionSource = "denial_tracker"  // Downgrade or fallback detection
	SourcePathGuard DecisionSource = "path_guard"      // Path security validation
	SourceAuto      DecisionSource = "auto_allow"      // Auto-approve session or DB rule match
	SourceDangerous DecisionSource = "dangerous_target" // Dangerous removal target detection
)

// DenialReason provides a machine-readable reason for denial.
type DenialReason string

const (
	ReasonPlanMode        DenialReason = "plan_mode_write_block"
	ReasonRuleDenied      DenialReason = "rule_denied"
	ReasonUserDenied      DenialReason = "user_denied"
	ReasonDowngrade       DenialReason = "downgrade_blocked"
	ReasonPathSecurity    DenialReason = "path_security_blocked"
	ReasonDangerousTarget DenialReason = "dangerous_target_blocked"
)

// DecisionResult is the structured result of a permission evaluation.
// It replaces the bare bool return, providing the reason, source, and
// a human/LLM-readable message for every decision.
type DecisionResult struct {
	Allowed      bool           // Whether the operation is permitted
	Prompted     bool           // Whether a TUI prompt was shown to the user
	Source       DecisionSource // Where the decision came from
	Reason       DenialReason   // Machine-readable reason (only set when denied)
	RuleDecision Decision       // The raw rule evaluation result (allow/deny/ask/"")
	Message      string         // Human/LLM-readable explanation
}

// PermissionError wraps a DecisionResult as an error for tool-layer use.
// Tools return this instead of the bare ErrorPermissionDenied sentinel.
type PermissionError struct {
	Result DecisionResult
}

func (e *PermissionError) Error() string {
	if e.Result.Message != "" {
		return e.Result.Message
	}
	return "permission denied"
}

// Is makes PermissionError match errors.Is(err, ErrorPermissionDenied).
func (e *PermissionError) Is(target error) bool {
	return target == ErrorPermissionDenied
}

// NewPermissionError creates a PermissionError from a DecisionResult.
func NewPermissionError(r DecisionResult) *PermissionError {
	return &PermissionError{Result: r}
}

// AsPermissionError extracts a *PermissionError from an error chain.
// Returns (perr, true) if found, (nil, false) otherwise.
func AsPermissionError(err error) (*PermissionError, bool) {
	var pe *PermissionError
	if ok := errorAs(err, &pe); ok {
		return pe, true
	}
	return nil, false
}

// errorAs is a thin wrapper to avoid importing errors in this file.
// It uses the same interface-based unwrap that errors.As uses.
func errorAs(err error, target any) bool {
	if err == nil {
		return false
	}
	pe, ok := err.(*PermissionError)
	if ok {
		*(target.(**PermissionError)) = pe
		return true
	}
	// Check Unwrap chain
	type wrapper interface{ Unwrap() error }
	if w, ok := err.(wrapper); ok {
		return errorAs(w.Unwrap(), target)
	}
	return false
}

// UserMessage returns a message suitable for returning to the LLM as a tool result.
// It includes recovery hints based on the denial reason.
func (r DecisionResult) UserMessage() string {
	if r.Allowed {
		return ""
	}

	base := r.Message
	if base == "" {
		base = "Permission denied."
	}

	switch r.Reason {
	case ReasonPlanMode:
		return base + "\n\nYou are in plan (read-only) mode. Write operations are blocked. " +
			"Options:\n1. Use AskUser to ask the user to switch out of plan mode\n" +
			"2. Try a read-only approach instead\n3. Skip this step"
	case ReasonUserDenied:
		return base + "\n\nThe user denied this operation. " +
			"Options:\n1. Use AskUser to explain why this is needed and request permission\n" +
			"2. Try an alternative approach\n3. Skip this step"
	case ReasonRuleDenied:
		return base + "\n\nThis operation is blocked by a security rule. " +
			"Try a different approach."
	case ReasonDowngrade:
		return base + "\n\nThis operation was blocked because a similar operation was recently denied. " +
			"Do not retry the same operation."
	case ReasonPathSecurity, ReasonDangerousTarget:
		return base + "\n\nThis operation is blocked for security reasons. " +
			"Do not attempt to bypass this restriction."
	default:
		return base + "\n\nOptions:\n1. Use AskUser to ask the user for guidance\n" +
			"2. Try an alternative approach\n3. Skip this step"
	}
}

// Deny helpers for common denial patterns.

func denyPlanMode(toolName, description string) DecisionResult {
	return DecisionResult{
		Allowed: false,
		Source:  SourceMode,
		Reason:  ReasonPlanMode,
		Message: fmt.Sprintf("Blocked by plan mode: %s write command %q is not allowed in read-only mode.", toolName, truncateCmd(description, 80)),
	}
}

func denyRule() DecisionResult {
	return DecisionResult{
		Allowed: false,
		Source:  SourceRule,
		Reason:  ReasonRuleDenied,
		Message: "Blocked by security rule.",
	}
}

func denyDowngrade(reason string) DecisionResult {
	return DecisionResult{
		Allowed: false,
		Source:  SourceTracker,
		Reason:  ReasonDowngrade,
		Message: "Blocked: " + reason,
	}
}

func denyPathSecurity() DecisionResult {
	return DecisionResult{
		Allowed: false,
		Source:  SourcePathGuard,
		Reason:  ReasonPathSecurity,
		Message: "Blocked by path security check.",
	}
}

func denyDangerousTarget() DecisionResult {
	return DecisionResult{
		Allowed: false,
		Source:  SourceDangerous,
		Reason:  ReasonDangerousTarget,
		Message: "Blocked: dangerous removal target detected.",
	}
}

func allowAuto(source DecisionSource) DecisionResult {
	return DecisionResult{
		Allowed: true,
		Source:  source,
	}
}

func allowRule() DecisionResult {
	return DecisionResult{
		Allowed:      true,
		Source:       SourceRule,
		RuleDecision: Allow,
	}
}

func denyUser() DecisionResult {
	return DecisionResult{
		Allowed:  false,
		Prompted: true,
		Source:   SourceUser,
		Reason:   ReasonUserDenied,
		Message:  "Permission denied by user.",
	}
}

func allowUser(prompted bool) DecisionResult {
	return DecisionResult{
		Allowed:  true,
		Prompted: prompted,
		Source:   SourceUser,
	}
}

// truncateCmd truncates a command string for display.
func truncateCmd(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
