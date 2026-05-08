package components

import (
	"strings"
	"testing"
)

func TestParseUnifiedDiff_SingleHunk(t *testing.T) {
	diff := `--- a/main.go
+++ b/main.go
@@ -10,3 +10,4 @@
 context line
-removed line
+added line 1
+added line 2`

	lines := ParseUnifiedDiff(diff)

	// Find the context line
	var contextLine *DiffLine
	var removeLine *DiffLine
	var addLines []*DiffLine

	for i := range lines {
		switch lines[i].Kind {
		case "context":
			contextLine = &lines[i]
		case "remove":
			removeLine = &lines[i]
		case "add":
			addLines = append(addLines, &lines[i])
		}
	}

	if contextLine == nil {
		t.Fatal("expected context line")
	}
	if *contextLine.OldNo != 10 || *contextLine.NewNo != 10 {
		t.Errorf("context line: oldNo=%d, newNo=%d, want 10, 10", *contextLine.OldNo, *contextLine.NewNo)
	}

	if removeLine == nil {
		t.Fatal("expected remove line")
	}
	if *removeLine.OldNo != 11 {
		t.Errorf("remove line: oldNo=%d, want 11", *removeLine.OldNo)
	}
	if removeLine.NewNo != nil {
		t.Error("remove line should have nil newNo")
	}

	if len(addLines) != 2 {
		t.Fatalf("expected 2 add lines, got %d", len(addLines))
	}
	if *addLines[0].NewNo != 11 {
		t.Errorf("first add line: newNo=%d, want 11", *addLines[0].NewNo)
	}
	if *addLines[1].NewNo != 12 {
		t.Errorf("second add line: newNo=%d, want 12", *addLines[1].NewNo)
	}
}

func TestParseUnifiedDiff_MultiHunk(t *testing.T) {
	diff := `@@ -1,2 +1,2 @@
-old first
+new first
 same
@@ -20,2 +20,3 @@
 context
+inserted
 more context`

	lines := ParseUnifiedDiff(diff)

	// Verify second hunk starts at line 20
	var secondHunkContext *DiffLine
	for i := range lines {
		if lines[i].Kind == "context" && lines[i].OldNo != nil && *lines[i].OldNo == 20 {
			secondHunkContext = &lines[i]
			break
		}
	}

	if secondHunkContext == nil {
		t.Fatal("expected context line at old line 20")
	}
}

func TestParseUnifiedDiff_EmptyDiff(t *testing.T) {
	lines := ParseUnifiedDiff("")
	// Should have at least 1 line (empty string splits to 1 element)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
}

func TestParseDiffHunkHeader(t *testing.T) {
	tests := []struct {
		header         string
		expectedOld    int
		expectedNew    int
	}{
		{"@@ -10,5 +20,7 @@", 10, 20},
		{"@@ -1 +1 @@", 1, 1},
		{"@@ -100,3 +200,5 @@ func main()", 100, 200},
		{"invalid", 1, 1},
	}

	for _, tt := range tests {
		oldStart, newStart := parseDiffHunkHeader(tt.header)
		if oldStart != tt.expectedOld || newStart != tt.expectedNew {
			t.Errorf("parseDiffHunkHeader(%q) = (%d, %d), want (%d, %d)",
				tt.header, oldStart, newStart, tt.expectedOld, tt.expectedNew)
		}
	}
}

func TestRenderDiff_ContainsGutter(t *testing.T) {
	diff := `@@ -1,2 +1,3 @@
 context
-removed
+added1
+added2`

	result := RenderDiff(diff)

	// Should contain line numbers (the "│" separator)
	if !strings.Contains(result, "│") {
		t.Error("expected gutter separator │ in rendered diff")
	}

	// Should contain line number "1"
	if !strings.Contains(result, "1") {
		t.Error("expected line number 1 in rendered diff")
	}
}

func TestRenderDiff_PureAddition(t *testing.T) {
	diff := `@@ -5,0 +5,2 @@
+new line 1
+new line 2`

	lines := ParseUnifiedDiff(diff)

	addCount := 0
	for _, l := range lines {
		if l.Kind == "add" {
			addCount++
			if l.OldNo != nil {
				t.Error("added lines should have nil OldNo")
			}
			if l.NewNo == nil {
				t.Error("added lines should have non-nil NewNo")
			}
		}
	}
	if addCount != 2 {
		t.Errorf("expected 2 add lines, got %d", addCount)
	}
}

func TestRenderDiff_PureDeletion(t *testing.T) {
	diff := `@@ -10,2 +10,0 @@
-deleted line 1
-deleted line 2`

	lines := ParseUnifiedDiff(diff)

	removeCount := 0
	for _, l := range lines {
		if l.Kind == "remove" {
			removeCount++
			if l.OldNo == nil {
				t.Error("removed lines should have non-nil OldNo")
			}
			if l.NewNo != nil {
				t.Error("removed lines should have nil NewNo")
			}
		}
	}
	if removeCount != 2 {
		t.Errorf("expected 2 remove lines, got %d", removeCount)
	}
}

func TestRenderStructuredDiff_Full(t *testing.T) {
	diff := `@@ -1,2 +1,3 @@
 context
-removed
+added1
+added2`

	result := RenderStructuredDiff(diff, 100, false)
	if !strings.Contains(result, "│") {
		t.Error("full mode should contain gutter separator")
	}
}

func TestRenderStructuredDiff_Compact(t *testing.T) {
	diff := `@@ -1,2 +1,3 @@
 context
-removed
+added1
+added2`

	result := RenderStructuredDiff(diff, 100, true)
	if strings.Contains(result, "│") {
		t.Error("compact mode should not contain gutter separator")
	}
	if !strings.Contains(result, "+") {
		t.Error("compact mode should show added lines")
	}
}

func TestRenderCompactDiffLines_MaxLines(t *testing.T) {
	// Build a diff with 12 changed lines
	var sb strings.Builder
	sb.WriteString("@@ -1,12 +1,12 @@\n")
	for i := 0; i < 12; i++ {
		sb.WriteString("-old\n+new\n")
	}
	lines := ParseUnifiedDiff(sb.String())
	result := RenderCompactDiffLines(lines, 100)
	// Should mention remaining lines
	if !strings.Contains(result, "lines (ctrl+o to expand)") {
		t.Error("expected truncation hint for >8 changed lines")
	}
}

func TestRenderDiffLines_LineNumberContinuity(t *testing.T) {
	diff := `@@ -1,4 +1,4 @@
 line 1
-old line 2
+new line 2
 line 3
 line 4`

	lines := ParseUnifiedDiff(diff)

	// Check continuity: old line numbers should go 1, 2, 3, 4
	expectedOld := []int{1, 2, 3, 4}
	oldIdx := 0
	for _, l := range lines {
		if l.OldNo != nil {
			if oldIdx >= len(expectedOld) {
				t.Fatalf("too many old line numbers")
			}
			if *l.OldNo != expectedOld[oldIdx] {
				t.Errorf("old line number: got %d, want %d", *l.OldNo, expectedOld[oldIdx])
			}
			oldIdx++
		}
	}
}

// ---- Enhanced diff tests ----

func TestWordDiff_Basic(t *testing.T) {
	oldSegs, newSegs := wordDiff("-hello world foo", "+hello changed foo")

	// Both sides should have segments
	if len(oldSegs) == 0 {
		t.Fatal("expected oldSegs to be non-empty")
	}
	if len(newSegs) == 0 {
		t.Fatal("expected newSegs to be non-empty")
	}

	// Find at least one changed segment in old and new
	oldChanged := false
	for _, s := range oldSegs {
		if s.Changed {
			oldChanged = true
			break
		}
	}
	newChanged := false
	for _, s := range newSegs {
		if s.Changed {
			newChanged = true
			break
		}
	}
	if !oldChanged {
		t.Error("expected at least one changed segment in old line")
	}
	if !newChanged {
		t.Error("expected at least one changed segment in new line")
	}
}

func TestWordDiff_IdenticalLines(t *testing.T) {
	oldSegs, newSegs := wordDiff("-hello world", "+hello world")

	// Identical content — no segment should be Changed
	for _, s := range oldSegs {
		if s.Changed {
			t.Errorf("expected no changed segments for identical lines, got Changed=true for %q", s.Text)
		}
	}
	for _, s := range newSegs {
		if s.Changed {
			t.Errorf("expected no changed segments for identical lines, got Changed=true for %q", s.Text)
		}
	}
}

func TestWordDiff_LongLines(t *testing.T) {
	// Lines longer than 200 chars should fall back to whole-line marking
	long := strings.Repeat("x", 210)
	oldSegs, newSegs := wordDiff("-"+long, "+"+long+"y")

	if len(oldSegs) != 1 || !oldSegs[0].Changed {
		t.Error("expected single changed segment for long old line")
	}
	if len(newSegs) != 1 || !newSegs[0].Changed {
		t.Error("expected single changed segment for long new line")
	}
}

func TestFoldContext_ShortContext(t *testing.T) {
	// Build a diff with fewer than 8 context lines — should not fold
	diff := "@@ -1,5 +1,5 @@\n"
	for i := 0; i < 5; i++ {
		diff += " context\n"
	}
	diff += "-old\n+new\n"
	lines := ParseUnifiedDiff(diff)
	folded := foldContext(lines, 8)

	// No fold lines expected
	for _, l := range folded {
		if l.Kind == "fold" {
			t.Errorf("unexpected fold line for short context: %q", l.Text)
		}
	}
	// Output length should equal input length
	if len(folded) != len(lines) {
		t.Errorf("short context: expected %d lines, got %d", len(lines), len(folded))
	}
}

func TestFoldContext_LongContext(t *testing.T) {
	// Build a diff with 15 context lines — should fold the middle
	diff := "@@ -1,18 +1,18 @@\n-changed\n+replaced\n"
	for i := 0; i < 15; i++ {
		diff += " context\n"
	}
	diff += "-changed2\n+replaced2\n"
	lines := ParseUnifiedDiff(diff)
	folded := foldContext(lines, 8)

	hasFold := false
	for _, l := range folded {
		if l.Kind == "fold" {
			hasFold = true
			if !strings.Contains(l.Text, "unchanged lines") {
				t.Errorf("fold line text unexpected: %q", l.Text)
			}
		}
	}
	if !hasFold {
		t.Error("expected a fold line for 15 context lines with maxContext=8")
	}
	// Folded result should be shorter than original
	if len(folded) >= len(lines) {
		t.Errorf("folded output (%d) should be shorter than original (%d)", len(folded), len(lines))
	}
}

func TestFoldContext_MultipleSections(t *testing.T) {
	// Two long context runs separated by changes
	diff := "@@ -1,30 +1,30 @@\n"
	for i := 0; i < 12; i++ {
		diff += " ctx1\n"
	}
	diff += "-old\n+new\n"
	for i := 0; i < 12; i++ {
		diff += " ctx2\n"
	}
	lines := ParseUnifiedDiff(diff)
	folded := foldContext(lines, 8)

	foldCount := 0
	for _, l := range folded {
		if l.Kind == "fold" {
			foldCount++
		}
	}
	if foldCount < 2 {
		t.Errorf("expected at least 2 fold lines for two long context sections, got %d", foldCount)
	}
}

func TestRenderEnhancedDiff_Basic(t *testing.T) {
	diff := `@@ -1,3 +1,3 @@
 context line
-old value
+new value
 context line`

	result := RenderEnhancedDiff(diff, "", 120)
	if result == "" {
		t.Fatal("expected non-empty result from RenderEnhancedDiff")
	}
	// Should contain gutter separator
	if !strings.Contains(result, "│") {
		t.Error("expected gutter separator │ in enhanced diff output")
	}
}

func TestRenderEnhancedDiff_WithSyntax(t *testing.T) {
	diff := `@@ -1,2 +1,2 @@
-func hello() {}
+func hello() { return }`

	result := RenderEnhancedDiff(diff, "go", 120)
	if result == "" {
		t.Fatal("expected non-empty result from RenderEnhancedDiff with syntax")
	}
}

func TestRenderEnhancedDiff_WithFold(t *testing.T) {
	// Build a diff that will trigger folding (>8 context lines)
	diff := "@@ -1,20 +1,20 @@\n-old\n+new\n"
	for i := 0; i < 15; i++ {
		diff += " same line\n"
	}
	diff += "-old2\n+new2\n"

	result := RenderEnhancedDiff(diff, "", 120)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	// The fold indicator should appear in the output
	if !strings.Contains(result, "unchanged lines") {
		t.Error("expected fold indicator in enhanced diff with long context")
	}
}
