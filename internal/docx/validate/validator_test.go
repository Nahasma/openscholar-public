package validate

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/docx"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeAnchor returns a minimal StableAnchor for test nodes.
func makeAnchor(paraID string) docx.StableAnchor {
	return docx.StableAnchor{Part: "word/document.xml", ParaID: paraID}
}

// heading creates a NodeHeading ViewNode.
func heading(level int, content string) docx.ViewNode {
	return docx.ViewNode{
		Type:    docx.NodeHeading,
		Level:   level,
		Content: content,
		Anchor:  makeAnchor(content),
	}
}

// para creates a NodeParagraph ViewNode.
func para(content string) docx.ViewNode {
	return docx.ViewNode{
		Type:    docx.NodeParagraph,
		Content: content,
		Anchor:  makeAnchor(content),
	}
}

// tableNode creates a NodeTable with rows × cols cell structure.
func tableNode(rows [][]string) docx.ViewNode {
	node := docx.ViewNode{Type: docx.NodeTable, Anchor: makeAnchor("tbl")}
	for _, row := range rows {
		rowNode := docx.ViewNode{Type: docx.NodeParagraph}
		for _, cell := range row {
			rowNode.Children = append(rowNode.Children, docx.ViewNode{
				Type:    docx.NodeParagraph,
				Content: cell,
			})
		}
		node.Children = append(node.Children, rowNode)
	}
	return node
}

// imageNode creates a NodeImage ViewNode with a given rId.
func imageNode(rID string) docx.ViewNode {
	return docx.ViewNode{
		Type:   docx.NodeImage,
		Anchor: makeAnchor("img-" + rID),
		Metadata: map[string]string{
			"rId": rID,
		},
	}
}

// countDiags returns the number of diagnostics with a given rule ID.
func countDiags(diags []Diagnostic, ruleID string) int {
	n := 0
	for _, d := range diags {
		if d.RuleID == ruleID {
			n++
		}
	}
	return n
}

// hasDiag returns true if there is at least one diagnostic with the given ruleID and level.
func hasDiag(diags []Diagnostic, ruleID string, level DiagLevel) bool {
	for _, d := range diags {
		if d.RuleID == ruleID && d.Level == level {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// createTestDocxWithRels builds a minimal docx that has a known rId in rels.
// ---------------------------------------------------------------------------

func createTestDocxWithRels(t *testing.T, rIds []string) *docx.Package {
	t.Helper()

	const contentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

	const rootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

	const document = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body><w:p><w:r><w:t>test</w:t></w:r></w:p></w:body>
</w:document>`

	// Build document.xml.rels with the given rIds
	var relEntries bytes.Buffer
	relEntries.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	relEntries.WriteString("\n<Relationships xmlns=\"http://schemas.openxmlformats.org/package/2006/relationships\">\n")
	for _, id := range rIds {
		relEntries.WriteString(`  <Relationship Id="` + id + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/` + id + `.png"/>` + "\n")
	}
	relEntries.WriteString("</Relationships>")

	entries := []struct{ name, data string }{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", rootRels},
		{"word/_rels/document.xml.rels", relEntries.String()},
		{"word/document.xml", document},
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("createTestDocxWithRels: create %q: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.data)); err != nil {
			t.Fatalf("createTestDocxWithRels: write %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("createTestDocxWithRels: close zip: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.docx")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("createTestDocxWithRels: write file: %v", err)
	}

	pkg, err := docx.Open(path)
	if err != nil {
		t.Fatalf("createTestDocxWithRels: open pkg: %v", err)
	}
	t.Cleanup(func() { pkg.Close() })
	return pkg
}

// ---------------------------------------------------------------------------
// FMT-001 Tests
// ---------------------------------------------------------------------------

func TestFMT001_HeadingContinuity(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "Chapter 1"),
		para("Some content"),
		heading(3, "Sub-sub section"), // skips H2 → should warn
	}
	rule := headingContinuityRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected warning for H1→H3 jump, got none")
	}
	if diags[0].Level != DiagWarning {
		t.Errorf("expected DiagWarning, got %s", diags[0].Level)
	}
	if diags[0].RuleID != "FMT-001" {
		t.Errorf("expected RuleID FMT-001, got %s", diags[0].RuleID)
	}
}

func TestFMT001_NormalHeadings(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "Chapter 1"),
		para("intro"),
		heading(2, "Section 1.1"),
		para("detail"),
		heading(3, "Section 1.1.1"),
		para("more detail"),
	}
	rule := headingContinuityRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for H1→H2→H3 sequence, got %d", len(diags))
	}
}

// ---------------------------------------------------------------------------
// FMT-002 Tests
// ---------------------------------------------------------------------------

func TestFMT002_EmptyAfterHeading(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "Overview"),
		heading(2, "Details"), // no paragraph between → should warn
		para("actual content"),
	}
	rule := emptyAfterHeadingRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected warning for consecutive headings with no content between them")
	}
	if diags[0].RuleID != "FMT-002" {
		t.Errorf("expected RuleID FMT-002, got %s", diags[0].RuleID)
	}
}

func TestFMT002_HeadingWithContent(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "Overview"),
		para("This is the overview content."),
		heading(2, "Details"),
		para("These are the details."),
	}
	rule := emptyAfterHeadingRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics when headings have content between them, got %d", len(diags))
	}
}

// ---------------------------------------------------------------------------
// FMT-003 Tests
// ---------------------------------------------------------------------------

func TestFMT003_TableIntegrity(t *testing.T) {
	// Row 1 has 3 cells, row 2 has 2 cells — inconsistent
	tbl := tableNode([][]string{
		{"A", "B", "C"},
		{"X", "Y"},
	})
	nodes := []docx.ViewNode{tbl}
	rule := tableIntegrityRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected warning for inconsistent column count")
	}
	if diags[0].RuleID != "FMT-003" {
		t.Errorf("expected RuleID FMT-003, got %s", diags[0].RuleID)
	}
}

func TestFMT003_ConsistentTable(t *testing.T) {
	tbl := tableNode([][]string{
		{"A", "B"},
		{"C", "D"},
		{"E", "F"},
	})
	nodes := []docx.ViewNode{tbl}
	rule := tableIntegrityRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for consistent table, got %d", len(diags))
	}
}

// ---------------------------------------------------------------------------
// FMT-004 Tests
// ---------------------------------------------------------------------------

func TestFMT004_ImageRefMissing(t *testing.T) {
	// Package has rId2 in rels, but image node uses rId99 (missing)
	pkg := createTestDocxWithRels(t, []string{"rId2"})
	nodes := []docx.ViewNode{imageNode("rId99")}
	rule := imageReferenceRule{}
	diags := rule.Check(pkg, nodes)
	if len(diags) == 0 {
		t.Fatal("expected error for missing rId in rels")
	}
	if diags[0].Level != DiagError {
		t.Errorf("expected DiagError, got %s", diags[0].Level)
	}
	if diags[0].RuleID != "FMT-004" {
		t.Errorf("expected RuleID FMT-004, got %s", diags[0].RuleID)
	}
}

func TestFMT004_ImageRefPresent(t *testing.T) {
	pkg := createTestDocxWithRels(t, []string{"rId2"})
	nodes := []docx.ViewNode{imageNode("rId2")}
	rule := imageReferenceRule{}
	diags := rule.Check(pkg, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics when rId exists in rels, got %d", len(diags))
	}
}

// ---------------------------------------------------------------------------
// CNT-001 Tests
// ---------------------------------------------------------------------------

func TestCNT001_MissingSections(t *testing.T) {
	// paper requires: 摘要, 引言, 方法, 结论 — only provide 摘要 and 引言
	nodes := []docx.ViewNode{
		heading(1, "摘要"),
		para("摘要内容"),
		heading(1, "引言"),
		para("引言内容"),
		// 方法 and 结论 are missing
	}
	rule := &requiredSectionsRule{}
	rule.SetDocType("paper")
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected errors for missing required sections (方法, 结论)")
	}
	// Should get 2 errors: 方法 and 结论
	if len(diags) != 2 {
		t.Errorf("expected 2 missing section errors, got %d", len(diags))
	}
	for _, d := range diags {
		if d.Level != DiagError {
			t.Errorf("expected DiagError for missing section, got %s", d.Level)
		}
	}
}

func TestCNT001_AllSectionsPresent(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "摘要"),
		para("摘要文字"),
		heading(1, "引言"),
		para("引言文字"),
		heading(1, "方法"),
		para("方法文字"),
		heading(1, "结论"),
		para("结论文字"),
	}
	rule := &requiredSectionsRule{}
	rule.SetDocType("paper")
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics when all sections present, got %d", len(diags))
	}
}

func TestCNT001_EnglishAliases(t *testing.T) {
	// English headings should also satisfy the paper requirement
	nodes := []docx.ViewNode{
		heading(1, "Abstract"),
		para("Abstract text here."),
		heading(1, "Introduction"),
		para("Introduction text."),
		heading(1, "Methods"),
		para("Methods text."),
		heading(1, "Conclusions"),
		para("Conclusion text."),
	}
	rule := &requiredSectionsRule{}
	rule.SetDocType("paper")
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for English aliases, got %d", len(diags))
	}
}

// ---------------------------------------------------------------------------
// CNT-002 Tests
// ---------------------------------------------------------------------------

func TestCNT002_FigureNumberGap(t *testing.T) {
	nodes := []docx.ViewNode{
		para("如图 1 所示，实验结果良好"),
		para("从图 3 可以看出结论"),       // gap: skips 图 2
	}
	rule := figureNumberingRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected warning for figure number gap (1 → 3)")
	}
	if diags[0].RuleID != "CNT-002" {
		t.Errorf("expected RuleID CNT-002, got %s", diags[0].RuleID)
	}
}

func TestCNT002_SequentialFigures(t *testing.T) {
	nodes := []docx.ViewNode{
		para("See Figure 1 for results."),
		para("Figure 2 shows the comparison."),
		para("Figure 3 is the conclusion chart."),
	}
	rule := figureNumberingRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for sequential figure numbers, got %d", len(diags))
	}
}

// ---------------------------------------------------------------------------
// CNT-003 Tests
// ---------------------------------------------------------------------------

func TestCNT003_AbstractTooShort(t *testing.T) {
	// Abstract with fewer than 100 chars/words
	nodes := []docx.ViewNode{
		heading(1, "摘要"),
		para("短摘要。"), // way under 100
	}
	rule := abstractLengthRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected info diagnostic for abstract too short")
	}
	if diags[0].Level != DiagInfo {
		t.Errorf("expected DiagInfo, got %s", diags[0].Level)
	}
	if diags[0].RuleID != "CNT-003" {
		t.Errorf("expected RuleID CNT-003, got %s", diags[0].RuleID)
	}
}

func TestCNT003_AbstractGoodLength(t *testing.T) {
	// Build a paragraph with exactly 150 CJK characters
	content := make([]rune, 150)
	for i := range content {
		content[i] = '文'
	}
	nodes := []docx.ViewNode{
		heading(1, "Abstract"),
		para(string(content)),
	}
	rule := abstractLengthRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for 150-char abstract, got %d", len(diags))
	}
}

func TestCNT003_NoAbstractSection(t *testing.T) {
	// No abstract heading → rule should skip silently
	nodes := []docx.ViewNode{
		heading(1, "Introduction"),
		para("Some intro text."),
	}
	rule := abstractLengthRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics when no abstract heading present, got %d", len(diags))
	}
}

// ---------------------------------------------------------------------------
// CNT-004 Tests
// ---------------------------------------------------------------------------

func TestCNT004_PlaceholderResidue(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "{{title}}"),
		para("请在 {{author}} 处填写作者姓名。"),
		para("Normal paragraph without placeholders."),
	}
	rule := placeholderResidueRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 2 {
		t.Errorf("expected 2 placeholder diagnostics, got %d", len(diags))
	}
	for _, d := range diags {
		if d.Level != DiagError {
			t.Errorf("expected DiagError for placeholder, got %s", d.Level)
		}
		if d.RuleID != "CNT-004" {
			t.Errorf("expected RuleID CNT-004, got %s", d.RuleID)
		}
	}
}

func TestCNT004_NoPlaceholders(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "Title"),
		para("This document has no placeholders."),
	}
	rule := placeholderResidueRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics when no placeholders present, got %d", len(diags))
	}
}

// ---------------------------------------------------------------------------
// Integration Tests
// ---------------------------------------------------------------------------

func TestValidateAll(t *testing.T) {
	// Document with multiple issues at once:
	// - H1 → H3 (FMT-001)
	// - Two consecutive headings with no content (FMT-002)
	// - Placeholder residue (CNT-004)
	// - Figure number gap (CNT-002)
	nodes := []docx.ViewNode{
		heading(1, "Introduction"),
		heading(3, "Detail"),      // FMT-001: jump from H1 to H3
		heading(4, "Sub-detail"),  // FMT-002: consecutive headings; also FMT-001: H3→H4 is fine
		para("如图 1 所示"),
		para("如图 3 所示"), // CNT-002: gap
		para("作者: {{author}}"), // CNT-004: placeholder
	}

	v := NewValidator(DefaultRules()...)
	diags := v.Validate(nil, nodes, "")

	if !hasDiag(diags, "FMT-001", DiagWarning) {
		t.Error("expected FMT-001 warning")
	}
	if !hasDiag(diags, "FMT-002", DiagWarning) {
		t.Error("expected FMT-002 warning")
	}
	if !hasDiag(diags, "CNT-002", DiagWarning) {
		t.Error("expected CNT-002 warning")
	}
	if !hasDiag(diags, "CNT-004", DiagError) {
		t.Error("expected CNT-004 error")
	}

	// Verify sort order: errors before warnings before info
	for i := 1; i < len(diags); i++ {
		if diags[i-1].Level > diags[i].Level {
			t.Errorf("diagnostics not sorted by severity at index %d: %s > %s",
				i, diags[i-1].Level, diags[i].Level)
		}
	}
}

func TestValidateFilterByDocType(t *testing.T) {
	// For docType="patent", CNT-001 should require: 权利要求书, 说明书, 摘要
	// Provide only 摘要 — should get 2 missing section errors (权利要求书, 说明书)
	nodes := []docx.ViewNode{
		heading(1, "摘要"),
		para("摘要内容"),
	}

	v := NewValidator(DefaultRules()...)
	diags := v.Validate(nil, nodes, "patent")

	cnt001Count := countDiags(diags, "CNT-001")
	if cnt001Count != 2 {
		t.Errorf("for docType=patent with only 摘要, expected 2 CNT-001 errors, got %d", cnt001Count)
	}

	// For docType="paper", different sections are required
	nodes2 := []docx.ViewNode{
		heading(1, "摘要"),
		para("摘要内容"),
	}
	diags2 := v.Validate(nil, nodes2, "paper")
	cnt001Count2 := countDiags(diags2, "CNT-001")
	// paper requires: 摘要, 引言, 方法, 结论 — only 摘要 present → 3 missing
	if cnt001Count2 != 3 {
		t.Errorf("for docType=paper with only 摘要, expected 3 CNT-001 errors, got %d", cnt001Count2)
	}
}

func TestValidateDocTypeEmpty_SkipsCNT001(t *testing.T) {
	// With empty docType, CNT-001 should NOT trigger
	nodes := []docx.ViewNode{
		heading(1, "Random Section"),
		para("Content here."),
	}
	v := NewValidator(DefaultRules()...)
	diags := v.Validate(nil, nodes, "")
	if countDiags(diags, "CNT-001") > 0 {
		t.Error("CNT-001 should not trigger when docType is empty")
	}
}
