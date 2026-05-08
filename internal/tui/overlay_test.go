package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// mockOverlay is a minimal Overlay for testing.
type mockOverlay struct {
	id          string
	kind        OverlayKind
	blocks      bool
	updateFn    func(tea.Msg) (Overlay, *OverlayResult, tea.Cmd)
	viewContent string
}

func newMockBlocking(id string) *mockOverlay {
	return &mockOverlay{id: id, kind: OverlayBlocking, blocks: true}
}

func newMockNonBlocking(id string) *mockOverlay {
	return &mockOverlay{id: id, kind: OverlayNonBlocking, blocks: false}
}

func (m *mockOverlay) ID() string        { return m.id }
func (m *mockOverlay) Kind() OverlayKind { return m.kind }
func (m *mockOverlay) BlocksInput() bool { return m.blocks }
func (m *mockOverlay) View(width, height int) string {
	if m.viewContent != "" {
		return m.viewContent
	}
	return "mock:" + m.id
}
func (m *mockOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	if m.updateFn != nil {
		return m.updateFn(msg)
	}
	return m, nil, nil
}

func TestOverlayManager_PushAndTop(t *testing.T) {
	om := NewOverlayManager()
	if !om.IsEmpty() {
		t.Fatal("new manager should be empty")
	}
	if om.Top() != nil {
		t.Fatal("Top should be nil when empty")
	}

	o1 := newMockBlocking("perm")
	om.Push(o1)
	if om.IsEmpty() {
		t.Fatal("should not be empty after push")
	}
	if om.Top().ID() != "perm" {
		t.Fatalf("Top should be perm, got %s", om.Top().ID())
	}
	if om.Depth() != 1 {
		t.Fatalf("Depth should be 1, got %d", om.Depth())
	}
}

func TestOverlayManager_BlockingQueueing(t *testing.T) {
	om := NewOverlayManager()
	o1 := newMockBlocking("perm1")
	o2 := newMockBlocking("perm2")

	om.Push(o1)
	om.Push(o2) // should be queued

	if om.Depth() != 1 {
		t.Fatalf("Depth should be 1 (second is pending), got %d", om.Depth())
	}
	if om.PendingCount() != 1 {
		t.Fatalf("PendingCount should be 1, got %d", om.PendingCount())
	}
	if om.Top().ID() != "perm1" {
		t.Fatalf("Top should be perm1, got %s", om.Top().ID())
	}
}

func TestOverlayManager_PopPromotesPending(t *testing.T) {
	om := NewOverlayManager()
	o1 := newMockBlocking("perm1")
	o2 := newMockBlocking("perm2")

	om.Push(o1)
	om.Push(o2) // queued

	result := om.Pop()
	if result == nil {
		t.Fatal("Pop should return a result")
	}
	if om.Top().ID() != "perm2" {
		t.Fatalf("After pop, pending should be promoted; got %s", om.Top().ID())
	}
	if om.PendingCount() != 0 {
		t.Fatalf("PendingCount should be 0 after promotion, got %d", om.PendingCount())
	}
}

func TestOverlayManager_NonBlockingCoexists(t *testing.T) {
	om := NewOverlayManager()
	o1 := newMockBlocking("perm")
	o2 := newMockNonBlocking("help")

	om.Push(o1)
	om.Push(o2) // non-blocking should stack directly

	if om.Depth() != 2 {
		t.Fatalf("Depth should be 2, got %d", om.Depth())
	}
	if om.Top().ID() != "help" {
		t.Fatalf("Top should be help, got %s", om.Top().ID())
	}
}

func TestOverlayManager_HasBlocking(t *testing.T) {
	om := NewOverlayManager()
	if om.HasBlocking() {
		t.Fatal("empty manager should not have blocking")
	}

	om.Push(newMockNonBlocking("help"))
	if om.HasBlocking() {
		t.Fatal("non-blocking only should not report HasBlocking")
	}

	om.Push(newMockBlocking("perm"))
	if !om.HasBlocking() {
		t.Fatal("should have blocking after pushing blocking overlay")
	}
}

func TestOverlayManager_PopEmpty(t *testing.T) {
	om := NewOverlayManager()
	result := om.Pop()
	if result != nil {
		t.Fatal("Pop on empty should return nil")
	}
}

func TestOverlayManager_PopByID(t *testing.T) {
	om := NewOverlayManager()
	om.Push(newMockBlocking("perm"))
	om.Push(newMockNonBlocking("help"))

	result := om.PopByID("perm")
	if result == nil {
		t.Fatal("PopByID should return result for existing overlay")
	}
	if om.Depth() != 1 {
		t.Fatalf("Depth should be 1 after PopByID, got %d", om.Depth())
	}
	if om.Top().ID() != "help" {
		t.Fatalf("Top should be help after removing perm, got %s", om.Top().ID())
	}
}

func TestOverlayManager_PopByID_NotFound(t *testing.T) {
	om := NewOverlayManager()
	om.Push(newMockBlocking("perm"))

	result := om.PopByID("nonexistent")
	if result != nil {
		t.Fatal("PopByID should return nil for non-existing ID")
	}
	if om.Depth() != 1 {
		t.Fatalf("Depth should remain 1, got %d", om.Depth())
	}
}

func TestOverlayManager_PopByID_PromotesPending(t *testing.T) {
	om := NewOverlayManager()
	om.Push(newMockBlocking("perm1"))
	om.Push(newMockBlocking("perm2")) // queued

	om.PopByID("perm1")
	if om.Top().ID() != "perm2" {
		t.Fatalf("PopByID should promote pending; got %s", om.Top().ID())
	}
}

func TestOverlayManager_RouteKey_Consumed(t *testing.T) {
	om := NewOverlayManager()
	o := newMockBlocking("perm")
	o.updateFn = func(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
		return o, nil, nil // handle but don't finish
	}
	om.Push(o)

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, consumed := om.RouteKey(msg)
	if !consumed {
		t.Fatal("blocking overlay should consume keys")
	}
}

func TestOverlayManager_RouteKey_OverlayDone(t *testing.T) {
	om := NewOverlayManager()
	o := newMockBlocking("perm")
	o.updateFn = func(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
		return o, &OverlayResult{Action: "accept"}, nil
	}
	om.Push(o)

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, consumed := om.RouteKey(msg)
	if !consumed {
		t.Fatal("should be consumed when overlay finishes")
	}
	if !om.IsEmpty() {
		t.Fatal("overlay should be removed after returning result")
	}
}

func TestOverlayManager_RouteKey_Empty(t *testing.T) {
	om := NewOverlayManager()
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, consumed := om.RouteKey(msg)
	if consumed {
		t.Fatal("empty manager should not consume keys")
	}
}

func TestOverlayManager_FindByID(t *testing.T) {
	om := NewOverlayManager()
	om.Push(newMockBlocking("perm"))
	om.Push(newMockNonBlocking("help"))

	found := om.FindByID("perm")
	if found == nil || found.ID() != "perm" {
		t.Fatal("FindByID should find perm")
	}
	if om.FindByID("nonexistent") != nil {
		t.Fatal("FindByID should return nil for missing ID")
	}
}

func TestOverlayManager_ReplaceTop(t *testing.T) {
	om := NewOverlayManager()
	om.Push(newMockBlocking("old"))

	replacement := newMockBlocking("new")
	om.ReplaceTop(replacement)
	if om.Top().ID() != "new" {
		t.Fatalf("ReplaceTop should update top; got %s", om.Top().ID())
	}
}

func TestOverlayManager_RouteKey_NonBlockingPassThrough(t *testing.T) {
	om := NewOverlayManager()
	o := newMockNonBlocking("help")
	updateCalled := false
	o.updateFn = func(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
		updateCalled = true
		return o, nil, nil
	}
	om.Push(o)

	// Non-Esc key should NOT be consumed and should NOT call Update
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, consumed := om.RouteKey(msg)
	if consumed {
		t.Fatal("non-blocking overlay should not consume non-Esc keys")
	}
	if updateCalled {
		t.Fatal("Update should not be called for pass-through keys")
	}

	// Esc should be consumed and routed to Update
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	_, consumed = om.RouteKey(escMsg)
	if !consumed {
		t.Fatal("non-blocking overlay should consume Esc")
	}
	if !updateCalled {
		t.Fatal("Update should be called for Esc key")
	}
}

func TestOverlayManager_MultiplePopDrain(t *testing.T) {
	om := NewOverlayManager()
	om.Push(newMockBlocking("a"))
	om.Push(newMockBlocking("b")) // pending
	om.Push(newMockBlocking("c")) // pending

	if om.PendingCount() != 2 {
		t.Fatalf("expected 2 pending, got %d", om.PendingCount())
	}

	om.Pop() // removes a, promotes b
	if om.Top().ID() != "b" {
		t.Fatalf("expected b, got %s", om.Top().ID())
	}

	om.Pop() // removes b, promotes c
	if om.Top().ID() != "c" {
		t.Fatalf("expected c, got %s", om.Top().ID())
	}

	om.Pop() // removes c, empty
	if !om.IsEmpty() {
		t.Fatal("should be empty after draining all")
	}
}

func TestUpdate_WindowSizeMsgRoutesToOverlayStack(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.features.OverlayStack = true
	m.width = 80
	m.height = 24
	m.recalcLayout()

	overlay := newMockNonBlocking("help")
	calls := 0
	overlay.updateFn = func(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
		if _, ok := msg.(tea.WindowSizeMsg); ok {
			calls++
		}
		return overlay, nil, nil
	}
	m.overlays.Push(overlay)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated := next.(Model)
	if calls != 1 {
		t.Fatalf("overlay should receive one WindowSizeMsg, got %d", calls)
	}
	if updated.width != 100 || updated.height != 30 {
		t.Fatalf("model size not updated: got %dx%d", updated.width, updated.height)
	}
}
