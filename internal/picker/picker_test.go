package picker

import "testing"

func TestMoveWraps(t *testing.T) {
	s := New(8)
	s.Items = []FileEntry{{RelPath: "a"}, {RelPath: "b"}, {RelPath: "c"}}
	s.Active = true
	s.Selected = 0
	s.MoveUp()
	if s.Selected != 2 {
		t.Fatalf("MoveUp wrap expected 2, got %d", s.Selected)
	}
	s.MoveDown()
	if s.Selected != 0 {
		t.Fatalf("MoveDown wrap expected 0, got %d", s.Selected)
	}
}

func TestUpdateQueryPreservesSelectionByPath(t *testing.T) {
	s := New(8)
	idx := []FileEntry{{RelPath: "alpha.md"}, {RelPath: "beta.md"}, {RelPath: "gamma.md"}}
	s.UpdateQuery("", idx)
	s.Selected = 1
	s.UpdateQuery("", idx)
	if s.Items[s.Selected].RelPath != "beta.md" {
		t.Fatalf("expected selection preserved on beta.md, got %q", s.Items[s.Selected].RelPath)
	}
}

func TestUpdateQueryFallsBackToZeroWhenSelectionMissing(t *testing.T) {
	s := New(8)
	s.UpdateQuery("", []FileEntry{{RelPath: "alpha.md"}, {RelPath: "beta.md"}})
	s.Selected = 1
	s.UpdateQuery("", []FileEntry{{RelPath: "alpha.md"}})
	if s.Selected != 0 {
		t.Fatalf("expected selection reset to 0, got %d", s.Selected)
	}
}
