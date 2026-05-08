package components

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var inputANSIRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripInputANSI(s string) string {
	return inputANSIRe.ReplaceAllString(s, "")
}

func runeColumn(line, needle string) int {
	idx := strings.Index(line, needle)
	if idx < 0 {
		return -1
	}
	return len([]rune(line[:idx]))
}

func firstRuneColumnBy(line string, match func(rune) bool) int {
	for col, r := range []rune(line) {
		if match(r) {
			return col
		}
	}
	return -1
}

func TestInputModel_MultilineRendersAllLinesAndAlignsContinuation(t *testing.T) {
	m := NewInputModel()
	m.SetWidth(100)
	m.SetValue("First line\nSecond line\nThird line")

	plain := stripInputANSI(m.View())
	if strings.Count(plain, FigPrompt) != 1 {
		t.Fatalf("prompt count = %d, want 1\n%s", strings.Count(plain, FigPrompt), plain)
	}

	lines := strings.Split(plain, "\n")
	var firstCol, secondCol, thirdCol = -1, -1, -1
	for _, line := range lines {
		if strings.Contains(line, "First line") {
			firstCol = runeColumn(line, "First line")
		}
		if strings.Contains(line, "Second line") {
			secondCol = runeColumn(line, "Second line")
		}
		if strings.Contains(line, "Third line") {
			thirdCol = runeColumn(line, "Third line")
		}
	}

	if firstCol < 0 || secondCol < 0 || thirdCol < 0 {
		t.Fatalf("missing multiline content in rendered input:\n%s", plain)
	}
	if secondCol != firstCol || thirdCol != firstCol {
		t.Fatalf("continuation alignment mismatch: first=%d second=%d third=%d\n%s", firstCol, secondCol, thirdCol, plain)
	}
}

func TestInputModel_SoftWrapContinuationUsesBlankPrompt(t *testing.T) {
	m := NewInputModel()
	m.SetWidth(24)
	m.SetValue("ABCDEFGHIJKLMNOPQRSTUVWXYZ")

	plain := stripInputANSI(m.View())
	if strings.Count(plain, FigPrompt) != 1 {
		t.Fatalf("prompt count = %d, want 1\n%s", strings.Count(plain, FigPrompt), plain)
	}

	lines := strings.Split(plain, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected wrapped content lines, got:\n%s", plain)
	}

	first := lines[1]
	second := lines[2]
	if strings.TrimSpace(second) == "" {
		t.Fatalf("expected soft-wrapped continuation line, got empty line:\n%s", plain)
	}
	firstCol := firstRuneColumnBy(first, func(r rune) bool { return r >= 'A' && r <= 'Z' })
	secondCol := firstRuneColumnBy(second, func(r rune) bool { return r >= 'A' && r <= 'Z' })
	if firstCol < 0 || secondCol < 0 {
		t.Fatalf("expected alphabetic content on wrapped lines:\n%s", plain)
	}
	if secondCol != firstCol {
		t.Fatalf("soft-wrap continuation not aligned: first=%d second=%d\n%s", firstCol, secondCol, plain)
	}
}

func TestInputModel_WordWrapAndExactWidthHeightsMatchTextarea(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantHeight int
	}{
		{
			name:       "word_wrap_spaces",
			value:      "123456789 abcdefghi XYZXYZXYZ",
			wantHeight: 3,
		},
		{
			name:       "exact_width_cursor_row",
			value:      "ABCDEFGHIJKLMNOPQR",
			wantHeight: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewInputModel()
			m.SetWidth(24)
			m.SetValue(tc.value)

			if got := m.Textarea.Height(); got != tc.wantHeight {
				t.Fatalf("textarea height = %d, want %d\n%s", got, tc.wantHeight, stripInputANSI(m.View()))
			}
			if got := m.Height(); got != tc.wantHeight+2 {
				t.Fatalf("input height = %d, want %d", got, tc.wantHeight+2)
			}
		})
	}
}

func TestInputModel_UpdateKeepsWordWrappedRowsVisible(t *testing.T) {
	m := NewInputModel()
	m.SetWidth(24)

	for _, r := range "123456789 abcdefghi XYZXYZXYZ" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	plain := stripInputANSI(m.View())
	for _, token := range []string{"123456789", "abcdefghi", "XYZXYZXYZ"} {
		if !strings.Contains(plain, token) {
			t.Fatalf("typed word-wrapped input missing %q:\n%s", token, plain)
		}
	}
	if strings.Count(plain, FigPrompt) != 1 {
		t.Fatalf("prompt count = %d, want 1\n%s", strings.Count(plain, FigPrompt), plain)
	}
	if got := m.Textarea.Height(); got != 3 {
		t.Fatalf("textarea height after Update = %d, want 3\n%s", got, plain)
	}
}
