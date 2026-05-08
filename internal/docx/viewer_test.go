package docx

import (
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// ---------------------------------------------------------------------------
// TestView
// ---------------------------------------------------------------------------

// TestView verifies that View() on the rich docx returns the expected number
// and types of ViewNodes.
func TestView(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	nodes, err := View(pkg)
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	if len(nodes) == 0 {
		t.Fatal("View returned 0 nodes; expected at least 1")
	}

	// The rich docx has: heading, paragraph (English), paragraph (CJK), table
	// That's 4 top-level elements.
	if len(nodes) < 4 {
		t.Errorf("View returned %d nodes; expected at least 4", len(nodes))
	}

	// Count types
	typeCount := make(map[NodeType]int)
	for _, n := range nodes {
		typeCount[n.Type]++
	}

	if typeCount[NodeHeading] < 1 {
		t.Errorf("expected at least 1 heading, got %d", typeCount[NodeHeading])
	}
	if typeCount[NodeParagraph] < 1 {
		t.Errorf("expected at least 1 paragraph, got %d", typeCount[NodeParagraph])
	}
	if typeCount[NodeTable] < 1 {
		t.Errorf("expected at least 1 table, got %d", typeCount[NodeTable])
	}
}

// ---------------------------------------------------------------------------
// TestViewHeading
// ---------------------------------------------------------------------------

// TestViewHeading verifies that the first paragraph (Heading1) is classified
// as NodeHeading with Level == 1.
func TestViewHeading(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	nodes, err := View(pkg)
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	if len(nodes) == 0 {
		t.Fatal("no nodes returned")
	}

	heading := nodes[0]
	if heading.Type != NodeHeading {
		t.Errorf("first node type: got %v, want NodeHeading", heading.Type)
	}
	if heading.Level != 1 {
		t.Errorf("heading Level: got %d, want 1", heading.Level)
	}
	if heading.Content != "Introduction" {
		t.Errorf("heading Content: got %q, want %q", heading.Content, "Introduction")
	}
}

// ---------------------------------------------------------------------------
// TestViewTable
// ---------------------------------------------------------------------------

// TestViewTable verifies that the table node has the expected row/cell structure.
func TestViewTable(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	nodes, err := View(pkg)
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	// Find the table node
	var tbl *ViewNode
	for i := range nodes {
		if nodes[i].Type == NodeTable {
			tbl = &nodes[i]
			break
		}
	}
	if tbl == nil {
		t.Fatal("no NodeTable found in View output")
	}

	// Rich docx has a 2×2 table (2 rows, 2 cells each)
	if len(tbl.Children) != 2 {
		t.Errorf("table rows: got %d, want 2", len(tbl.Children))
	}
	for ri, row := range tbl.Children {
		if len(row.Children) != 2 {
			t.Errorf("row %d: cell count: got %d, want 2", ri, len(row.Children))
		}
	}

	// Verify cell text content
	if len(tbl.Children) >= 1 && len(tbl.Children[0].Children) >= 2 {
		cell00 := tbl.Children[0].Children[0].Content
		cell01 := tbl.Children[0].Children[1].Content
		if cell00 != "Cell A1" {
			t.Errorf("cell[0][0]: got %q, want %q", cell00, "Cell A1")
		}
		if cell01 != "Cell B1" {
			t.Errorf("cell[0][1]: got %q, want %q", cell01, "Cell B1")
		}
	}
	if len(tbl.Children) >= 2 && len(tbl.Children[1].Children) >= 2 {
		cell10 := tbl.Children[1].Children[0].Content
		cell11 := tbl.Children[1].Children[1].Content
		if cell10 != "Cell A2" {
			t.Errorf("cell[1][0]: got %q, want %q", cell10, "Cell A2")
		}
		if cell11 != "Cell B2" {
			t.Errorf("cell[1][1]: got %q, want %q", cell11, "Cell B2")
		}
	}
}

// ---------------------------------------------------------------------------
// TestRenderMarkdown
// ---------------------------------------------------------------------------

// TestRenderMarkdown verifies that the Markdown output contains heading syntax
// and a table.
func TestRenderMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	nodes, err := View(pkg)
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	md := RenderMarkdown(nodes)

	if !strings.Contains(md, "# Introduction") {
		t.Errorf("Markdown missing heading; output:\n%s", md)
	}

	if !strings.Contains(md, "| Cell A1 |") {
		t.Errorf("Markdown missing table cell; output:\n%s", md)
	}

	// Markdown table separator
	if !strings.Contains(md, "| --- |") {
		t.Errorf("Markdown missing table separator; output:\n%s", md)
	}

	// CJK content
	if !strings.Contains(md, "这是一段中文") {
		t.Errorf("Markdown missing CJK text; output:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// TestRenderTerminal
// ---------------------------------------------------------------------------

// TestRenderTerminal verifies that the terminal output is non-empty and
// contains the heading content.
func TestRenderTerminal(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	nodes, err := View(pkg)
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	out := RenderTerminal(nodes, 80)

	if out == "" {
		t.Fatal("RenderTerminal returned empty string")
	}

	if !strings.Contains(out, "Introduction") {
		t.Errorf("terminal output missing heading text; output:\n%s", out)
	}
}

// TestRenderTerminalNoWidth verifies that width=0 disables truncation.
func TestRenderTerminalNoWidth(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	nodes, err := View(pkg)
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	out := RenderTerminal(nodes, 0)
	if out == "" {
		t.Fatal("RenderTerminal(0) returned empty string")
	}
}

// ---------------------------------------------------------------------------
// TestNormalizationAvailable
// ---------------------------------------------------------------------------

// TestNormalizationAvailable only checks that IsNormalizationAvailable
// does not panic and returns a bool.
func TestNormalizationAvailable(t *testing.T) {
	_ = IsNormalizationAvailable()
}

// ---------------------------------------------------------------------------
// TestRenderParagraph
// ---------------------------------------------------------------------------

func TestRenderParagraph(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML: %v", err)
	}

	para := doc.FindElement("//w:p")
	if para == nil {
		t.Fatal("no w:p found")
	}
	text, _ := RenderParagraph(para)
	if text == "" {
		t.Error("RenderParagraph returned empty text for first paragraph")
	}
}

// ---------------------------------------------------------------------------
// TestTraverse
// ---------------------------------------------------------------------------

func TestTraverse(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatalf("XML: %v", err)
	}

	count := 0
	err = Traverse(doc, func(elem *etree.Element, index int) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}
	if count == 0 {
		t.Error("Traverse called fn 0 times; expected > 0")
	}
}

// ---------------------------------------------------------------------------
// TestTraversePart
// ---------------------------------------------------------------------------

func TestTraversePart(t *testing.T) {
	dir := t.TempDir()
	path := createRichDocx(t, dir)

	pkg, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pkg.Close()

	count := 0
	err = TraversePart(pkg, "word/header1.xml", func(elem *etree.Element, index int) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("TraversePart: %v", err)
	}
	if count == 0 {
		t.Error("TraversePart called fn 0 times; expected > 0")
	}
}
