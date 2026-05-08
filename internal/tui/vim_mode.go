// Package tui provides terminal UI components for OpenScholar.
// This file implements VimMode: a pure-logic vim normal mode handler for the
// chat viewport. It is consumed by key_handlers.go (Stage 3 integration).
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// VimModeState represents whether vim mode is in normal or insert mode.
type VimModeState int

const (
	// VimInsert is the default state. In this state all keys pass through to the
	// underlying input component unchanged, exactly as if vim mode were disabled.
	VimInsert VimModeState = iota

	// VimNormal intercepts navigation keys and routes them to viewport actions.
	VimNormal
)

// VimAction identifies a viewport action triggered by a vim key binding.
type VimAction string

const (
	// Single-key actions
	VimActionScrollDown   VimAction = "vim.scroll_down"    // j
	VimActionScrollUp     VimAction = "vim.scroll_up"      // k
	VimActionGotoBottom   VimAction = "vim.goto_bottom"    // G
	VimActionSearch       VimAction = "vim.search"         // /
	VimActionNextMatch    VimAction = "vim.next_match"     // n
	VimActionPrevMatch    VimAction = "vim.prev_match"     // N
	VimActionToggleFold   VimAction = "vim.toggle_fold"    // o
	VimActionHalfPageDown VimAction = "vim.half_page_down" // ctrl+d
	VimActionHalfPageUp   VimAction = "vim.half_page_up"   // ctrl+u

	// Mode transition
	VimActionInsertMode VimAction = "vim.insert_mode" // i or a

	// Compound-key actions (resolved via pending buffer)
	VimActionGotoTop  VimAction = "vim.goto_top"  // gg
	VimActionYankBlock VimAction = "vim.yank_block" // yy
)

// VimMode is a pure-logic vim normal mode handler. It does not hold any
// reference to the TUI Model; callers are responsible for applying the
// returned VimAction to the viewport.
//
// Lifecycle:
//
//	enabled=false → HandleKey always returns ("", false); behaves as passthrough.
//	enabled=true, mode=VimInsert → same passthrough; Esc transitions to VimNormal.
//	enabled=true, mode=VimNormal → intercepts navigation keys, returns VimAction.
type VimMode struct {
	enabled bool
	mode    VimModeState
	// pending holds a partial compound key sequence (e.g. "g" waiting for the
	// second "g" to form "gg", or "y" waiting for "y" to form "yy").
	// It is cleared on the next key press regardless of whether a compound
	// sequence is recognised, preventing stale state from accumulating.
	pending string
}

// NewVimMode creates a disabled VimMode in insert state.
func NewVimMode() *VimMode {
	return &VimMode{
		enabled: false,
		mode:    VimInsert,
	}
}

// SetEnabled enables or disables vim mode. When disabled, HandleKey always
// returns ("", false) and the state is reset to VimInsert.
func (v *VimMode) SetEnabled(enabled bool) {
	v.enabled = enabled
	if !enabled {
		v.mode = VimInsert
		v.pending = ""
	}
}

// Enabled reports whether vim mode is enabled.
func (v *VimMode) Enabled() bool { return v.enabled }

// Mode returns the current VimModeState.
func (v *VimMode) Mode() VimModeState { return v.mode }

// EnterNormal switches to VimNormal mode. This is the handler for Esc when
// vim is enabled and the input area is not processing. Callers (key_handlers.go)
// call this method and then re-focus the viewport instead of the textarea.
func (v *VimMode) EnterNormal() {
	if v.enabled {
		v.mode = VimNormal
		v.pending = ""
	}
}

// EnterInsert switches back to VimInsert mode. Called by key_handlers.go when
// a VimActionInsertMode is returned, so it can re-focus the textarea.
func (v *VimMode) EnterInsert() {
	v.mode = VimInsert
	v.pending = ""
}

// HandleKey processes a key message and returns the corresponding VimAction.
//
// Returns (action, true) when the key is consumed by vim normal mode.
// Returns ("", false) when:
//   - vim mode is disabled,
//   - current state is VimInsert,
//   - the key starts a compound sequence (pending is set; caller should not
//     pass the key to the textarea),
//   - the key does not match any known vim binding.
//
// The second return value (consumed bool) is true whenever the key should NOT
// be forwarded to the underlying input component. This includes both the case
// where an action was produced and the case where a partial compound key was
// buffered.
func (v *VimMode) HandleKey(msg tea.KeyMsg) (VimAction, bool) {
	if !v.enabled || v.mode == VimInsert {
		return "", false
	}

	key := msg.String()

	// Resolve pending compound sequences first.
	if v.pending != "" {
		combined := v.pending + key
		v.pending = ""
		switch combined {
		case "gg":
			return VimActionGotoTop, true
		case "yy":
			return VimActionYankBlock, true
		}
		// Unknown compound sequence: discard silently (consumed but no action).
		return "", true
	}

	// Single-key dispatch in normal mode.
	switch key {
	case "j":
		return VimActionScrollDown, true
	case "k":
		return VimActionScrollUp, true
	case "G":
		return VimActionGotoBottom, true
	case "g":
		// Buffer "g"; wait for the second character.
		v.pending = "g"
		return "", true
	case "y":
		// Buffer "y"; wait for the second character.
		v.pending = "y"
		return "", true
	case "/":
		return VimActionSearch, true
	case "n":
		return VimActionNextMatch, true
	case "N":
		return VimActionPrevMatch, true
	case "o":
		return VimActionToggleFold, true
	case "i", "a":
		v.EnterInsert()
		return VimActionInsertMode, true
	case "ctrl+d":
		return VimActionHalfPageDown, true
	case "ctrl+u":
		return VimActionHalfPageUp, true
	}

	// Unrecognised key in normal mode: consume silently to prevent accidental
	// characters leaking into the input textarea.
	return "", true
}

// IsNormalMode reports whether vim normal mode is currently active.
// Convenience helper for view rendering (e.g. status bar indicator).
func (v *VimMode) IsNormalMode() bool {
	return v.enabled && v.mode == VimNormal
}
