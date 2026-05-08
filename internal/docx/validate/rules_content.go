package validate

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/openscholar/openscholar/internal/docx"
)

// ContentRules returns all P0 content rules.
func ContentRules() []Rule {
	return []Rule{
		&requiredSectionsRule{},
		figureNumberingRule{},
		abstractLengthRule{},
		placeholderResidueRule{},
	}
}

// ---------------------------------------------------------------------------
// CNT-001: 必填章节完整性
// ---------------------------------------------------------------------------

// requiredSectionsMap maps docType → list of required heading texts.
var requiredSectionsMap = map[string][]string{
	"patent":   {"权利要求书", "说明书", "摘要"},
	"paper":    {"摘要", "引言", "方法", "结论"},
	"proposal": {"项目概述", "技术路线", "预期成果", "经费预算"},
}

// englishAliases allows English synonyms for paper section headings.
var englishAliases = map[string][]string{
	"摘要": {"abstract"},
	"引言": {"introduction"},
	"方法": {"method", "methods", "methodology"},
	"结论": {"conclusion", "conclusions"},
}

// requiredSectionsRule implements both Rule and docTypeAware.
type requiredSectionsRule struct {
	docType string
}

func (r *requiredSectionsRule) ID() string                     { return "CNT-001" }
func (r *requiredSectionsRule) Name() string                   { return "必填章节完整性" }
func (r *requiredSectionsRule) Level() DiagLevel               { return DiagError }
func (r *requiredSectionsRule) SetDocType(docType string)      { r.docType = docType }
func (r *requiredSectionsRule) Applicable(docType string) bool {
	if docType == "" {
		return false
	}
	_, ok := requiredSectionsMap[docType]
	return ok
}

func (r *requiredSectionsRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	required, ok := requiredSectionsMap[r.docType]
	if !ok {
		return nil
	}

	// Collect all heading texts (lowercased) present in the document.
	found := make(map[string]bool)
	for _, n := range nodes {
		if n.Type == docx.NodeHeading {
			lower := strings.ToLower(strings.TrimSpace(n.Content))
			found[lower] = true
		}
	}

	var diags []Diagnostic
	for _, section := range required {
		sectionLower := strings.ToLower(section)
		if found[sectionLower] {
			continue
		}
		// Check English aliases.
		matched := false
		if aliases, ok := englishAliases[section]; ok {
			for _, alias := range aliases {
				if found[strings.ToLower(alias)] {
					matched = true
					break
				}
			}
		}
		if !matched {
			diags = append(diags, Diagnostic{
				Level:      DiagError,
				RuleID:     r.ID(),
				Message:    fmt.Sprintf("缺少必填章节: %q", section),
				Suggestion: fmt.Sprintf("在文档中添加标题为 %q 的章节", section),
			})
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// CNT-002: 图表编号连续性
// ---------------------------------------------------------------------------

var (
	figureRe = regexp.MustCompile(`(?i)(?:图|figure)\s*(\d+)`)
	tableRe  = regexp.MustCompile(`(?i)(?:表|table)\s*(\d+)`)
)

type figureNumberingRule struct{}

func (r figureNumberingRule) ID() string                     { return "CNT-002" }
func (r figureNumberingRule) Name() string                   { return "图表编号连续性" }
func (r figureNumberingRule) Level() DiagLevel               { return DiagWarning }
func (r figureNumberingRule) Applicable(docType string) bool { return true }

func (r figureNumberingRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic

	var figNums []int
	var figAnchors []docx.StableAnchor
	var tableNums []int
	var tableAnchors []docx.StableAnchor

	for _, n := range nodes {
		if n.Type == docx.NodeParagraph || n.Type == docx.NodeHeading {
			text := n.Content

			if matches := figureRe.FindAllStringSubmatch(text, -1); matches != nil {
				for _, m := range matches {
					var num int
					fmt.Sscanf(m[1], "%d", &num)
					if num > 0 {
						figNums = append(figNums, num)
						figAnchors = append(figAnchors, n.Anchor)
					}
				}
			}

			if matches := tableRe.FindAllStringSubmatch(text, -1); matches != nil {
				for _, m := range matches {
					var num int
					fmt.Sscanf(m[1], "%d", &num)
					if num > 0 {
						tableNums = append(tableNums, num)
						tableAnchors = append(tableAnchors, n.Anchor)
					}
				}
			}
		}
	}

	diags = append(diags, checkSequence(figNums, figAnchors, "图", r.ID())...)
	diags = append(diags, checkSequence(tableNums, tableAnchors, "表", r.ID())...)

	return diags
}

// checkSequence verifies that nums form a 1-based consecutive sequence.
func checkSequence(nums []int, anchors []docx.StableAnchor, kind, ruleID string) []Diagnostic {
	if len(nums) == 0 {
		return nil
	}
	var diags []Diagnostic
	if nums[0] != 1 {
		diags = append(diags, Diagnostic{
			Level:      DiagWarning,
			RuleID:     ruleID,
			Location:   anchors[0],
			Message:    fmt.Sprintf("%s编号未从 1 开始，首个编号为 %d", kind, nums[0]),
			Suggestion: fmt.Sprintf("将 %s编号重新从 1 开始排列", kind),
		})
	}
	for i := 1; i < len(nums); i++ {
		if nums[i] != nums[i-1]+1 {
			diags = append(diags, Diagnostic{
				Level:      DiagWarning,
				RuleID:     ruleID,
				Location:   anchors[i],
				Message:    fmt.Sprintf("%s编号不连续: %d → %d", kind, nums[i-1], nums[i]),
				Suggestion: fmt.Sprintf("检查%s编号是否有遗漏或重复", kind),
			})
		}
	}
	return diags
}

// ---------------------------------------------------------------------------
// CNT-003: 摘要字数范围
// ---------------------------------------------------------------------------

const (
	abstractMinLen = 100
	abstractMaxLen = 500
)

type abstractLengthRule struct{}

func (r abstractLengthRule) ID() string                     { return "CNT-003" }
func (r abstractLengthRule) Name() string                   { return "摘要字数范围" }
func (r abstractLengthRule) Level() DiagLevel               { return DiagInfo }
func (r abstractLengthRule) Applicable(docType string) bool { return true }

func (r abstractLengthRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic

	// Find the 摘要/Abstract heading.
	abstractIdx := -1
	for i, n := range nodes {
		if n.Type == docx.NodeHeading {
			lower := strings.ToLower(strings.TrimSpace(n.Content))
			if lower == "摘要" || lower == "abstract" {
				abstractIdx = i
				break
			}
		}
	}
	if abstractIdx < 0 {
		return diags
	}

	// Collect paragraph text until the next heading.
	var sb strings.Builder
	for i := abstractIdx + 1; i < len(nodes); i++ {
		n := nodes[i]
		if n.Type == docx.NodeHeading {
			break
		}
		if n.Type == docx.NodeParagraph && n.Content != "" {
			sb.WriteString(n.Content)
		}
	}

	text := sb.String()
	// Use rune count (CJK-friendly) vs word count and take the larger.
	charCount := utf8.RuneCountInString(text)
	wordCount := len(strings.Fields(text))
	count := charCount
	if wordCount > count {
		count = wordCount
	}

	abstractAnchor := nodes[abstractIdx].Anchor

	if count < abstractMinLen {
		diags = append(diags, Diagnostic{
			Level:      DiagInfo,
			RuleID:     r.ID(),
			Location:   abstractAnchor,
			Message:    fmt.Sprintf("摘要内容偏少: 约 %d 字/词（建议 %d–%d）", count, abstractMinLen, abstractMaxLen),
			Suggestion: "扩充摘要内容，涵盖研究背景、方法、结果和结论",
		})
	} else if count > abstractMaxLen {
		diags = append(diags, Diagnostic{
			Level:      DiagInfo,
			RuleID:     r.ID(),
			Location:   abstractAnchor,
			Message:    fmt.Sprintf("摘要内容偏多: 约 %d 字/词（建议 %d–%d）", count, abstractMinLen, abstractMaxLen),
			Suggestion: "精简摘要，突出核心贡献",
		})
	}

	return diags
}

// ---------------------------------------------------------------------------
// CNT-004: 占位符残留
// ---------------------------------------------------------------------------

var placeholderRe = regexp.MustCompile(`\{\{[^}]+\}\}`)

type placeholderResidueRule struct{}

func (r placeholderResidueRule) ID() string                     { return "CNT-004" }
func (r placeholderResidueRule) Name() string                   { return "占位符残留" }
func (r placeholderResidueRule) Level() DiagLevel               { return DiagError }
func (r placeholderResidueRule) Applicable(docType string) bool { return true }

func (r placeholderResidueRule) Check(pkg *docx.Package, nodes []docx.ViewNode) []Diagnostic {
	var diags []Diagnostic
	for _, n := range nodes {
		if n.Type == docx.NodeParagraph || n.Type == docx.NodeHeading {
			if matches := placeholderRe.FindAllString(n.Content, -1); matches != nil {
				for _, m := range matches {
					diags = append(diags, Diagnostic{
						Level:      DiagError,
						RuleID:     r.ID(),
						Location:   n.Anchor,
						Message:    fmt.Sprintf("发现未替换的占位符: %s", m),
						Suggestion: "将占位符替换为实际内容",
					})
				}
			}
		}
	}
	return diags
}
