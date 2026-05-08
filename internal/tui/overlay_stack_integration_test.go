package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Additional mock helpers for integration tests
// ---------------------------------------------------------------------------

// newMockBlockingWithUpdate creates a blocking overlay with a custom update function.
func newMockBlockingWithUpdate(id string, fn func(tea.Msg) (Overlay, *OverlayResult, tea.Cmd)) *mockOverlay {
	o := newMockBlocking(id)
	o.updateFn = fn
	return o
}

// newMockNonBlockingWithUpdate creates a non-blocking overlay with a custom update function.
func newMockNonBlockingWithUpdate(id string, fn func(tea.Msg) (Overlay, *OverlayResult, tea.Cmd)) *mockOverlay {
	o := newMockNonBlocking(id)
	o.updateFn = fn
	return o
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

// TestOverlayStackIntegration_BlockingOverlayConsumesAllKeys verifies that a
// blocking overlay reports consumed=true for every key press.
func TestOverlayStackIntegration_BlockingOverlayConsumesAllKeys(t *testing.T) {
	om := NewOverlayManager()
	o := newMockBlocking("dialog")
	om.Push(o)

	keys := []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyEsc},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
		{Type: tea.KeyRunes, Runes: []rune{'z'}},
	}

	for _, k := range keys {
		_, consumed := om.RouteKey(k)
		if !consumed {
			t.Errorf("blocking overlay should consume key %v, but consumed=false", k)
		}
	}
}

// TestOverlayStackIntegration_NonBlockingOverlaySelectiveConsume verifies that
// a non-blocking overlay returns consumed=false (BlocksInput()=false) so the
// underlying chat layer can still receive input.
func TestOverlayStackIntegration_NonBlockingOverlaySelectiveConsume(t *testing.T) {
	om := NewOverlayManager()
	o := newMockNonBlocking("help")
	om.Push(o)

	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'j'}},
		{Type: tea.KeyRunes, Runes: []rune{'k'}},
		{Type: tea.KeySpace},
	}

	for _, k := range keys {
		_, consumed := om.RouteKey(k)
		// Non-blocking overlay has BlocksInput()=false, so consumed should be false
		// (the overlay is still updated but does not claim the key for the layer below)
		if consumed {
			t.Errorf("non-blocking overlay should NOT report consumed=true for key %v", k)
		}
	}
}

// TestOverlayStackIntegration_StackOrdering verifies that overlays are pushed
// and popped in LIFO order.
func TestOverlayStackIntegration_StackOrdering(t *testing.T) {
	om := NewOverlayManager()

	// Push non-blocking overlays — they stack directly regardless of blocking state
	ids := []string{"first", "second", "third"}
	for _, id := range ids {
		om.Push(newMockNonBlocking(id))
	}

	if om.Depth() != 3 {
		t.Fatalf("expected depth 3, got %d", om.Depth())
	}

	// Pop in reverse order
	expected := []string{"third", "second", "first"}
	for i, want := range expected {
		top := om.Top()
		if top == nil {
			t.Fatalf("step %d: Top is nil, expected %s", i, want)
		}
		if top.ID() != want {
			t.Errorf("step %d: expected top=%s, got %s", i, want, top.ID())
		}
		om.Pop()
	}

	if !om.IsEmpty() {
		t.Fatal("stack should be empty after popping all overlays")
	}
}

// TestOverlayStackIntegration_PendingQueuePromotion verifies that when the
// active blocking overlay is closed the first pending blocking overlay is
// promoted to the stack.
func TestOverlayStackIntegration_PendingQueuePromotion(t *testing.T) {
	om := NewOverlayManager()

	// First blocking overlay occupies the slot
	om.Push(newMockBlocking("b1"))
	// Second and third are queued
	om.Push(newMockBlocking("b2"))
	om.Push(newMockBlocking("b3"))

	if om.Depth() != 1 {
		t.Fatalf("expected depth 1 (two pending), got %d", om.Depth())
	}
	if om.PendingCount() != 2 {
		t.Fatalf("expected 2 pending, got %d", om.PendingCount())
	}

	// Pop b1 → b2 promoted
	om.Pop()
	if om.Top() == nil || om.Top().ID() != "b2" {
		t.Fatalf("expected b2 promoted, got %v", om.Top())
	}
	if om.PendingCount() != 1 {
		t.Fatalf("expected 1 pending after first promotion, got %d", om.PendingCount())
	}

	// Pop b2 → b3 promoted
	om.Pop()
	if om.Top() == nil || om.Top().ID() != "b3" {
		t.Fatalf("expected b3 promoted, got %v", om.Top())
	}
	if om.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after second promotion, got %d", om.PendingCount())
	}

	// Pop b3 → empty
	om.Pop()
	if !om.IsEmpty() {
		t.Fatal("stack should be empty after all pops")
	}
}

// TestOverlayStackIntegration_RouteKeyResultHandling verifies that when an
// overlay's Update() returns a non-nil result the overlay is automatically
// removed from the stack and the pending queue is promoted if applicable.
func TestOverlayStackIntegration_RouteKeyResultHandling(t *testing.T) {
	om := NewOverlayManager()

	// Create a blocking overlay that returns a result on the first key press
	var calls int
	o := newMockBlockingWithUpdate("confirm", func(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
		calls++
		return nil, &OverlayResult{Action: "accept", Data: "yes"}, nil
	})

	// Queue a second blocking overlay
	om.Push(o)
	pending := newMockBlocking("next")
	om.Push(pending) // goes to pending queue

	if om.PendingCount() != 1 {
		t.Fatalf("expected 1 pending before RouteKey, got %d", om.PendingCount())
	}

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, consumed := om.RouteKey(msg)

	if !consumed {
		t.Fatal("key should be consumed when overlay returns result")
	}
	if calls != 1 {
		t.Fatalf("updateFn should be called once, got %d", calls)
	}
	// After the result, the first overlay is gone and pending is promoted
	if om.IsEmpty() {
		t.Fatal("pending overlay should have been promoted")
	}
	if om.Top().ID() != "next" {
		t.Fatalf("expected promoted overlay 'next', got %s", om.Top().ID())
	}
}

// TestOverlayStackIntegration_MixedBlockingNonBlocking verifies correct
// behavior when blocking and non-blocking overlays coexist in the stack.
func TestOverlayStackIntegration_MixedBlockingNonBlocking(t *testing.T) {
	om := NewOverlayManager()

	blocking := newMockBlocking("confirm")
	nonBlocking1 := newMockNonBlocking("help")
	nonBlocking2 := newMockNonBlocking("status")

	// Push in mixed order
	om.Push(blocking)    // stack: [confirm]
	om.Push(nonBlocking1) // stack: [confirm, help]
	om.Push(nonBlocking2) // stack: [confirm, help, status]

	if om.Depth() != 3 {
		t.Fatalf("expected depth 3, got %d", om.Depth())
	}
	if om.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", om.PendingCount())
	}
	if !om.HasBlocking() {
		t.Fatal("should report HasBlocking with blocking in stack")
	}

	// Top should be non-blocking status
	if om.Top().ID() != "status" {
		t.Fatalf("expected top=status, got %s", om.Top().ID())
	}

	// Pop status
	om.Pop()
	if om.Top().ID() != "help" {
		t.Fatalf("expected top=help after pop, got %s", om.Top().ID())
	}

	// Pop help (non-blocking — no pending promotion happens)
	om.Pop()
	if om.Top().ID() != "confirm" {
		t.Fatalf("expected top=confirm, got %s", om.Top().ID())
	}

	// Pop blocking confirm
	om.Pop()
	if !om.IsEmpty() {
		t.Fatal("stack should be empty")
	}
}

// TestOverlayStackIntegration_FindByID verifies that FindByID locates an
// overlay anywhere in a deep stack.
func TestOverlayStackIntegration_FindByID(t *testing.T) {
	om := NewOverlayManager()

	layers := []string{"base", "mid", "top"}
	for _, id := range layers {
		om.Push(newMockNonBlocking(id))
	}

	for _, id := range layers {
		found := om.FindByID(id)
		if found == nil {
			t.Errorf("FindByID(%q) returned nil", id)
			continue
		}
		if found.ID() != id {
			t.Errorf("FindByID(%q) returned wrong overlay id=%q", id, found.ID())
		}
	}

	if om.FindByID("ghost") != nil {
		t.Error("FindByID should return nil for non-existent id")
	}
}

// TestOverlayStackIntegration_PopByIDPromotes verifies that PopByID for a
// blocking overlay triggers promotion of the pending queue.
func TestOverlayStackIntegration_PopByIDPromotes(t *testing.T) {
	om := NewOverlayManager()

	b1 := newMockBlocking("perm-a")
	b2 := newMockBlocking("perm-b")
	b3 := newMockBlocking("perm-c")

	om.Push(b1)
	om.Push(b2) // pending
	om.Push(b3) // pending

	if om.PendingCount() != 2 {
		t.Fatalf("expected 2 pending, got %d", om.PendingCount())
	}

	// Remove the active blocking overlay by ID
	result := om.PopByID("perm-a")
	if result == nil {
		t.Fatal("PopByID should return a result for existing overlay")
	}

	// perm-b should be promoted from pending
	if om.Top() == nil || om.Top().ID() != "perm-b" {
		t.Fatalf("expected perm-b promoted, got %v", om.Top())
	}
	if om.PendingCount() != 1 {
		t.Fatalf("expected 1 pending after promotion, got %d", om.PendingCount())
	}
}
