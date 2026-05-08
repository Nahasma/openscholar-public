package workflow

import "fmt"

// DocState represents the current state of a DOCX document in the pipeline.
type DocState string

const (
	StateDraft          DocState = "draft"
	StateFilling        DocState = "filling"
	StateReview         DocState = "review"
	StateApproved       DocState = "approved"
	StateExported       DocState = "exported"
	StateArchived       DocState = "archived"
	StateValidateFailed DocState = "validate_failed"
	StateRevision       DocState = "revision"
)

// validTransitions defines which state transitions are legal.
var validTransitions = map[DocState][]DocState{
	StateDraft:          {StateFilling, StateArchived},
	StateFilling:        {StateReview, StateValidateFailed, StateDraft},
	StateReview:         {StateApproved, StateRevision},
	StateRevision:       {StateReview, StateFilling},
	StateApproved:       {StateExported},
	StateExported:       {StateArchived, StateRevision},
	StateValidateFailed: {StateFilling},
}

// Transition validates and returns nil if the transition from current to target is legal.
// Returns an error describing the invalid transition otherwise.
func Transition(current DocState, target DocState) error {
	allowed, ok := validTransitions[current]
	if !ok {
		return fmt.Errorf("unknown state: %q", current)
	}
	for _, s := range allowed {
		if s == target {
			return nil
		}
	}
	return fmt.Errorf("invalid transition: %q → %q (allowed: %v)", current, target, allowed)
}

// AllStates returns all defined states.
func AllStates() []DocState {
	return []DocState{
		StateDraft, StateFilling, StateReview, StateApproved,
		StateExported, StateArchived, StateValidateFailed, StateRevision,
	}
}

// IsTerminal returns true if the state has no outgoing transitions.
func IsTerminal(state DocState) bool {
	allowed, ok := validTransitions[state]
	return !ok || len(allowed) == 0
}
