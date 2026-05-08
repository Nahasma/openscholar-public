package tools

import (
	"strings"
	"testing"
)

func TestRenderNumberedLinesAppliesOffsetLimit(t *testing.T) {
	got := renderNumberedLines([]string{"a", "b", "c", "d"}, 2, 2, 2000)
	if !strings.Contains(got, "     2\tb") || !strings.Contains(got, "     3\tc") {
		t.Fatalf("expected numbered offset output, got %q", got)
	}
	if strings.Contains(got, "a") || strings.Contains(got, "d") {
		t.Fatalf("unexpected lines outside selected range: %q", got)
	}
}
