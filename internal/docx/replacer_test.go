package docx

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildParaForReplace is like buildPara but returns the paragraph AND the
// parent document so we can call SetXML if needed.
func buildParaForReplace(texts ...string) *etree.Element {
	return buildPara(texts...)
}

// paraHasRPr checks that the first w:r in the paragraph has a w:rPr child.
func paraHasRPr(para *etree.Element) bool {
	for _, child := range para.ChildElements() {
		if child.Tag == "r" {
			if child.FindElement("rPr") != nil {
				return true
			}
		}
	}
	return false
}

// countRuns counts the number of w:r elements in a paragraph.
func countRuns(para *etree.Element) int {
	n := 0
	for _, child := range para.ChildElements() {
		if child.Tag == "r" {
			n++
		}
	}
	return n
}

// createDocxWithContent builds a minimal docx with the given document XML body
// content (the part between <w:body> and </w:body>).
func createDocxWithContent(t *testing.T, dir, name, bodyXML string) string {
	t.Helper()

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
` + bodyXML + `
  </w:body>
</w:document>`

	entries := []struct {
		name string
		data string
	}{
		{"[Content_Types].xml", minContentTypes},
		{"_rels/.rels", minRootRels},
		{"word/_rels/document.xml.rels", minDocumentRels},
		{"word/document.xml", docXML},
	}

	path := filepath.Join(dir, name)
	writeDOCX(t, path, entries, nil)
	return path
}

// openDocAndGetFirstPara opens word/document.xml and returns the first w:p element.
func openDocAndGetFirstPara(t *testing.T, pkg *Package) *etree.Element {
	t.Helper()
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML: %v", err)
	}
	root := doc.Root()
	if root == nil {
		t.Fatal("no root element")
	}
	body := root.FindElement("//body")
	if body == nil {
		t.Fatal("no w:body")
	}
	for _, child := range body.ChildElements() {
		if child.Tag == "p" {
			return child
		}
	}
	t.Fatal("no w:p found")
	return nil
}

// ---------------------------------------------------------------------------
// 1. TestReplaceSingleRun
// ---------------------------------------------------------------------------

func TestReplaceSingleRun(t *testing.T) {
	para := buildParaForReplace("{{title}}")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ms))
	}

	if err := Replace(para, ms[0], "My Title"); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := paragraphText(para)
	if got != "My Title" {
		t.Errorf("text after replace: want %q, got %q", "My Title", got)
	}
}

// ---------------------------------------------------------------------------
// 2. TestReplaceCrossRun
// ---------------------------------------------------------------------------

func TestReplaceCrossRun(t *testing.T) {
	para := buildParaForReplace("{{ti", "tle}}")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ms))
	}

	if err := Replace(para, ms[0], "Cross Result"); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := paragraphText(para)
	if got != "Cross Result" {
		t.Errorf("text after replace: want %q, got %q", "Cross Result", got)
	}
}

// ---------------------------------------------------------------------------
// 3. TestReplacePreservesStyle
// ---------------------------------------------------------------------------

func TestReplacePreservesStyle(t *testing.T) {
	para := buildParaWithRPr("{{styled}}")
	// Verify rPr was added.
	if !paraHasRPr(para) {
		t.Fatal("test setup: expected rPr on first run")
	}

	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ms))
	}

	if err := Replace(para, ms[0], "Styled Value"); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// After replace, the run carrying "Styled Value" should have an rPr.
	found := false
	for _, child := range para.ChildElements() {
		if child.Tag == "r" {
			txt := runText(child)
			if strings.Contains(txt, "Styled Value") {
				if child.FindElement("rPr") != nil {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("replacement run does not carry the original rPr")
	}
}

// ---------------------------------------------------------------------------
// 4. TestReplaceMultipleInParagraph
// ---------------------------------------------------------------------------

func TestReplaceMultipleInParagraph(t *testing.T) {
	para := buildParaForReplace("{{a}} and {{b}}")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(ms))
	}

	// Replace back-to-front.
	for i := len(ms) - 1; i >= 0; i-- {
		val := "VALUE_" + ms[i].Key
		if err := Replace(para, ms[i], val); err != nil {
			t.Fatalf("Replace[%d]: %v", i, err)
		}
	}

	got := paragraphText(para)
	if !strings.Contains(got, "VALUE_a") || !strings.Contains(got, "VALUE_b") {
		t.Errorf("text after replace: %q", got)
	}
}

// ---------------------------------------------------------------------------
// 5. TestReplaceWithEmptyString
// ---------------------------------------------------------------------------

func TestReplaceWithEmptyString(t *testing.T) {
	para := buildParaForReplace("prefix{{del}}suffix")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ms))
	}

	if err := Replace(para, ms[0], ""); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := paragraphText(para)
	if got != "prefixsuffix" {
		t.Errorf("text after replace: want %q, got %q", "prefixsuffix", got)
	}
}

// ---------------------------------------------------------------------------
// 6. TestReplaceWithLongString
// ---------------------------------------------------------------------------

func TestReplaceWithLongString(t *testing.T) {
	para := buildParaForReplace("{{x}}")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ms))
	}

	long := strings.Repeat("Long content repeating. ", 50)
	if err := Replace(para, ms[0], long); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := paragraphText(para)
	if got != long {
		t.Errorf("text length mismatch: want %d, got %d", len(long), len(got))
	}
}

// ---------------------------------------------------------------------------
// 7. TestReplaceWithChinese
// ---------------------------------------------------------------------------

func TestReplaceWithChinese(t *testing.T) {
	para := buildParaForReplace("{{title}}")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ms))
	}

	if err := Replace(para, ms[0], "论文标题"); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := paragraphText(para)
	if got != "论文标题" {
		t.Errorf("text after replace: want %q, got %q", "论文标题", got)
	}
}

// ---------------------------------------------------------------------------
// 8. TestReplaceAllBasic
// ---------------------------------------------------------------------------

func TestReplaceAllBasic(t *testing.T) {
	dir := t.TempDir()
	path := createDocxWithContent(t, dir, "basic.docx", `    <w:p>
      <w:r><w:t>{{greeting}}</w:t></w:r>
    </w:p>`)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	results, err := ReplaceAll(pkg, map[string]any{"greeting": "Hello, World!"})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(results), results)
	}
	if !results[0].Success {
		t.Errorf("result not success: %s", results[0].Error)
	}
	if results[0].NewValue != "Hello, World!" {
		t.Errorf("NewValue: want %q, got %q", "Hello, World!", results[0].NewValue)
	}
}

// ---------------------------------------------------------------------------
// 9. TestReplaceAllMultipleParagraphs
// ---------------------------------------------------------------------------

func TestReplaceAllMultipleParagraphs(t *testing.T) {
	dir := t.TempDir()
	bodyXML := `    <w:p><w:r><w:t>{{author}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{title}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>Static text</w:t></w:r></w:p>`

	path := createDocxWithContent(t, dir, "multi.docx", bodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	data := map[string]any{
		"author": "Jane Doe",
		"title":  "Research Paper",
	}
	results, err := ReplaceAll(pkg, data)
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}
	if successCount != 2 {
		t.Errorf("expected 2 successes, got %d: %+v", successCount, results)
	}
}

// ---------------------------------------------------------------------------
// 10. TestReplaceAllMissingKey
// ---------------------------------------------------------------------------

func TestReplaceAllMissingKey(t *testing.T) {
	dir := t.TempDir()
	path := createDocxWithContent(t, dir, "missing.docx", `    <w:p>
      <w:r><w:t>{{missing_key}}</w:t></w:r>
    </w:p>`)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	results, err := ReplaceAll(pkg, map[string]any{})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected failure for missing key, got success")
	}
	if results[0].Error == "" {
		t.Error("expected non-empty Error for missing key")
	}
}

// ---------------------------------------------------------------------------
// 11. TestReplaceAllPreservesUnmodifiedParts
// ---------------------------------------------------------------------------

func TestReplaceAllPreservesUnmodifiedParts(t *testing.T) {
	dir := t.TempDir()
	path := createDocxWithContent(t, dir, "preserve.docx", `    <w:p>
      <w:r><w:t>{{x}}</w:t></w:r>
    </w:p>`)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	// Record SHA256 of the relationships file (unmodified).
	beforeHashes := hashParts(pkg)

	_, err = ReplaceAll(pkg, map[string]any{"x": "replaced"})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	afterHashes := hashParts(pkg)

	for uri, before := range beforeHashes {
		if uri == "word/document.xml" {
			continue // this one changed
		}
		after, ok := afterHashes[uri]
		if !ok {
			t.Errorf("part %q disappeared after ReplaceAll", uri)
			continue
		}
		if before != after {
			t.Errorf("unmodified part %q SHA256 changed", uri)
		}
	}
}

// ---------------------------------------------------------------------------
// 12. TestReplaceRoundTrip
// ---------------------------------------------------------------------------

func TestReplaceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := createDocxWithContent(t, dir, "roundtrip.docx", `    <w:p>
      <w:r><w:t>{{content}}</w:t></w:r>
    </w:p>`)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	_, err = ReplaceAll(pkg, map[string]any{"content": "Replaced Content"})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	savePath := filepath.Join(dir, "roundtrip_out.docx")
	if err := pkg.Save(savePath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	pkg2, err := Open(savePath)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	defer pkg2.Close()

	doc2, err := pkg2.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML: %v", err)
	}

	// Collect all w:t text from the saved document (Replace may leave multiple runs).
	allWT := doc2.FindElements("//w:t")
	if len(allWT) == 0 {
		t.Fatal("no w:t found in saved docx")
	}
	var fullText strings.Builder
	for _, wt := range allWT {
		fullText.WriteString(wt.Text())
	}
	if !strings.Contains(fullText.String(), "Replaced Content") {
		t.Errorf("saved text: want to contain %q, got %q", "Replaced Content", fullText.String())
	}
}

// ---------------------------------------------------------------------------
// 13. TestReplaceSurroundingText
// ---------------------------------------------------------------------------

func TestReplaceSurroundingText(t *testing.T) {
	para := buildParaForReplace("prefix{{key}}suffix")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ms))
	}

	if err := Replace(para, ms[0], "VALUE"); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := paragraphText(para)
	if got != "prefixVALUEsuffix" {
		t.Errorf("text after replace: want %q, got %q", "prefixVALUEsuffix", got)
	}
}

// ---------------------------------------------------------------------------
// 14. TestReplaceAdjacentPlaceholders
// ---------------------------------------------------------------------------

func TestReplaceAdjacentPlaceholders(t *testing.T) {
	para := buildParaForReplace("{{a}}{{b}}")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(ms))
	}

	// Replace back to front to avoid offset drift.
	for i := len(ms) - 1; i >= 0; i-- {
		val := map[string]string{"a": "FIRST", "b": "SECOND"}[ms[i].Key]
		if err := Replace(para, ms[i], val); err != nil {
			t.Fatalf("Replace[%d]: %v", i, err)
		}
	}

	got := paragraphText(para)
	if !strings.Contains(got, "FIRST") || !strings.Contains(got, "SECOND") {
		t.Errorf("adjacent replace result: %q", got)
	}
}

// ---------------------------------------------------------------------------
// 15. TestReplaceChineseKey
// ---------------------------------------------------------------------------

func TestReplaceChineseKey(t *testing.T) {
	para := buildParaForReplace("{{标题}}")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ms))
	}

	if err := Replace(para, ms[0], "替换后的标题"); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := paragraphText(para)
	if got != "替换后的标题" {
		t.Errorf("text after replace: want %q, got %q", "替换后的标题", got)
	}
}

// ---------------------------------------------------------------------------
// 16. TestReplaceCrossRunSurroundingText
// ---------------------------------------------------------------------------

func TestReplaceCrossRunSurroundingText(t *testing.T) {
	// "before {{ti" + "tle}} after" — placeholder spans 2 runs.
	para := buildParaForReplace("before {{ti", "tle}} after")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d: %+v", len(ms), ms)
	}

	if err := Replace(para, ms[0], "RESULT"); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := paragraphText(para)
	if !strings.Contains(got, "before ") || !strings.Contains(got, "RESULT") || !strings.Contains(got, " after") {
		t.Errorf("text after replace: %q", got)
	}
}

// ---------------------------------------------------------------------------
// 17. TestReplaceAllWithTableCell
// ---------------------------------------------------------------------------

func TestReplaceAllWithTableCell(t *testing.T) {
	dir := t.TempDir()
	bodyXML := `    <w:tbl>
      <w:tr>
        <w:tc>
          <w:p><w:r><w:t>{{cell}}</w:t></w:r></w:p>
        </w:tc>
      </w:tr>
    </w:tbl>`

	path := createDocxWithContent(t, dir, "table.docx", bodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	results, err := ReplaceAll(pkg, map[string]any{"cell": "Cell Value"})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}
	if successCount != 1 {
		t.Errorf("expected 1 success (in table cell), got %d: %+v", successCount, results)
	}
}

// ---------------------------------------------------------------------------
// 18. TestReplace3RunPlaceholder
// ---------------------------------------------------------------------------

func TestReplace3RunPlaceholder(t *testing.T) {
	para := buildParaForReplace("{{", "ke", "y}}")
	ms := FindPlaceholders(para, DefaultSyntax())
	if len(ms) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ms))
	}

	if err := Replace(para, ms[0], "THREE"); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := paragraphText(para)
	if got != "THREE" {
		t.Errorf("text after replace: want %q, got %q", "THREE", got)
	}
}

// ---------------------------------------------------------------------------
// 19. TestReplaceAllReturnsResults
// ---------------------------------------------------------------------------

func TestReplaceAllReturnsResults(t *testing.T) {
	dir := t.TempDir()
	bodyXML := `    <w:p><w:r><w:t>{{a}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{b}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{c}}</w:t></w:r></w:p>`

	path := createDocxWithContent(t, dir, "results.docx", bodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	results, err := ReplaceAll(pkg, map[string]any{
		"a": 1,
		"b": true,
		"c": "hello",
	})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("unexpected failure: %+v", r)
		}
	}
}

// ---------------------------------------------------------------------------
// 20. TestReplaceNoMatchIsNoop
// ---------------------------------------------------------------------------

func TestReplaceNoMatchIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := createDocxWithContent(t, dir, "noop.docx", `    <w:p>
      <w:r><w:t>No placeholders here</w:t></w:r>
    </w:p>`)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	results, err := ReplaceAll(pkg, map[string]any{"x": "y"})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for no-match paragraph, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Ensure writeDOCX and minContentTypes are accessible (they are, same package).
// This dummy reference silences "unused import" for zip/bytes/os in this file.
// ---------------------------------------------------------------------------

var _ = zip.NewWriter
var _ = bytes.NewBuffer
var _ = os.TempDir
