package template

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openscholar/openscholar/internal/docx"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const (
	testContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

	testRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

	testDocumentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
</Relationships>`
)

// buildTestDocx creates a temporary .docx with the given document body XML.
func buildTestDocx(t *testing.T, bodyXML string) *docx.Package {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.docx")

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
` + bodyXML + `
  </w:body>
</w:document>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	entries := []struct{ name, data string }{
		{"[Content_Types].xml", testContentTypes},
		{"_rels/.rels", testRootRels},
		{"word/_rels/document.xml.rels", testDocumentRels},
		{"word/document.xml", docXML},
	}
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("buildTestDocx: create %q: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.data)); err != nil {
			t.Fatalf("buildTestDocx: write %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("buildTestDocx: close zip: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("buildTestDocx: write file: %v", err)
	}

	pkg, err := docx.Open(path)
	if err != nil {
		t.Fatalf("buildTestDocx: open: %v", err)
	}
	t.Cleanup(func() { _ = pkg.Close() })
	return pkg
}

// buildTestDocxWithCC creates a .docx containing an sdt (content control).
func buildTestDocxWithCC(t *testing.T, alias string) *docx.Package {
	t.Helper()
	body := fmt.Sprintf(`    <w:sdt>
      <w:sdtPr><w:alias w:val="%s"/></w:sdtPr>
      <w:sdtContent><w:p><w:r><w:t>Default text</w:t></w:r></w:p></w:sdtContent>
    </w:sdt>`, alias)
	return buildTestDocx(t, body)
}

// buildTestDocxWithBookmark creates a .docx with a bookmark.
func buildTestDocxWithBookmark(t *testing.T, bookmarkName string) *docx.Package {
	t.Helper()
	body := fmt.Sprintf(`    <w:p>
      <w:bookmarkStart w:id="1" w:name="%s"/>
      <w:r><w:t>Bookmarked content</w:t></w:r>
      <w:bookmarkEnd w:id="1"/>
    </w:p>`, bookmarkName)
	return buildTestDocx(t, body)
}

// findSlot returns the first SlotSpec with the given name, or nil.
func findSlot(spec *TemplateSpec, name string) *SlotSpec {
	for i := range spec.Slots {
		if spec.Slots[i].Name == name {
			return &spec.Slots[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 1. TestAnalyzeEmpty
// ---------------------------------------------------------------------------

func TestAnalyzeEmpty(t *testing.T) {
	pkg := buildTestDocx(t, `    <w:p><w:r><w:t>No placeholders here</w:t></w:r></w:p>`)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(spec.Slots) != 0 {
		t.Errorf("expected 0 slots, got %d: %+v", len(spec.Slots), spec.Slots)
	}
	if len(spec.ProtectedZones) != 0 {
		t.Errorf("expected 0 protected zones, got %d", len(spec.ProtectedZones))
	}
}

// ---------------------------------------------------------------------------
// 2. TestAnalyzePlaceholders
// ---------------------------------------------------------------------------

func TestAnalyzePlaceholders(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{title}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{content}}</w:t></w:r></w:p>`
	pkg := buildTestDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(spec.Slots) != 2 {
		t.Fatalf("expected 2 slots, got %d: %+v", len(spec.Slots), spec.Slots)
	}

	titleSlot := findSlot(spec, "title")
	if titleSlot == nil {
		t.Error("slot 'title' not found")
	} else if titleSlot.DataType != SlotText {
		t.Errorf("slot 'title' DataType: want SlotText, got %v", titleSlot.DataType)
	} else if titleSlot.AnchorType != AnchorPlaceholder {
		t.Errorf("slot 'title' AnchorType: want AnchorPlaceholder, got %v", titleSlot.AnchorType)
	}

	contentSlot := findSlot(spec, "content")
	if contentSlot == nil {
		t.Error("slot 'content' not found")
	} else if contentSlot.DataType != SlotText {
		t.Errorf("slot 'content' DataType: want SlotText, got %v", contentSlot.DataType)
	}
}

// ---------------------------------------------------------------------------
// 3. TestAnalyzeLoop
// ---------------------------------------------------------------------------

func TestAnalyzeLoop(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{#items}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{name}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{/items}}</w:t></w:r></w:p>`
	pkg := buildTestDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	itemsSlot := findSlot(spec, "items")
	if itemsSlot == nil {
		t.Fatal("slot 'items' not found")
	}
	if itemsSlot.DataType != SlotLoop {
		t.Errorf("slot 'items' DataType: want SlotLoop, got %v", itemsSlot.DataType)
	}
	if itemsSlot.AnchorType != AnchorPlaceholder {
		t.Errorf("slot 'items' AnchorType: want AnchorPlaceholder, got %v", itemsSlot.AnchorType)
	}
}

// ---------------------------------------------------------------------------
// 4. TestAnalyzeCondition
// ---------------------------------------------------------------------------

func TestAnalyzeCondition(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{?visible}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>Optional content</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{/visible}}</w:t></w:r></w:p>`
	pkg := buildTestDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	visSlot := findSlot(spec, "visible")
	if visSlot == nil {
		t.Fatal("slot 'visible' not found")
	}
	if visSlot.DataType != SlotConditional {
		t.Errorf("slot 'visible' DataType: want SlotConditional, got %v", visSlot.DataType)
	}
}

// ---------------------------------------------------------------------------
// 5. TestAnalyzeImage
// ---------------------------------------------------------------------------

func TestAnalyzeImage(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{img:logo}}</w:t></w:r></w:p>`
	pkg := buildTestDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	logoSlot := findSlot(spec, "logo")
	if logoSlot == nil {
		t.Fatal("slot 'logo' not found")
	}
	if logoSlot.DataType != SlotImage {
		t.Errorf("slot 'logo' DataType: want SlotImage, got %v", logoSlot.DataType)
	}
}

// ---------------------------------------------------------------------------
// 6. TestAnalyzeProtected
// ---------------------------------------------------------------------------

func TestAnalyzeProtected(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{!frozen}}</w:t></w:r></w:p>`
	pkg := buildTestDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Protected zones must not generate a Slot.
	if s := findSlot(spec, "frozen"); s != nil {
		t.Error("slot 'frozen' should not exist for protected placeholder")
	}
	// But it should appear in ProtectedZones.
	found := false
	for _, pz := range spec.ProtectedZones {
		if pz.Name == "frozen" {
			found = true
			break
		}
	}
	if !found {
		t.Error("protected zone 'frozen' not found in ProtectedZones")
	}
}

// ---------------------------------------------------------------------------
// 7. TestAnalyzeSections
// ---------------------------------------------------------------------------

func TestAnalyzeSections(t *testing.T) {
	body := `    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Chapter One</w:t></w:r></w:p>
    <w:p><w:r><w:t>Some text</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Section 1.1</w:t></w:r></w:p>
    <w:p><w:r><w:t>More text</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Chapter Two</w:t></w:r></w:p>`
	pkg := buildTestDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(spec.Sections) != 2 {
		t.Fatalf("expected 2 top-level sections, got %d: %+v", len(spec.Sections), spec.Sections)
	}
	if spec.Sections[0].Title != "Chapter One" {
		t.Errorf("section[0].Title: want %q, got %q", "Chapter One", spec.Sections[0].Title)
	}
	if spec.Sections[0].Level != 1 {
		t.Errorf("section[0].Level: want 1, got %d", spec.Sections[0].Level)
	}
	if len(spec.Sections[0].Children) != 1 {
		t.Fatalf("section[0].Children: want 1, got %d", len(spec.Sections[0].Children))
	}
	if spec.Sections[0].Children[0].Title != "Section 1.1" {
		t.Errorf("section[0].Children[0].Title: want %q, got %q", "Section 1.1", spec.Sections[0].Children[0].Title)
	}
	if spec.Sections[1].Title != "Chapter Two" {
		t.Errorf("section[1].Title: want %q, got %q", "Chapter Two", spec.Sections[1].Title)
	}
}

// ---------------------------------------------------------------------------
// 8. TestAnalyzeMixed
// ---------------------------------------------------------------------------

func TestAnalyzeMixed(t *testing.T) {
	// Mix: CC "cc_title", bookmark "bm_author", placeholder {{subtitle}},
	// loop {{#refs}}, condition {{?abstract}}, image {{img:figure}},
	// protected {{!legal}}.
	body := `    <w:sdt>
      <w:sdtPr><w:alias w:val="cc_title"/></w:sdtPr>
      <w:sdtContent><w:p><w:r><w:t>CC Title</w:t></w:r></w:p></w:sdtContent>
    </w:sdt>
    <w:p>
      <w:bookmarkStart w:id="1" w:name="bm_author"/>
      <w:r><w:t>Author</w:t></w:r>
      <w:bookmarkEnd w:id="1"/>
    </w:p>
    <w:p><w:r><w:t>{{subtitle}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{#refs}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{ref_text}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{/refs}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{?abstract}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>Abstract content</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{/abstract}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{img:figure}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{!legal}}</w:t></w:r></w:p>`
	pkg := buildTestDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Verify CC slot.
	ccSlot := findSlot(spec, "cc_title")
	if ccSlot == nil {
		t.Error("slot 'cc_title' not found")
	} else if ccSlot.AnchorType != AnchorContentControl {
		t.Errorf("cc_title AnchorType: want AnchorContentControl, got %v", ccSlot.AnchorType)
	}

	// Verify bookmark slot.
	bmSlot := findSlot(spec, "bm_author")
	if bmSlot == nil {
		t.Error("slot 'bm_author' not found")
	} else if bmSlot.AnchorType != AnchorBookmark {
		t.Errorf("bm_author AnchorType: want AnchorBookmark, got %v", bmSlot.AnchorType)
	}

	// Verify placeholder slots.
	subtitleSlot := findSlot(spec, "subtitle")
	if subtitleSlot == nil {
		t.Error("slot 'subtitle' not found")
	} else if subtitleSlot.AnchorType != AnchorPlaceholder {
		t.Errorf("subtitle AnchorType: want AnchorPlaceholder, got %v", subtitleSlot.AnchorType)
	}

	refsSlot := findSlot(spec, "refs")
	if refsSlot == nil {
		t.Error("slot 'refs' not found")
	} else if refsSlot.DataType != SlotLoop {
		t.Errorf("refs DataType: want SlotLoop, got %v", refsSlot.DataType)
	}

	abstractSlot := findSlot(spec, "abstract")
	if abstractSlot == nil {
		t.Error("slot 'abstract' not found")
	} else if abstractSlot.DataType != SlotConditional {
		t.Errorf("abstract DataType: want SlotConditional, got %v", abstractSlot.DataType)
	}

	figureSlot := findSlot(spec, "figure")
	if figureSlot == nil {
		t.Error("slot 'figure' not found")
	} else if figureSlot.DataType != SlotImage {
		t.Errorf("figure DataType: want SlotImage, got %v", figureSlot.DataType)
	}

	// Protected zone must not be a slot.
	if s := findSlot(spec, "legal"); s != nil {
		t.Error("slot 'legal' should not exist (it is protected)")
	}
	foundPZ := false
	for _, pz := range spec.ProtectedZones {
		if pz.Name == "legal" {
			foundPZ = true
		}
	}
	if !foundPZ {
		t.Error("protected zone 'legal' not found in ProtectedZones")
	}

	// ref_text is a placeholder inside the loop body — it should be a slot.
	refTextSlot := findSlot(spec, "ref_text")
	if refTextSlot == nil {
		t.Error("slot 'ref_text' not found")
	}

	// Metadata should contain slot_count.
	if spec.Metadata["slot_count"] == "" {
		t.Error("Metadata missing slot_count")
	}
}

// ---------------------------------------------------------------------------
// 9. TestAnalyzeContentControl
// ---------------------------------------------------------------------------

func TestAnalyzeContentControl(t *testing.T) {
	pkg := buildTestDocxWithCC(t, "document_title")

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	slot := findSlot(spec, "document_title")
	if slot == nil {
		t.Fatal("slot 'document_title' not found")
	}
	if slot.AnchorType != AnchorContentControl {
		t.Errorf("AnchorType: want AnchorContentControl, got %v", slot.AnchorType)
	}
	if slot.DataType != SlotText {
		t.Errorf("DataType: want SlotText, got %v", slot.DataType)
	}
}

// ---------------------------------------------------------------------------
// 10. TestAnalyzeBookmark
// ---------------------------------------------------------------------------

func TestAnalyzeBookmark(t *testing.T) {
	pkg := buildTestDocxWithBookmark(t, "author_name")

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	slot := findSlot(spec, "author_name")
	if slot == nil {
		t.Fatal("slot 'author_name' not found")
	}
	if slot.AnchorType != AnchorBookmark {
		t.Errorf("AnchorType: want AnchorBookmark, got %v", slot.AnchorType)
	}
}

// ---------------------------------------------------------------------------
// 11. TestAnalyzeBuiltinBookmarkIgnored
// ---------------------------------------------------------------------------

func TestAnalyzeBuiltinBookmarkIgnored(t *testing.T) {
	body := `    <w:p>
      <w:bookmarkStart w:id="1" w:name="_GoBack"/>
      <w:r><w:t>Content</w:t></w:r>
      <w:bookmarkEnd w:id="1"/>
    </w:p>`
	pkg := buildTestDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if s := findSlot(spec, "_GoBack"); s != nil {
		t.Error("built-in bookmark '_GoBack' should not generate a slot")
	}
}

// ---------------------------------------------------------------------------
// 12. TestAnalyzeDeduplicate — same name via different mechanisms
// ---------------------------------------------------------------------------

func TestAnalyzeDeduplicate(t *testing.T) {
	// CC "dup_key" and placeholder {{dup_key}}: only one slot should exist (CC wins).
	body := `    <w:sdt>
      <w:sdtPr><w:alias w:val="dup_key"/></w:sdtPr>
      <w:sdtContent><w:p><w:r><w:t>CC text</w:t></w:r></w:p></w:sdtContent>
    </w:sdt>
    <w:p><w:r><w:t>{{dup_key}}</w:t></w:r></w:p>`
	pkg := buildTestDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	count := 0
	for _, s := range spec.Slots {
		if s.Name == "dup_key" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 slot named 'dup_key', got %d", count)
	}
	// The surviving slot should be AnchorContentControl (higher priority).
	slot := findSlot(spec, "dup_key")
	if slot != nil && slot.AnchorType != AnchorContentControl {
		t.Errorf("surviving slot AnchorType: want AnchorContentControl, got %v", slot.AnchorType)
	}
}
