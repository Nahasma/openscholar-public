package components

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	addStyle             lipgloss.Style
	removeStyle          lipgloss.Style
	hunkStyle            lipgloss.Style
	gutterStyle          lipgloss.Style
	addHighlightStyle    lipgloss.Style
	removeHighlightStyle lipgloss.Style
	foldStyle            lipgloss.Style
)

func initDiffStyles() {
	addStyle = lipgloss.NewStyle().Foreground(ColorDiffAddFg)
	removeStyle = lipgloss.NewStyle().Foreground(ColorDiffDelFg)
	hunkStyle = lipgloss.NewStyle().Foreground(ColorBlue)
	gutterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	addHighlightStyle = lipgloss.NewStyle().Foreground(ColorDiffAddFg).Background(lipgloss.Color("22"))
	removeHighlightStyle = lipgloss.NewStyle().Foreground(ColorDiffDelFg).Background(lipgloss.Color("52"))
	foldStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
}

// DiffLine represents a parsed line from a unified diff.
type DiffLine struct {
	OldNo *int   // line number in old file (nil for added lines)
	NewNo *int   // line number in new file (nil for removed lines)
	Kind  string // "add", "remove", "context", "hunk", "header"
	Text  string // raw line text
}

// ParseUnifiedDiff parses a unified diff string into structured DiffLines.
func ParseUnifiedDiff(diff string) []DiffLine {
	var lines []DiffLine
	oldNo, newNo := 0, 0

	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			// Parse hunk header: @@ -a,b +c,d @@
			o, n := parseDiffHunkHeader(line)
			oldNo = o
			newNo = n
			lines = append(lines, DiffLine{Kind: "hunk", Text: line})

		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
			lines = append(lines, DiffLine{Kind: "header", Text: line})

		case strings.HasPrefix(line, "+"):
			no := newNo
			lines = append(lines, DiffLine{NewNo: &no, Kind: "add", Text: line})
			newNo++

		case strings.HasPrefix(line, "-"):
			no := oldNo
			lines = append(lines, DiffLine{OldNo: &no, Kind: "remove", Text: line})
			oldNo++

		default:
			if oldNo > 0 || newNo > 0 {
				// Context line (inside a hunk)
				o, n := oldNo, newNo
				lines = append(lines, DiffLine{OldNo: &o, NewNo: &n, Kind: "context", Text: line})
				oldNo++
				newNo++
			} else {
				lines = append(lines, DiffLine{Kind: "header", Text: line})
			}
		}
	}

	return lines
}

// parseDiffHunkHeader extracts start line numbers from @@ -a,b +c,d @@
func parseDiffHunkHeader(line string) (oldStart, newStart int) {
	// Find the range info between @@ markers
	start := strings.Index(line, "@@")
	if start < 0 {
		return 1, 1
	}
	rest := line[start+2:]
	end := strings.Index(rest, "@@")
	if end < 0 {
		end = len(rest)
	}
	rangeInfo := strings.TrimSpace(rest[:end])

	// Parse "-a,b +c,d" or "-a +c"
	parts := strings.Fields(rangeInfo)
	for _, p := range parts {
		if strings.HasPrefix(p, "-") {
			nums := strings.SplitN(p[1:], ",", 2)
			if n, err := strconv.Atoi(nums[0]); err == nil {
				oldStart = n
			}
		} else if strings.HasPrefix(p, "+") {
			nums := strings.SplitN(p[1:], ",", 2)
			if n, err := strconv.Atoi(nums[0]); err == nil {
				newStart = n
			}
		}
	}

	if oldStart == 0 {
		oldStart = 1
	}
	if newStart == 0 {
		newStart = 1
	}
	return
}

// RenderDiff renders a unified diff with line number gutter.
func RenderDiff(diff string) string {
	lines := ParseUnifiedDiff(diff)
	return RenderDiffLines(lines)
}

// RenderDiffLines renders pre-parsed diff lines with gutter.
func RenderDiffLines(lines []DiffLine) string {
	// Calculate gutter width based on max line number
	maxNo := 0
	for _, dl := range lines {
		if dl.OldNo != nil && *dl.OldNo > maxNo {
			maxNo = *dl.OldNo
		}
		if dl.NewNo != nil && *dl.NewNo > maxNo {
			maxNo = *dl.NewNo
		}
	}
	gutterWidth := len(strconv.Itoa(maxNo))
	if gutterWidth < 3 {
		gutterWidth = 3
	}

	var sb strings.Builder
	for _, dl := range lines {
		gutter := renderGutter(dl, gutterWidth)
		content := renderContent(dl)
		sb.WriteString(gutter)
		sb.WriteString(content)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// renderGutter produces the line number gutter for a diff line.
func renderGutter(dl DiffLine, width int) string {
	switch dl.Kind {
	case "hunk", "header", "fold":
		// No gutter for hunk headers, file headers, and fold indicators
		return ""
	case "add":
		old := strings.Repeat(" ", width)
		new := fmt.Sprintf("%*d", width, *dl.NewNo)
		return gutterStyle.Render(old+" │"+new+" │ ")
	case "remove":
		old := fmt.Sprintf("%*d", width, *dl.OldNo)
		new := strings.Repeat(" ", width)
		return gutterStyle.Render(old+" │"+new+" │ ")
	case "context":
		old := fmt.Sprintf("%*d", width, *dl.OldNo)
		new := fmt.Sprintf("%*d", width, *dl.NewNo)
		return gutterStyle.Render(old+" │"+new+" │ ")
	default:
		return ""
	}
}

// RenderStructuredDiff is a unified entry point for diff rendering (without language info).
// For syntax highlighting, use RenderStructuredDiffLang instead.
func RenderStructuredDiff(diffContent string, width int, compact bool) string {
	return RenderStructuredDiffLang(diffContent, "", width, compact)
}

// RenderStructuredDiffLang renders a diff with optional syntax highlighting.
// In compact mode only changed lines are shown; in full mode enhanced rendering
// (word-level diff, syntax highlighting, context folding) is applied.
func RenderStructuredDiffLang(diffContent, lang string, width int, compact bool) string {
	if compact {
		hunks := ParseUnifiedDiff(diffContent)
		return RenderCompactDiffLines(hunks, width)
	}
	return RenderEnhancedDiff(diffContent, lang, width)
}

// RenderCompactDiffLines renders only the changed lines from parsed diff lines.
// Shows at most maxLines changed lines with colored background.
func RenderCompactDiffLines(lines []DiffLine, width int) string {
	compactAddSt := lipgloss.NewStyle().Foreground(ColorDiffAddFg).Background(ColorDiffAddBg)
	compactRemoveSt := lipgloss.NewStyle().Foreground(ColorDiffDelFg).Background(ColorDiffDelBg)
	indent := "     "
	maxLines := 8
	contentWidth := width - 10

	var changed []DiffLine
	for _, dl := range lines {
		if dl.Kind == "add" || dl.Kind == "remove" {
			changed = append(changed, dl)
		}
	}

	showCount := len(changed)
	if showCount > maxLines {
		showCount = maxLines
	}

	var sb strings.Builder
	for i := 0; i < showCount; i++ {
		dl := changed[i]
		text := dl.Text
		if len(text) > 0 {
			text = text[1:] // remove +/- prefix
		}
		runes := []rune(text)
		if len(runes) > contentWidth {
			text = string(runes[:contentWidth])
		}
		if dl.Kind == "add" {
			sb.WriteString(indent + compactAddSt.Render("+ "+text) + "\n")
		} else {
			sb.WriteString(indent + compactRemoveSt.Render("- "+text) + "\n")
		}
	}

	if len(changed) > maxLines {
		remaining := len(changed) - maxLines
		muteStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
		sb.WriteString(indent + muteStyle.Render(fmt.Sprintf("+%d lines (ctrl+o to expand)", remaining)) + "\n")
	}

	return sb.String()
}

// renderContent applies syntax highlighting to a diff line.
func renderContent(dl DiffLine) string {
	switch dl.Kind {
	case "add":
		return addStyle.Render(dl.Text)
	case "remove":
		return removeStyle.Render(dl.Text)
	case "hunk":
		return hunkStyle.Render(dl.Text)
	case "fold":
		return foldStyle.Render(dl.Text)
	default:
		return dl.Text
	}
}

// DiffSegment represents a character-level diff segment.
type DiffSegment struct {
	Text    string
	Changed bool
}

// wordDiff computes word-level differences between two lines.
// Returns segments for old and new lines, marking changed words.
// Falls back to whole-line marking for very long lines (>200 chars).
func wordDiff(oldLine, newLine string) (oldSegs, newSegs []DiffSegment) {
	// Strip leading +/- prefix before diffing content
	oldContent := oldLine
	if len(oldContent) > 0 && oldContent[0] == '-' {
		oldContent = oldContent[1:]
	}
	newContent := newLine
	if len(newContent) > 0 && newContent[0] == '+' {
		newContent = newContent[1:]
	}

	// For very long lines, fall back to whole-line marking
	if len(oldContent) > 200 || len(newContent) > 200 {
		return []DiffSegment{{Text: oldLine, Changed: true}},
			[]DiffSegment{{Text: newLine, Changed: true}}
	}

	oldWords := tokenizeWords(oldContent)
	newWords := tokenizeWords(newContent)

	// Compute LCS of word sequences
	lcs := computeLCS(oldWords, newWords)

	// Build segments using the LCS alignment
	oldSegs = buildSegments(oldWords, lcs)
	newSegs = buildSegments(newWords, lcs)

	// Prepend the +/- prefix as unchanged
	if len(oldLine) > 0 {
		prefix := string(oldLine[0])
		oldSegs = append([]DiffSegment{{Text: prefix, Changed: false}}, oldSegs...)
	}
	if len(newLine) > 0 {
		prefix := string(newLine[0])
		newSegs = append([]DiffSegment{{Text: prefix, Changed: false}}, newSegs...)
	}

	return
}

// tokenizeWords splits a string into a sequence of words and separators,
// preserving all whitespace as separate tokens.
func tokenizeWords(s string) []string {
	var tokens []string
	start := 0
	inSpace := false
	for i, ch := range s {
		isSpace := ch == ' ' || ch == '\t'
		if i == 0 {
			inSpace = isSpace
			continue
		}
		if isSpace != inSpace {
			tokens = append(tokens, s[start:i])
			start = i
			inSpace = isSpace
		}
	}
	if start < len(s) {
		tokens = append(tokens, s[start:])
	}
	return tokens
}

// computeLCS finds the longest common subsequence of two string slices.
// Returns the LCS as a slice of strings.
func computeLCS(a, b []string) []string {
	m, n := len(a), len(b)
	// Use a flat DP table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] > dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to find LCS
	lcs := make([]string, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			lcs = append(lcs, a[i-1])
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	// Reverse
	for left, right := 0, len(lcs)-1; left < right; left, right = left+1, right-1 {
		lcs[left], lcs[right] = lcs[right], lcs[left]
	}
	return lcs
}

// buildSegments creates DiffSegments from tokens aligned with an LCS.
// isOld determines which side we are building (old removes / new adds).
func buildSegments(tokens, lcs []string) []DiffSegment {
	var segs []DiffSegment
	lcsIdx := 0
	for _, tok := range tokens {
		if lcsIdx < len(lcs) && tok == lcs[lcsIdx] {
			segs = append(segs, DiffSegment{Text: tok, Changed: false})
			lcsIdx++
		} else {
			segs = append(segs, DiffSegment{Text: tok, Changed: true})
		}
	}
	return segs
}

// renderSegments renders a slice of DiffSegments using add or remove highlight styles.
func renderSegments(segs []DiffSegment, isAdd bool) string {
	var sb strings.Builder
	for _, seg := range segs {
		if seg.Changed {
			if isAdd {
				sb.WriteString(addHighlightStyle.Render(seg.Text))
			} else {
				sb.WriteString(removeHighlightStyle.Render(seg.Text))
			}
		} else {
			if isAdd {
				sb.WriteString(addStyle.Render(seg.Text))
			} else {
				sb.WriteString(removeStyle.Render(seg.Text))
			}
		}
	}
	return sb.String()
}

// renderSegmentsWithSyntax renders word-diff segments, applying syntax highlighting
// to unchanged segments when lang is provided. Changed segments get the deeper
// highlight background to stand out. This combines word-diff and syntax coloring.
func renderSegmentsWithSyntax(segs []DiffSegment, isAdd bool, lang string, width int) string {
	if lang == "" {
		return renderSegments(segs, isAdd)
	}

	// Reconstruct the full line content (excluding the +/- prefix) for syntax highlighting.
	var contentBuilder strings.Builder
	prefixSeg := ""
	startIdx := 0
	if len(segs) > 0 && (segs[0].Text == "+" || segs[0].Text == "-") {
		prefixSeg = segs[0].Text
		startIdx = 1
	}
	for _, seg := range segs[startIdx:] {
		contentBuilder.WriteString(seg.Text)
	}
	highlighted := HighlightCode(contentBuilder.String(), lang, width)

	// For simplicity, render changed segments with highlight style, unchanged with syntax colors.
	// When syntax highlighting is active, we render the prefix and changed tokens with diff styles,
	// and let unchanged tokens use the highlighted version for visual richness.
	var sb strings.Builder
	if prefixSeg != "" {
		if isAdd {
			sb.WriteString(addStyle.Render(prefixSeg))
		} else {
			sb.WriteString(removeStyle.Render(prefixSeg))
		}
	}

	// Check if any segment is actually changed; if not, just return syntax highlighted line.
	anyChanged := false
	for _, seg := range segs[startIdx:] {
		if seg.Changed {
			anyChanged = true
			break
		}
	}
	if !anyChanged {
		sb.WriteString(highlighted)
		return sb.String()
	}

	// Mix: changed segments get highlight style, unchanged get base diff style.
	for _, seg := range segs[startIdx:] {
		if seg.Changed {
			if isAdd {
				sb.WriteString(addHighlightStyle.Render(seg.Text))
			} else {
				sb.WriteString(removeHighlightStyle.Render(seg.Text))
			}
		} else {
			if isAdd {
				sb.WriteString(addStyle.Render(seg.Text))
			} else {
				sb.WriteString(removeStyle.Render(seg.Text))
			}
		}
	}
	return sb.String()
}

// renderDiffLineWithSyntax applies syntax highlighting to a diff line's content.
// The gutter prefix (+/-/ ) is rendered with stable diff colors; the content
// gets syntax highlighting if lang is provided. Falls back to plain coloring on error.
func renderDiffLineWithSyntax(dl DiffLine, lang string, width int) string {
	if lang == "" {
		return renderContent(dl)
	}

	switch dl.Kind {
	case "add", "remove":
		content := dl.Text
		prefix := ""
		if len(content) > 0 {
			prefix = string(content[0])
			content = content[1:]
		}
		highlighted := HighlightCode(content, lang, width)
		if dl.Kind == "add" {
			return addStyle.Render(prefix) + highlighted
		}
		return removeStyle.Render(prefix) + highlighted

	case "context":
		content := dl.Text
		prefix := ""
		if len(content) > 0 && content[0] == ' ' {
			prefix = " "
			content = content[1:]
		}
		highlighted := HighlightCode(content, lang, width)
		return prefix + highlighted

	default:
		return renderContent(dl)
	}
}

// foldContext collapses long runs of context lines.
// Keeps the first keepEdge and last keepEdge context lines within each run,
// replacing the middle with a single "fold" DiffLine.
func foldContext(lines []DiffLine, maxContext int) []DiffLine {
	const keepEdge = 3

	// Find ranges of consecutive context lines
	type run struct{ start, end int }
	var runs []run
	i := 0
	for i < len(lines) {
		if lines[i].Kind == "context" {
			j := i
			for j < len(lines) && lines[j].Kind == "context" {
				j++
			}
			runs = append(runs, run{i, j})
			i = j
		} else {
			i++
		}
	}

	// Build output, folding runs that exceed maxContext
	out := make([]DiffLine, 0, len(lines))
	lastEnd := 0
	for _, r := range runs {
		// Copy everything before this run
		out = append(out, lines[lastEnd:r.start]...)
		runLen := r.end - r.start
		if runLen <= maxContext {
			// Short run: keep as-is
			out = append(out, lines[r.start:r.end]...)
		} else {
			// Long run: keep first keepEdge + fold + last keepEdge
			head := keepEdge
			tail := keepEdge
			if head+tail >= runLen {
				out = append(out, lines[r.start:r.end]...)
			} else {
				out = append(out, lines[r.start:r.start+head]...)
				foldCount := runLen - head - tail
				foldLine := DiffLine{
					Kind: "fold",
					Text: fmt.Sprintf("... %d unchanged lines ...", foldCount),
				}
				out = append(out, foldLine)
				out = append(out, lines[r.end-tail:r.end]...)
			}
		}
		lastEnd = r.end
	}
	// Copy remainder
	out = append(out, lines[lastEnd:]...)
	return out
}

// RenderEnhancedDiff renders a diff with word-level highlighting, optional syntax
// coloring, and context folding. It is a drop-in replacement for RenderStructuredDiff
// when richer output is desired. lang may be "" to skip syntax highlighting.
func RenderEnhancedDiff(diffContent, lang string, width int) string {
	lines := ParseUnifiedDiff(diffContent)
	lines = foldContext(lines, 8)

	// Pair adjacent remove+add lines for word diff
	type pairInfo struct {
		removeIdx int
		addIdx    int
	}
	var pairs []pairInfo
	for i := 0; i < len(lines)-1; i++ {
		if lines[i].Kind == "remove" && lines[i+1].Kind == "add" {
			pairs = append(pairs, pairInfo{i, i + 1})
		}
	}

	// Build word-diff segment maps (index → segments)
	type segPair struct {
		oldSegs []DiffSegment
		newSegs []DiffSegment
	}
	wordDiffMap := make(map[int]segPair)
	for _, p := range pairs {
		os, ns := wordDiff(lines[p.removeIdx].Text, lines[p.addIdx].Text)
		wordDiffMap[p.removeIdx] = segPair{os, ns}
		wordDiffMap[p.addIdx] = segPair{os, ns}
	}

	// Calculate gutter width
	maxNo := 0
	for _, dl := range lines {
		if dl.OldNo != nil && *dl.OldNo > maxNo {
			maxNo = *dl.OldNo
		}
		if dl.NewNo != nil && *dl.NewNo > maxNo {
			maxNo = *dl.NewNo
		}
	}
	gutterWidth := len(strconv.Itoa(maxNo))
	if gutterWidth < 3 {
		gutterWidth = 3
	}

	var sb strings.Builder
	for i, dl := range lines {
		gutter := renderGutter(dl, gutterWidth)
		sb.WriteString(gutter)

		switch dl.Kind {
		case "fold":
			sb.WriteString(foldStyle.Render(dl.Text))

		case "remove":
			if sp, ok := wordDiffMap[i]; ok {
				// Word-diff rendering with syntax highlight on unchanged segments
				sb.WriteString(renderSegmentsWithSyntax(sp.oldSegs, false, lang, width))
			} else {
				sb.WriteString(renderDiffLineWithSyntax(dl, lang, width))
			}

		case "add":
			if sp, ok := wordDiffMap[i]; ok {
				sb.WriteString(renderSegmentsWithSyntax(sp.newSegs, true, lang, width))
			} else {
				sb.WriteString(renderDiffLineWithSyntax(dl, lang, width))
			}

		default:
			sb.WriteString(renderDiffLineWithSyntax(dl, lang, width))
		}

		sb.WriteByte('\n')
	}
	return sb.String()
}
