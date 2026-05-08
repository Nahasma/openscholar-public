package validate

import (
	"fmt"

	"github.com/openscholar/openscholar/internal/docx"
)

// FormatRules returns all P0 format rules.
func FormatRules() []Rule {
	return []Rule{
		headingContinuityRule{},
		emptyAfterHeadingRule{},
		tableIntegrityRule{},
		imageReferenceRule{},
	}
}

// ---------------------------------------------------------------------------
// FMT-001: 标题层级连续性
// Heading 级别不能跳级（如 H1 直接到 H3 而中间没有 H2）
// ---------------------------------------------------------------------------

type headingContinuityRule struct{}

func (r headingContinuityRule) ID() string                     { return "FMT-001" }
func (r headingContinuityRule) Name() string                   { return "标题层级连续性" }
func (r headingContinuityRule) Level() DiagLevel               { return DiagWarning }
func (r headingContinuityRule) Applicable(docType string) bool { return true }

func (r headingContinuityRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic
	lastLevel := 0
	for _, n := range nodes {
		if n.Type == docx.NodeHeading {
			if lastLevel > 0 && n.Level > lastLevel+1 {
				diags = append(diags, Diagnostic{
					Level:      DiagWarning,
					RuleID:     r.ID(),
					Location:   n.Anchor,
					Message:    fmt.Sprintf("标题层级跳跃: H%d → H%d", lastLevel, n.Level),
					Suggestion: fmt.Sprintf("补充 H%d 层级标题或调整当前标题层级", lastLevel+1),
				})
			}
			lastLevel = n.Level
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// FMT-002: 空段落检查
// 相邻两个 Heading 中间无 NodeParagraph 内容
// ---------------------------------------------------------------------------

type emptyAfterHeadingRule struct{}

func (r emptyAfterHeadingRule) ID() string                     { return "FMT-002" }
func (r emptyAfterHeadingRule) Name() string                   { return "空段落检查" }
func (r emptyAfterHeadingRule) Level() DiagLevel               { return DiagWarning }
func (r emptyAfterHeadingRule) Applicable(docType string) bool { return true }

func (r emptyAfterHeadingRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic
	prevHeadingIdx := -1
	for i, n := range nodes {
		if n.Type == docx.NodeHeading {
			if prevHeadingIdx >= 0 {
				// Check if there is any non-heading content between prevHeadingIdx and i
				hasContent := false
				for k := prevHeadingIdx + 1; k < i; k++ {
					if nodes[k].Type != docx.NodeHeading {
						hasContent = true
						break
					}
				}
				if !hasContent {
					diags = append(diags, Diagnostic{
						Level:      DiagWarning,
						RuleID:     r.ID(),
						Location:   nodes[prevHeadingIdx].Anchor,
						Message:    fmt.Sprintf("标题 %q 后无正文内容，紧跟下一个标题", nodes[prevHeadingIdx].Content),
						Suggestion: "在两个标题之间添加正文段落，或合并相关标题",
					})
				}
			}
			prevHeadingIdx = i
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// FMT-003: 表格完整性
// 表格行列数一致（所有行 Children 数量相同），无空行
// ---------------------------------------------------------------------------

type tableIntegrityRule struct{}

func (r tableIntegrityRule) ID() string                     { return "FMT-003" }
func (r tableIntegrityRule) Name() string                   { return "表格完整性" }
func (r tableIntegrityRule) Level() DiagLevel               { return DiagWarning }
func (r tableIntegrityRule) Applicable(docType string) bool { return true }

func (r tableIntegrityRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic
	for _, n := range nodes {
		if n.Type != docx.NodeTable {
			continue
		}
		if len(n.Children) == 0 {
			continue
		}
		// Determine expected column count from the first row.
		expectedCols := len(n.Children[0].Children)
		for rowIdx, row := range n.Children {
			cols := len(row.Children)
			// Empty row check
			if cols == 0 {
				diags = append(diags, Diagnostic{
					Level:      DiagWarning,
					RuleID:     r.ID(),
					Location:   n.Anchor,
					Message:    fmt.Sprintf("表格第 %d 行为空行", rowIdx+1),
					Suggestion: "删除空行或填充内容",
				})
				continue
			}
			// Inconsistent column count
			if cols != expectedCols {
				diags = append(diags, Diagnostic{
					Level:      DiagWarning,
					RuleID:     r.ID(),
					Location:   n.Anchor,
					Message:    fmt.Sprintf("表格列数不一致: 第 1 行 %d 列，第 %d 行 %d 列", expectedCols, rowIdx+1, cols),
					Suggestion: "确保所有行的列数一致，检查合并单元格",
				})
			}
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// FMT-004: 图片引用完整性
// NodeImage 的 Metadata["rId"] 在 word/document.xml.rels 中存在
// ---------------------------------------------------------------------------

type imageReferenceRule struct{}

func (r imageReferenceRule) ID() string                     { return "FMT-004" }
func (r imageReferenceRule) Name() string                   { return "图片引用完整性" }
func (r imageReferenceRule) Level() DiagLevel               { return DiagError }
func (r imageReferenceRule) Applicable(docType string) bool { return true }

func (r imageReferenceRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic

	// Load relations once (or use nil pkg gracefully)
	var rels *docx.Relations
	if pkg != nil {
		var err error
		rels, err = docx.LoadRelations(pkg, "word/document.xml")
		if err != nil {
			// If we can't load relations, skip this rule
			return diags
		}
	}

	for _, n := range nodes {
		diags = append(diags, r.checkNode(n, rels)...)
	}
	return diags
}

func (r imageReferenceRule) checkNode(n docx.ViewNode, rels *docx.Relations) []Diagnostic {
	var diags []Diagnostic
	if n.Type == docx.NodeImage {
		rID := ""
		if n.Metadata != nil {
			rID = n.Metadata["rId"]
		}
		if rID == "" {
			diags = append(diags, Diagnostic{
				Level:      DiagError,
				RuleID:     r.ID(),
				Location:   n.Anchor,
				Message:    "图片节点缺少 rId 引用",
				Suggestion: "检查文档中的图片嵌入方式，确保关系 ID 正确",
			})
		} else if rels != nil {
			if _, ok := rels.Get(rID); !ok {
				diags = append(diags, Diagnostic{
					Level:      DiagError,
					RuleID:     r.ID(),
					Location:   n.Anchor,
					Message:    fmt.Sprintf("图片引用 %q 在关系文件中不存在", rID),
					Suggestion: "重新插入图片或修复 word/document.xml.rels 中的关系定义",
				})
			}
		}
	}
	// Also check children (e.g., images nested inside paragraphs)
	for _, child := range n.Children {
		diags = append(diags, r.checkNode(child, rels)...)
	}
	return diags
}
