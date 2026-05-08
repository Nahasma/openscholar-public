package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTerminalSafeWidth(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{0, 0},
		{1, 0},
		{2, 1},
		{80, 79},
	}
	for _, tt := range tests {
		if got := TerminalSafeWidth(tt.width); got != tt.want {
			t.Fatalf("TerminalSafeWidth(%d) = %d, want %d", tt.width, got, tt.want)
		}
	}
}

func TestSafeLineBudgetWidth(t *testing.T) {
	budget := SafeLineBudget{TerminalWidth: 10, LeftIndent: 3}
	if got := budget.Width(); got != 6 {
		t.Fatalf("budget width = %d, want 6", got)
	}
	if got := (SafeLineBudget{TerminalWidth: 4, LeftIndent: 4}).Width(); got != 0 {
		t.Fatalf("over-indented budget = %d, want 0", got)
	}
}

func TestSafeRule(t *testing.T) {
	if got := SafeRule(10, 0, "─"); lipgloss.Width(got) != 9 {
		t.Fatalf("safe rule width = %d, want 9 (%q)", lipgloss.Width(got), got)
	}
	if got := SafeRule(10, 2, "─"); lipgloss.Width(got) != 7 {
		t.Fatalf("indented safe rule width = %d, want 7 (%q)", lipgloss.Width(got), got)
	}
	if got := SafeRule(1, 0, "─"); got != "" {
		t.Fatalf("width=1 safe rule = %q, want empty", got)
	}
}

func TestSafePadRight(t *testing.T) {
	got := SafePadRight("abc", SafeLineBudget{TerminalWidth: 6})
	if lipgloss.Width(got) != 5 {
		t.Fatalf("SafePadRight width = %d, want 5 (%q)", lipgloss.Width(got), got)
	}
	if !strings.HasSuffix(got, "  ") {
		t.Fatalf("SafePadRight should pad to safe width, got %q", got)
	}
}

func TestSafeTruncateLineWithBudget(t *testing.T) {
	got := SafeTruncateLineWithBudget("abcdef   ", SafeLineBudget{TerminalWidth: 5})
	if lipgloss.Width(got) > 4 {
		t.Fatalf("truncated width = %d, want <= 4 (%q)", lipgloss.Width(got), got)
	}
	if strings.HasSuffix(got, " ") {
		t.Fatalf("SafeTruncateLineWithBudget should trim trailing plain spaces, got %q", got)
	}
}
