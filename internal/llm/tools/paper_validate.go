package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

type paperValidateTool struct {
	permissions permission.Service
}

type paperValidateParams struct {
	WorkDir string `json:"work_dir,omitempty"`
}

func NewPaperValidateTool(perms permission.Service) BaseTool {
	return &paperValidateTool{permissions: perms}
}

func (t *paperValidateTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "PaperValidate",
		Description: "Validate a LaTeX paper project for common quality issues: missing \\bibliographystyle, cite key consistency, hard-coded citation numbers, cite key sanity, section length balance, metadata completeness, and orphaned bib entries. Returns a structured report with FATAL/ERROR/WARNING severity levels. This is a read-only analysis tool that does not modify any files.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"work_dir": map[string]any{
					"type":        "string",
					"description": "Working directory containing the LaTeX project. Defaults to current directory if omitted.",
				},
			},
		},
		Required: []string{},
	}
}

// severity levels
const (
	sevFatal   = "FATAL"
	sevError   = "ERROR"
	sevWarning = "WARNING"
)

type validationIssue struct {
	Severity string
	Message  string
}

func (t *paperValidateTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params paperValidateParams
	if call.Input != "" {
		if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
		}
	}

	workDir := params.WorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to get working directory: %v", err)), nil
		}
	}

	var issues []validationIssue

	// 1. Check \bibliographystyle presence
	issues = append(issues, checkBibStyle(workDir)...)

	// 2. Check cite key consistency
	issues = append(issues, checkCiteKeyConsistency(workDir)...)

	// 3. Check hard-coded citation numbers
	issues = append(issues, checkHardCodedCitations(workDir)...)

	// 4. Check cite key sanity
	issues = append(issues, checkCiteKeySanity(workDir)...)

	// 5. Check section length balance
	issues = append(issues, checkSectionBalance(workDir)...)

	// 6. Check metadata completeness
	issues = append(issues, checkMetadata(workDir)...)

	// 7. Check orphaned bib entries (already covered in consistency check as warnings)

	// 8. Check placeholder text
	issues = append(issues, checkPlaceholders(workDir)...)

	// 9. Check reference count by paper type
	issues = append(issues, checkReferenceCount(workDir)...)

	// 10. Check duplicate content
	issues = append(issues, checkDuplicates(workDir)...)

	// 11. Check float placement distribution
	issues = append(issues, checkFloatPlacement(workDir)...)

	// 12. Check table existence for survey papers
	issues = append(issues, checkTableExistence(workDir)...)

	// Format report
	return NewTextResponse(formatReport(issues)), nil
}

// checkBibStyle checks for \bibliographystyle{} in main.tex
func checkBibStyle(workDir string) []validationIssue {
	mainTex := filepath.Join(workDir, "main.tex")
	content, err := os.ReadFile(mainTex)
	if err != nil {
		return []validationIssue{{sevWarning, fmt.Sprintf("Cannot read main.tex: %v", err)}}
	}

	re := regexp.MustCompile(`\\bibliographystyle\{`)
	if !re.Match(content) {
		return []validationIssue{{sevFatal, `Missing \bibliographystyle{} in main.tex — all citations will show as (?)`}}
	}
	return nil
}

// checkCiteKeyConsistency cross-checks cite keys in .tex files against .bib entries
func checkCiteKeyConsistency(workDir string) []validationIssue {
	texKeys := extractCiteKeysFromTex(workDir)
	bibKeys := extractBibKeys(workDir)

	var issues []validationIssue

	// Keys in tex but not in bib
	var missing []string
	for key := range texKeys {
		if _, ok := bibKeys[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		issues = append(issues, validationIssue{
			sevError,
			fmt.Sprintf("%d cite keys in .tex but missing from .bib: [%s]", len(missing), strings.Join(missing, ", ")),
		})
	}

	// Keys in bib but not in tex (orphaned)
	var orphaned []string
	for key := range bibKeys {
		if _, ok := texKeys[key]; !ok {
			orphaned = append(orphaned, key)
		}
	}
	sort.Strings(orphaned)
	if len(orphaned) > 0 {
		issues = append(issues, validationIssue{
			sevWarning,
			fmt.Sprintf("%d orphaned bib entries (in .bib but never cited): [%s]", len(orphaned), strings.Join(orphaned, ", ")),
		})
	}

	return issues
}

// checkHardCodedCitations finds [N] patterns outside math environments
func checkHardCodedCitations(workDir string) []validationIssue {
	texFiles := findTexFiles(workDir)
	re := regexp.MustCompile(`\[(\d+)\]`)
	mathEnvRe := regexp.MustCompile(`\$.*\$|\\begin\{(equation|align|math|array|gather|multline)`)

	var issues []validationIssue
	for _, f := range texFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(workDir, f)
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			// Skip lines in math environments or comments
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "%") {
				continue
			}
			if mathEnvRe.MatchString(line) {
				continue
			}
			matches := re.FindAllStringIndex(line, -1)
			for _, m := range matches {
				// Check context: likely a citation if preceded by text
				before := ""
				if m[0] > 0 {
					before = line[:m[0]]
				}
				// Skip if inside \cite{}, \ref{}, array index, etc.
				if strings.HasSuffix(before, `\cite{`) || strings.HasSuffix(before, `\citep{`) ||
					strings.HasSuffix(before, `\citet{`) || strings.HasSuffix(before, `\ref{`) ||
					strings.HasSuffix(before, `$`) || strings.HasSuffix(before, `{`) {
					continue
				}
				citation := line[m[0]:m[1]]
				issues = append(issues, validationIssue{
					sevError,
					fmt.Sprintf("Hard-coded citation %s at %s:%d", citation, relPath, i+1),
				})
			}
		}
	}
	return issues
}

// checkCiteKeySanity validates cite key naming conventions
func checkCiteKeySanity(workDir string) []validationIssue {
	bibKeys := extractBibKeys(workDir)
	validKeyRe := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_:-]{3,}$`)

	// Known HTML attributes and other suspicious patterns
	htmlAttrs := map[string]bool{
		"noopener": true, "nofollow": true, "noreferrer": true,
		"target": true, "blank": true, "self": true,
		"class": true, "style": true, "onclick": true,
	}

	var issues []validationIssue
	for key := range bibKeys {
		if !validKeyRe.MatchString(key) {
			issues = append(issues, validationIssue{
				sevError,
				fmt.Sprintf("Invalid cite key format %q in references.bib (must be >=4 chars, start with letter, alphanumeric)", key),
			})
		} else if htmlAttrs[strings.ToLower(key)] {
			issues = append(issues, validationIssue{
				sevError,
				fmt.Sprintf("Suspicious cite key %q in references.bib (appears to be an HTML attribute, likely LLM hallucination)", key),
			})
		}
	}
	return issues
}

// checkSectionBalance checks if section lengths are reasonably balanced
func checkSectionBalance(workDir string) []validationIssue {
	mainTex := filepath.Join(workDir, "main.tex")
	content, err := os.ReadFile(mainTex)
	if err != nil {
		return nil
	}

	inputRe := regexp.MustCompile(`\\input\{([^}]+)\}`)
	matches := inputRe.FindAllStringSubmatch(string(content), -1)

	type sectionInfo struct {
		name  string
		lines int
	}
	var sections []sectionInfo
	maxLines := 0

	for _, m := range matches {
		path := m[1]
		if !strings.HasSuffix(path, ".tex") {
			path += ".tex"
		}
		fullPath := filepath.Join(workDir, path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		lineCount := strings.Count(string(data), "\n") + 1
		name := filepath.Base(path)
		name = strings.TrimSuffix(name, ".tex")
		sections = append(sections, sectionInfo{name, lineCount})
		if lineCount > maxLines {
			maxLines = lineCount
		}
	}

	var issues []validationIssue
	if maxLines > 0 {
		threshold := maxLines / 4
		for _, s := range sections {
			if s.lines < threshold && s.lines > 0 {
				issues = append(issues, validationIssue{
					sevWarning,
					fmt.Sprintf("Section %q (%d lines) is significantly shorter than the longest section (%d lines)", s.name, s.lines, maxLines),
				})
			}
		}
	}
	return issues
}

// checkMetadata checks \author{} and \title{} in main.tex
func checkMetadata(workDir string) []validationIssue {
	mainTex := filepath.Join(workDir, "main.tex")
	content, err := os.ReadFile(mainTex)
	if err != nil {
		return nil
	}

	var issues []validationIssue
	text := string(content)

	// Check \author{}
	authorRe := regexp.MustCompile(`\\author\{([^}]*)\}`)
	authorMatch := authorRe.FindStringSubmatch(text)
	if authorMatch == nil {
		issues = append(issues, validationIssue{sevWarning, `Missing \author{} in main.tex`})
	} else if strings.TrimSpace(authorMatch[1]) == "" {
		issues = append(issues, validationIssue{sevWarning, `\author{} is empty in main.tex`})
	}

	// Check \title{}
	titleRe := regexp.MustCompile(`\\title\{([^}]*)\}`)
	titleMatch := titleRe.FindStringSubmatch(text)
	if titleMatch == nil {
		issues = append(issues, validationIssue{sevWarning, `Missing \title{} in main.tex`})
	} else {
		titleContent := strings.TrimSpace(titleMatch[1])
		if titleContent == "" {
			issues = append(issues, validationIssue{sevWarning, `\title{} is empty in main.tex`})
		} else if hasGarbledChars(titleContent) {
			issues = append(issues, validationIssue{sevWarning, `\title{} contains garbled/non-printable characters in main.tex`})
		}
	}

	return issues
}

// checkPlaceholders scans .tex files for leftover placeholder text
func checkPlaceholders(workDir string) []validationIssue {
	texFiles := findTexFiles(workDir)
	placeholderRe := regexp.MustCompile(`(?i)(TODO|FIXME|TBD|placeholder|would appear here|lorem ipsum|INSERT\s+.*\s+HERE)`)

	var issues []validationIssue
	for _, f := range texFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(workDir, f)
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "%") {
				continue
			}
			if placeholderRe.MatchString(line) {
				match := placeholderRe.FindString(line)
				issues = append(issues, validationIssue{
					sevError,
					fmt.Sprintf("Placeholder text %q found at %s:%d", match, relPath, i+1),
				})
			}
		}
	}

	// Also check for todonotes package
	mainTex := filepath.Join(workDir, "main.tex")
	if content, err := os.ReadFile(mainTex); err == nil {
		if strings.Contains(string(content), `\usepackage{todonotes}`) {
			issues = append(issues, validationIssue{
				sevWarning,
				`Draft package \usepackage{todonotes} still present in main.tex`,
			})
		}
	}

	return issues
}

// checkReferenceCount validates minimum citation count based on paper type.
// Uses config.PaperType when available, falls back to heuristic detection.
func checkReferenceCount(workDir string) []validationIssue {
	bibKeys := extractBibKeys(workDir)
	count := len(bibKeys)
	if count == 0 {
		return nil // No bib file or empty — other checks will catch this
	}

	// Determine paper type from config or heuristic
	paperType := config.PaperTypeResearch
	if cfg := config.Get(); cfg != nil {
		paperType = cfg.GetPaperType()
	} else if detectSurveyPaper(workDir) {
		paperType = config.PaperTypeSurvey
	}

	minRefs := getMinReferences(paperType)

	var issues []validationIssue
	if count < minRefs/2 {
		issues = append(issues, validationIssue{
			sevError,
			fmt.Sprintf("%s paper has only %d references (minimum recommended: %d, critical threshold: %d)", paperType, count, minRefs, minRefs/2),
		})
	} else if count < minRefs {
		issues = append(issues, validationIssue{
			sevWarning,
			fmt.Sprintf("%s paper has %d references (recommended: %d+)", paperType, count, minRefs),
		})
	}
	return issues
}

// getMinReferences returns the minimum recommended reference count by paper type.
func getMinReferences(paperType config.PaperType) int {
	switch paperType {
	case config.PaperTypeSurvey:
		return 50
	case config.PaperTypeResearch:
		return 20
	case config.PaperTypePosition:
		return 15
	case config.PaperTypeThesis:
		return 40
	default:
		return 20
	}
}

// checkDuplicates detects duplicate bib entries, labels, and figure paths
func checkDuplicates(workDir string) []validationIssue {
	var issues []validationIssue

	// 1. Duplicate bib entries (same title, different keys)
	issues = append(issues, checkDuplicateBibTitles(workDir)...)

	// 2. Duplicate \label{} definitions
	issues = append(issues, checkDuplicateLabels(workDir)...)

	// 3. Duplicate \includegraphics{} paths
	issues = append(issues, checkDuplicateGraphics(workDir)...)

	return issues
}

func checkDuplicateBibTitles(workDir string) []validationIssue {
	bibFiles, _ := filepath.Glob(filepath.Join(workDir, "*.bib"))
	titleRe := regexp.MustCompile(`(?i)title\s*=\s*\{([^}]+)\}`)
	entryRe := regexp.MustCompile(`@\w+\{([^,]+),`)

	type bibEntry struct {
		key   string
		title string
	}
	var entries []bibEntry

	for _, f := range bibFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text := string(content)
		// Split by @ entries
		entryBlocks := regexp.MustCompile(`(?m)^@`).Split(text, -1)
		for _, block := range entryBlocks {
			if strings.TrimSpace(block) == "" {
				continue
			}
			block = "@" + block
			keyMatch := entryRe.FindStringSubmatch(block)
			titleMatch := titleRe.FindStringSubmatch(block)
			if keyMatch != nil && titleMatch != nil {
				entries = append(entries, bibEntry{
					key:   strings.TrimSpace(keyMatch[1]),
					title: strings.ToLower(strings.TrimSpace(titleMatch[1])),
				})
			}
		}
	}

	// Find duplicates
	titleMap := make(map[string][]string)
	for _, e := range entries {
		titleMap[e.title] = append(titleMap[e.title], e.key)
	}

	var issues []validationIssue
	for title, keys := range titleMap {
		if len(keys) > 1 {
			truncTitle := title
			if len(truncTitle) > 60 {
				truncTitle = truncTitle[:60] + "..."
			}
			issues = append(issues, validationIssue{
				sevError,
				fmt.Sprintf("Duplicate bib entries with same title %q: keys [%s]", truncTitle, strings.Join(keys, ", ")),
			})
		}
	}
	return issues
}

func checkDuplicateLabels(workDir string) []validationIssue {
	texFiles := findTexFiles(workDir)
	labelRe := regexp.MustCompile(`\\label\{([^}]+)\}`)
	labelMap := make(map[string][]string) // label -> [file:line, ...]

	for _, f := range texFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(workDir, f)
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			matches := labelRe.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				loc := fmt.Sprintf("%s:%d", relPath, i+1)
				labelMap[m[1]] = append(labelMap[m[1]], loc)
			}
		}
	}

	var issues []validationIssue
	for label, locs := range labelMap {
		if len(locs) > 1 {
			issues = append(issues, validationIssue{
				sevError,
				fmt.Sprintf(`Duplicate \label{%s} defined at: %s`, label, strings.Join(locs, ", ")),
			})
		}
	}
	return issues
}

func checkDuplicateGraphics(workDir string) []validationIssue {
	texFiles := findTexFiles(workDir)
	graphicsRe := regexp.MustCompile(`\\includegraphics(?:\[[^\]]*\])?\{([^}]+)\}`)
	pathMap := make(map[string][]string) // normalized path -> [file:line, ...]

	for _, f := range texFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(workDir, f)
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			matches := graphicsRe.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				imgPath := strings.TrimSpace(m[1])
				loc := fmt.Sprintf("%s:%d", relPath, i+1)
				pathMap[imgPath] = append(pathMap[imgPath], loc)
			}
		}
	}

	var issues []validationIssue
	for imgPath, locs := range pathMap {
		if len(locs) > 1 {
			issues = append(issues, validationIssue{
				sevWarning,
				fmt.Sprintf(`Same image %q included %d times at: %s`, imgPath, len(locs), strings.Join(locs, ", ")),
			})
		}
	}
	return issues
}

// checkFloatPlacement checks if figures/tables are clustered at the end of the document
func checkFloatPlacement(workDir string) []validationIssue {
	texFiles := findTexFiles(workDir)
	floatRe := regexp.MustCompile(`\\begin\{(figure|table)\}`)

	// Collect all line positions across all tex files
	totalLines := 0
	var floatPositions []int

	for _, f := range texFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if floatRe.MatchString(line) {
				floatPositions = append(floatPositions, totalLines+i)
			}
		}
		totalLines += len(lines)
	}

	if len(floatPositions) < 2 || totalLines == 0 {
		return nil
	}

	// Check if 80%+ of floats are in the last 20% of lines
	threshold := int(float64(totalLines) * 0.8)
	endFloats := 0
	for _, pos := range floatPositions {
		if pos >= threshold {
			endFloats++
		}
	}

	var issues []validationIssue
	ratio := float64(endFloats) / float64(len(floatPositions))
	if ratio >= 0.8 {
		issues = append(issues, validationIssue{
			sevWarning,
			fmt.Sprintf("%.0f%% of figures/tables (%d/%d) are clustered in the last 20%% of the document — distribute them closer to their references using [htbp] placement",
				ratio*100, endFloats, len(floatPositions)),
		})
	}

	return issues
}

// checkTableExistence checks if survey papers contain comparison tables
func checkTableExistence(workDir string) []validationIssue {
	if !detectSurveyPaper(workDir) {
		return nil
	}

	texFiles := findTexFiles(workDir)
	tableRe := regexp.MustCompile(`\\begin\{(table|tabular|longtable)\}`)

	for _, f := range texFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if tableRe.Match(content) {
			return nil // Found at least one table
		}
	}

	return []validationIssue{{
		sevWarning,
		"Survey paper contains no comparison tables — consider adding method comparison or benchmark summary tables",
	}}
}

// detectSurveyPaper checks if the paper is a survey/review by examining title and section structure
func detectSurveyPaper(workDir string) bool {
	mainTex := filepath.Join(workDir, "main.tex")
	content, err := os.ReadFile(mainTex)
	if err != nil {
		return false
	}
	text := strings.ToLower(string(content))

	// Check title for survey keywords
	titleRe := regexp.MustCompile(`(?i)\\title\{([^}]*)\}`)
	if m := titleRe.FindStringSubmatch(string(content)); m != nil {
		titleLower := strings.ToLower(m[1])
		surveyKeywords := []string{"survey", "review", "overview", "comprehensive", "tutorial", "taxonomy"}
		for _, kw := range surveyKeywords {
			if strings.Contains(titleLower, kw) {
				return true
			}
		}
	}

	// Check section names for typical survey structure
	surveyIndicators := 0
	sectionPatterns := []string{"related work", "background", "taxonomy", "classification", "applications", "challenges", "future direction", "benchmark"}
	for _, p := range sectionPatterns {
		if strings.Contains(text, p) {
			surveyIndicators++
		}
	}
	return surveyIndicators >= 4
}

// --- Helper functions ---

func extractCiteKeysFromTex(workDir string) map[string]bool {
	texFiles := findTexFiles(workDir)
	citeRe := regexp.MustCompile(`\\(?:cite|citep|citet|citeauthor|citeyear)\{([^}]+)\}`)
	keys := make(map[string]bool)

	for _, f := range texFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		matches := citeRe.FindAllStringSubmatch(string(content), -1)
		for _, m := range matches {
			// Handle multiple keys in one \cite{key1,key2}
			for k := range strings.SplitSeq(m[1], ",") {
				k = strings.TrimSpace(k)
				if k != "" {
					keys[k] = true
				}
			}
		}
	}
	return keys
}

func extractBibKeys(workDir string) map[string]bool {
	bibFiles, _ := filepath.Glob(filepath.Join(workDir, "*.bib"))
	entryRe := regexp.MustCompile(`@\w+\{([^,]+),`)
	keys := make(map[string]bool)

	for _, f := range bibFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		matches := entryRe.FindAllStringSubmatch(string(content), -1)
		for _, m := range matches {
			key := strings.TrimSpace(m[1])
			if key != "" {
				keys[key] = true
			}
		}
	}
	return keys
}

func findTexFiles(workDir string) []string {
	var files []string
	_ = filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".tex") {
			files = append(files, path)
		}
		// Don't descend into hidden directories or common non-tex dirs
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		return nil
	})
	return files
}

func hasGarbledChars(s string) bool {
	garbledCount := 0
	for _, r := range s {
		// Check for unusual Unicode ranges that indicate encoding corruption
		if r > 0x2000 && !unicode.IsLetter(r) && !unicode.IsPunct(r) && !unicode.IsSpace(r) {
			garbledCount++
		}
	}
	return garbledCount > 3
}

func formatReport(issues []validationIssue) string {
	if len(issues) == 0 {
		return "=== Paper Validation Report ===\nNo issues found. All checks passed.\n=== 0 fatal, 0 errors, 0 warnings ==="
	}

	var b strings.Builder
	b.WriteString("=== Paper Validation Report ===\n")

	// Sort by severity: FATAL > ERROR > WARNING
	sevOrder := map[string]int{sevFatal: 0, sevError: 1, sevWarning: 2}
	sort.Slice(issues, func(i, j int) bool {
		return sevOrder[issues[i].Severity] < sevOrder[issues[j].Severity]
	})

	fatal, errors, warnings := 0, 0, 0
	for _, issue := range issues {
		fmt.Fprintf(&b, "%s: %s\n", issue.Severity, issue.Message)
		switch issue.Severity {
		case sevFatal:
			fatal++
		case sevError:
			errors++
		case sevWarning:
			warnings++
		}
	}

	fmt.Fprintf(&b, "=== %d fatal, %d errors, %d warnings ===", fatal, errors, warnings)

	// Add guidance
	if fatal > 0 || errors > 0 {
		b.WriteString("\n\nFATAL and ERROR issues must be fixed before final compilation.")
	}

	return b.String()
}
