package components

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderMarkdown_PlainTextFastPath_NoInjectedBlankLines(t *testing.T) {
	content := "plain line one\nplain line two"
	rendered := RenderMarkdown(content, 80)
	if strings.Contains(rendered, "\n\n") {
		t.Fatalf("plain text should not inject extra blank lines: %q", rendered)
	}
}

func TestRenderMarkdown_MarkdownBlock_StillRendered(t *testing.T) {
	content := "- item one\n- item two"
	rendered := RenderMarkdown(content, 80)
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "item one") || !strings.Contains(plain, "item two") {
		t.Fatalf("markdown list content missing: %q", plain)
	}
}

func TestRenderMarkdown_InlineMarkdownBypassesPlainTextFastPath(t *testing.T) {
	rendered := RenderMarkdown("This has **bold** and `code`.", 80)
	plain := stripANSI(rendered)
	if strings.Contains(plain, "**bold**") || strings.Contains(plain, "`code`") {
		t.Fatalf("inline markdown should still be rendered by markdown renderer, got %q", plain)
	}
	if isPlainTextForFastPath("1. ordered item") {
		t.Fatalf("ordered lists should not use the plain-text fast path")
	}
	if isPlainTextForFastPath("[label](https://example.com)") {
		t.Fatalf("links should not use the plain-text fast path")
	}
	if isPlainTextForFastPath("### deeper heading") {
		t.Fatalf("deep headings should not use the plain-text fast path")
	}
	if isPlainTextForFastPath("This has *italic* text") {
		t.Fatalf("single-emphasis markdown should not use the plain-text fast path")
	}
}

func TestIsPlainTextForFastPath_ThematicBreaks(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "---", want: false},
		{line: "------", want: false},
		{line: "***", want: false},
		{line: "___", want: false},
		{line: "- - -", want: false},
		{line: "* * *", want: false},
		{line: "_ _ _", want: false},
		{line: "a---b", want: true},
		{line: "普通文本 - 不是分割线", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if got := isPlainTextForFastPath(tt.line); got != tt.want {
				t.Fatalf("isPlainTextForFastPath(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestRenderMarkdown_DiffCoTSnippetWidths(t *testing.T) {
	content := "## DiffCoT 阅读报告摘要\n\n------\n\n正文第一段。"
	for _, width := range []int{20, 40, 80, 120} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			out := RenderMarkdown(content, width)
			plain := stripANSI(out)
			if !strings.Contains(plain, "DiffCoT") || !strings.Contains(plain, "阅读报告摘要") {
				t.Fatalf("heading text missing, got %q", plain)
			}
			if strings.Contains(plain, "\n------\n") || strings.HasSuffix(plain, "------") || strings.HasPrefix(plain, "------") {
				t.Fatalf("thematic break should not remain raw text, got %q", plain)
			}
			for i, line := range strings.Split(out, "\n") {
				if lipgloss.Width(line) > width {
					t.Fatalf("line %d width exceeds %d: %q", i, width, stripANSI(line))
				}
			}
		})
	}
}

func TestRenderMarkdown_HeadingsDoNotLeakMarkdownPrefixes(t *testing.T) {
	content := "## H2 Title\n\n### H3 Title\n\n#### H4 Title\n\n##### H5 Title\n\n###### H6 Title"
	out := RenderMarkdown(content, 80)
	plain := stripANSI(out)
	re := regexp.MustCompile(`(?m)^#{2,6}\s`)
	if re.MatchString(plain) {
		t.Fatalf("markdown heading prefixes leaked in rendered output: %q", plain)
	}
	for _, want := range []string{"H2 Title", "H3 Title", "H4 Title", "H5 Title", "H6 Title"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing heading text %q in output: %q", want, plain)
		}
	}
}

func TestRenderMarkdown_HorizontalRuleUsesWidthAwareLine(t *testing.T) {
	content := "before\n\n---\n\nafter"
	for _, width := range []int{20, 40, 80} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			out := RenderMarkdown(content, width)
			plain := stripANSI(out)
			if strings.Contains(plain, "--------") {
				t.Fatalf("default glamour fixed dash rule leaked: %q", plain)
			}
			wantRule := strings.Repeat(FigHeavyLine, normalizeMarkdownRenderWidth(width))
			if !strings.Contains(plain, wantRule) {
				t.Fatalf("missing width-aware heavy rule %q in output: %q", wantRule, plain)
			}
		})
	}
}

func TestRenderMarkdown_HorizontalRuleWidthCacheIsolation(t *testing.T) {
	content := "before\n\n---\n\nafter"
	widths := []int{20, 80, 20}
	for i, width := range widths {
		out := RenderMarkdown(content, width)
		plain := stripANSI(out)
		wantRule := strings.Repeat(FigHeavyLine, normalizeMarkdownRenderWidth(width))
		if !strings.Contains(plain, wantRule) {
			t.Fatalf("step %d width %d missing expected rule %q in %q", i, width, wantRule, plain)
		}
	}
}

func TestRenderMarkdown_LaTeXPreservedAsSource(t *testing.T) {
	content := `Math $s_{k-m}^{\sigma}$ and $x_i + y_j$ and $a_{**k**}$.

\[
E_{t+1} = \alpha_i + \beta_j
\]

$$
a_b + c_d
$$`
	out := RenderMarkdown(content, 80)
	plain := stripANSI(out)
	for _, want := range []string{
		`$s_{k-m}^{\sigma}$`,
		`$x_i + y_j$`,
		`$a_{**k**}$`,
		`\[`,
		`E_{t+1} = \alpha_i + \beta_j`,
		`$$`,
		`a_b + c_d`,
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected preserved math %q in %q", want, plain)
		}
	}
}

func TestRenderMarkdown_CommonMathOperators(t *testing.T) {
	content := `Self-attention is \(O(n^2) \to O(n)\), and \(\frac{\alpha_i}{\beta_j} \leq 1\).`
	out := RenderMarkdown(content, 80)
	plain := stripANSI(out)
	for _, want := range []string{`\(`, `O(n^2) \to O(n)`, `\frac{\alpha_i}{\beta_j} \leq 1`} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected preserved math %q in %q", want, plain)
		}
	}
}

func TestRenderMarkdown_InlineDollarDoesNotCrossParagraphs(t *testing.T) {
	content := "Cost $5\n\n## Heading\n\nCost $6"
	out := RenderMarkdown(content, 80)
	plain := stripANSI(out)
	if strings.Contains(plain, "## Heading") {
		t.Fatalf("heading was hidden inside cross-paragraph dollar span: %q", plain)
	}
	for _, want := range []string{"Cost $5", "Heading", "Cost $6"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected %q preserved in %q", want, plain)
		}
	}
}

func TestRenderMarkdown_MathPlaceholderDoesNotReplaceUserText(t *testing.T) {
	content := "OpenScholarMathToken0End and $x_i$"
	out := RenderMarkdown(content, 80)
	plain := stripANSI(out)
	for _, want := range []string{"OpenScholarMathToken0End", "$x_i$"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected %q preserved in %q", want, plain)
		}
	}
}

func TestRenderMarkdown_InlineDollarIgnoresCodeSpans(t *testing.T) {
	content := "Use `$HOME` and `$PATH`, then math $x_i$."
	out := RenderMarkdown(content, 80)
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

func TestHasThematicBreakLine_CommonMarkBoundaries(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "simple_dash", line: "---", want: true},
		{name: "spaces_dash", line: "- - -", want: true},
		{name: "grouped_spaces_dash", line: "-- -- --", want: true},
		{name: "stars", line: "***", want: true},
		{name: "underscores", line: "___", want: true},
		{name: "three_leading_spaces_ok", line: "   ---", want: true},
		{name: "four_leading_spaces_code", line: "    ---", want: false},
		{name: "table_separator", line: "| --- | --- |", want: false},
		{name: "list_item", line: "- ---", want: false},
		{name: "inline_text", line: "a---b", want: false},
		{name: "mixed_markers", line: "- * -", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasThematicBreakLine(tt.line); got != tt.want {
				t.Fatalf("hasThematicBreakLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
