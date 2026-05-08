// Package keybinding provides a declarative key binding framework with scope isolation.
// It replaces hard-coded switch-case key handling in TUI with a registry of actions
// that can be resolved by scope and key combination.
package keybinding

import (
	"fmt"
	"strings"
	"sync"
)

// Scope defines the context in which a key binding is active.
type Scope string

const (
	ScopeGlobal  Scope = "global"  // Active in all states
	ScopeChat    Scope = "chat"    // Active only in chat state
	ScopeDialog  Scope = "dialog"  // Active only in dialog states
	ScopeOverlay Scope = "overlay" // Active when overlay is visible
	ScopeVim     Scope = "vim"     // Active only when vim normal mode is enabled
)

// Action identifies a user-triggerable action.
type Action string

const (
	// Global actions (available everywhere)
	ActionQuit        Action = "quit"
	ActionCancelOrQuit Action = "cancel_or_quit"
	ActionRedraw      Action = "redraw"

	// Chat actions (chat state only)
	ActionCycleMode      Action = "cycle_mode"
	ActionNewSession     Action = "new_session"
	ActionSessionBrowser Action = "session_browser"
	ActionHistorySearch  Action = "history_search"
	ActionTextSearch     Action = "text_search"
	ActionToolNavDown    Action = "tool_nav_down"
	ActionToolNavUp      Action = "tool_nav_up"
	ActionToolToggle     Action = "tool_toggle"
	ActionHelp           Action = "help"
	ActionSend           Action = "send"
	ActionCancel         Action = "cancel"
	ActionPageUp         Action = "page_up"
	ActionPageDown       Action = "page_down"

	// Overlay actions
	ActionCopyResponse       Action = "copy_response"
	ActionCopyCodeBlock      Action = "copy_code_block"
	ActionOverlayAccept      Action = "overlay.accept"
	ActionOverlayReject      Action = "overlay.reject"
	ActionOverlayDismiss     Action = "overlay.dismiss"
	ActionOverlayInputToggle Action = "overlay.input_toggle"
	ActionScrollTop          Action = "scroll_top"
	ActionScrollBottom       Action = "scroll_bottom"

	// Vim mode actions (ScopeVim, only active in vim normal mode)
	ActionVimScrollDown   Action = "vim.scroll_down"    // j
	ActionVimScrollUp     Action = "vim.scroll_up"      // k
	ActionVimGotoTop      Action = "vim.goto_top"       // gg (compound)
	ActionVimGotoBottom   Action = "vim.goto_bottom"    // G
	ActionVimSearch       Action = "vim.search"         // /
	ActionVimNextMatch    Action = "vim.next_match"     // n
	ActionVimPrevMatch    Action = "vim.prev_match"     // N
	ActionVimToggleFold   Action = "vim.toggle_fold"    // o
	ActionVimYankBlock    Action = "vim.yank_block"     // yy (compound)
	ActionVimInsertMode   Action = "vim.insert_mode"    // i or a
	ActionVimHalfPageDown Action = "vim.half_page_down" // ctrl+d
	ActionVimHalfPageUp   Action = "vim.half_page_up"   // ctrl+u
)

// Binding maps a key combination to an action within a scope.
type Binding struct {
	ID          string
	Scope       Scope
	Key         string // Key string, e.g. "ctrl+r", "ctrl+c", "shift+tab", "?"
	Description string
	Reserved    bool // If true, cannot be rebound by user
}

// Registry manages key bindings and resolves key presses to actions.
type Registry struct {
	mu       sync.RWMutex
	bindings map[Scope]map[string]actionBinding // scope -> key -> binding+action
}

type actionBinding struct {
	binding Binding
	action  Action
}

// NewRegistry creates a new empty binding registry.
func NewRegistry() *Registry {
	return &Registry{
		bindings: map[Scope]map[string]actionBinding{
			ScopeGlobal:  {},
			ScopeChat:    {},
			ScopeDialog:  {},
			ScopeOverlay: {},
			ScopeVim:     {},
		},
	}
}

// Register adds a binding to the registry.
func (r *Registry) Register(b Binding, a Action) {
	r.mu.Lock()
	defer r.mu.Unlock()

	scope := b.Scope
	if _, ok := r.bindings[scope]; !ok {
		r.bindings[scope] = map[string]actionBinding{}
	}
	r.bindings[scope][normalizeKey(b.Key)] = actionBinding{binding: b, action: a}
}

// Resolve looks up the action for a key press in a given scope.
// It first checks the specific scope, then falls back to ScopeGlobal.
// Returns the action and true if found, or ("", false) if no binding matches.
func (r *Registry) Resolve(scope Scope, key string) (Action, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalized := normalizeKey(key)

	// Check specific scope first
	if scopeBindings, ok := r.bindings[scope]; ok {
		if ab, ok := scopeBindings[normalized]; ok {
			return ab.action, true
		}
	}

	// Fall back to global scope
	if scope != ScopeGlobal {
		if globalBindings, ok := r.bindings[ScopeGlobal]; ok {
			if ab, ok := globalBindings[normalized]; ok {
				return ab.action, true
			}
		}
	}

	return "", false
}

// ActionContext provides runtime state for context-aware key resolution.
type ActionContext struct {
	OverlayActive bool
	IsProcessing  bool
	HasInput      bool
	InputEmpty    bool
}

// ResolveWithContext resolves a key press considering runtime context.
// When overlay is active, overlay scope bindings take highest priority.
// Falls back to specific scope, then global.
func (r *Registry) ResolveWithContext(key string, scope Scope, ctx ActionContext) (Action, bool) {
	normalized := normalizeKey(key)
	r.mu.RLock()
	defer r.mu.RUnlock()

	// overlay active → overlay scope first
	if ctx.OverlayActive {
		if obs, ok := r.bindings[ScopeOverlay]; ok {
			if ab, ok := obs[normalized]; ok {
				return ab.action, true
			}
		}
	}

	// specific scope
	if scopeBindings, ok := r.bindings[scope]; ok {
		if ab, ok := scopeBindings[normalized]; ok {
			return ab.action, true
		}
	}

	// global fallback
	if scope != ScopeGlobal {
		if globalBindings, ok := r.bindings[ScopeGlobal]; ok {
			if ab, ok := globalBindings[normalized]; ok {
				return ab.action, true
			}
		}
	}

	return "", false
}

// Bindings returns all bindings for a given scope.
func (r *Registry) Bindings(scope Scope) []Binding {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Binding
	if scopeBindings, ok := r.bindings[scope]; ok {
		for _, ab := range scopeBindings {
			result = append(result, ab.binding)
		}
	}
	return result
}

// AllBindings returns all registered bindings across all scopes.
func (r *Registry) AllBindings() []Binding {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Binding
	for _, scopeBindings := range r.bindings {
		for _, ab := range scopeBindings {
			result = append(result, ab.binding)
		}
	}
	return result
}

// ValidateUserOverrides checks that user overrides don't rebind reserved keys,
// don't conflict with existing bindings in the same scope, and don't reference
// unknown actions.
func (r *Registry) ValidateUserOverrides(overrides map[string]string) []error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error

	// Collect reserved keys
	reservedKeys := map[string]bool{}
	for _, scopeBindings := range r.bindings {
		for _, ab := range scopeBindings {
			if ab.binding.Reserved {
				reservedKeys[normalizeKey(ab.binding.Key)] = true
			}
		}
	}

	// Build action→(scope, key) index to find existing actions
	actionIndex := map[Action]struct{ scope Scope; key string }{}
	for scope, scopeBindings := range r.bindings {
		for key, ab := range scopeBindings {
			actionIndex[ab.action] = struct{ scope Scope; key string }{scope, key}
		}
	}

	// Track new key assignments to detect inter-override conflicts
	newAssignments := map[string]string{} // normalized_key → actionName

	for actionName, newKey := range overrides {
		action := Action(actionName)
		normalized := normalizeKey(newKey)

		// Check unknown action
		if _, exists := actionIndex[action]; !exists {
			errs = append(errs, fmt.Errorf("unknown action %q", actionName))
			continue
		}

		// Check reserved keys
		if reservedKeys[normalized] {
			errs = append(errs, fmt.Errorf("key %q is reserved and cannot be rebound (action: %s)", newKey, actionName))
			continue
		}

		// Check conflict with existing bindings (same scope, different action)
		info := actionIndex[action]
		if scopeBindings, ok := r.bindings[info.scope]; ok {
			if existing, ok := scopeBindings[normalized]; ok && existing.action != action {
				errs = append(errs, fmt.Errorf("key %q is already bound to %s in scope %s (action: %s)",
					newKey, existing.action, info.scope, actionName))
				continue
			}
		}

		// Check inter-override conflicts (two overrides targeting same key)
		if prevAction, ok := newAssignments[normalized]; ok {
			errs = append(errs, fmt.Errorf("key %q assigned to both %s and %s", newKey, prevAction, actionName))
			continue
		}
		newAssignments[normalized] = actionName
	}

	return errs
}

// ApplyOverrides applies user-defined key overrides.
// It remaps existing actions to new keys. Only applies if all overrides are valid.
func (r *Registry) ApplyOverrides(overrides map[string]string) error {
	errs := r.ValidateUserOverrides(overrides)
	if len(errs) > 0 {
		return errs[0]
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for actionName, newKey := range overrides {
		action := Action(actionName)
		normalized := normalizeKey(newKey)

		// Find and remove existing binding for this action
		found := false
		for scope, scopeBindings := range r.bindings {
			for key, ab := range scopeBindings {
				if ab.action == action {
					delete(scopeBindings, key)
					// Re-register with new key
					ab.binding.Key = newKey
					r.bindings[scope][normalized] = ab
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	return nil
}

// normalizeKey converts a key string to a canonical form for comparison.
func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
