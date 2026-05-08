package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

type docQualityGateTool struct{}

type docQualityGateParams struct {
	FilePath string `json:"file_path"`
}

// NewDocQualityGateTool creates a tool for checking Markdown document quality.
func NewDocQualityGateTool() BaseTool {
	return &docQualityGateTool{}
}

// Info returns metadata for the DocQualityGate tool.
func (t *docQualityGateTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "DocQualityGate",
		Description: "Check a Markdown document for quality issues before export: empty sections, broken image references, incomplete lists, placeholder text, and missing promised assets. Returns a structured report with ERROR/WARNING severity levels.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the Markdown file to check",
				},
			},
			"required": []string{"file_path"},
		},
		Required: []string{"file_path"},
	}
}

// QualityIssue represents a single quality issue found in the document.
type QualityIssue struct {
	Severity string // "ERROR" or "WARNING"
	Category string // e.g., "empty_section", "broken_image", "empty_list"
	Line     int    // line number (1-based), 0 if not applicable
	Message  string
}

// Run executes the document quality gate check on the specified file.
func (t *docQualityGateTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params docQualityGateParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	if params.FilePath == "" {
		return NewTextErrorResponse("file_path is required"), nil
	}

	data, err := os.ReadFile(params.FilePath)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Cannot read file: %v", err)), nil
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	issues := checkDocument(lines, content, params.FilePath)

	return formatQualityReport(params.FilePath, issues), nil
}

// checkDocument runs all quality checks and returns a merged list of issues.
func checkDocument(lines []string, content string, filePath string) []QualityIssue {
	var issues []QualityIssue
	issues = append(issues, checkEmptySections(lines)...)
	issues = append(issues, checkEmptyLists(lines)...)
	issues = append(issues, checkBrokenImageRefs(lines, filePath)...)
	issues = append(issues, checkDocPlaceholders(lines)...)
	issues = append(issues, checkShortSections(lines)...)
	issues = append(issues, checkPromisedAssets(lines)...)
	issues = append(issues, checkDanglingColons(lines)...)
	return issues
}

// headingLevel returns the heading level (1-6) if the line is a heading, otherwise 0.
func headingLevel(line string) int {
	trimmed := strings.TrimRight(line, " \t")
	if len(trimmed) == 0 {
		return 0
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level > 0 && level < len(trimmed) && trimmed[level] == ' ' {
		return level
	}
	return 0
}

// checkEmptySections finds headings with no content before the next heading.
func checkEmptySections(lines []string) []QualityIssue {
	var issues []QualityIssue

	type headingEntry struct {
		lineNum int
		text    string
	}

	var headings []headingEntry
	for i, line := range lines {
		if lvl := headingLevel(line); lvl > 0 {
			headings = append(headings, headingEntry{lineNum: i + 1, text: line})
		}
	}

	for idx, h := range headings {
		// Skip H1 title headings — it's normal for a document title to be
		// immediately followed by a H2 without body text in between.
		if headingLevel(h.text) == 1 {
			continue
		}

		// Find the range of lines for this section
		start := h.lineNum // 1-based
		var end int
		if idx+1 < len(headings) {
			end = headings[idx+1].lineNum - 1
		} else {
			end = len(lines)
		}

		// Check if there is any non-blank content between start and end (exclusive of heading line itself)
		hasContent := false
		for i := start; i < end && i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) != "" {
				hasContent = true
				break
			}
		}

		if !hasContent {
			issues = append(issues, QualityIssue{
				Severity: "ERROR",
				Category: "empty_section",
				Line:     h.lineNum,
				Message:  fmt.Sprintf("Section '%s' has no content", strings.TrimSpace(h.text)),
			})
		}
	}

	return issues
}

// checkEmptyLists finds list markers with no meaningful content.
func checkEmptyLists(lines []string) []QualityIssue {
	var issues []QualityIssue

	// Regex for bare list markers: "- " or "* " or "N. " at end of line
	bareListRe := regexp.MustCompile(`^(\s*)([-*]|\d+\.)\s*$`)

	// Patterns for list-introduction phrases followed by blank or heading
	listIntroPatterns := []string{"包括：", "例如：", "局限：", "如下：", "以下：", "包括:", "例如:", "局限:", "如下:", "以下:"}

	for i, line := range lines {
		lineNum := i + 1

		if bareListRe.MatchString(line) {
			issues = append(issues, QualityIssue{
				Severity: "ERROR",
				Category: "empty_list",
				Line:     lineNum,
				Message:  "List item marker with no content",
			})
			continue
		}

		// Check list-introduction phrases
		trimmed := strings.TrimSpace(line)
		for _, pat := range listIntroPatterns {
			if strings.HasSuffix(trimmed, pat) || trimmed == pat {
				// Check if next non-blank line is a heading or EOF
				nextNonBlank := -1
				for j := i + 1; j < len(lines); j++ {
					if strings.TrimSpace(lines[j]) != "" {
						nextNonBlank = j
						break
					}
				}
				if nextNonBlank == -1 || headingLevel(lines[nextNonBlank]) > 0 {
					issues = append(issues, QualityIssue{
						Severity: "ERROR",
						Category: "empty_list",
						Line:     lineNum,
						Message:  fmt.Sprintf("List introduction '%s' followed by no list items", pat),
					})
				}
				break
			}
		}
	}

	return issues
}

// checkBrokenImageRefs checks for broken or empty image references.
func checkBrokenImageRefs(lines []string, docPath string) []QualityIssue {
	var issues []QualityIssue

	imageRe := regexp.MustCompile(`!\[([^\]]*)\]\(([^\)]*)\)`)
	docDir := filepath.Dir(docPath)

	for i, line := range lines {
		lineNum := i + 1
		matches := imageRe.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			// match[4]:match[5] is the URL group
			urlStart, urlEnd := match[4], match[5]
			altStart, altEnd := match[2], match[3]
			url := line[urlStart:urlEnd]
			alt := line[altStart:altEnd]
			_ = alt

			if strings.TrimSpace(url) == "" {
				issues = append(issues, QualityIssue{
					Severity: "ERROR",
					Category: "broken_image",
					Line:     lineNum,
					Message:  fmt.Sprintf("Image reference has empty URL: ![%s]()", alt),
				})
				continue
			}

			// Check local file references
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "data:") {
				localPath := url
				if !filepath.IsAbs(localPath) {
					localPath = filepath.Join(docDir, url)
				}
				if _, err := os.Stat(localPath); os.IsNotExist(err) {
					issues = append(issues, QualityIssue{
						Severity: "WARNING",
						Category: "broken_image",
						Line:     lineNum,
						Message:  fmt.Sprintf("Image file not found: %s", url),
					})
				}
			}
		}
	}

	return issues
}

// checkDocPlaceholders finds common placeholder patterns in the document.
func checkDocPlaceholders(lines []string) []QualityIssue {
	var issues []QualityIssue

	placeholders := []string{
		"TODO", "FIXME", "TBD", "XXX",
		"[待补充]", "[placeholder]", "Lorem ipsum",
	}

	for i, line := range lines {
		lineNum := i + 1
		upper := strings.ToUpper(line)
		for _, ph := range placeholders {
			checkStr := ph
			if ph == "TODO" || ph == "FIXME" || ph == "TBD" || ph == "XXX" || ph == "Lorem ipsum" {
				checkStr = strings.ToUpper(ph)
				if strings.Contains(upper, checkStr) {
					issues = append(issues, QualityIssue{
						Severity: "WARNING",
						Category: "placeholder",
						Line:     lineNum,
						Message:  fmt.Sprintf("Placeholder text found: '%s'", ph),
					})
					break
				}
			} else {
				if strings.Contains(line, checkStr) {
					issues = append(issues, QualityIssue{
						Severity: "WARNING",
						Category: "placeholder",
						Line:     lineNum,
						Message:  fmt.Sprintf("Placeholder text found: '%s'", ph),
					})
					break
				}
			}
		}
	}

	return issues
}

// checkShortSections finds sections with very little content (fewer than 3 non-blank lines).
func checkShortSections(lines []string) []QualityIssue {
	var issues []QualityIssue

	type sectionInfo struct {
		lineNum int
		text    string
		level   int
	}

	var sections []sectionInfo
	for i, line := range lines {
		if lvl := headingLevel(line); lvl > 0 {
			sections = append(sections, sectionInfo{lineNum: i + 1, text: line, level: lvl})
		}
	}

	for idx, sec := range sections {
		// Skip H1 title headings — document titles often have minimal or no body.
		if sec.level == 1 {
			continue
		}

		start := sec.lineNum // 1-based
		var end int
		if idx+1 < len(sections) {
			end = sections[idx+1].lineNum - 1
		} else {
			end = len(lines)
		}

		// Count non-blank lines in section (excluding heading line itself)
		nonBlank := 0
		for i := start; i < end && i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) != "" {
				nonBlank++
			}
		}

		if nonBlank > 0 && nonBlank < 3 {
			issues = append(issues, QualityIssue{
				Severity: "WARNING",
				Category: "short_section",
				Line:     sec.lineNum,
				Message:  fmt.Sprintf("Section '%s' has very little content (%d non-blank line(s))", strings.TrimSpace(sec.text), nonBlank),
			})
		}
	}

	return issues
}

// checkPromisedAssets finds headings that promise visual content but the section has none.
func checkPromisedAssets(lines []string) []QualityIssue {
	var issues []QualityIssue

	// Keyword categories: each maps to what asset types satisfy the promise
	type promiseRule struct {
		keywords  []string
		assetType string // "image", "table", "data"
	}
	rules := []promiseRule{
		{keywords: []string{"图", "图解", "示意", "流程", "架构"}, assetType: "image"},
		{keywords: []string{"对比表", "对比"}, assetType: "table"},
		{keywords: []string{"数据", "统计"}, assetType: "data"},
	}

	type sectionInfo struct {
		lineNum int
		text    string
	}

	var sections []sectionInfo
	for i, line := range lines {
		if headingLevel(line) > 0 {
			sections = append(sections, sectionInfo{lineNum: i + 1, text: line})
		}
	}

	for idx, sec := range sections {
		// Check which promise rule matches
		var matchedRule *promiseRule
		for ri := range rules {
			for _, kw := range rules[ri].keywords {
				if strings.Contains(sec.text, kw) {
					matchedRule = &rules[ri]
					break
				}
			}
			if matchedRule != nil {
				break
			}
		}
		if matchedRule == nil {
			continue
		}

		start := sec.lineNum // 1-based
		var end int
		if idx+1 < len(sections) {
			end = sections[idx+1].lineNum - 1
		} else {
			end = len(lines)
		}

		// Scan section content for the promised asset type
		satisfied := false
		for i := start; i < end && i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			switch matchedRule.assetType {
			case "image":
				if strings.Contains(trimmed, "![") || strings.HasPrefix(trimmed, "```") {
					satisfied = true
				}
			case "table":
				// Markdown table: line contains | separators
				if strings.Contains(trimmed, "|") && strings.Count(trimmed, "|") >= 2 {
					satisfied = true
				}
				// Also accept images/code as table alternatives
				if strings.Contains(trimmed, "![") || strings.HasPrefix(trimmed, "```") {
					satisfied = true
				}
			case "data":
				// Accept: links, images, code blocks, or citation-like patterns
				if strings.Contains(trimmed, "http") || strings.Contains(trimmed, "![") ||
					strings.HasPrefix(trimmed, "```") || strings.Contains(trimmed, "[") {
					satisfied = true
				}
			}
			if satisfied {
				break
			}
		}

		if !satisfied {
			issues = append(issues, QualityIssue{
				Severity: "ERROR",
				Category: "promised_asset",
				Line:     sec.lineNum,
				Message:  fmt.Sprintf("Section '%s' promises %s content but none found", strings.TrimSpace(sec.text), matchedRule.assetType),
			})
		}
	}

	return issues
}

// checkDanglingColons finds lines ending with a colon-like pattern followed by no list content.
func checkDanglingColons(lines []string) []QualityIssue {
	var issues []QualityIssue

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check if line ends with "：" (full-width) or ":" (ASCII)
		endsWithColon := false
		if strings.HasSuffix(trimmed, "：") {
			endsWithColon = true
		} else if strings.HasSuffix(trimmed, ":") {
			// Avoid false positives: skip URLs, code lines, headings
			if !strings.Contains(trimmed, "://") && headingLevel(trimmed) == 0 && !strings.HasPrefix(trimmed, "```") {
				endsWithColon = true
			}
		}

		if !endsWithColon {
			continue
		}

		// Find next non-blank line
		nextNonBlankIdx := -1
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) != "" {
				nextNonBlankIdx = j
				break
			}
		}

		if nextNonBlankIdx == -1 || headingLevel(lines[nextNonBlankIdx]) > 0 {
			issues = append(issues, QualityIssue{
				Severity: "ERROR",
				Category: "dangling_colon",
				Line:     lineNum,
				Message:  "Dangling list introduction with no items",
			})
		}
	}

	return issues
}

// formatQualityReport formats the quality check results into a tool response.
func formatQualityReport(filePath string, issues []QualityIssue) ToolResponse {
	if len(issues) == 0 {
		return NewTextResponse(fmt.Sprintf("✓ Document quality check passed: %s\nNo issues found.", filePath))
	}

	var sb strings.Builder
	errors := 0
	warnings := 0
	for _, issue := range issues {
		if issue.Severity == "ERROR" {
			errors++
		} else {
			warnings++
		}
	}

	fmt.Fprintf(&sb, "Document quality check: %s\n%d ERROR(s), %d WARNING(s)\n\n", filePath, errors, warnings)

	for _, issue := range issues {
		if issue.Line > 0 {
			fmt.Fprintf(&sb, "[%s] Line %d (%s): %s\n", issue.Severity, issue.Line, issue.Category, issue.Message)
		} else {
			fmt.Fprintf(&sb, "[%s] (%s): %s\n", issue.Severity, issue.Category, issue.Message)
		}
	}

	if errors > 0 {
		sb.WriteString("\nRecommendation: Fix all ERROR issues before exporting.")
	}

	return NewTextResponse(sb.String())
}

// Ensure utf8 import is used to avoid compile error; used for rune-safe operations if needed.
var _ = utf8.RuneCountInString
