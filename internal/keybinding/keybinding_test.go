package keybinding

import (
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestRegisterAndResolve(t *testing.T) {
	r := NewRegistry()
	r.Register(Binding{
		ID: "test", Scope: ScopeChat, Key: "ctrl+r",
		Description: "Test binding",
	}, ActionHistorySearch)

	// Resolve in correct scope
	action, ok := r.Resolve(ScopeChat, "ctrl+r")
	if !ok {
		t.Fatal("expected binding to be found")
	}
	if action != ActionHistorySearch {
		t.Fatalf("expected %s, got %s", ActionHistorySearch, action)
	}

	// Resolve in wrong scope (not global, so should not find)
	_, ok = r.Resolve(ScopeDialog, "ctrl+r")
	if ok {
		t.Fatal("expected binding NOT to be found in different scope")
	}
}

func TestGlobalScopeFallback(t *testing.T) {
	r := NewRegistry()
	r.Register(Binding{
		ID: "global_test", Scope: ScopeGlobal, Key: "ctrl+c",
		Description: "Global action", Reserved: true,
	}, ActionCancelOrQuit)

	// Should be resolvable from any scope via global fallback
	action, ok := r.Resolve(ScopeChat, "ctrl+c")
	if !ok {
		t.Fatal("expected global binding to be found via fallback")
	}
	if action != ActionCancelOrQuit {
		t.Fatalf("expected %s, got %s", ActionCancelOrQuit, action)
	}

	action, ok = r.Resolve(ScopeDialog, "ctrl+c")
	if !ok {
		t.Fatal("expected global binding to be found via fallback in dialog")
	}
	if action != ActionCancelOrQuit {
		t.Fatalf("expected %s, got %s", ActionCancelOrQuit, action)
	}
}

func TestScopePriority(t *testing.T) {
	r := NewRegistry()

	// Register same key in both global and chat scope
	r.Register(Binding{
		ID: "global_esc", Scope: ScopeGlobal, Key: "esc",
		Description: "Global escape",
	}, ActionQuit)

	r.Register(Binding{
		ID: "chat_esc", Scope: ScopeChat, Key: "esc",
		Description: "Chat escape",
	}, ActionCancel)

	// Chat scope should take priority
	action, ok := r.Resolve(ScopeChat, "esc")
	if !ok {
		t.Fatal("expected binding to be found")
	}
	if action != ActionCancel {
		t.Fatalf("expected chat-scoped %s, got %s", ActionCancel, action)
	}

	// Global scope resolves its own binding
	action, ok = r.Resolve(ScopeGlobal, "esc")
	if !ok {
		t.Fatal("expected binding to be found")
	}
	if action != ActionQuit {
		t.Fatalf("expected global-scoped %s, got %s", ActionQuit, action)
	}
}

func TestUnregisteredKeyPassthrough(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Resolve(ScopeChat, "ctrl+z")
	if ok {
		t.Fatal("expected unregistered key to not resolve")
	}
}

func TestKeyNormalization(t *testing.T) {
	r := NewRegistry()
	r.Register(Binding{
		ID: "test", Scope: ScopeChat, Key: "Ctrl+R",
		Description: "Test",
	}, ActionHistorySearch)

	// Should match regardless of case
	action, ok := r.Resolve(ScopeChat, "ctrl+r")
	if !ok {
		t.Fatal("expected case-insensitive match")
	}
	if action != ActionHistorySearch {
		t.Fatalf("expected %s, got %s", ActionHistorySearch, action)
	}
}

func TestValidateUserOverrides_ReservedKey(t *testing.T) {
	r := DefaultRegistry()

	errs := r.ValidateUserOverrides(map[string]string{
		"history_search": "ctrl+c", // trying to rebind to reserved key
	})
	if len(errs) == 0 {
		t.Fatal("expected error for reserved key rebinding")
	}
}

func TestValidateUserOverrides_ValidKey(t *testing.T) {
	r := DefaultRegistry()

	errs := r.ValidateUserOverrides(map[string]string{
		"history_search": "ctrl+h", // non-reserved key
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestApplyOverrides(t *testing.T) {
	r := DefaultRegistry()

	err := r.ApplyOverrides(map[string]string{
		string(ActionHistorySearch): "ctrl+h",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Old key should no longer resolve
	_, ok := r.Resolve(ScopeChat, "ctrl+r")
	if ok {
		t.Fatal("expected old key binding to be removed")
	}

	// New key should resolve
	action, ok := r.Resolve(ScopeChat, "ctrl+h")
	if !ok {
		t.Fatal("expected new key binding to resolve")
	}
	if action != ActionHistorySearch {
		t.Fatalf("expected %s, got %s", ActionHistorySearch, action)
	}
}

func TestValidateUserOverrides_ConflictWithExisting(t *testing.T) {
	r := DefaultRegistry()

	// Try to rebind history_search to ctrl+n (already used by new_session)
	errs := r.ValidateUserOverrides(map[string]string{
		string(ActionHistorySearch): "ctrl+n",
	})
	if len(errs) == 0 {
		t.Fatal("expected error for conflicting key override")
	}
}

func TestValidateUserOverrides_InterOverrideConflict(t *testing.T) {
	r := DefaultRegistry()

	// Two overrides targeting the same key
	errs := r.ValidateUserOverrides(map[string]string{
		string(ActionHistorySearch): "ctrl+h",
		string(ActionTextSearch):    "ctrl+h",
	})
	if len(errs) == 0 {
		t.Fatal("expected error for inter-override conflict")
	}
}

func TestValidateUserOverrides_UnknownAction(t *testing.T) {
	r := DefaultRegistry()

	errs := r.ValidateUserOverrides(map[string]string{
		"typo_action": "ctrl+h",
	})
	if len(errs) == 0 {
		t.Fatal("expected error for unknown action")
	}
}

func TestApplyOverrides_ReservedKeyRejected(t *testing.T) {
	r := DefaultRegistry()

	err := r.ApplyOverrides(map[string]string{
		string(ActionHistorySearch): "ctrl+c",
	})
	if err == nil {
		t.Fatal("expected error for reserved key override")
	}
}

func TestDefaultRegistry_Completeness(t *testing.T) {
	r := DefaultRegistry()

	// All expected actions should be resolvable
	expectedChat := map[Action]string{
		ActionCycleMode:      "shift+tab",
		ActionNewSession:     "ctrl+n",
		ActionSessionBrowser: "ctrl+p",
		ActionHistorySearch:  "ctrl+r",
		ActionTextSearch:     "ctrl+f",
		ActionToolNavDown:    "ctrl+j",
		ActionToolNavUp:      "ctrl+k",
		ActionToolToggle:     "ctrl+o",
		ActionCancel:         "esc",
		ActionSend:           "enter",
		ActionHelp:           "?",
		ActionPageUp:         "pgup",
		ActionPageDown:       "pgdown",
	}

	for action, key := range expectedChat {
		resolved, ok := r.Resolve(ScopeChat, key)
		if !ok {
			t.Errorf("expected chat binding for key %q (action %s)", key, action)
			continue
		}
		if resolved != action {
			t.Errorf("key %q: expected action %s, got %s", key, action, resolved)
		}
	}

	// Global bindings
	expectedGlobal := map[Action]string{
		ActionCancelOrQuit: "ctrl+c",
		ActionRedraw:       "ctrl+l",
	}

	for action, key := range expectedGlobal {
		resolved, ok := r.Resolve(ScopeGlobal, key)
		if !ok {
			t.Errorf("expected global binding for key %q (action %s)", key, action)
			continue
		}
		if resolved != action {
			t.Errorf("key %q: expected action %s, got %s", key, action, resolved)
		}
	}
}

func TestIsReserved(t *testing.T) {
	tests := []struct {
		key      string
		reserved bool
	}{
		{"ctrl+c", true},
		{"enter", true},
		{"esc", true},
		{"Ctrl+C", true}, // case insensitive
		{"ctrl+r", false},
		{"ctrl+f", false},
		{"shift+tab", false},
	}

	for _, tt := range tests {
		got := IsReserved(tt.key)
		if got != tt.reserved {
			t.Errorf("IsReserved(%q) = %v, want %v", tt.key, got, tt.reserved)
		}
	}
}

func TestBindings(t *testing.T) {
	r := DefaultRegistry()

	chatBindings := r.Bindings(ScopeChat)
	if len(chatBindings) == 0 {
		t.Fatal("expected chat bindings to be non-empty")
	}

	globalBindings := r.Bindings(ScopeGlobal)
	if len(globalBindings) == 0 {
		t.Fatal("expected global bindings to be non-empty")
	}

	overlayBindings := r.Bindings(ScopeOverlay)
	vimBindings := r.Bindings(ScopeVim)
	all := r.AllBindings()
	if len(all) != len(chatBindings)+len(globalBindings)+len(overlayBindings)+len(vimBindings) {
		t.Errorf("AllBindings count mismatch: %d != %d + %d + %d + %d",
			len(all), len(chatBindings), len(globalBindings), len(overlayBindings), len(vimBindings))
	}
}

func TestResolveWithContext_OverlayPriority(t *testing.T) {
	r := DefaultRegistry()

	// Without overlay: enter resolves to send in chat scope
	action, ok := r.ResolveWithContext("enter", ScopeChat, ActionContext{})
	if !ok || action != ActionSend {
		t.Fatalf("expected %s without overlay, got %s (ok=%v)", ActionSend, action, ok)
	}

	// With overlay active: enter resolves to overlay.accept
	action, ok = r.ResolveWithContext("enter", ScopeChat, ActionContext{OverlayActive: true})
	if !ok || action != ActionOverlayAccept {
		t.Fatalf("expected %s with overlay, got %s (ok=%v)", ActionOverlayAccept, action, ok)
	}
}

func TestResolveWithContext_OverlayEscDismiss(t *testing.T) {
	r := DefaultRegistry()

	// With overlay: esc → overlay.dismiss
	action, ok := r.ResolveWithContext("esc", ScopeChat, ActionContext{OverlayActive: true})
	if !ok || action != ActionOverlayDismiss {
		t.Fatalf("expected %s, got %s (ok=%v)", ActionOverlayDismiss, action, ok)
	}

	// Without overlay: esc → cancel (chat scope)
	action, ok = r.ResolveWithContext("esc", ScopeChat, ActionContext{})
	if !ok || action != ActionCancel {
		t.Fatalf("expected %s, got %s (ok=%v)", ActionCancel, action, ok)
	}
}

func TestResolveWithContext_FallbackToScopeAndGlobal(t *testing.T) {
	r := DefaultRegistry()

	// Overlay active but key not in overlay scope → falls through to chat scope
	action, ok := r.ResolveWithContext("ctrl+f", ScopeChat, ActionContext{OverlayActive: true})
	if !ok || action != ActionTextSearch {
		t.Fatalf("expected fallthrough to chat scope, got %s (ok=%v)", action, ok)
	}

	// Overlay active, key in global only → falls through to global
	action, ok = r.ResolveWithContext("ctrl+c", ScopeChat, ActionContext{OverlayActive: true})
	if !ok || action != ActionCancelOrQuit {
		t.Fatalf("expected fallthrough to global, got %s (ok=%v)", action, ok)
	}
}

func TestResolveWithContext_NoOverlay(t *testing.T) {
	r := DefaultRegistry()

	// Without overlay: behaves exactly like Resolve
	action, ok := r.ResolveWithContext("ctrl+n", ScopeChat, ActionContext{})
	if !ok || action != ActionNewSession {
		t.Fatalf("expected %s, got %s (ok=%v)", ActionNewSession, action, ok)
	}
}

func TestNewActions_Registered(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		scope  Scope
		key    string
		action Action
	}{
		{ScopeOverlay, "enter", ActionOverlayAccept},
		{ScopeOverlay, "esc", ActionOverlayDismiss},
		{ScopeOverlay, "tab", ActionOverlayInputToggle},
		{ScopeChat, "home", ActionScrollTop},
		{ScopeChat, "end", ActionScrollBottom},
		{ScopeChat, "ctrl+y", ActionCopyResponse},
		{ScopeChat, "ctrl+shift+y", ActionCopyCodeBlock},
	}

	for _, tt := range tests {
		action, ok := r.Resolve(tt.scope, tt.key)
		if !ok {
			t.Errorf("expected binding for %s/%s", tt.scope, tt.key)
			continue
		}
		if action != tt.action {
			t.Errorf("key %s in scope %s: expected %s, got %s", tt.key, tt.scope, tt.action, action)
		}
	}
}

func TestDefaultRegistry_Completeness_WithNewActions(t *testing.T) {
	r := DefaultRegistry()

	// Overlay bindings
	expectedOverlay := map[Action]string{
		ActionOverlayAccept:      "enter",
		ActionOverlayDismiss:     "esc",
		ActionOverlayInputToggle: "tab",
	}

	for action, key := range expectedOverlay {
		resolved, ok := r.Resolve(ScopeOverlay, key)
		if !ok {
			t.Errorf("expected overlay binding for key %q (action %s)", key, action)
			continue
		}
		if resolved != action {
			t.Errorf("key %q: expected action %s, got %s", key, action, resolved)
		}
	}

	// New chat bindings
	expectedNewChat := map[Action]string{
		ActionScrollTop:     "home",
		ActionScrollBottom:  "end",
		ActionCopyResponse:  "ctrl+y",
		ActionCopyCodeBlock: "ctrl+shift+y",
	}

	for action, key := range expectedNewChat {
		resolved, ok := r.Resolve(ScopeChat, key)
		if !ok {
			t.Errorf("expected chat binding for key %q (action %s)", key, action)
			continue
		}
		if resolved != action {
			t.Errorf("key %q: expected action %s, got %s", key, action, resolved)
		}
	}
}
