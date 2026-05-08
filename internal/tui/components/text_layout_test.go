package components

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"你好", 4},       // 2 CJK × 2 = 4
		{"AB你好CD", 8},   // 2 + 4 + 2
		{"", 0},
	}
	for _, tt := range tests {
		got := displayWidth(tt.input)
		if got != tt.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestTruncateDisplay(t *testing.T) {
	tests := []struct {
		input    string
		maxWidth int
		wantOK   bool // result display width <= maxWidth
		wantUTF8 bool // result is valid UTF-8
	}{
		{"hello world", 5, true, true},
		{"普通人需要知道的AI", 10, true, true},
		{"AI是什么thing", 8, true, true},
		{"短", 10, true, true},
		{"hello", 0, true, true},
		{"", 5, true, true},
		{"你好世界", 3, true, true},
		{"abc", 3, true, true},
	}
	for _, tt := range tests {
		result := truncateDisplay(tt.input, tt.maxWidth)
		if tt.wantUTF8 && !utf8.ValidString(result) {
			t.Errorf("truncateDisplay(%q, %d) = %q: invalid UTF-8", tt.input, tt.maxWidth, result)
		}
		if tt.wantOK && displayWidth(result) > tt.maxWidth {
			t.Errorf("truncateDisplay(%q, %d) = %q: width %d > %d",
				tt.input, tt.maxWidth, result, displayWidth(result), tt.maxWidth)
		}
	}
}

func TestTruncateDisplay_Content(t *testing.T) {
	// No truncation needed
	if got := truncateDisplay("hi", 10); got != "hi" {
		t.Errorf("truncateDisplay('hi', 10) = %q, want 'hi'", got)
	}

	// Empty on zero width
	if got := truncateDisplay("hello", 0); got != "" {
		t.Errorf("truncateDisplay('hello', 0) = %q, want ''", got)
	}

	// Truncation appends …
	result := truncateDisplay("普通人需要知道的AI", 5)
	if displayWidth(result) > 5 {
		t.Errorf("width %d > 5 for %q", displayWidth(result), result)
	}
}

func TestTruncateDisplayStart(t *testing.T) {
	tests := []struct {
		input    string
		maxWidth int
	}{
		{"hello world", 5},
		{"普通人需要知道的AI", 10},
		{"你好世界测试数据", 6},
		{"short", 20},
		{"x", 0},
	}
	for _, tt := range tests {
		result := truncateDisplayStart(tt.input, tt.maxWidth)
		if !utf8.ValidString(result) {
			t.Errorf("truncateDisplayStart(%q, %d) = %q: invalid UTF-8", tt.input, tt.maxWidth, result)
		}
		if tt.maxWidth > 0 && displayWidth(result) > tt.maxWidth {
			t.Errorf("truncateDisplayStart(%q, %d) = %q: width %d > %d",
				tt.input, tt.maxWidth, result, displayWidth(result), tt.maxWidth)
		}
	}

	// Should preserve tail
	result := truncateDisplayStart("abcdefghij", 5)
	if result != "…ghij" {
		t.Errorf("truncateDisplayStart('abcdefghij', 5) = %q, want '…ghij'", result)
	}

	// No truncation needed
	if got := truncateDisplayStart("hi", 10); got != "hi" {
		t.Errorf("want 'hi', got %q", got)
	}
}

func TestTruncateDisplayNoTail(t *testing.T) {
	result := truncateDisplayNoTail("你好世界", 5)
	if !utf8.ValidString(result) {
		t.Errorf("invalid UTF-8: %q", result)
	}
	if displayWidth(result) > 5 {
		t.Errorf("width %d > 5 for %q", displayWidth(result), result)
	}
	// 你好 = 4, 世 would exceed 5+2=6, so should be "你好"
	if result != "你好" {
		t.Errorf("truncateDisplayNoTail('你好世界', 5) = %q, want '你好'", result)
	}
}

func TestTruncatePathMiddle(t *testing.T) {
	tests := []struct {
		path     string
		maxWidth int
	}{
		{"src/components/deeply/nested/folder/MyComponent.tsx", 30},
		{"/Users/test/Documents/中文路径/论文/file.go", 25},
		{"short.go", 20},
		{"no-slash", 5},
	}
	for _, tt := range tests {
		result := truncatePathMiddle(tt.path, tt.maxWidth)
		if !utf8.ValidString(result) {
			t.Errorf("truncatePathMiddle(%q, %d) = %q: invalid UTF-8", tt.path, tt.maxWidth, result)
		}
		w := displayWidth(result)
		if w > tt.maxWidth {
			t.Errorf("truncatePathMiddle(%q, %d) = %q: width %d > %d",
				tt.path, tt.maxWidth, result, w, tt.maxWidth)
		}
	}

	// No truncation
	if got := truncatePathMiddle("a/b.go", 20); got != "a/b.go" {
		t.Errorf("want 'a/b.go', got %q", got)
	}
}

func TestWrapDisplay(t *testing.T) {
	// CJK wrapping
	lines := wrapDisplay("你好世界测试", 5)
	for i, line := range lines {
		w := displayWidth(line)
		if w > 5 {
			t.Errorf("wrapDisplay line %d: %q width %d > 5", i, line, w)
		}
		if !utf8.ValidString(line) {
			t.Errorf("wrapDisplay line %d: %q invalid UTF-8", i, line)
		}
	}
	// 你好(4) 世界(4) 测试(4) — each pair fits in 5
	if len(lines) != 3 {
		t.Errorf("wrapDisplay('你好世界测试', 5) = %d lines, want 3", len(lines))
	}

	// No wrap needed
	lines = wrapDisplay("hi", 10)
	if len(lines) != 1 || lines[0] != "hi" {
		t.Errorf("wrapDisplay('hi', 10) = %v, want ['hi']", lines)
	}

	// ASCII
	lines = wrapDisplay("abcdefghij", 4)
	if len(lines) != 3 { // abcd efgh ij
		t.Errorf("wrapDisplay('abcdefghij', 4) = %d lines, want 3", len(lines))
	}

	// maxWidth smaller than single CJK char — should not produce empty lines
	lines = wrapDisplay("你", 1)
	if len(lines) != 1 {
		t.Errorf("wrapDisplay('你', 1) = %d lines, want 1, got %v", len(lines), lines)
	}
	for _, line := range lines {
		if line == "" {
			t.Errorf("wrapDisplay('你', 1) produced empty line")
		}
	}

	// Multiple wide chars with maxWidth=1 — each on its own line, no empty lines
	lines = wrapDisplay("你好", 1)
	if len(lines) != 2 {
		t.Errorf("wrapDisplay('你好', 1) = %d lines, want 2, got %v", len(lines), lines)
	}
	for i, line := range lines {
		if line == "" {
			t.Errorf("wrapDisplay('你好', 1) line %d is empty", i)
		}
	}
}

func TestWrapDisplayString(t *testing.T) {
	result := wrapDisplayString("你好世界", 5)
	if !utf8.ValidString(result) {
		t.Errorf("invalid UTF-8: %q", result)
	}
	// Should contain newline
	lines := len(result) - len(strings.ReplaceAll(result, "\n", ""))
	if lines < 1 {
		t.Errorf("wrapDisplayString('你好世界', 5) should contain newlines")
	}
}

func init() {
	// ensure strings import is used
	_ = strings.NewReader
}
