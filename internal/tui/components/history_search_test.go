package components

import (
	"testing"
)

func TestHistorySearch_Deduplication(t *testing.T) {
	m := NewHistorySearch()
	m.SetEntries([]string{"hello", "world", "hello", "test", "world"})

	if len(m.entries) != 3 {
		t.Fatalf("expected 3 deduplicated entries, got %d", len(m.entries))
	}
	// First occurrence wins (most recent)
	if m.entries[0] != "hello" || m.entries[1] != "world" || m.entries[2] != "test" {
		t.Errorf("unexpected order: %v", m.entries)
	}
}

func TestHistorySearch_EmptyEntries(t *testing.T) {
	m := NewHistorySearch()
	m.SetEntries([]string{})
	if len(m.entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(m.entries))
	}
}

func TestHistorySearch_SkipsBlankEntries(t *testing.T) {
	m := NewHistorySearch()
	m.SetEntries([]string{"", "  ", "hello", "\t"})
	if len(m.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.entries))
	}
}

func TestHistorySearch_Filter(t *testing.T) {
	m := NewHistorySearch()
	m.SetEntries([]string{"git status", "make build", "git log", "npm install"})
	m.SetWidth(80)
	m.Show()

	// Initially all entries match
	if len(m.matches) != 4 {
		t.Fatalf("expected 4 matches, got %d", len(m.matches))
	}

	// Simulate typing "git"
	m.input.SetValue("git")
	m.filterMatches()
	if len(m.matches) != 2 {
		t.Fatalf("expected 2 matches for 'git', got %d", len(m.matches))
	}
}

func TestHistorySearch_Selected(t *testing.T) {
	m := NewHistorySearch()
	m.SetEntries([]string{"first", "second", "third"})
	m.Show()

	selected, ok := m.Selected()
	if !ok {
		t.Fatal("expected selection")
	}
	if selected != "first" {
		t.Errorf("expected 'first', got %q", selected)
	}
}

func TestHistorySearch_Visibility(t *testing.T) {
	m := NewHistorySearch()
	if m.Visible() {
		t.Fatal("expected hidden initially")
	}
	m.Show()
	if !m.Visible() {
		t.Fatal("expected visible after Show")
	}
	m.Hide()
	if m.Visible() {
		t.Fatal("expected hidden after Hide")
	}
}

func TestHistorySearch_SetWidthClampsInputWidth(t *testing.T) {
	m := NewHistorySearch()
	m.SetWidth(2)
	if m.input.Width != 1 {
		t.Fatalf("input width = %d, want 1", m.input.Width)
	}
}
