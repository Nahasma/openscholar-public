package docx

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Shared XML fixtures
// ---------------------------------------------------------------------------

const (
	minContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

	minRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

	minDocumentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
</Relationships>`

	minDocument = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r>
        <w:t>Hello World</w:t>
      </w:r>
    </w:p>
  </w:body>
</w:document>`
)

// ---------------------------------------------------------------------------
// createMinimalDocx
// ---------------------------------------------------------------------------

// createMinimalDocx builds a minimal valid .docx in dir and returns its path.
func createMinimalDocx(t *testing.T, dir string) string {
	t.Helper()

	entries := []struct {
		name string
		data string
	}{
		{"[Content_Types].xml", minContentTypes},
		{"_rels/.rels", minRootRels},
		{"word/_rels/document.xml.rels", minDocumentRels},
		{"word/document.xml", minDocument},
	}

	path := filepath.Join(dir, "minimal.docx")
	writeDOCX(t, path, entries, nil)
	return path
}

// ---------------------------------------------------------------------------
// createRichDocx
// ---------------------------------------------------------------------------

// createRichDocx builds a richer .docx (multi-paragraph, CJK, heading,
// 2×2 table, page header, binary image placeholder) in dir and returns its path.
func createRichDocx(t *testing.T, dir string) string {
	t.Helper()

	const richContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Default Extension="png" ContentType="image/png"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>
</Types>`

	const richRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

	const richDocumentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>
</Relationships>`

	const richDocument = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:pPr><w:pStyle w:val="Heading1"/></w:pPr>
      <w:r><w:t>Introduction</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>This is a test paragraph in English.</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>这是一段中文测试文字，用于验证多语言支持。</w:t></w:r>
    </w:p>
    <w:tbl>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Cell A1</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Cell B1</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Cell A2</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Cell B2</w:t></w:r></w:p></w:tc>
      </w:tr>
    </w:tbl>
  </w:body>
</w:document>`

	const richHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:p><w:r><w:t>Page Header</w:t></w:r></w:p>
</w:hdr>`

	xmlEntries := []struct {
		name string
		data string
	}{
		{"[Content_Types].xml", richContentTypes},
		{"_rels/.rels", richRootRels},
		{"word/_rels/document.xml.rels", richDocumentRels},
		{"word/document.xml", richDocument},
		{"word/header1.xml", richHeader},
	}

	// Fake PNG-magic + a few opaque bytes (not a valid image, just binary data).
	binaryEntries := []binaryEntry{
		{"word/media/image1.png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01}},
	}

	path := filepath.Join(dir, "rich.docx")
	writeDOCX(t, path, xmlEntries, binaryEntries)
	return path
}

// ---------------------------------------------------------------------------
// Internal helper — builds a ZIP/DOCX from string and binary entry lists.
// ---------------------------------------------------------------------------

type binaryEntry struct {
	name string
	data []byte
}

func writeDOCX(t *testing.T, path string, xmlEntries []struct{ name, data string }, binEntries []binaryEntry) {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, e := range xmlEntries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("writeDOCX: create %q: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.data)); err != nil {
			t.Fatalf("writeDOCX: write %q: %v", e.name, err)
		}
	}

	for _, e := range binEntries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("writeDOCX: create binary %q: %v", e.name, err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatalf("writeDOCX: write binary %q: %v", e.name, err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("writeDOCX: close zip: %v", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writeDOCX: write file %q: %v", path, err)
	}
}

// ---------------------------------------------------------------------------
// hashParts — compute SHA256 for every part in the package.
// ---------------------------------------------------------------------------

func hashParts(pkg *Package) map[string][32]byte {
	m := make(map[string][32]byte, len(pkg.parts))
	for uri, part := range pkg.parts {
		m[uri] = sha256.Sum256(part.Data)
	}
	return m
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestOpen verifies that Open reads a minimal docx without error and exposes
// the expected parts.
func TestOpen(t *testing.T) {
	dir := t.TempDir()
	path := createMinimalDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}
	defer pkg.Close()

	if !pkg.HasPart("word/document.xml") {
		t.Error("HasPart(word/document.xml): expected true, got false")
	}
	if !pkg.HasPart("[Content_Types].xml") {
		t.Error("HasPart([Content_Types].xml): expected true, got false")
	}

	parts := pkg.Parts()
	if len(parts) != 4 {
		t.Errorf("Parts() length: want 4, got %d (parts: %v)", len(parts), parts)
	}

	part, err := pkg.Part("word/document.xml")
	if err != nil {
		t.Fatalf("Part(word/document.xml): %v", err)
	}
	if len(part.Data) == 0 {
		t.Error("Part.Data is empty")
	}
}

// TestOpenNotExist verifies that opening a missing file returns an error.
func TestOpenNotExist(t *testing.T) {
	_, err := Open("/nonexistent/path/that/does/not/exist.docx")
	if err == nil {
		t.Fatal("Open: expected error for non-existent file, got nil")
	}
}

// TestRoundTrip opens a minimal docx, saves it to a new path, re-opens it,
// and verifies every part's SHA256 is identical.
func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := createMinimalDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	origHashes := hashParts(pkg)

	savePath := filepath.Join(dir, "roundtrip.docx")
	if err := pkg.Save(savePath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	pkg2, err := Open(savePath)
	if err != nil {
		t.Fatalf("Open saved file: %v", err)
	}
	defer pkg2.Close()

	newHashes := hashParts(pkg2)

	for uri, origHash := range origHashes {
		newHash, ok := newHashes[uri]
		if !ok {
			t.Errorf("part %q missing after round-trip", uri)
			continue
		}
		if origHash != newHash {
			t.Errorf("part %q: SHA256 mismatch after round-trip", uri)
		}
	}
	for uri := range newHashes {
		if _, ok := origHashes[uri]; !ok {
			t.Errorf("unexpected extra part %q after round-trip", uri)
		}
	}
}

// TestRoundTripRich is the same as TestRoundTrip but uses the richer docx.
func TestRoundTripRich(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open rich: %v", err)
	}
	defer pkg.Close()

	origHashes := hashParts(pkg)

	savePath := filepath.Join(dir, "roundtrip_rich.docx")
	if err := pkg.Save(savePath); err != nil {
		t.Fatalf("Save rich: %v", err)
	}

	pkg2, err := Open(savePath)
	if err != nil {
		t.Fatalf("Open saved rich: %v", err)
	}
	defer pkg2.Close()

	newHashes := hashParts(pkg2)

	for uri, origHash := range origHashes {
		newHash, ok := newHashes[uri]
		if !ok {
			t.Errorf("rich: part %q missing after round-trip", uri)
			continue
		}
		if origHash != newHash {
			t.Errorf("rich: part %q SHA256 mismatch after round-trip", uri)
		}
	}
	for uri := range newHashes {
		if _, ok := origHashes[uri]; !ok {
			t.Errorf("rich: unexpected extra part %q after round-trip", uri)
		}
	}
}

// TestXML verifies that XML() returns a parsed DOM that can be navigated.
func TestXML(t *testing.T) {
	dir := t.TempDir()
	path := createMinimalDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML: %v", err)
	}
	if doc == nil {
		t.Fatal("XML returned nil document")
	}

	if body := doc.FindElement("//w:body"); body == nil {
		t.Error("w:body element not found")
	}

	wt := doc.FindElement("//w:t")
	if wt == nil {
		t.Fatal("w:t element not found")
	}
	if got := wt.Text(); got != "Hello World" {
		t.Errorf("w:t text: want %q, got %q", "Hello World", got)
	}
}

// TestSetXML modifies the document XML, saves, re-opens, and verifies that
// the change persisted while unmodified parts retain their original SHA256.
func TestSetXML(t *testing.T) {
	dir := t.TempDir()
	path := createMinimalDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	// Remember SHA256 of the unmodified relationship file.
	relsPart, err := pkg.Part("_rels/.rels")
	if err != nil {
		t.Fatalf("Part(_rels/.rels): %v", err)
	}
	origRelsHash := sha256.Sum256(relsPart.Data)

	// Mutate w:t text.
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML: %v", err)
	}
	wt := doc.FindElement("//w:t")
	if wt == nil {
		t.Fatal("w:t not found")
	}
	wt.SetText("Modified")

	if err := pkg.SetXML("word/document.xml", doc); err != nil {
		t.Fatalf("SetXML: %v", err)
	}

	savePath := filepath.Join(dir, "modified.docx")
	if err := pkg.Save(savePath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	pkg2, err := Open(savePath)
	if err != nil {
		t.Fatalf("Open modified: %v", err)
	}
	defer pkg2.Close()

	doc2, err := pkg2.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML (modified): %v", err)
	}
	wt2 := doc2.FindElement("//w:t")
	if wt2 == nil {
		t.Fatal("w:t not found in modified docx")
	}
	if got := wt2.Text(); got != "Modified" {
		t.Errorf("w:t text: want %q, got %q", "Modified", got)
	}

	// Unmodified part must retain the same SHA256.
	relsPart2, err := pkg2.Part("_rels/.rels")
	if err != nil {
		t.Fatalf("Part(_rels/.rels) after save: %v", err)
	}
	if sha256.Sum256(relsPart2.Data) != origRelsHash {
		t.Error("_rels/.rels SHA256 changed unexpectedly after SetXML on document.xml")
	}
}

// TestSetPart adds a brand-new part, saves, re-opens, and verifies it is
// present with byte-identical content.
func TestSetPart(t *testing.T) {
	dir := t.TempDir()
	path := createMinimalDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	newData := []byte(`<?xml version="1.0"?><root><item>test</item></root>`)
	pkg.SetPart("word/newfile.xml", newData)

	savePath := filepath.Join(dir, "with_new_part.docx")
	if err := pkg.Save(savePath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	pkg2, err := Open(savePath)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	defer pkg2.Close()

	if !pkg2.HasPart("word/newfile.xml") {
		t.Fatal("HasPart(word/newfile.xml): expected true after SetPart+Save")
	}

	part, err := pkg2.Part("word/newfile.xml")
	if err != nil {
		t.Fatalf("Part(word/newfile.xml): %v", err)
	}
	if !bytes.Equal(part.Data, newData) {
		t.Errorf("new part data mismatch:\n  want: %q\n   got: %q", newData, part.Data)
	}
}

// TestXMLCaching verifies that XML() returns the same *etree.Document pointer
// on repeated calls, and that SetPart invalidates the cached pointer.
func TestXMLCaching(t *testing.T) {
	dir := t.TempDir()
	path := createMinimalDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	doc1, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML (1st call): %v", err)
	}
	doc2, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML (2nd call): %v", err)
	}
	if doc1 != doc2 {
		t.Error("XML caching: 2nd call returned a different pointer; expected same cached instance")
	}

	// SetPart should clear the Doc field, so the next XML() call re-parses.
	pkg.SetPart("word/document.xml", []byte(minDocument))

	doc3, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML (after SetPart): %v", err)
	}
	if doc3 == doc1 {
		t.Error("XML caching: pointer after SetPart is the same; cache should have been invalidated by SetPart")
	}
}

// TestClose verifies that Close returns no error and that a second Close does
// not panic.
func TestClose(t *testing.T) {
	dir := t.TempDir()
	path := createMinimalDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := pkg.Close(); err != nil {
		t.Fatalf("Close (first): %v", err)
	}

	// Second Close must not panic — the implementation guards zipReader == nil.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("second Close panicked: %v", r)
		}
	}()
	_ = pkg.Close()
}

// TestSaveSameAsInput opens a file and saves it back to the same path.
// Because Open reads all data into memory first, overwriting the source file
// should succeed and produce an identical document.
func TestSaveSameAsInput(t *testing.T) {
	dir := t.TempDir()
	path := createMinimalDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	origHashes := hashParts(pkg)

	if err := pkg.Save(path); err != nil {
		// An explicit error is acceptable (e.g., if the implementation
		// disallows overwriting). Just log and skip the integrity check.
		t.Logf("Save(same path) returned error (acceptable): %v", err)
		return
	}

	pkg2, err := Open(path)
	if err != nil {
		t.Fatalf("Open after same-path save: %v", err)
	}
	defer pkg2.Close()

	newHashes := hashParts(pkg2)
	for uri, origHash := range origHashes {
		newHash, ok := newHashes[uri]
		if !ok {
			t.Errorf("part %q missing after same-path save", uri)
			continue
		}
		if origHash != newHash {
			t.Errorf("part %q: SHA256 mismatch after same-path save", uri)
		}
	}
}
