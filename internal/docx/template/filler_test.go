package template

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/docx"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildFillerDocx builds a docx with custom body XML, opens it, and registers
// a cleanup to close it. Same pattern as analyzer_test.go.
func buildFillerDocx(t *testing.T, bodyXML string) *docx.Package {
	t.Helper()
	return buildTestDocx(t, bodyXML)
}

// extractAllText collects all w:t text from word/document.xml.
func extractAllText(t *testing.T, pkg *docx.Package) string {
	t.Helper()
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("extractAllText: XML: %v", err)
	}
	allWT := doc.FindElements("//t")
	var sb strings.Builder
	for _, wt := range allWT {
		sb.WriteString(wt.Text())
	}
	return sb.String()
}

// minimalSpec creates a TemplateSpec with a single SlotText slot.
func minimalSpec(name string) *TemplateSpec {
	return &TemplateSpec{
		Slots: []SlotSpec{
			{Name: name, AnchorType: AnchorPlaceholder, DataType: SlotText},
		},
		Metadata: map[string]string{},
	}
}

// writeTempImage writes a minimal fake PNG to a temp file and returns its path.
func writeTempImage(t *testing.T, ext string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test."+ext)
	// Minimal PNG magic bytes + some padding.
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x01}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writeTempImage: %v", err)
	}
	return path
}

// saveAndReopen saves the package to a temp file, then re-opens it.
func saveAndReopen(t *testing.T, pkg *docx.Package) *docx.Package {
	t.Helper()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.docx")
	if err := pkg.Save(outPath); err != nil {
		t.Fatalf("saveAndReopen: Save: %v", err)
	}
	pkg2, err := docx.Open(outPath)
	if err != nil {
		t.Fatalf("saveAndReopen: Open: %v", err)
	}
	t.Cleanup(func() { _ = pkg2.Close() })
	return pkg2
}

// buildDocxWithRelations builds a minimal docx that also includes a proper
// [Content_Types].xml so that image insertion works.
func buildDocxWithRelations(t *testing.T, bodyXML string) *docx.Package {
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
			t.Fatalf("buildDocxWithRelations: create %q: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.data)); err != nil {
			t.Fatalf("buildDocxWithRelations: write %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("buildDocxWithRelations: close zip: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("buildDocxWithRelations: write file: %v", err)
	}
	pkg, err := docx.Open(path)
	if err != nil {
		t.Fatalf("buildDocxWithRelations: open: %v", err)
	}
	t.Cleanup(func() { _ = pkg.Close() })
	return pkg
}

// ---------------------------------------------------------------------------
// 1. TestFillVariables
// ---------------------------------------------------------------------------

func TestFillVariables(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{greeting}}</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)
	spec := minimalSpec("greeting")

	result, err := Fill(pkg, spec, map[string]any{"greeting": "Hello, World!"})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill errors: %v", result.Errors)
	}

	text := extractAllText(t, pkg)
	if !strings.Contains(text, "Hello, World!") {
		t.Errorf("expected 'Hello, World!' in document text, got %q", text)
	}
}

// ---------------------------------------------------------------------------
// 2. TestFillWithSpec
// ---------------------------------------------------------------------------

func TestFillWithSpec(t *testing.T) {
	body := `    <w:p><w:r><w:t>Author: {{author}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>Title: {{title}}</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	data := map[string]any{
		"author": "Jane Doe",
		"title":  "Research Paper",
	}
	result, err := Fill(pkg, spec, data)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill errors: %v", result.Errors)
	}

	text := extractAllText(t, pkg)
	if !strings.Contains(text, "Jane Doe") {
		t.Errorf("expected 'Jane Doe' in text, got %q", text)
	}
	if !strings.Contains(text, "Research Paper") {
		t.Errorf("expected 'Research Paper' in text, got %q", text)
	}
}

// ---------------------------------------------------------------------------
// 3. TestFillLoop
// ---------------------------------------------------------------------------

func TestFillLoop(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{#items}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>Item: {{name}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{/items}}</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)

	spec := &TemplateSpec{
		Slots: []SlotSpec{
			{Name: "items", DataType: SlotLoop, AnchorType: AnchorPlaceholder},
		},
		Metadata: map[string]string{},
	}

	data := map[string]any{
		"items": []map[string]any{
			{"name": "Alpha"},
			{"name": "Beta"},
			{"name": "Gamma"},
		},
	}
	result, err := Fill(pkg, spec, data)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill errors: %v", result.Errors)
	}

	text := extractAllText(t, pkg)
	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		if !strings.Contains(text, name) {
			t.Errorf("expected %q in expanded loop, text: %q", name, text)
		}
	}
	// The loop markers themselves must be gone.
	if strings.Contains(text, "{{#items}}") || strings.Contains(text, "{{/items}}") {
		t.Error("loop markers should have been removed after expansion")
	}
}

// ---------------------------------------------------------------------------
// 4. TestFillConditionTrue
// ---------------------------------------------------------------------------

func TestFillConditionTrue(t *testing.T) {
	body := `    <w:p><w:r><w:t>Before</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{?show_abstract}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>Abstract text here</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{/show_abstract}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>After</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)

	spec := &TemplateSpec{
		Slots: []SlotSpec{
			{Name: "show_abstract", DataType: SlotConditional, AnchorType: AnchorPlaceholder},
		},
		Metadata: map[string]string{},
	}

	result, err := Fill(pkg, spec, map[string]any{"show_abstract": true})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill errors: %v", result.Errors)
	}

	text := extractAllText(t, pkg)
	if !strings.Contains(text, "Abstract text here") {
		t.Errorf("expected abstract content to be present when condition is true, got: %q", text)
	}
	// Control markers should be removed.
	if strings.Contains(text, "{{?show_abstract}}") || strings.Contains(text, "{{/show_abstract}}") {
		t.Error("condition markers should have been removed")
	}
}

// ---------------------------------------------------------------------------
// 5. TestFillConditionFalse
// ---------------------------------------------------------------------------

func TestFillConditionFalse(t *testing.T) {
	body := `    <w:p><w:r><w:t>Before</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{?show_extra}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>Extra content</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{/show_extra}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>After</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)

	spec := &TemplateSpec{
		Slots: []SlotSpec{
			{Name: "show_extra", DataType: SlotConditional, AnchorType: AnchorPlaceholder},
		},
		Metadata: map[string]string{},
	}

	result, err := Fill(pkg, spec, map[string]any{"show_extra": false})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill errors: %v", result.Errors)
	}

	text := extractAllText(t, pkg)
	if strings.Contains(text, "Extra content") {
		t.Error("expected conditional content to be hidden when condition is false")
	}
	if !strings.Contains(text, "Before") || !strings.Contains(text, "After") {
		t.Errorf("content outside condition block should be preserved, got: %q", text)
	}
}

// ---------------------------------------------------------------------------
// 6. TestFillImage
// ---------------------------------------------------------------------------

func TestFillImage(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{img:logo}}</w:t></w:r></w:p>`
	pkg := buildDocxWithRelations(t, body)

	imgPath := writeTempImage(t, "png")

	spec := &TemplateSpec{
		Slots: []SlotSpec{
			{Name: "logo", DataType: SlotImage, AnchorType: AnchorPlaceholder},
		},
		Metadata: map[string]string{},
	}

	result, err := Fill(pkg, spec, map[string]any{
		"logo": ImageData{Path: imgPath, Width: 914400, Height: 914400},
	})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill errors: %v", result.Errors)
	}

	// Verify that the image was added to the package.
	if !pkg.HasPart("word/media/logo.png") {
		t.Error("expected word/media/logo.png to be added to package")
	}

	// Verify that the relationship was recorded (rels file updated).
	if !pkg.HasPart("word/_rels/document.xml.rels") {
		t.Error("expected word/_rels/document.xml.rels to exist")
	}
	relsDoc, err := pkg.XML("word/_rels/document.xml.rels")
	if err != nil {
		t.Fatalf("load rels: %v", err)
	}
	rels := relsDoc.FindElements("//Relationship")
	found := false
	for _, rel := range rels {
		if rel.SelectAttrValue("Type", "") == docx.RelTypeImage {
			found = true
		}
	}
	if !found {
		t.Error("expected image relationship in document.xml.rels")
	}
}

// ---------------------------------------------------------------------------
// 7. TestFillMissingRequired
// ---------------------------------------------------------------------------

func TestFillMissingRequired(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{must_have}}</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)

	spec := &TemplateSpec{
		Slots: []SlotSpec{
			{Name: "must_have", DataType: SlotText, AnchorType: AnchorPlaceholder, Required: true},
		},
		Metadata: map[string]string{},
	}

	result, err := Fill(pkg, spec, map[string]any{})
	if err != nil {
		t.Fatalf("Fill returned unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Error("expected Errors to be non-empty for missing required slot")
	}
}

// ---------------------------------------------------------------------------
// 8. TestFillExtraData
// ---------------------------------------------------------------------------

func TestFillExtraData(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{known}}</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)
	spec := minimalSpec("known")

	data := map[string]any{
		"known":   "present",
		"unknown": "extra value that has no slot",
	}
	result, err := Fill(pkg, spec, data)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill should not error for extra data keys, errors: %v", result.Errors)
	}
	// There should be a warning about the unknown key.
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "unknown") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning about unknown key, got warnings: %v", result.Warnings)
	}
}

// ---------------------------------------------------------------------------
// 9. TestFillRoundTrip
// ---------------------------------------------------------------------------

func TestFillRoundTrip(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{document_title}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{document_body}}</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)

	spec, err := Analyze(pkg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	data := map[string]any{
		"document_title": "My Paper",
		"document_body":  "This is the body.",
	}
	if _, err := Fill(pkg, spec, data); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	// Save and re-open.
	pkg2 := saveAndReopen(t, pkg)

	text := extractAllText(t, pkg2)
	if !strings.Contains(text, "My Paper") {
		t.Errorf("after round-trip: expected 'My Paper', got %q", text)
	}
	if !strings.Contains(text, "This is the body.") {
		t.Errorf("after round-trip: expected body text, got %q", text)
	}
}

// ---------------------------------------------------------------------------
// 10. TestFillChinese
// ---------------------------------------------------------------------------

func TestFillChinese(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{标题}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{摘要}}</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)

	spec := &TemplateSpec{
		Slots: []SlotSpec{
			{Name: "标题", DataType: SlotText, AnchorType: AnchorPlaceholder},
			{Name: "摘要", DataType: SlotText, AnchorType: AnchorPlaceholder},
		},
		Metadata: map[string]string{},
	}

	data := map[string]any{
		"标题": "深度学习在自然语言处理中的应用",
		"摘要": "本文探讨了深度学习技术在NLP领域的最新进展。",
	}
	result, err := Fill(pkg, spec, data)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill errors: %v", result.Errors)
	}

	text := extractAllText(t, pkg)
	if !strings.Contains(text, "深度学习在自然语言处理中的应用") {
		t.Errorf("expected Chinese title in document, got: %q", text)
	}
	if !strings.Contains(text, "本文探讨") {
		t.Errorf("expected Chinese abstract in document, got: %q", text)
	}
}

// ---------------------------------------------------------------------------
// 11. TestFillDefaultValue
// ---------------------------------------------------------------------------

func TestFillDefaultValue(t *testing.T) {
	body := `    <w:p><w:r><w:t>{{optional_field}}</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)

	spec := &TemplateSpec{
		Slots: []SlotSpec{
			{
				Name:       "optional_field",
				DataType:   SlotText,
				AnchorType: AnchorPlaceholder,
				Default:    "default_value",
			},
		},
		Metadata: map[string]string{},
	}

	// Don't supply the key — the default should be used.
	result, err := Fill(pkg, spec, map[string]any{})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill errors: %v", result.Errors)
	}

	text := extractAllText(t, pkg)
	if !strings.Contains(text, "default_value") {
		t.Errorf("expected default value in document, got: %q", text)
	}
}

// ---------------------------------------------------------------------------
// 12. TestFillLoopWithAnySlice
// ---------------------------------------------------------------------------

func TestFillLoopWithAnySlice(t *testing.T) {
	// Verify that []any is also accepted as loop data.
	body := `    <w:p><w:r><w:t>{{#rows}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{val}}</w:t></w:r></w:p>
    <w:p><w:r><w:t>{{/rows}}</w:t></w:r></w:p>`
	pkg := buildFillerDocx(t, body)

	spec := &TemplateSpec{
		Slots: []SlotSpec{
			{Name: "rows", DataType: SlotLoop, AnchorType: AnchorPlaceholder},
		},
		Metadata: map[string]string{},
	}

	data := map[string]any{
		"rows": []any{
			map[string]any{"val": "Row1"},
			map[string]any{"val": "Row2"},
		},
	}
	result, err := Fill(pkg, spec, data)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Fill errors: %v", result.Errors)
	}

	text := extractAllText(t, pkg)
	if !strings.Contains(text, "Row1") || !strings.Contains(text, "Row2") {
		t.Errorf("expected Row1 and Row2 in output, got: %q", text)
	}
}
