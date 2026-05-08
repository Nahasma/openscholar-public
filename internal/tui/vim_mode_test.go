package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// makeNamedKey creates a tea.KeyMsg for special / combo keys like "ctrl+d".
// We rely on msg.String() returning the key name, so we set the Type field to
// a named key type where possible, and fall back to constructing via Alt+rune
// for ctrl sequences. In practice VimMode.HandleKey only calls msg.String(),
// so we can build a minimal KeyMsg that produces the right String() output by
// encoding the key as the Paste field is not used; instead we embed the key
// identifier directly through tea.Key.
//
// The simplest approach: use tea.KeyMsg whose .String() equals the desired
// key string.  bubbletea resolves .String() as the Alt prefix + key name or
// rune sequence, so we construct the struct directly.
func makeNamedKey(keyStr string) tea.KeyMsg {
	// Map common ctrl combos used in tests to their bubbletea KeyType.
	switch keyStr {
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		// For single printable characters, treat as rune sequence.
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(keyStr)}
	}
}

// --------------------------------------------------------------------------
// Constructor / initial state
// --------------------------------------------------------------------------

func TestVimMode_NewVimMode(t *testing.T) {
	v := NewVimMode()
	if v == nil {
		t.Fatal("NewVimMode returned nil")
	}
	if v.Enabled() {
		t.Error("new VimMode should be disabled")
	}
	if v.Mode() != VimInsert {
		t.Error("new VimMode should start in VimInsert state")
	}
	if v.IsNormalMode() {
		t.Error("IsNormalMode should be false for a new VimMode")
	}
}

// --------------------------------------------------------------------------
// Disabled state — all keys pass through
// --------------------------------------------------------------------------

func TestVimMode_Disabled_KeyPassthrough(t *testing.T) {
	v := NewVimMode() // disabled by default

	keys := []string{"j", "k", "G", "g", "y", "/", "n", "N", "o", "i", "a"}
	for _, k := range keys {
		action, consumed := v.HandleKey(makeNamedKey(k))
		if consumed {
			t.Errorf("disabled VimMode: key %q should not be consumed", k)
		}
		if action != "" {
			t.Errorf("disabled VimMode: key %q should return no action, got %q", k, action)
		}
	}
}

func TestVimMode_Disabled_CtrlKeys(t *testing.T) {
	v := NewVimMode()

	for _, k := range []tea.KeyMsg{makeNamedKey("ctrl+d"), makeNamedKey("ctrl+u")} {
		action, consumed := v.HandleKey(k)
		if consumed || action != "" {
			t.Errorf("disabled VimMode: ctrl key should not be consumed/actioned")
		}
	}
}

// --------------------------------------------------------------------------
// SetEnabled resets state
// --------------------------------------------------------------------------

func TestVimMode_SetEnabled_ResetsState(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	v.EnterNormal()

	// Simulate a pending "g" key.
	v.HandleKey(makeNamedKey("g"))
	if v.pending == "" {
		t.Fatal("expected pending to be set after 'g'")
	}

	// Disabling should clear pending and revert to insert.
	v.SetEnabled(false)
	if v.pending != "" {
		t.Error("SetEnabled(false) should clear pending")
	}
	if v.Mode() != VimInsert {
		t.Error("SetEnabled(false) should reset mode to VimInsert")
	}
	if v.Enabled() {
		t.Error("SetEnabled(false) should set enabled=false")
	}
}

func TestVimMode_SetEnabled_True(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	if !v.Enabled() {
		t.Error("SetEnabled(true) should set enabled=true")
	}
	// Mode stays VimInsert until EnterNormal is called.
	if v.Mode() != VimInsert {
		t.Error("SetEnabled(true) alone should keep mode as VimInsert")
	}
}

// --------------------------------------------------------------------------
// Insert mode — keys still pass through even when enabled
// --------------------------------------------------------------------------

func TestVimMode_InsertMode_Passthrough(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	// mode is VimInsert

	for _, k := range []string{"j", "k", "G", "i", "a", "o"} {
		action, consumed := v.HandleKey(makeNamedKey(k))
		if consumed || action != "" {
			t.Errorf("VimInsert: key %q should pass through, got action=%q consumed=%v", k, action, consumed)
		}
	}
}

// --------------------------------------------------------------------------
// Normal mode — single key actions
// --------------------------------------------------------------------------

func TestVimMode_NormalMode_SingleKeys(t *testing.T) {
	tests := []struct {
		key    tea.KeyMsg
		action VimAction
	}{
		{makeNamedKey("j"), VimActionScrollDown},
		{makeNamedKey("k"), VimActionScrollUp},
		{makeNamedKey("G"), VimActionGotoBottom},
		{makeNamedKey("/"), VimActionSearch},
		{makeNamedKey("n"), VimActionNextMatch},
		{makeNamedKey("N"), VimActionPrevMatch},
		{makeNamedKey("o"), VimActionToggleFold},
		{makeNamedKey("ctrl+d"), VimActionHalfPageDown},
		{makeNamedKey("ctrl+u"), VimActionHalfPageUp},
	}

	for _, tt := range tests {
		v := NewVimMode()
		v.SetEnabled(true)
		v.EnterNormal()

		action, consumed := v.HandleKey(tt.key)
		if !consumed {
			t.Errorf("key %q: expected consumed=true", tt.key.String())
		}
		if action != tt.action {
			t.Errorf("key %q: expected action %q, got %q", tt.key.String(), tt.action, action)
		}
	}
}

// --------------------------------------------------------------------------
// Normal mode — mode switch keys (i, a)
// --------------------------------------------------------------------------

func TestVimMode_NormalMode_InsertModeSwitch_i(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	v.EnterNormal()

	action, consumed := v.HandleKey(makeNamedKey("i"))
	if !consumed {
		t.Error("'i' in normal mode should be consumed")
	}
	if action != VimActionInsertMode {
		t.Errorf("expected VimActionInsertMode, got %q", action)
	}
	if v.Mode() != VimInsert {
		t.Error("'i' should switch mode to VimInsert")
	}
	if v.IsNormalMode() {
		t.Error("IsNormalMode should be false after 'i'")
	}
}

func TestVimMode_NormalMode_InsertModeSwitch_a(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	v.EnterNormal()

	action, consumed := v.HandleKey(makeNamedKey("a"))
	if !consumed {
		t.Error("'a' in normal mode should be consumed")
	}
	if action != VimActionInsertMode {
		t.Errorf("expected VimActionInsertMode, got %q", action)
	}
	if v.Mode() != VimInsert {
		t.Error("'a' should switch mode to VimInsert")
	}
}

// --------------------------------------------------------------------------
// Compound keys — gg (goto top)
// --------------------------------------------------------------------------

func TestVimMode_CompoundKey_gg(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	v.EnterNormal()

	// First 'g': should be consumed but no action yet.
	action, consumed := v.HandleKey(makeNamedKey("g"))
	if !consumed {
		t.Error("first 'g' should be consumed (pending)")
	}
	if action != "" {
		t.Errorf("first 'g' should return no action, got %q", action)
	}
	if v.pending != "g" {
		t.Errorf("pending should be 'g', got %q", v.pending)
	}

	// Second 'g': should resolve to VimActionGotoTop.
	action, consumed = v.HandleKey(makeNamedKey("g"))
	if !consumed {
		t.Error("second 'g' (completing gg) should be consumed")
	}
	if action != VimActionGotoTop {
		t.Errorf("expected VimActionGotoTop, got %q", action)
	}
	if v.pending != "" {
		t.Error("pending should be cleared after compound key resolved")
	}
}

// --------------------------------------------------------------------------
// Compound keys — yy (yank block)
// --------------------------------------------------------------------------

func TestVimMode_CompoundKey_yy(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	v.EnterNormal()

	// First 'y'
	action, consumed := v.HandleKey(makeNamedKey("y"))
	if !consumed {
		t.Error("first 'y' should be consumed (pending)")
	}
	if action != "" {
		t.Errorf("first 'y' should return no action, got %q", action)
	}
	if v.pending != "y" {
		t.Errorf("pending should be 'y', got %q", v.pending)
	}

	// Second 'y'
	action, consumed = v.HandleKey(makeNamedKey("y"))
	if !consumed {
		t.Error("second 'y' (completing yy) should be consumed")
	}
	if action != VimActionYankBlock {
		t.Errorf("expected VimActionYankBlock, got %q", action)
	}
	if v.pending != "" {
		t.Error("pending should be cleared after compound key resolved")
	}
}

// --------------------------------------------------------------------------
// Compound keys — unknown/cancelled sequences
// --------------------------------------------------------------------------

func TestVimMode_CompoundKey_UnknownSequence(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	v.EnterNormal()

	// 'g' followed by an unexpected key (not 'g').
	v.HandleKey(makeNamedKey("g")) // sets pending="g"
	action, consumed := v.HandleKey(makeNamedKey("j"))
	// The combined sequence "gj" is unknown: consumed=true (no passthrough),
	// but action should be empty.
	if !consumed {
		t.Error("second key of unknown compound should still be consumed")
	}
	if action != "" {
		t.Errorf("unknown compound sequence should return no action, got %q", action)
	}
	if v.pending != "" {
		t.Error("pending should be cleared after unknown compound")
	}
}

func TestVimMode_CompoundKey_yUnknown(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	v.EnterNormal()

	v.HandleKey(makeNamedKey("y")) // sets pending="y"
	action, consumed := v.HandleKey(makeNamedKey("k"))
	if !consumed {
		t.Error("second key of unknown compound should be consumed")
	}
	if action != "" {
		t.Errorf("unknown compound sequence should return no action, got %q", action)
	}
}

// --------------------------------------------------------------------------
// EnterNormal / EnterInsert round-trip
// --------------------------------------------------------------------------

func TestVimMode_NormalInsertRoundTrip(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)

	// Start in insert
	if v.Mode() != VimInsert {
		t.Error("should start in VimInsert")
	}

	v.EnterNormal()
	if v.Mode() != VimNormal {
		t.Error("EnterNormal should switch to VimNormal")
	}
	if !v.IsNormalMode() {
		t.Error("IsNormalMode should return true")
	}

	v.EnterInsert()
	if v.Mode() != VimInsert {
		t.Error("EnterInsert should switch back to VimInsert")
	}
	if v.IsNormalMode() {
		t.Error("IsNormalMode should be false after EnterInsert")
	}
}

func TestVimMode_EnterNormal_Disabled_NoOp(t *testing.T) {
	v := NewVimMode() // disabled
	v.EnterNormal()
	if v.Mode() != VimInsert {
		t.Error("EnterNormal on disabled VimMode should be a no-op")
	}
}

// --------------------------------------------------------------------------
// IsNormalMode helper
// --------------------------------------------------------------------------

func TestVimMode_IsNormalMode(t *testing.T) {
	v := NewVimMode()

	// disabled → always false
	v.EnterNormal() // no-op when disabled
	if v.IsNormalMode() {
		t.Error("IsNormalMode should be false when disabled")
	}

	v.SetEnabled(true)
	if v.IsNormalMode() {
		t.Error("IsNormalMode should be false immediately after SetEnabled(true)")
	}

	v.EnterNormal()
	if !v.IsNormalMode() {
		t.Error("IsNormalMode should be true after EnterNormal")
	}
}

// --------------------------------------------------------------------------
// Unrecognised keys in normal mode are consumed (not leaked to textarea)
// --------------------------------------------------------------------------

func TestVimMode_NormalMode_UnknownKeyConsumed(t *testing.T) {
	v := NewVimMode()
	v.SetEnabled(true)
	v.EnterNormal()

	// A completely unrecognised key should be consumed but return no action.
	action, consumed := v.HandleKey(makeNamedKey("z"))
	if !consumed {
		t.Error("unrecognised key in normal mode should be consumed")
	}
	if action != "" {
		t.Errorf("unrecognised key should return no action, got %q", action)
	}
}
