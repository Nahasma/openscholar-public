package validate

import (
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/docx"
)

// ---------------------------------------------------------------------------
// local helpers (non-conflicting with validator_test.go helpers)
// ---------------------------------------------------------------------------

// listItem creates a NodeListItem ViewNode.
func listItemNode(content string) docx.ViewNode {
	return docx.ViewNode{Type: docx.NodeListItem, Content: content}
}

// diagHasLevel returns true if any diagnostic has the given level.
func diagHasLevel(diags []Diagnostic, level DiagLevel) bool {
	for _, d := range diags {
		if d.Level == level {
			return true
		}
	}
	return false
}

// diagHasMessage returns true if any diagnostic message contains substr.
func diagHasMessage(diags []Diagnostic, substr string) bool {
	for _, d := range diags {
		if strings.Contains(d.Message, substr) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// PAT-001: 权利要求编号递增连续
// ---------------------------------------------------------------------------

// TestPAT001_ClaimNumberGap: claims 1, 2, 4 (skips 3) → DiagError
func TestPAT001_ClaimNumberGap(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "权利要求书"),
		para("1. 一种装置，包括A。"),
		para("2. 根据权利要求1所述的装置，还包括B。"),
		para("4. 根据权利要求1所述的装置，还包括C。"), // gap: 3 is missing
	}
	rule := patentClaimNumberRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected error for gap in claim numbering, got none")
	}
	if !diagHasLevel(diags, DiagError) {
		t.Errorf("expected DiagError, got: %+v", diags)
	}
	if !diagHasMessage(diags, "4") {
		t.Errorf("expected message mentioning '4', got: %v", diags[0].Message)
	}
}

// TestPAT001_ClaimNumberOK: claims 1, 2, 3 → no diagnostics
func TestPAT001_ClaimNumberOK(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "权利要求书"),
		para("1. 一种装置，包括A。"),
		para("2. 根据权利要求1所述的装置，还包括B。"),
		para("3. 根据权利要求1所述的装置，还包括C。"),
	}
	rule := patentClaimNumberRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics, got: %+v", diags)
	}
}

// ---------------------------------------------------------------------------
// PAT-002: 从属权利要求引用在先权利要求
// ---------------------------------------------------------------------------

// TestPAT002_DependentClaimBadRef: claim 2 references claim 5 (forward ref) → DiagError
func TestPAT002_DependentClaimBadRef(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "权利要求书"),
		para("1. 一种方法，包括步骤A。"),
		para("2. 根据权利要求5所述的方法，还包括步骤B。"), // 5 > 2 → error
		para("3. 根据权利要求1所述的方法，还包括步骤C。"),
	}
	rule := patentDependentClaimRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected error for bad dependent claim reference, got none")
	}
	if !diagHasLevel(diags, DiagError) {
		t.Errorf("expected DiagError, got: %+v", diags)
	}
	if !diagHasMessage(diags, "5") {
		t.Errorf("expected message mentioning '5', got: %v", diags[0].Message)
	}
}

// TestPAT002_DependentClaimOK: claim 2 references claim 1 → no diagnostics
func TestPAT002_DependentClaimOK(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "权利要求书"),
		para("1. 一种方法，包括步骤A。"),
		para("2. 根据权利要求1所述的方法，还包括步骤B。"),
	}
	rule := patentDependentClaimRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics, got: %+v", diags)
	}
}

// ---------------------------------------------------------------------------
// PAT-003: 术语与说明书一致
// ---------------------------------------------------------------------------

// TestPAT003_TermInconsistency: 权利要求用「控制模块」但说明书没有 → DiagWarning
func TestPAT003_TermInconsistency(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "说明书"),
		para("本发明涉及一种处理单元，用于执行各种操作。"),
		heading(1, "权利要求书"),
		para("1. 一种控制模块，包括处理器。"), // 「控制模块」 not in description
	}
	rule := patentTermConsistencyRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected warning for term inconsistency, got none")
	}
	if !diagHasLevel(diags, DiagWarning) {
		t.Errorf("expected DiagWarning, got: %+v", diags)
	}
}

// ---------------------------------------------------------------------------
// PPR-001: 摘要字数 150-300
// ---------------------------------------------------------------------------

// TestPPR001_AbstractLength: abstract < 150 words → DiagWarning
func TestPPR001_AbstractLength(t *testing.T) {
	// 45 CJK chars = 45 word-count, well under 150
	shortText := strings.Repeat("短文字", 15)
	nodes := []docx.ViewNode{
		heading(1, "摘要"),
		para(shortText),
		heading(1, "引言"),
	}
	rule := paperAbstractLengthRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected warning for short abstract, got none")
	}
	if !diagHasLevel(diags, DiagWarning) {
		t.Errorf("expected DiagWarning, got: %+v", diags)
	}
}

// TestPPR001_AbstractOK: abstract with 200 CJK chars → no diagnostics
func TestPPR001_AbstractOK(t *testing.T) {
	okText := strings.Repeat("文", 200)
	nodes := []docx.ViewNode{
		heading(1, "摘要"),
		para(okText),
		heading(1, "引言"),
	}
	rule := paperAbstractLengthRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for 200-char abstract, got: %+v", diags)
	}
}

// ---------------------------------------------------------------------------
// PPR-002: 图表必须被正文引用
// ---------------------------------------------------------------------------

// TestPPR002_UnreferencedFigure: 图1 defined but never cited → DiagWarning
func TestPPR002_UnreferencedFigure(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "方法"),
		para("本文使用了新颖的实验设计。"),
		para("图1 实验架构示意图"),   // defines 图1
		para("该方案具有较高的效率。"), // does NOT reference 图1
	}
	rule := paperFigureReferenceRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected warning for unreferenced figure, got none")
	}
	if !diagHasLevel(diags, DiagWarning) {
		t.Errorf("expected DiagWarning, got: %+v", diags)
	}
}

// TestPPR002_FigureReferenced: 图1 referenced in body → no diagnostics
func TestPPR002_FigureReferenced(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "方法"),
		para("如图1所示，该架构包含三个模块。"), // references 图1
		para("图1 实验架构示意图"),          // defines 图1
	}
	rule := paperFigureReferenceRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics when figure is referenced, got: %+v", diags)
	}
}

// ---------------------------------------------------------------------------
// PPR-003: 参考文献格式合规
// ---------------------------------------------------------------------------

// TestPPR003_BibFormat: references without [N] prefix → DiagInfo
func TestPPR003_BibFormat(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "References"),
		para("Smith J. A study on something. Nature. 2020."), // no [1] prefix
		para("Jones A. Another study. Science. 2021."),       // no [2] prefix
	}
	rule := paperBibFormatRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected info for missing bib numbering, got none")
	}
	if !diagHasLevel(diags, DiagInfo) {
		t.Errorf("expected DiagInfo, got: %+v", diags)
	}
}

// TestPPR003_BibFormatOK: [1][2][3] sequential → no diagnostics
func TestPPR003_BibFormatOK(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "参考文献"),
		para("[1] 张三. 深度学习综述. 人工智能学报. 2021."),
		para("[2] 李四. 自然语言处理. ACL. 2022."),
		para("[3] 王五. 计算机视觉. CVPR. 2023."),
	}
	rule := paperBibFormatRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for well-formed references, got: %+v", diags)
	}
}

// ---------------------------------------------------------------------------
// PRP-001: 预算总额 = 分项之和
// ---------------------------------------------------------------------------

// TestPRP001_BudgetMismatch: items sum to 300 but total says 400 → DiagError
func TestPRP001_BudgetMismatch(t *testing.T) {
	budgetTable := tableNode([][]string{
		{"设备费", "100"},
		{"材料费", "200"},
		{"合计", "400"}, // should be 300, mismatch
	})
	nodes := []docx.ViewNode{
		heading(1, "经费预算"),
		budgetTable,
	}
	rule := proposalBudgetConsistencyRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected error for budget mismatch, got none")
	}
	if !diagHasLevel(diags, DiagError) {
		t.Errorf("expected DiagError, got: %+v", diags)
	}
}

// TestPRP001_BudgetOK: items sum == total → no diagnostics
func TestPRP001_BudgetOK(t *testing.T) {
	budgetTable := tableNode([][]string{
		{"设备费", "100"},
		{"材料费", "200"},
		{"合计", "300"},
	})
	nodes := []docx.ViewNode{
		heading(1, "经费预算"),
		budgetTable,
	}
	rule := proposalBudgetConsistencyRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for matching budget, got: %+v", diags)
	}
}

// ---------------------------------------------------------------------------
// PRP-002: 必填字段完整性
// ---------------------------------------------------------------------------

// TestPRP002_MissingFields: 缺少「技术路线」→ DiagError
func TestPRP002_MissingFields(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "项目概述"),
		para("本项目旨在研究AI技术。"),
		// 「技术路线」 is intentionally missing
		heading(1, "预期成果"),
		para("预期发表3篇论文。"),
		heading(1, "经费预算"),
		para("总经费50万元。"),
		heading(1, "研究基础"),
		para("团队已有10年研究经验。"),
	}
	rule := proposalRequiredFieldsRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) == 0 {
		t.Fatal("expected error for missing required field, got none")
	}
	if !diagHasLevel(diags, DiagError) {
		t.Errorf("expected DiagError, got: %+v", diags)
	}
	if !diagHasMessage(diags, "技术路线") {
		t.Errorf("expected message mentioning 技术路线, got: %+v", diags)
	}
}

// TestPRP002_AllFieldsPresent: all required headings present → no diagnostics
func TestPRP002_AllFieldsPresent(t *testing.T) {
	nodes := []docx.ViewNode{
		heading(1, "项目概述"),
		para("..."),
		heading(1, "技术路线"),
		para("..."),
		heading(1, "预期成果"),
		para("..."),
		heading(1, "经费预算"),
		para("..."),
		heading(1, "研究基础"),
		para("..."),
	}
	rule := proposalRequiredFieldsRule{}
	diags := rule.Check(nil, nodes)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics, got: %+v", diags)
	}
}

// ---------------------------------------------------------------------------
// TestDomainRulesApplicability: patent rules don't fire on paper/proposal docs
// ---------------------------------------------------------------------------

func TestDomainRulesApplicability(t *testing.T) {
	rules := DomainRules()

	patentRuleIDs := map[string]bool{
		"PAT-001": true,
		"PAT-002": true,
		"PAT-003": true,
	}
	paperRuleIDs := map[string]bool{
		"PPR-001": true,
		"PPR-002": true,
		"PPR-003": true,
	}
	proposalRuleIDs := map[string]bool{
		"PRP-001": true,
		"PRP-002": true,
	}

	for _, r := range rules {
		id := r.ID()
		switch {
		case patentRuleIDs[id]:
			if r.Applicable("paper") {
				t.Errorf("rule %s should not apply to 'paper' documents", id)
			}
			if r.Applicable("proposal") {
				t.Errorf("rule %s should not apply to 'proposal' documents", id)
			}
			if !r.Applicable("patent") {
				t.Errorf("rule %s should apply to 'patent' documents", id)
			}
		case paperRuleIDs[id]:
			if r.Applicable("patent") {
				t.Errorf("rule %s should not apply to 'patent' documents", id)
			}
			if r.Applicable("proposal") {
				t.Errorf("rule %s should not apply to 'proposal' documents", id)
			}
			if !r.Applicable("paper") {
				t.Errorf("rule %s should apply to 'paper' documents", id)
			}
		case proposalRuleIDs[id]:
			if r.Applicable("patent") {
				t.Errorf("rule %s should not apply to 'patent' documents", id)
			}
			if r.Applicable("paper") {
				t.Errorf("rule %s should not apply to 'paper' documents", id)
			}
			if !r.Applicable("proposal") {
				t.Errorf("rule %s should apply to 'proposal' documents", id)
			}
		default:
			t.Logf("unknown rule ID: %s (skipping applicability check)", id)
		}
	}
}

// TestDomainRulesCount: DomainRules returns exactly 8 rules
func TestDomainRulesCount(t *testing.T) {
	rules := DomainRules()
	if len(rules) != 8 {
		t.Errorf("expected 8 domain rules, got %d", len(rules))
	}
}
