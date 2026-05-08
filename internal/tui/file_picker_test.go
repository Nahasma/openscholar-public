package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/picker"
)

func TestFilePickerActivatesViaModelUpdateKeyMsg(t *testing.T) {
	m := newTestModel()
	m.composer.fileIndex = []picker.FileEntry{{RelPath: "README.md"}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	mm := updated.(*Model)

	if !mm.composer.filePicker.Active {
		t.Fatal("file picker should activate after typing @")
	}
	if len(mm.composer.filePicker.Items) == 0 {
		t.Fatal("file picker should have items")
	}
}

func TestFilePickerConsumesUpDownBeforeHistory(t *testing.T) {
	m := newTestModel()
	m.composer.filePicker.Active = true
	m.composer.filePicker.Items = []picker.FileEntry{{RelPath: "a.md"}, {RelPath: "b.md"}}
	m.composer.filePicker.Selected = 0
	m.composer.input.SetValue("@")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	mm := updated.(*Model)
	if mm.composer.filePicker.Selected != 1 {
		t.Fatalf("down should move picker selection, got %d", mm.composer.filePicker.Selected)
	}
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyUp})
	mm = updated.(*Model)
	if mm.composer.filePicker.Selected != 0 {
		t.Fatalf("up should move picker selection, got %d", mm.composer.filePicker.Selected)
	}
}

func TestFileIndexMsgRefreshesActivePickerWithSameQuery(t *testing.T) {
	m := newTestModel()
	m.composer.input.SetValue("@foo")
	m.composer.fileIndexRoot = "/repo"
	m.composer.fileIndexRefreshing = true
	m.composer.filePicker.UpdateQuery("foo", []picker.FileEntry{
		{RelPath: "foo-old.md"},
		{RelPath: "foo-keep.md"},
	})
	m.composer.filePicker.Selected = 1

	updated, _ := m.Update(fileIndexMsg{
		root: "/repo",
		entries: []picker.FileEntry{
			{RelPath: "foo-new.md"},
			{RelPath: "foo-keep.md"},
		},
	})
	mv := updated.(Model)
	mm := &mv

	if got := len(mm.composer.filePicker.Items); got != 2 {
		t.Fatalf("expected refreshed picker items, got %d", got)
	}
	paths := map[string]bool{}
	for _, item := range mm.composer.filePicker.Items {
		paths[item.RelPath] = true
	}
	if paths["foo-old.md"] || !paths["foo-new.md"] {
		t.Fatalf("picker did not refresh from fileIndexMsg, items = %#v", mm.composer.filePicker.Items)
	}
	if mm.composer.filePicker.Items[mm.composer.filePicker.Selected].RelPath != "foo-keep.md" {
		t.Fatalf("expected selection to preserve foo-keep.md, selected = %#v", mm.composer.filePicker.Items[mm.composer.filePicker.Selected])
	}
	if mm.composer.fileIndexRefreshing {
		t.Fatal("fileIndexMsg should not immediately queue another refresh")
	}
}
