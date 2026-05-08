package components

import (
	"testing"
)

func TestBlockList_NewEmpty(t *testing.T) {
	bl := NewBlockList()
	if bl.Len() != 0 {
		t.Errorf("expected empty, got %d", bl.Len())
	}
	if bl.Get(0) != nil {
		t.Error("Get(0) on empty list should return nil")
	}
	if bl.GetByID("nonexistent") != nil {
		t.Error("GetByID on empty list should return nil")
	}
}

func TestBlockList_RebuildAll(t *testing.T) {
	bl := NewBlockList()
	blocks := []BlockVM{
		{ID: "msg1-0", MsgID: "msg1", Kind: BlockUser, Content: "hello"},
		{ID: "msg2-0", MsgID: "msg2", Kind: BlockAssistantMarkdown, Content: "world"},
		{ID: "msg2-1", MsgID: "msg2", Kind: BlockToolHeader, Content: "tool"},
	}
	bl.RebuildAll(blocks)

	if bl.Len() != 3 {
		t.Fatalf("expected 3 blocks, got %d", bl.Len())
	}
	if bl.Get(0).Content != "hello" {
		t.Errorf("block 0 content = %q", bl.Get(0).Content)
	}
	if bl.GetByID("msg2-1").Kind != BlockToolHeader {
		t.Errorf("expected BlockToolHeader, got %d", bl.GetByID("msg2-1").Kind)
	}

	indices := bl.BlocksForMsg("msg2")
	if len(indices) != 2 {
		t.Fatalf("expected 2 blocks for msg2, got %d", len(indices))
	}
	if indices[0] != 1 || indices[1] != 2 {
		t.Errorf("msg2 indices = %v, want [1,2]", indices)
	}
}

func TestBlockList_ReplaceMessage_Append(t *testing.T) {
	bl := NewBlockList()
	bl.RebuildAll([]BlockVM{
		{ID: "msg1-0", MsgID: "msg1", Kind: BlockUser, Content: "first"},
	})

	// Append new message (no existing blocks for msg2)
	bl.ReplaceMessage("msg2", []BlockVM{
		{ID: "msg2-0", MsgID: "msg2", Kind: BlockAssistantMarkdown, Content: "second"},
	})

	if bl.Len() != 2 {
		t.Fatalf("expected 2, got %d", bl.Len())
	}
	if bl.Get(1).Content != "second" {
		t.Errorf("block 1 content = %q", bl.Get(1).Content)
	}
}

func TestBlockList_ReplaceMessage_InPlace(t *testing.T) {
	bl := NewBlockList()
	bl.RebuildAll([]BlockVM{
		{ID: "msg1-0", MsgID: "msg1", Kind: BlockUser, Content: "user"},
		{ID: "msg2-0", MsgID: "msg2", Kind: BlockAssistantMarkdown, Content: "old-md"},
		{ID: "msg2-1", MsgID: "msg2", Kind: BlockToolHeader, Content: "old-tool"},
		{ID: "msg3-0", MsgID: "msg3", Kind: BlockUser, Content: "next"},
	})

	// Replace msg2 blocks with a single new block
	bl.ReplaceMessage("msg2", []BlockVM{
		{ID: "msg2-0", MsgID: "msg2", Kind: BlockAssistantMarkdown, Content: "new-md"},
	})

	if bl.Len() != 3 {
		t.Fatalf("expected 3, got %d", bl.Len())
	}
	if bl.Get(0).Content != "user" {
		t.Errorf("block 0 = %q", bl.Get(0).Content)
	}
	if bl.Get(1).Content != "new-md" {
		t.Errorf("block 1 = %q", bl.Get(1).Content)
	}
	if bl.Get(2).Content != "next" {
		t.Errorf("block 2 = %q", bl.Get(2).Content)
	}
	// byID should reflect new state
	if bl.GetByID("msg2-1") != nil {
		t.Error("old msg2-1 should be gone")
	}
	if bl.GetByID("msg3-0") == nil {
		t.Error("msg3-0 should still exist")
	}
}

func TestBlockList_IDUniqueness(t *testing.T) {
	bl := NewBlockList()
	bl.RebuildAll([]BlockVM{
		{ID: "a-0", MsgID: "a", Kind: BlockUser},
		{ID: "a-1", MsgID: "a", Kind: BlockToolHeader},
		{ID: "b-0", MsgID: "b", Kind: BlockUser},
	})

	seen := make(map[string]bool)
	for i := 0; i < bl.Len(); i++ {
		id := bl.Get(i).ID
		if seen[id] {
			t.Errorf("duplicate block ID: %s", id)
		}
		seen[id] = true
	}
}

func TestBlockVM_MarkDirty(t *testing.T) {
	b := BlockVM{
		Rendered: "cached",
		Height:   5,
		Dirty:    false,
	}
	b.MarkDirty()
	if !b.Dirty {
		t.Error("expected Dirty=true")
	}
	if b.Rendered != "" {
		t.Error("expected Rendered cleared")
	}
	if b.Height != 0 {
		t.Error("expected Height cleared")
	}
}

func TestMakeBlockID(t *testing.T) {
	id := MakeBlockID("msg-123", 2)
	if id != "msg-123-2" {
		t.Errorf("got %q", id)
	}
}

func TestBlockList_Get_OutOfRange(t *testing.T) {
	bl := NewBlockList()
	bl.RebuildAll([]BlockVM{{ID: "x-0", MsgID: "x"}})
	if bl.Get(-1) != nil {
		t.Error("Get(-1) should return nil")
	}
	if bl.Get(1) != nil {
		t.Error("Get(1) should return nil for len=1")
	}
}

func TestBlockList_BlocksForMsg_Empty(t *testing.T) {
	bl := NewBlockList()
	indices := bl.BlocksForMsg("nonexistent")
	if len(indices) != 0 {
		t.Errorf("expected empty, got %v", indices)
	}
}

func TestBoundaryRowsBetween(t *testing.T) {
	user := BlockVM{MsgID: "u1", Kind: BlockUser}
	asst := BlockVM{MsgID: "a1", Kind: BlockAssistantMarkdown}
	sys := BlockVM{MsgID: "s1", Kind: BlockSystem}
	cmdActivity := BlockVM{MsgID: "c1", Kind: BlockCommandActivity}
	sameMsg := BlockVM{MsgID: "u1", Kind: BlockAssistantMarkdown}
	hidden := BlockVM{MsgID: "h1", Kind: BlockThinking, WidthKey: 80, Height: 0, Rendered: ""}

	if got := BoundaryRowsBetween(user, asst); got != 1 {
		t.Fatalf("user->assistant boundary rows=%d, want 1", got)
	}
	if got := BoundaryRowsBetween(asst, user); got != 1 {
		t.Fatalf("assistant->user boundary rows=%d, want 1", got)
	}
	if got := BoundaryRowsBetween(user, sys); got != 1 {
		t.Fatalf("user->system boundary rows=%d, want 1", got)
	}
	if got := BoundaryRowsBetween(cmdActivity, user); got != 1 {
		t.Fatalf("command-activity->user boundary rows=%d, want 1", got)
	}
	if got := BoundaryRowsBetween(user, sameMsg); got != 0 {
		t.Fatalf("same-msg boundary rows=%d, want 0", got)
	}
	if got := BoundaryRowsBetween(user, hidden); got != 0 {
		t.Fatalf("hidden boundary rows=%d, want 0", got)
	}
}
