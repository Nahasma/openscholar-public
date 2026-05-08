package docx

import (
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// buildPara constructs a w:p element with one w:r / w:t per text string.
func buildPara(texts ...string) *etree.Element {
	p := etree.NewElement("p")
	p.Space = "w"
	for _, text := range texts {
		r := p.CreateElement("r")
		r.Space = "w"
		t := r.CreateElement("t")
		t.Space = "w"
		t.SetText(text)
	}
	return p
}

// buildParaWithRPr constructs a w:p where the first run has a w:rPr child
// (bold, italic) before its w:t.
func buildParaWithRPr(texts ...string) *etree.Element {
	p := etree.NewElement("p")
	p.Space = "w"
	for i, text := range texts {
		r := p.CreateElement("r")
		r.Space = "w"
		if i == 0 {
			rpr := r.CreateElement("rPr")
			rpr.Space = "w"
			b := rpr.CreateElement("b")
			b.Space = "w"
			it := rpr.CreateElement("i")
			it.Space = "w"
			_ = it
		}
		t := r.CreateElement("t")
		t.Space = "w"
		t.SetText(text)
	}
	return p
}

func syn() PlaceholderSyntax { return DefaultSyntax() }

// assertMatchCount fails the test when the number of matches != want.
func assertMatchCount(t *testing.T, matches []PlaceholderMatch, want int) {
	t.Helper()
	if len(matches) != want {
		t.Fatalf("expected %d match(es), got %d: %+v", want, len(matches), matches)
	}
}

// assertMatch verifies basic properties of a single match.
func assertMatch(t *testing.T, m PlaceholderMatch, fullText, key string, pt PlaceholderType) {
	t.Helper()
	if m.FullText != fullText {
		t.Errorf("FullText: want %q, got %q", fullText, m.FullText)
	}
	if m.Key != key {
		t.Errorf("Key: want %q, got %q", key, m.Key)
	}
	if m.Type != pt {
		t.Errorf("Type: want %v, got %v", pt, m.Type)
	}
}

// ---------------------------------------------------------------------------
// A. Single-run placeholders
// ---------------------------------------------------------------------------

func TestFindSingleRunVariable(t *testing.T) {
	p := buildPara("{{title}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{title}}", "title", PTVariable)
	if ms[0].StartRun != 0 || ms[0].EndRun != 0 {
		t.Errorf("run span: want 0-0, got %d-%d", ms[0].StartRun, ms[0].EndRun)
	}
	if ms[0].StartOffset != 0 {
		t.Errorf("StartOffset: want 0, got %d", ms[0].StartOffset)
	}
}

func TestFindSingleRunLoopStart(t *testing.T) {
	p := buildPara("{{#items}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{#items}}", "items", PTLoopStart)
}

func TestFindSingleRunLoopEnd(t *testing.T) {
	p := buildPara("{{/items}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{/items}}", "items", PTLoopEnd)
}

func TestFindSingleRunCondStart(t *testing.T) {
	p := buildPara("{{?visible}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{?visible}}", "visible", PTCondStart)
}

func TestFindSingleRunImage(t *testing.T) {
	p := buildPara("{{img:logo}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{img:logo}}", "logo", PTImage)
}

func TestFindSingleRunProtected(t *testing.T) {
	p := buildPara("{{!frozen}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{!frozen}}", "frozen", PTProtected)
}

func TestFindSingleRunExpression(t *testing.T) {
	p := buildPara("{{title | upper}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{title | upper}}", "title | upper", PTExpression)
}

func TestFindSingleRunTwoPlaceholders(t *testing.T) {
	p := buildPara("{{a}}{{b}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 2)
	assertMatch(t, ms[0], "{{a}}", "a", PTVariable)
	assertMatch(t, ms[1], "{{b}}", "b", PTVariable)
}

func TestFindSingleRunNoMatch(t *testing.T) {
	p := buildPara("no placeholders here")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 0)
}

func TestFindEmptyParagraph(t *testing.T) {
	p := buildPara()
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 0)
}

func TestFindSingleRunChineseKey(t *testing.T) {
	p := buildPara("{{中文变量}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{中文变量}}", "中文变量", PTVariable)
}

// ---------------------------------------------------------------------------
// B. Cross 2-run placeholders
// ---------------------------------------------------------------------------

func TestFindCross2RunOpenDelim(t *testing.T) {
	p := buildPara("{{", "title}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{title}}", "title", PTVariable)
	if ms[0].StartRun != 0 || ms[0].EndRun != 1 {
		t.Errorf("run span: want 0-1, got %d-%d", ms[0].StartRun, ms[0].EndRun)
	}
}

func TestFindCross2RunCloseDelim(t *testing.T) {
	p := buildPara("{{title", "}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{title}}", "title", PTVariable)
	if ms[0].StartRun != 0 || ms[0].EndRun != 1 {
		t.Errorf("run span: want 0-1, got %d-%d", ms[0].StartRun, ms[0].EndRun)
	}
}

func TestFindCross2RunKeyMiddle(t *testing.T) {
	p := buildPara("{{ti", "tle}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{title}}", "title", PTVariable)
}

func TestFindCross2RunWithSurroundingText(t *testing.T) {
	p := buildPara("prefix{{", "title}}suffix")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{title}}", "title", PTVariable)
}

func TestFindCross2RunLoopStart(t *testing.T) {
	p := buildPara("{{#it", "ems}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{#items}}", "items", PTLoopStart)
}

func TestFindCross2RunImage(t *testing.T) {
	p := buildPara("{{img:", "logo}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{img:logo}}", "logo", PTImage)
}

func TestFindCross2RunTwoPlaceholders(t *testing.T) {
	// First placeholder spans runs 0-1, second spans runs 2-3.
	p := buildPara("{{a", "}}", "{{b", "}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 2)
	assertMatch(t, ms[0], "{{a}}", "a", PTVariable)
	assertMatch(t, ms[1], "{{b}}", "b", PTVariable)
}

func TestFindCross2RunWithTextBetween(t *testing.T) {
	p := buildPara("before {{", "key}} after")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{key}}", "key", PTVariable)
}

func TestFindCross2RunExpression(t *testing.T) {
	p := buildPara("{{name ", "| lower}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{name | lower}}", "name | lower", PTExpression)
}

func TestFindCross2RunLoopEnd(t *testing.T) {
	p := buildPara("{{/ite", "ms}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{/items}}", "items", PTLoopEnd)
}

// ---------------------------------------------------------------------------
// C. Cross 3+ run placeholders
// ---------------------------------------------------------------------------

func TestFindCross4Run(t *testing.T) {
	p := buildPara("{{", "ti", "tle", "}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{title}}", "title", PTVariable)
	if ms[0].StartRun != 0 || ms[0].EndRun != 3 {
		t.Errorf("run span: want 0-3, got %d-%d", ms[0].StartRun, ms[0].EndRun)
	}
}

func TestFindCross3RunLoopStart(t *testing.T) {
	p := buildPara("{{", "#", "items}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{#items}}", "items", PTLoopStart)
}

func TestFindCross3RunMixedPlaceholders(t *testing.T) {
	// {{a}} in one run, then {{b}} split across 2 runs.
	p := buildPara("{{a}}", " text ", "{{b", "}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 2)
	assertMatch(t, ms[0], "{{a}}", "a", PTVariable)
	assertMatch(t, ms[1], "{{b}}", "b", PTVariable)
}

func TestFindCross3RunWithText(t *testing.T) {
	// Normal text interleaved.
	p := buildPara("Hello ", "{{", "world}}", " bye")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{world}}", "world", PTVariable)
}

func TestFindCross5Run(t *testing.T) {
	p := buildPara("{", "{", "ke", "y", "}}")
	ms := FindPlaceholders(p, syn())
	// "{" alone won't make "{{" — first two chars together do.
	// Result depends on whether charmap joins correctly.
	// Concatenated: "{{key}}" → valid match.
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{key}}", "key", PTVariable)
}

func TestFindCross3RunMultipleMixed(t *testing.T) {
	p := buildPara("{{a}}", "{{b", "}} and {{c}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 3)
	assertMatch(t, ms[0], "{{a}}", "a", PTVariable)
	assertMatch(t, ms[1], "{{b}}", "b", PTVariable)
	assertMatch(t, ms[2], "{{c}}", "c", PTVariable)
}

func TestFindCross3RunProtected(t *testing.T) {
	p := buildPara("{{!", "frozen", "}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{!frozen}}", "frozen", PTProtected)
}

func TestFindCross3RunImageKey(t *testing.T) {
	p := buildPara("{{img:", "log", "o}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{img:logo}}", "logo", PTImage)
}

func TestFindCross3RunExpression(t *testing.T) {
	p := buildPara("{{name", " | ", "upper}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{name | upper}}", "name | upper", PTExpression)
}

// ---------------------------------------------------------------------------
// D. Edge cases
// ---------------------------------------------------------------------------

func TestFindEdgeSingleBrace(t *testing.T) {
	p := buildPara("{")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 0)
}

func TestFindEdgeEmptyKey(t *testing.T) {
	p := buildPara("{{}}")
	ms := FindPlaceholders(p, syn())
	// {{}} has an empty inner → classified as PTVariable with Key="".
	// Our regex matches {{.*?}} — empty inner is valid.
	assertMatchCount(t, ms, 1)
	if ms[0].Key != "" {
		t.Errorf("Key: want empty, got %q", ms[0].Key)
	}
}

func TestFindEdgeUnclosed(t *testing.T) {
	p := buildPara("{{key")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 0)
}

func TestFindEdgeNoOpen(t *testing.T) {
	p := buildPara("key}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 0)
}

func TestFindEdgeSpaceInsideKey(t *testing.T) {
	p := buildPara("{{ key }}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	if ms[0].Key != " key " {
		t.Errorf("Key: want %q, got %q", " key ", ms[0].Key)
	}
}

func TestFindEdgeTwoPlaceholdersWithChineseBetween(t *testing.T) {
	p := buildPara("{{a}}中间{{b}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 2)
	assertMatch(t, ms[0], "{{a}}", "a", PTVariable)
	assertMatch(t, ms[1], "{{b}}", "b", PTVariable)
}

func TestFindEdgeLongKey(t *testing.T) {
	longKey := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 100 chars
	p := buildPara("{{" + longKey + "}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	if ms[0].Key != longKey {
		t.Errorf("Key length: want %d, got %d", len(longKey), len(ms[0].Key))
	}
}

func TestFindEdgeEmptyRunText(t *testing.T) {
	// A run with empty text interspersed.
	p := buildPara("{{ti", "", "tle}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{title}}", "title", PTVariable)
}

func TestFindEdgeNestedInner(t *testing.T) {
	// "{{nested{{inner}}}}" — regex scans left-to-right and is non-greedy.
	// The first {{ is at position 0; the first }} found is at position 16
	// (closing {{inner}}), so the match is "{{nested{{inner}}" with
	// key "nested{{inner". There is no separate match for "{{inner}}".
	// This is the expected behavior: we do NOT recursively parse nested syntax.
	p := buildPara("{{nested{{inner}}}}")
	ms := FindPlaceholders(p, syn())
	// At least one match should be found.
	if len(ms) == 0 {
		t.Fatal("expected at least 1 match")
	}
	// The outer match "{{nested{{inner}}" should be captured (key contains "inner").
	found := false
	for _, m := range ms {
		if strings.Contains(m.Key, "inner") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a match whose key contains 'inner'; got %+v", ms)
	}
}

func TestFindEdgeOnlyDelimiters(t *testing.T) {
	p := buildPara("}}{{")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 0)
}

func TestFindEdgeMultipleRunsSomeEmpty(t *testing.T) {
	p := buildPara("", "{{key}}", "")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{key}}", "key", PTVariable)
}

// ---------------------------------------------------------------------------
// E. Chinese placeholders
// ---------------------------------------------------------------------------

func TestFindChineseKeySimple(t *testing.T) {
	p := buildPara("{{标题}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{标题}}", "标题", PTVariable)
}

func TestFindChineseKeyCrossRun(t *testing.T) {
	p := buildPara("{{", "标题}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{标题}}", "标题", PTVariable)
}

func TestFindChineseKeyWithContext(t *testing.T) {
	p := buildPara("这是{{名称}}的文本")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{名称}}", "名称", PTVariable)
}

func TestFindChineseLoopInSameRun(t *testing.T) {
	p := buildPara("{{#列表项}}内容{{/列表项}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 2)
	assertMatch(t, ms[0], "{{#列表项}}", "列表项", PTLoopStart)
	assertMatch(t, ms[1], "{{/列表项}}", "列表项", PTLoopEnd)
}

func TestFindChineseKeyMultiRunContext(t *testing.T) {
	// Chinese key split across 3 runs.
	p := buildPara("{{", "作者", "}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	assertMatch(t, ms[0], "{{作者}}", "作者", PTVariable)
}

// ---------------------------------------------------------------------------
// Additional offset correctness tests
// ---------------------------------------------------------------------------

func TestFindStartOffsetCorrectSingleRun(t *testing.T) {
	// "abc{{key}}def" — StartOffset should be 3 (after "abc"), EndOffset after "key}}".
	p := buildPara("abc{{key}}def")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	if ms[0].StartOffset != 3 {
		t.Errorf("StartOffset: want 3, got %d", ms[0].StartOffset)
	}
	// EndOffset = 3 + len("{{key}}") runes = 3 + 7 = 10
	if ms[0].EndOffset != 10 {
		t.Errorf("EndOffset: want 10, got %d", ms[0].EndOffset)
	}
}

func TestFindRunsSliceLength(t *testing.T) {
	p := buildPara("{{", "k", "e", "y", "}}")
	ms := FindPlaceholders(p, syn())
	assertMatchCount(t, ms, 1)
	if len(ms[0].Runs) != 5 {
		t.Errorf("Runs length: want 5, got %d", len(ms[0].Runs))
	}
}
