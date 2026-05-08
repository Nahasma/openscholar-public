package validate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/openscholar/openscholar/internal/docx"
)

// DomainRules returns all P1 domain-specific rules.
func DomainRules() []Rule {
	return []Rule{
		// Patent rules
		patentClaimNumberRule{},
		patentDependentClaimRule{},
		patentTermConsistencyRule{},
		// Paper rules
		paperAbstractLengthRule{},
		paperFigureReferenceRule{},
		paperBibFormatRule{},
		// Proposal rules
		proposalBudgetConsistencyRule{},
		proposalRequiredFieldsRule{},
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// claimsSection finds the section after a 「权利要求书」 heading and returns
// the paragraph nodes that belong to it (until the next heading).
func claimsSection(nodes []docx.ViewNode) []docx.ViewNode {
	inClaims := false
	var out []docx.ViewNode
	for _, n := range nodes {
		if n.Type == docx.NodeHeading {
			if strings.Contains(n.Content, "权利要求") {
				inClaims = true
				continue
			}
			if inClaims {
				break
			}
		}
		if inClaims && (n.Type == docx.NodeParagraph || n.Type == docx.NodeListItem) {
			out = append(out, n)
		}
	}
	return out
}

// descriptionSection returns nodes that belong to the 「说明书」 section.
func descriptionSection(nodes []docx.ViewNode) []docx.ViewNode {
	inDesc := false
	var out []docx.ViewNode
	for _, n := range nodes {
		if n.Type == docx.NodeHeading {
			content := strings.TrimSpace(n.Content)
			if strings.Contains(content, "说明书") || content == "具体实施方式" || content == "技术领域" || content == "背景技术" || content == "发明内容" {
				inDesc = true
				continue
			}
			// Stop at 权利要求书
			if strings.Contains(content, "权利要求") {
				inDesc = false
			}
		}
		if inDesc {
			out = append(out, n)
		}
	}
	return out
}

// reClaimNumber matches a leading claim number like "1." "2、" "3．"
var reClaimNumber = regexp.MustCompile(`^(\d+)[.、．]`)

// reDependentRef matches "根据权利要求X" or "如权利要求X所述"
var reDependentRef = regexp.MustCompile(`(?:根据权利要求|如权利要求)\s*(\d+)`)

// ---------------------------------------------------------------------------
// PAT-001: 权利要求编号递增连续
// ---------------------------------------------------------------------------

type patentClaimNumberRule struct{}

func (r patentClaimNumberRule) ID() string                     { return "PAT-001" }
func (r patentClaimNumberRule) Name() string                   { return "权利要求编号递增连续" }
func (r patentClaimNumberRule) Level() DiagLevel               { return DiagError }
func (r patentClaimNumberRule) Applicable(docType string) bool { return docType == "patent" }

func (r patentClaimNumberRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic
	claims := claimsSection(nodes)
	expected := 1
	for _, n := range claims {
		m := reClaimNumber.FindStringSubmatch(strings.TrimSpace(n.Content))
		if m == nil {
			continue
		}
		num, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if num != expected {
			diags = append(diags, Diagnostic{
				Level:      DiagError,
				RuleID:     r.ID(),
				Location:   n.Anchor,
				Message:    fmt.Sprintf("权利要求编号不连续: 期望 %d, 实际 %d", expected, num),
				Suggestion: fmt.Sprintf("将权利要求编号修正为 %d 或补充缺失的权利要求 %d", num, expected),
			})
			// Advance expected to keep checking subsequent claims.
			expected = num + 1
		} else {
			expected++
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// PAT-002: 从属权利要求引用在先权利要求
// ---------------------------------------------------------------------------

type patentDependentClaimRule struct{}

func (r patentDependentClaimRule) ID() string                     { return "PAT-002" }
func (r patentDependentClaimRule) Name() string                   { return "从属权利要求引用在先权利要求" }
func (r patentDependentClaimRule) Level() DiagLevel               { return DiagError }
func (r patentDependentClaimRule) Applicable(docType string) bool { return docType == "patent" }

func (r patentDependentClaimRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic
	claims := claimsSection(nodes)
	for _, n := range claims {
		content := strings.TrimSpace(n.Content)
		m := reClaimNumber.FindStringSubmatch(content)
		if m == nil {
			continue
		}
		currentNum, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		// Find all dependency references in this claim.
		refs := reDependentRef.FindAllStringSubmatch(content, -1)
		for _, ref := range refs {
			refNum, err := strconv.Atoi(ref[1])
			if err != nil {
				continue
			}
			if refNum >= currentNum {
				diags = append(diags, Diagnostic{
					Level:    DiagError,
					RuleID:   r.ID(),
					Location: n.Anchor,
					Message: fmt.Sprintf(
						"权利要求 %d 引用了权利要求 %d，从属引用必须引用编号更小的在先权利要求",
						currentNum, refNum),
					Suggestion: fmt.Sprintf("将引用修改为编号小于 %d 的权利要求", currentNum),
				})
			}
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// PAT-003: 术语与说明书一致
// ---------------------------------------------------------------------------

type patentTermConsistencyRule struct{}

func (r patentTermConsistencyRule) ID() string                     { return "PAT-003" }
func (r patentTermConsistencyRule) Name() string                   { return "权利要求术语与说明书一致" }
func (r patentTermConsistencyRule) Level() DiagLevel               { return DiagWarning }
func (r patentTermConsistencyRule) Applicable(docType string) bool { return docType == "patent" }

// reCamelCase matches CamelCase English words (two or more capital-led runs).
var reCamelCase = regexp.MustCompile(`[A-Z][a-z]+[A-Z][a-zA-Z]+`)

func extractTerms(nodes []docx.ViewNode) []string {
	seen := map[string]bool{}
	var terms []string
	for _, n := range nodes {
		// Extract Chinese noun phrases: sequences of CJK chars with len >= 4.
		content := n.Content
		var cjkRun []rune
		flush := func() {
			if len(cjkRun) >= 2 {
				// Produce all substrings of length 2..len as candidate terms,
				// but only keep those >= 4 chars.
				s := string(cjkRun)
				runeLen := utf8.RuneCountInString(s)
				if runeLen >= 2 {
					// Simple: use whole run as term if >= 2 runes.
					for start := 0; start <= runeLen-2; start++ {
						for end := start + 2; end <= runeLen; end++ {
							runes := []rune(s)
							sub := string(runes[start:end])
							if utf8.RuneCountInString(sub) >= 2 && !seen[sub] {
								seen[sub] = true
								terms = append(terms, sub)
							}
						}
					}
				}
			}
			cjkRun = cjkRun[:0]
		}
		for _, ch := range content {
			if unicode.Is(unicode.Han, ch) {
				cjkRun = append(cjkRun, ch)
			} else {
				flush()
			}
		}
		flush()
		// Extract CamelCase English terms.
		for _, m := range reCamelCase.FindAllString(content, -1) {
			if !seen[m] {
				seen[m] = true
				terms = append(terms, m)
			}
		}
	}
	return terms
}

func (r patentTermConsistencyRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic
	claims := claimsSection(nodes)
	desc := descriptionSection(nodes)

	// Build a combined text of description for fast lookup.
	var descBuf strings.Builder
	for _, n := range desc {
		descBuf.WriteString(n.Content)
		descBuf.WriteRune('\n')
	}
	descText := descBuf.String()

	// Build anchor map for claim nodes by content prefix.
	claimAnchorMap := map[string]docx.StableAnchor{}
	for _, n := range claims {
		claimAnchorMap[n.Content] = n.Anchor
	}

	// Extract terms from claims and check presence in description.
	terms := extractTerms(claims)
	for _, term := range terms {
		if !strings.Contains(descText, term) {
			// Find any claim anchor to report against.
			anchor := docx.StableAnchor{}
			for _, n := range claims {
				if strings.Contains(n.Content, term) {
					anchor = n.Anchor
					break
				}
			}
			diags = append(diags, Diagnostic{
				Level:      DiagWarning,
				RuleID:     r.ID(),
				Location:   anchor,
				Message:    fmt.Sprintf("权利要求术语 %q 在说明书中未出现，可能不一致", term),
				Suggestion: "确保说明书中使用相同术语，或在说明书中添加对该术语的定义",
			})
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// PPR-001: 摘要 150-300 字
// ---------------------------------------------------------------------------

type paperAbstractLengthRule struct{}

func (r paperAbstractLengthRule) ID() string                     { return "PPR-001" }
func (r paperAbstractLengthRule) Name() string                   { return "摘要字数 150-300 字" }
func (r paperAbstractLengthRule) Level() DiagLevel               { return DiagWarning }
func (r paperAbstractLengthRule) Applicable(docType string) bool { return docType == "paper" }

// countWords counts words: each CJK character counts as 1; English words
// (space-separated tokens) count as 1 per token.
func countWords(text string) int {
	count := 0
	inEnglishWord := false
	for _, ch := range text {
		if unicode.Is(unicode.Han, ch) {
			if inEnglishWord {
				count++ // flush previous English word
				inEnglishWord = false
			}
			count++
		} else if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			inEnglishWord = true
		} else {
			if inEnglishWord {
				count++
				inEnglishWord = false
			}
		}
	}
	if inEnglishWord {
		count++
	}
	return count
}

func (r paperAbstractLengthRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic
	inAbstract := false
	for _, n := range nodes {
		if n.Type == docx.NodeHeading {
			content := strings.TrimSpace(n.Content)
			if content == "Abstract" || content == "摘要" {
				inAbstract = true
				continue
			}
			if inAbstract {
				break // end of abstract section
			}
		}
		if inAbstract && n.Type == docx.NodeParagraph {
			wordCount := countWords(n.Content)
			if wordCount < 150 || wordCount > 300 {
				diags = append(diags, Diagnostic{
					Level:      DiagWarning,
					RuleID:     r.ID(),
					Location:   n.Anchor,
					Message:    fmt.Sprintf("摘要字数为 %d 字，应在 150-300 字之间", wordCount),
					Suggestion: "调整摘要长度至 150-300 字范围内",
				})
			}
			break // only check the first paragraph of the abstract
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// PPR-002: 图表必须被正文引用
// ---------------------------------------------------------------------------

type paperFigureReferenceRule struct{}

func (r paperFigureReferenceRule) ID() string                     { return "PPR-002" }
func (r paperFigureReferenceRule) Name() string                   { return "图表必须被正文引用" }
func (r paperFigureReferenceRule) Level() DiagLevel               { return DiagWarning }
func (r paperFigureReferenceRule) Applicable(docType string) bool { return docType == "paper" }

// reFigureLabel matches "图1" "Figure 1" "表1" "Table 1" etc.
var reFigureLabel = regexp.MustCompile(`(?:图|Figure\s+|表|Table\s+)(\d+)`)

func (r paperFigureReferenceRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic

	type figInfo struct {
		label  string
		anchor docx.StableAnchor
	}

	// First pass: collect all figure/table labels and their definition nodes.
	// A "definition" node is typically an image node or a paragraph containing
	// "图X" as the sole/primary identifier (caption-style).
	var figures []figInfo
	figureDefNodeIdx := map[string]int{} // label -> node index

	for i, n := range nodes {
		matches := reFigureLabel.FindAllStringSubmatch(n.Content, -1)
		for _, m := range matches {
			label := m[0] // full match, e.g. "图1" or "Figure 1"
			// Normalize: remove spaces for comparison key.
			key := strings.ReplaceAll(label, " ", "")
			if _, exists := figureDefNodeIdx[key]; !exists {
				figureDefNodeIdx[key] = i
				figures = append(figures, figInfo{label: key, anchor: n.Anchor})
			}
		}
		// Also check children (tables have rows).
		for _, child := range n.Children {
			for _, row := range child.Children {
				childMatches := reFigureLabel.FindAllStringSubmatch(row.Content, -1)
				for _, m := range childMatches {
					key := strings.ReplaceAll(m[0], " ", "")
					if _, exists := figureDefNodeIdx[key]; !exists {
						figureDefNodeIdx[key] = i
						figures = append(figures, figInfo{label: key, anchor: n.Anchor})
					}
				}
			}
		}
	}

	if len(figures) == 0 {
		return diags
	}

	// Build a full text corpus of all node content for reference checking.
	// A figure is "referenced" if its label appears in nodes OTHER than the
	// node where it was first identified.
	for _, fig := range figures {
		defIdx := figureDefNodeIdx[fig.label]
		referenced := false
		for i, n := range nodes {
			if i == defIdx {
				continue
			}
			normalized := strings.ReplaceAll(n.Content, " ", "")
			if strings.Contains(normalized, fig.label) {
				referenced = true
				break
			}
		}
		if !referenced {
			anchor := nodes[defIdx].Anchor
			diags = append(diags, Diagnostic{
				Level:      DiagWarning,
				RuleID:     r.ID(),
				Location:   anchor,
				Message:    fmt.Sprintf("图表 %q 在正文中未被引用", fig.label),
				Suggestion: fmt.Sprintf("在正文适当位置添加对 %q 的引用，如「如%s所示」", fig.label, fig.label),
			})
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// PPR-003: 参考文献格式合规
// ---------------------------------------------------------------------------

type paperBibFormatRule struct{}

func (r paperBibFormatRule) ID() string                     { return "PPR-003" }
func (r paperBibFormatRule) Name() string                   { return "参考文献格式合规" }
func (r paperBibFormatRule) Level() DiagLevel               { return DiagInfo }
func (r paperBibFormatRule) Applicable(docType string) bool { return docType == "paper" }

var reBibEntry = regexp.MustCompile(`^\[(\d+)\]`)

func (r paperBibFormatRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic
	inRefs := false
	expectedNum := 1
	for _, n := range nodes {
		if n.Type == docx.NodeHeading {
			content := strings.TrimSpace(n.Content)
			if content == "References" || content == "参考文献" {
				inRefs = true
				continue
			}
			if inRefs {
				break
			}
		}
		if !inRefs {
			continue
		}
		if n.Type != docx.NodeParagraph && n.Type != docx.NodeListItem {
			continue
		}
		content := strings.TrimSpace(n.Content)
		if content == "" {
			continue
		}
		m := reBibEntry.FindStringSubmatch(content)
		if m == nil {
			diags = append(diags, Diagnostic{
				Level:      DiagInfo,
				RuleID:     r.ID(),
				Location:   n.Anchor,
				Message:    fmt.Sprintf("参考文献条目格式不合规，应以 [N] 编号开头: %q", truncate(content, 50)),
				Suggestion: "将参考文献格式改为 [1] 作者. 标题. 期刊. 年份. 的标准格式",
			})
			continue
		}
		num, err := strconv.Atoi(m[1])
		if err == nil && num != expectedNum {
			diags = append(diags, Diagnostic{
				Level:      DiagInfo,
				RuleID:     r.ID(),
				Location:   n.Anchor,
				Message:    fmt.Sprintf("参考文献编号不连续: 期望 [%d], 实际 [%d]", expectedNum, num),
				Suggestion: fmt.Sprintf("将编号修正为 [%d] 或调整引用顺序", expectedNum),
			})
			expectedNum = num + 1
		} else {
			expectedNum++
		}
	}
	return diags
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// ---------------------------------------------------------------------------
// PRP-001: 预算总额 = 分项之和
// ---------------------------------------------------------------------------

type proposalBudgetConsistencyRule struct{}

func (r proposalBudgetConsistencyRule) ID() string                     { return "PRP-001" }
func (r proposalBudgetConsistencyRule) Name() string                   { return "预算总额与分项之和一致" }
func (r proposalBudgetConsistencyRule) Level() DiagLevel               { return DiagError }
func (r proposalBudgetConsistencyRule) Applicable(docType string) bool { return docType == "proposal" }

// reNumber extracts the first number (int or float) from a string.
var reNumber = regexp.MustCompile(`(\d+(?:\.\d+)?)`)

func parseFirstNumber(s string) (float64, bool) {
	m := reNumber.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// isTotalRow returns true if the row content contains 合计/总计 keywords.
func isTotalRow(cells []docx.ViewNode) bool {
	for _, c := range cells {
		if strings.Contains(c.Content, "合计") || strings.Contains(c.Content, "总计") || strings.Contains(c.Content, "总额") {
			return true
		}
	}
	return false
}

func (r proposalBudgetConsistencyRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic

	inBudget := false
	for _, n := range nodes {
		if n.Type == docx.NodeHeading {
			content := strings.TrimSpace(n.Content)
			if strings.Contains(content, "经费预算") || strings.Contains(content, "预算") {
				inBudget = true
				continue
			}
			if inBudget {
				inBudget = false
			}
		}
		if !inBudget || n.Type != docx.NodeTable {
			continue
		}
		// Found a budget table. Extract rows.
		var totalRow []docx.ViewNode
		var itemRows [][]docx.ViewNode
		for _, row := range n.Children {
			if isTotalRow(row.Children) {
				totalRow = row.Children
			} else {
				itemRows = append(itemRows, row.Children)
			}
		}
		if totalRow == nil {
			continue
		}

		// Extract total value from last numeric cell of totalRow.
		totalVal, hasTotalVal := extractLastNumber(totalRow)
		if !hasTotalVal {
			continue
		}

		// Sum up item rows.
		var sum float64
		for _, rowCells := range itemRows {
			v, ok := extractLastNumber(rowCells)
			if ok {
				sum += v
			}
		}

		// Allow 1e-6 float tolerance.
		diff := totalVal - sum
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.01 {
			diags = append(diags, Diagnostic{
				Level:    DiagError,
				RuleID:   r.ID(),
				Location: n.Anchor,
				Message: fmt.Sprintf(
					"预算合计 %.2f 与分项之和 %.2f 不一致，差额 %.2f",
					totalVal, sum, totalVal-sum),
				Suggestion: "核对各分项经费并确保合计行等于分项之和",
			})
		}
	}
	return diags
}

// extractLastNumber finds the last cell in a row that contains a number and returns it.
func extractLastNumber(cells []docx.ViewNode) (float64, bool) {
	for i := len(cells) - 1; i >= 0; i-- {
		v, ok := parseFirstNumber(cells[i].Content)
		if ok {
			return v, true
		}
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// PRP-002: 必填字段完整性
// ---------------------------------------------------------------------------

type proposalRequiredFieldsRule struct{}

func (r proposalRequiredFieldsRule) ID() string                     { return "PRP-002" }
func (r proposalRequiredFieldsRule) Name() string                   { return "必填字段完整性" }
func (r proposalRequiredFieldsRule) Level() DiagLevel               { return DiagError }
func (r proposalRequiredFieldsRule) Applicable(docType string) bool { return docType == "proposal" }

var requiredFields = []string{
	"项目概述",
	"技术路线",
	"预期成果",
	"经费预算",
	"研究基础",
}

func (r proposalRequiredFieldsRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic

	// Collect all heading texts.
	headings := make([]string, 0, 16)
	for _, n := range nodes {
		if n.Type == docx.NodeHeading {
			headings = append(headings, strings.TrimSpace(n.Content))
		}
	}

	for _, field := range requiredFields {
		found := false
		for _, h := range headings {
			if strings.Contains(h, field) {
				found = true
				break
			}
		}
		if !found {
			diags = append(diags, Diagnostic{
				Level:      DiagError,
				RuleID:     r.ID(),
				Location:   docx.StableAnchor{},
				Message:    fmt.Sprintf("申报书缺少必填章节: %q", field),
				Suggestion: fmt.Sprintf("添加标题为 %q 的章节并填写相关内容", field),
			})
		}
	}
	return diags
}
