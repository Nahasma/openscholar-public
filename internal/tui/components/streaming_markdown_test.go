package components

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// stripANSI removes ANSI escape sequences from s.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func TestStreamingMarkdownRenderer_IncrementalRendering(t *testing.T) {
	r := NewStreamingMarkdownRenderer()

	// First call: single incomplete paragraph (no \n\n yet)
	content1 := "Hello world"
	out1 := r.Render(content1, 80)
	if out1 == "" {
		t.Error("expected non-empty output for first render")
	}

	// Add a complete paragraph and start a new one.
	content2 := "Hello world\n\nSecond paragraph in progress"
	out2 := r.Render(content2, 80)
	if !strings.Contains(stripANSI(out2), "Hello world") {
		t.Errorf("expected output to contain 'Hello world', got: %q", out2)
	}
	// First paragraph should now be cached (1 segment).
	if len(r.cachedSegments) != 1 {
		t.Errorf("expected 1 cached segment, got %d", len(r.cachedSegments))
	}

	// Add another complete paragraph.
	content3 := "Hello world\n\nSecond paragraph\n\nThird paragraph in progress"
	out3 := r.Render(content3, 80)
	_ = out3
	if len(r.cachedSegments) != 2 {
		t.Errorf("expected 2 cached segments, got %d", len(r.cachedSegments))
	}

	_ = out1
	_ = out2
}

func TestStreamingMarkdownRenderer_WidthChangeInvalidatesCache(t *testing.T) {
	r := NewStreamingMarkdownRenderer()

	content := "Para one\n\nPara two in progress"
	r.Render(content, 80)
	if len(r.cachedSegments) != 1 {
		t.Fatalf("expected 1 cached segment, got %d", len(r.cachedSegments))
	}

	// Change width — cache must be invalidated.
	r.Render(content, 60)
	// After width change the renderer rebuilds from scratch; cached segments are regenerated.
	// The important thing is that we do NOT get a stale width=80 cache.
	if r.width != 60 {
		t.Errorf("expected width 60, got %d", r.width)
	}
}

func TestStreamingMarkdownRenderer_Reset(t *testing.T) {
	r := NewStreamingMarkdownRenderer()
	r.Render("Para one\n\nPara two in progress", 80)
	if len(r.cachedSegments) == 0 {
		t.Fatal("expected at least one cached segment before reset")
	}
	r.Reset()
	if len(r.cachedSegments) != 0 {
		t.Errorf("expected 0 cached segments after reset, got %d", len(r.cachedSegments))
	}
	if r.cachedSource != "" {
		t.Errorf("expected empty cachedSource after reset, got %q", r.cachedSource)
	}
}

func TestPatchIncompleteMarkdown_CodeFence(t *testing.T) {
	// Odd number of fences should be closed.
	content := "```go\nfunc main() {"
	patched := patchIncompleteMarkdown(content)
	if !strings.HasSuffix(patched, "```") {
		t.Errorf("expected patched to end with ```, got: %q", patched)
	}

	// Even number of fences should not be changed.
	content2 := "```go\ncode\n```"
	patched2 := patchIncompleteMarkdown(content2)
	if strings.Count(patched2, "```") != strings.Count(content2, "```") {
		t.Errorf("even fence count should not be modified, got: %q", patched2)
	}
}

func TestPatchIncompleteMarkdown_TildeFence(t *testing.T) {
	content := "~~~go\nfunc main() {"
	patched := patchIncompleteMarkdown(content)
	if !strings.HasSuffix(patched, "~~~") {
		t.Errorf("expected unmatched tilde fence to be closed with ~~~, got: %q", patched)
	}
}

func TestPatchIncompleteMarkdown_VariableLengthFence(t *testing.T) {
	content := "````markdown\n```text\nstill inside outer fence"
	patched := patchIncompleteMarkdown(content)
	if !strings.HasSuffix(patched, "````") {
		t.Errorf("expected unmatched four-backtick fence to be closed with same marker, got: %q", patched)
	}
	if strings.Count(patched, "````") != 2 {
		t.Errorf("shorter inner backtick fence should not close outer fence, got: %q", patched)
	}
}

func TestPatchIncompleteMarkdown_IgnoresClosedVariableLengthFenceContent(t *testing.T) {
	content := "````markdown\n```text\n**not inline markdown\n`````"
	patched := patchIncompleteMarkdown(content)
	if patched != content {
		t.Errorf("closed variable-length fence content should not trigger inline patching, got: %q", patched)
	}
}

func TestPatchIncompleteMarkdown_Bold(t *testing.T) {
	// Odd number of ** pairs → append **
	content := "This is **bold text"
	patched := patchIncompleteMarkdown(content)
	if !strings.HasSuffix(patched, "**") {
		t.Errorf("expected patched to end with **, got: %q", patched)
	}

	// Even number of ** → no change
	content2 := "This is **bold** text"
	patched2 := patchIncompleteMarkdown(content2)
	if strings.Count(patched2, "**") != strings.Count(content2, "**") {
		t.Errorf("even ** count should not be modified, got: %q", patched2)
	}
}

func TestPatchIncompleteMarkdown_InlineCode(t *testing.T) {
	// Odd backtick (not part of fence) → append `
	content := "Call `function"
	patched := patchIncompleteMarkdown(content)
	if !strings.HasSuffix(patched, "`") {
		t.Errorf("expected patched to end with `, got: %q", patched)
	}

	// Even backticks → no change
	content2 := "Call `function` now"
	patched2 := patchIncompleteMarkdown(content2)
	if strings.Count(patched2, "`") != strings.Count(content2, "`") {
		t.Errorf("even backtick count should not be modified, got: %q", patched2)
	}
}

func TestStreamingMarkdownRenderer_CodeFenceWithBlankLine(t *testing.T) {
	r := NewStreamingMarkdownRenderer()

	// A code block with a blank line inside should NOT be split into separate cached segments.
	content := "Before text\n\n```go\nfunc main() {\n\n\tfmt.Println(\"hello\")\n}\n```\n\nAfter text"
	out := r.Render(content, 80)
	plain := stripANSI(out)

	// The code block should not be corrupted by premature caching.
	if !strings.Contains(plain, "func main()") {
		t.Errorf("expected code block content preserved, got: %q", plain)
	}
	if !strings.Contains(plain, "After text") {
		t.Errorf("expected 'After text' in output, got: %q", plain)
	}

	// "Before text" paragraph (before the code fence) should be cached.
	// The code fence paragraphs should NOT be cached as separate segments
	// because the fence is open across the \n\n boundary.
	// After the fence closes, "After text" makes cacheable count = 4
	// (Before text, ```go..., empty, ...}```)
	// Actually with fence tracking: only "Before text" is cacheable before the fence opens.
	if len(r.cachedSegments) < 1 {
		t.Errorf("expected at least 1 cached segment for 'Before text', got %d", len(r.cachedSegments))
	}
}

func TestStreamingMarkdownRenderer_MixedFenceMarkersDoNotCloseEachOther(t *testing.T) {
	r := NewStreamingMarkdownRenderer()
	content := "~~~text\ninside tilde fence\n```\nstill code\n~~~\n\nAfter text"
	out := r.Render(content, 80)
	plain := stripANSI(out)
	if !strings.Contains(plain, "still code") || !strings.Contains(plain, "After text") {
		t.Fatalf("expected mixed fence content preserved, got: %q", plain)
	}
	if len(r.cachedSegments) != 1 {
		t.Fatalf("expected only the closed fenced block to be cached before live tail, got %d", len(r.cachedSegments))
	}
}

func TestStreamingMarkdownRenderer_ShorterFenceDoesNotCloseOuterFence(t *testing.T) {
	r := NewStreamingMarkdownRenderer()
	content := "````markdown\n```text\nstill inside outer fence\n````\n\nAfter text"
	out := r.Render(content, 80)
	plain := stripANSI(out)
	if !strings.Contains(plain, "still inside outer fence") || !strings.Contains(plain, "After text") {
		t.Fatalf("expected variable-length fence content preserved, got: %q", plain)
	}
	if len(r.cachedSegments) != 1 {
		t.Fatalf("expected only the fully closed fenced block to be cached before live tail, got %d", len(r.cachedSegments))
	}
}

func TestTrackFenceMarkerState_VariableLengthFence(t *testing.T) {
	open := trackFenceMarkerState("````markdown\n```text", "")
	if open != "````" {
		t.Fatalf("shorter backtick fence should not close four-backtick fence, open=%q", open)
	}
	if closed := trackFenceMarkerState("````", open); closed != "" {
		t.Fatalf("matching four-backtick fence should close, open=%q", closed)
	}
}

func TestTrackFenceMarkerState_FenceBoundaries(t *testing.T) {
	if open := trackFenceMarkerState("    ```go", ""); open != "" {
		t.Fatalf("four-space indented fence should be code content, open=%q", open)
	}
	if open := trackFenceMarkerState("\t```go", ""); open != "" {
		t.Fatalf("tab-indented fence should be code content, open=%q", open)
	}
	open := trackFenceMarkerState("```go\n```text", "")
	if open != "```" {
		t.Fatalf("closing fence with trailing text should not close, open=%q", open)
	}
	if closed := trackFenceMarkerState("```", open); closed != "" {
		t.Fatalf("plain matching fence should close, open=%q", closed)
	}
}

func TestPatchIncompleteMarkdown_DoesNotCloseOnFenceTrailingText(t *testing.T) {
	content := "```go\ncode\n```text"
	patched := patchIncompleteMarkdown(content)
	if !strings.HasSuffix(patched, "\n```") {
		t.Fatalf("trailing text fence should not close open fence, got %q", patched)
	}
}

func TestStreamingMarkdownRenderer_ListBulletPreserved(t *testing.T) {
	r := NewStreamingMarkdownRenderer()
	content := "- item one\n- item two"
	out := r.Render(content, 80)
	plain := stripANSI(out)
	if !strings.Contains(plain, "- item one") {
		t.Errorf("expected list bullet preserved, got: %q", plain)
	}
	if !strings.Contains(plain, "- item two") {
		t.Errorf("expected list bullet preserved, got: %q", plain)
	}
}

func TestRenderSegmentLightweight_Heading(t *testing.T) {
	out := renderSegmentLightweight("# Hello", 80)
	// Should be non-empty and not crash.
	if out == "" {
		t.Error("expected non-empty output for heading")
	}
}

func TestRenderSegmentLightweight_List(t *testing.T) {
	out := renderSegmentLightweight("- item one\n- item two", 80)
	plain := stripANSI(out)
	if !strings.Contains(plain, "item one") || !strings.Contains(plain, "item two") {
		t.Errorf("expected list items in output, got: %q", out)
	}
}

func TestRenderSegmentLightweight_Bold(t *testing.T) {
	out := renderSegmentLightweight("This is **bold** text", 80)
	if !strings.Contains(stripANSI(out), "bold") {
		t.Errorf("expected 'bold' in output, got: %q", out)
	}
}

func TestRenderSegmentLightweight_InlineCode(t *testing.T) {
	out := renderSegmentLightweight("Call `myFunc()` now", 80)
	if !strings.Contains(stripANSI(out), "myFunc()") {
		t.Errorf("expected 'myFunc()' in output, got: %q", out)
	}
}

func TestRenderSegmentLightweight_BlockSyntaxCoverage(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		check func(t *testing.T, out string)
	}{
		{
			name: "heading_h6",
			in:   "###### Deep",
			check: func(t *testing.T, out string) {
				if strings.Contains(stripANSI(out), "######") {
					t.Fatalf("raw heading marker leaked: %q", out)
				}
			},
		},
		{
			name: "ordered_dot",
			in:   "1. item",
			check: func(t *testing.T, out string) {
				if !strings.Contains(stripANSI(out), "1. item") {
					t.Fatalf("ordered list missing: %q", out)
				}
			},
		},
		{
			name: "ordered_paren",
			in:   "1) item",
			check: func(t *testing.T, out string) {
				if !strings.Contains(stripANSI(out), "1) item") {
					t.Fatalf("ordered paren list missing: %q", out)
				}
			},
		},
		{
			name: "plus_bullet",
			in:   "+ item",
			check: func(t *testing.T, out string) {
				if !strings.Contains(stripANSI(out), "+ item") {
					t.Fatalf("plus bullet missing: %q", out)
				}
			},
		},
		{
			name: "task_list",
			in:   "- [ ] todo\n- [x] done",
			check: func(t *testing.T, out string) {
				plain := stripANSI(out)
				if !strings.Contains(plain, "[ ] todo") || !strings.Contains(plain, "[x] done") {
					t.Fatalf("task list unreadable: %q", out)
				}
			},
		},
		{
			name: "blockquote",
			in:   "> quote",
			check: func(t *testing.T, out string) {
				if !strings.Contains(stripANSI(out), "quote") {
					t.Fatalf("blockquote text missing: %q", out)
				}
			},
		},
		{
			name: "thematic_break",
			in:   "------",
			check: func(t *testing.T, out string) {
				plain := stripANSI(out)
				if strings.Contains(plain, "------") {
					t.Fatalf("raw hr marker leaked: %q", out)
				}
				if !strings.Contains(plain, FigHeavyLine) {
					t.Fatalf("expected visible hr line, got: %q", out)
				}
			},
		},
		{
			name: "tilde_fence",
			in:   "~~~go\nfmt.Println(1)\n~~~",
			check: func(t *testing.T, out string) {
				plain := stripANSI(out)
				if !strings.Contains(plain, "~~~go") || !strings.Contains(plain, "fmt.Println(1)") {
					t.Fatalf("tilde fence not rendered: %q", out)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderSegmentLightweight(tt.in, 40)
			tt.check(t, out)
		})
	}
}

func TestRenderSegmentLightweight_ThematicBreakIndentBoundaries(t *testing.T) {
	out := renderSegmentLightweight("   ---", 40)
	if plain := stripANSI(out); !strings.Contains(plain, FigHeavyLine) {
		t.Fatalf("three-space indented thematic break should render as rule, got %q", plain)
	}

	out = renderSegmentLightweight("    ---", 40)
	plain := stripANSI(out)
	if strings.Contains(plain, FigHeavyLine) || !strings.Contains(plain, "---") {
		t.Fatalf("four-space indented thematic break should remain code-like text, got %q", plain)
	}
}

func TestRenderSegmentLightweight_HorizontalRuleUsesNormalizedWidth(t *testing.T) {
	out := renderSegmentLightweight("---", 42)
	plain := stripANSI(out)
	if strings.Contains(plain, strings.Repeat(FigHeavyLine, 42)) {
		t.Fatalf("live HR used raw width instead of normalized width: %q", plain)
	}
	if want := strings.Repeat(FigHeavyLine, normalizeMarkdownRenderWidth(42)); !strings.Contains(plain, want) {
		t.Fatalf("live HR missing normalized rule %q in %q", want, plain)
	}
}

func TestStreamingMarkdownRenderer_LaTeXPreservedAsSource(t *testing.T) {
	content := `Formula $s_{k-m}^{\sigma}$ and $x_i + y_j$.

\[
E_{t+1} = \alpha_i + \beta_j
\]

Tail $a_{**k**}$`
	r := NewStreamingMarkdownRenderer()
	out := r.Render(content, 80)
	plain := stripANSI(out)
	for _, want := range []string{
		`$s_{k-m}^{\sigma}$`,
		`$x_i + y_j$`,
		`\[`,
		`E_{t+1} = \alpha_i + \beta_j`,
		`$a_{**k**}$`,
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected preserved math %q in %q", want, plain)
		}
	}
}

func TestStreamingMarkdownRenderer_InlineDollarIgnoresCodeSpans(t *testing.T) {
	content := "Use `$HOME` and `$PATH`, then math $x_i$."
	r := NewStreamingMarkdownRenderer()
	out := r.Render(content, 80)
	plain := stripANSI(out)
	for _, want := range []string{"$HOME", "$PATH", "$x_i$"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected %q preserved in %q", want, plain)
		}
	}
	if strings.Contains(plain, "`$HOME") || strings.Contains(plain, "$PATH`") {
		t.Fatalf("code span backticks leaked around shell variables: %q", plain)
	}
}

func TestStreamingMarkdownRenderer_DiffCoTSnippetWidths(t *testing.T) {
	content := "## DiffCoT 阅读报告摘要\n\n------\n\n正文第一段。"
	for _, width := range []int{20, 40, 80, 120} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			r := NewStreamingMarkdownRenderer()
			out := r.Render(content, width)
			plain := stripANSI(out)
			if !strings.Contains(plain, "DiffCoT") || !strings.Contains(plain, "阅读报告摘要") {
				t.Fatalf("heading text missing: %q", plain)
			}
			if strings.Contains(plain, "\n------\n") || strings.HasSuffix(plain, "------") || strings.HasPrefix(plain, "------") {
				t.Fatalf("raw thematic break marker leaked: %q", plain)
			}
			for i, line := range strings.Split(out, "\n") {
				if lipgloss.Width(line) > width {
					t.Fatalf("line %d width exceeds %d: %q", i, width, stripANSI(line))
				}
			}
		})
	}
}
