package docx

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test fixtures — build DOCX files with specific body content
// ---------------------------------------------------------------------------

// createEditorDocx builds a DOCX whose body contains the given raw XML paragraphs.
func createEditorDocx(t *testing.T, dir, name, bodyXML string) string {
	t.Helper()

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
` + bodyXML + `
  </w:body>
</w:document>`

	entries := []struct{ name, data string }{
		{"[Content_Types].xml", minContentTypes},
		{"_rels/.rels", minRootRels},
		{"word/_rels/document.xml.rels", minDocumentRels},
		{"word/document.xml", docXML},
	}

	path := filepath.Join(dir, name)
	writeDOCX(t, path, entries, nil)
	return path
}

// bodyXMLForEditor produces a body with multiple paragraphs useful for editor tests.
// It includes: 2 plain paragraphs + 1 heading + 2 more paragraphs (section body).
const editorBodyXML = `    <w:p w14:paraId="AAA00001" xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml">
      <w:r><w:t>First paragraph</w:t></w:r>
    </w:p>
    <w:p w14:paraId="AAA00002" xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml">
      <w:r><w:t>Second paragraph</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:pStyle w:val="Heading1"/></w:pPr>
      <w:r><w:t>Section One</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>Body paragraph A</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>Body paragraph B</w:t></w:r>
    </w:p>`

// bodyWithBookmark includes a paragraph that contains a bookmarkStart element.
const bookmarkBodyXML = `    <w:p>
      <w:bookmarkStart w:id="1" w:name="myBookmark"/>
      <w:r><w:t>Bookmarked paragraph</w:t></w:r>
      <w:bookmarkEnd w:id="1"/>
    </w:p>
    <w:p>
      <w:r><w:t>After bookmark</w:t></w:r>
    </w:p>`

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// allParaTexts returns the text content of every w:p element in body, in order.
func allParaTexts(t *testing.T, pkg *Package) []string {
	t.Helper()
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML: %v", err)
	}
	body := getBody(doc)
	if body == nil {
		t.Fatal("w:body not found")
	}
	var texts []string
	for _, child := range body.ChildElements() {
		if child.Tag == "p" {
			text, _ := RenderParagraph(child)
			texts = append(texts, text)
		}
	}
	return texts
}

// countParas returns the number of w:p elements directly under w:body.
func countParas(t *testing.T, pkg *Package) int {
	t.Helper()
	return len(allParaTexts(t, pkg))
}

// ---------------------------------------------------------------------------
// 1. TestApplyReplaceText
// ---------------------------------------------------------------------------

func TestApplyReplaceText(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "replace_text.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	ops := []EditOperation{
		{
			Type:    OpReplaceText,
			Target:  StableAnchor{ParaID: "AAA00001"},
			Content: "Replaced first",
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("Applied: want 1, got %d", res.Applied)
	}
	if res.Skipped != 0 {
		t.Errorf("Skipped: want 0, got %d (warnings: %v)", res.Skipped, res.Warnings)
	}

	texts := allParaTexts(t, pkg)
	if len(texts) == 0 || texts[0] != "Replaced first" {
		t.Errorf("first paragraph text: want %q, got %q", "Replaced first", texts[0])
	}
}

// ---------------------------------------------------------------------------
// 2. TestApplyInsertAfter
// ---------------------------------------------------------------------------

func TestApplyInsertAfter(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "insert_after.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	before := countParas(t, pkg)

	ops := []EditOperation{
		{
			Type:    OpInsertAfter,
			Target:  StableAnchor{ParaID: "AAA00001"},
			Content: "Inserted after first",
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("Applied: want 1, got %d", res.Applied)
	}

	after := countParas(t, pkg)
	if after != before+1 {
		t.Errorf("paragraph count: want %d, got %d", before+1, after)
	}

	texts := allParaTexts(t, pkg)
	// The inserted paragraph should be at index 1 (right after the first)
	if len(texts) < 2 || texts[1] != "Inserted after first" {
		t.Errorf("texts[1]: want %q, got %q", "Inserted after first", texts[1])
	}
}

// ---------------------------------------------------------------------------
// 3. TestApplyInsertBefore
// ---------------------------------------------------------------------------

func TestApplyInsertBefore(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "insert_before.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	before := countParas(t, pkg)

	ops := []EditOperation{
		{
			Type:    OpInsertBefore,
			Target:  StableAnchor{ParaID: "AAA00001"},
			Content: "Inserted before first",
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("Applied: want 1, got %d", res.Applied)
	}

	after := countParas(t, pkg)
	if after != before+1 {
		t.Errorf("paragraph count: want %d, got %d", before+1, after)
	}

	texts := allParaTexts(t, pkg)
	if len(texts) == 0 || texts[0] != "Inserted before first" {
		t.Errorf("texts[0]: want %q, got %q", "Inserted before first", texts[0])
	}
}

// ---------------------------------------------------------------------------
// 4. TestApplyDeleteParagraph
// ---------------------------------------------------------------------------

func TestApplyDeleteParagraph(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "delete.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	before := countParas(t, pkg)

	ops := []EditOperation{
		{
			Type:   OpDeleteParagraph,
			Target: StableAnchor{ParaID: "AAA00002"},
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("Applied: want 1, got %d", res.Applied)
	}

	after := countParas(t, pkg)
	if after != before-1 {
		t.Errorf("paragraph count: want %d, got %d", before-1, after)
	}

	// "Second paragraph" should no longer be present
	texts := allParaTexts(t, pkg)
	for _, txt := range texts {
		if txt == "Second paragraph" {
			t.Error("deleted paragraph still present in document")
		}
	}
}

// ---------------------------------------------------------------------------
// 5. TestApplyReplaceSection
// ---------------------------------------------------------------------------

func TestApplyReplaceSection(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "replace_section.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	// Target the Heading1 "Section One" via offset (index 2 among paragraphs)
	ops := []EditOperation{
		{
			Type:    OpReplaceSection,
			Target:  StableAnchor{Offset: 2},
			Content: "New section body content",
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("Applied: want 1, got %d (warnings: %v)", res.Applied, res.Warnings)
	}

	texts := allParaTexts(t, pkg)

	// "Body paragraph A" and "Body paragraph B" should be gone
	for _, txt := range texts {
		if txt == "Body paragraph A" || txt == "Body paragraph B" {
			t.Errorf("old section content still present: %q", txt)
		}
	}

	// New content should be present
	found := false
	for _, txt := range texts {
		if txt == "New section body content" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("new section content not found; paragraphs: %v", texts)
	}
}

// ---------------------------------------------------------------------------
// 6. TestApplyMultipleOps
// ---------------------------------------------------------------------------

func TestApplyMultipleOps(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "multi_ops.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	ops := []EditOperation{
		{
			Type:    OpReplaceText,
			Target:  StableAnchor{ParaID: "AAA00001"},
			Content: "Op1: replaced",
		},
		{
			Type:    OpInsertAfter,
			Target:  StableAnchor{ParaID: "AAA00002"},
			Content: "Op2: inserted",
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 2 {
		t.Errorf("Applied: want 2, got %d (warnings: %v)", res.Applied, res.Warnings)
	}

	texts := allParaTexts(t, pkg)

	// Check first paragraph was replaced
	if len(texts) == 0 || texts[0] != "Op1: replaced" {
		t.Errorf("texts[0]: want %q, got %q", "Op1: replaced", texts[0])
	}

	// Check inserted paragraph exists
	found := false
	for _, txt := range texts {
		if txt == "Op2: inserted" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("inserted paragraph not found; paragraphs: %v", texts)
	}
}

// ---------------------------------------------------------------------------
// 7. TestApplyWithStyle
// ---------------------------------------------------------------------------

func TestApplyWithStyle(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "style.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	ops := []EditOperation{
		{
			Type:    OpReplaceText,
			Target:  StableAnchor{ParaID: "AAA00001"},
			Content: "Bold text",
			Style:   map[string]string{"bold": "true"},
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("Applied: want 1, got %d", res.Applied)
	}

	// Verify the run has w:rPr > w:b
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML: %v", err)
	}
	body := getBody(doc)
	if body == nil {
		t.Fatal("no body")
	}

	// Use RenderRuns to verify bold style
	for _, child := range body.ChildElements() {
		if child.Tag == "p" {
			runs := RenderRuns(child)
			if len(runs) == 0 {
				t.Fatal("no runs found in replaced paragraph")
			}
			if !runs[0].Bold {
				t.Error("run[0].Bold: want true, got false")
			}
			if runs[0].Text != "Bold text" {
				t.Errorf("run[0].Text: want %q, got %q", "Bold text", runs[0].Text)
			}
			break
		}
	}
}

// ---------------------------------------------------------------------------
// 8. TestApplyByOffset
// ---------------------------------------------------------------------------

func TestApplyByOffset(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "offset.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	// Offset 1 should be "Second paragraph"
	ops := []EditOperation{
		{
			Type:    OpReplaceText,
			Target:  StableAnchor{Offset: 1}, // 0-based index among paragraphs
			Content: "Replaced by offset",
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("Applied: want 1, got %d (warnings: %v)", res.Applied, res.Warnings)
	}

	texts := allParaTexts(t, pkg)
	if len(texts) < 2 || texts[1] != "Replaced by offset" {
		t.Errorf("texts[1]: want %q, got %q", "Replaced by offset", texts[1])
	}
}

// ---------------------------------------------------------------------------
// 9. TestApplyInvalidTarget
// ---------------------------------------------------------------------------

func TestApplyInvalidTarget(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "invalid_target.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	ops := []EditOperation{
		{
			Type:    OpReplaceText,
			Target:  StableAnchor{ParaID: "NONEXISTENT"},
			Content: "Should not be applied",
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: unexpected error: %v", err)
	}
	if res.Applied != 0 {
		t.Errorf("Applied: want 0, got %d", res.Applied)
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped: want 1, got %d", res.Skipped)
	}
	if len(res.Warnings) == 0 {
		t.Error("Warnings: want at least 1, got 0")
	}
}

// ---------------------------------------------------------------------------
// 10. TestApplyRoundTrip
// ---------------------------------------------------------------------------

func TestApplyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "roundtrip.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	ops := []EditOperation{
		{
			Type:    OpReplaceText,
			Target:  StableAnchor{ParaID: "AAA00001"},
			Content: "Persisted text",
		},
	}

	if _, err := Apply(pkg, ops); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	savePath := filepath.Join(dir, "roundtrip_saved.docx")
	if err := pkg.Save(savePath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Re-open and verify
	pkg2, err := Open(savePath)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	defer pkg2.Close()

	texts := allParaTexts(t, pkg2)
	if len(texts) == 0 || texts[0] != "Persisted text" {
		t.Errorf("after reload, texts[0]: want %q, got %q", "Persisted text", texts[0])
	}

	// Also verify via View()
	nodes, err := View(pkg2)
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	found := false
	for _, n := range nodes {
		if strings.Contains(n.Content, "Persisted text") {
			found = true
			break
		}
	}
	if !found {
		t.Error("View: 'Persisted text' not found in ViewNodes after round-trip")
	}
}

// ---------------------------------------------------------------------------
// 11. TestApplyInsertChinese
// ---------------------------------------------------------------------------

func TestApplyInsertChinese(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "chinese.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	chineseText := "这是一段中文内容，用于测试多语言支持。"

	ops := []EditOperation{
		{
			Type:    OpInsertAfter,
			Target:  StableAnchor{ParaID: "AAA00001"},
			Content: chineseText,
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("Applied: want 1, got %d (warnings: %v)", res.Applied, res.Warnings)
	}

	texts := allParaTexts(t, pkg)
	found := false
	for _, txt := range texts {
		if txt == chineseText {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Chinese text not found in paragraphs: %v", texts)
	}
}

// ---------------------------------------------------------------------------
// 12. TestApplyEmptyOps
// ---------------------------------------------------------------------------

func TestApplyEmptyOps(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "empty_ops.docx", editorBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	before := countParas(t, pkg)

	res, err := Apply(pkg, []EditOperation{})
	if err != nil {
		t.Fatalf("Apply(empty): unexpected error: %v", err)
	}
	if res.Applied != 0 {
		t.Errorf("Applied: want 0, got %d", res.Applied)
	}
	if res.Skipped != 0 {
		t.Errorf("Skipped: want 0, got %d", res.Skipped)
	}

	// Document must be unchanged
	after := countParas(t, pkg)
	if after != before {
		t.Errorf("paragraph count changed unexpectedly: was %d, now %d", before, after)
	}
}

// ---------------------------------------------------------------------------
// 13. TestApplyByBookmark
// ---------------------------------------------------------------------------

func TestApplyByBookmark(t *testing.T) {
	dir := t.TempDir()
	path := createEditorDocx(t, dir, "bookmark.docx", bookmarkBodyXML)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	ops := []EditOperation{
		{
			Type:    OpReplaceText,
			Target:  StableAnchor{Bookmark: "myBookmark"},
			Content: "Replaced via bookmark",
		},
	}

	res, err := Apply(pkg, ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("Applied: want 1, got %d (warnings: %v)", res.Applied, res.Warnings)
	}

	texts := allParaTexts(t, pkg)
	found := false
	for _, txt := range texts {
		if txt == "Replaced via bookmark" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("bookmark-replaced text not found; paragraphs: %v", texts)
	}
}
