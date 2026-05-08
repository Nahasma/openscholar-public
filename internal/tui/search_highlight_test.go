package tui

import (
	"strings"
	"testing"
)

func TestApplySearchHighlight_NoQuery(t *testing.T) {
	lines := []string{"hello world", "foo bar"}
	result := applySearchHighlight(lines, searchHighlightInfo{})
	if len(result) != len(lines) {
		t.Fatalf("expected %d lines, got %d", len(lines), len(result))
	}
	for i, line := range result {
		if line != lines[i] {
			t.Errorf("line %d: expected %q, got %q", i, lines[i], line)
		}
	}
}

func TestApplySearchHighlight_BasicMatch(t *testing.T) {
	lines := []string{"hello world", "foo bar", "hello again"}
	info := searchHighlightInfo{Query: "hello", CurrentLine: -1, CurrentCol: -1}
	result := applySearchHighlight(lines, info)

	// Lines with "hello" should be modified (contain highlight escape codes)
	if result[0] == lines[0] {
		t.Error("line 0 should be highlighted but was unchanged")
	}
	if result[1] != lines[1] {
		t.Error("line 1 should be unchanged")
	}
	if result[2] == lines[2] {
		t.Error("line 2 should be highlighted but was unchanged")
	}
}

func TestApplySearchHighlight_CaseInsensitive(t *testing.T) {
	lines := []string{"Hello World", "HELLO"}
	info := searchHighlightInfo{Query: "hello", CurrentLine: -1, CurrentCol: -1}
	result := applySearchHighlight(lines, info)

	if result[0] == lines[0] {
		t.Error("line 0 should be highlighted (case insensitive)")
	}
	if result[1] == lines[1] {
		t.Error("line 1 should be highlighted (case insensitive)")
	}
}

func TestApplySearchHighlight_MultipleMatchesInLine(t *testing.T) {
	lines := []string{"foo bar foo baz foo"}
	info := searchHighlightInfo{Query: "foo", CurrentLine: -1, CurrentCol: -1}
	result := applySearchHighlight(lines, info)

	if result[0] == lines[0] {
		t.Error("line should be highlighted")
	}
	// Count how many times the highlight style appears
	// Each match should produce a styled segment
	count := strings.Count(result[0], "foo")
	if count < 3 {
		t.Errorf("expected 3 'foo' matches in highlighted line, found %d occurrences", count)
	}
}

func TestApplySearchHighlight_NoMatch(t *testing.T) {
	lines := []string{"hello world", "foo bar"}
	info := searchHighlightInfo{Query: "xyz", CurrentLine: -1, CurrentCol: -1}
	result := applySearchHighlight(lines, info)

	for i, line := range result {
		if line != lines[i] {
			t.Errorf("line %d should be unchanged when no match", i)
		}
	}
}

func TestApplySearchHighlight_EmptyLines(t *testing.T) {
	lines := []string{"", "hello", ""}
	info := searchHighlightInfo{Query: "hello", CurrentLine: -1, CurrentCol: -1}
	result := applySearchHighlight(lines, info)

	if result[0] != "" {
		t.Error("empty line should remain empty")
	}
	if result[1] == lines[1] {
		t.Error("line with match should be highlighted")
	}
	if result[2] != "" {
		t.Error("empty line should remain empty")
	}
}

func TestRuneIndex_Basic(t *testing.T) {
	tests := []struct {
		name     string
		haystack string
		needle   string
		want     int
	}{
		{"found at start", "hello", "hel", 0},
		{"found in middle", "hello", "ell", 1},
		{"found at end", "hello", "llo", 2},
		{"not found", "hello", "xyz", -1},
		{"empty needle", "hello", "", 0},
		{"needle longer", "hi", "hello", -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runeIndex([]rune(tc.haystack), []rune(tc.needle))
			if got != tc.want {
				t.Errorf("runeIndex(%q, %q) = %d, want %d", tc.haystack, tc.needle, got, tc.want)
			}
		})
	}
}

func TestRuneOffsetToVisual_ASCII(t *testing.T) {
	runes := []rune("hello")
	tests := []struct {
		offset int
		want   int
	}{
		{0, 0},
		{1, 1},
		{5, 5},
	}
	for _, tc := range tests {
		got := runeOffsetToVisual(runes, tc.offset)
		if got != tc.want {
			t.Errorf("runeOffsetToVisual(offset=%d) = %d, want %d", tc.offset, got, tc.want)
		}
	}
}

func TestRuneOffsetToVisual_CJK(t *testing.T) {
	// CJK characters are 2 cells wide
	runes := []rune("你好world")
	// rune 0: 你 → visual 0-1
	// rune 1: 好 → visual 2-3
	// rune 2: w → visual 4
	tests := []struct {
		offset int
		want   int
	}{
		{0, 0},
		{1, 2}, // after 你
		{2, 4}, // after 好
		{3, 5}, // after w
	}
	for _, tc := range tests {
		got := runeOffsetToVisual(runes, tc.offset)
		if got != tc.want {
			t.Errorf("runeOffsetToVisual(CJK, offset=%d) = %d, want %d", tc.offset, got, tc.want)
		}
	}
}
