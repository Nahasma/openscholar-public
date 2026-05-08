package components

import (
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

// rendererPool caches glamour renderers by width.
type rendererPool struct {
	mu    sync.RWMutex
	items map[int]*glamour.TermRenderer
}

var pool *rendererPool

func init() {
	pool = &rendererPool{
		items: make(map[int]*glamour.TermRenderer),
	}
}

func normalizeMarkdownRenderWidth(width int) int {
	// Minimum width: Glamour needs at least 10 to produce anything useful
	if width < 10 {
		width = 10
	}
	// Round to nearest 5 to reduce cache entries while preserving narrow accuracy
	width = (width / 5) * 5
	if width < 10 {
		width = 10
	}
	return width
}

func baseMarkdownStyle() ansi.StyleConfig {
	var style ansi.StyleConfig
	if IsDarkTheme {
		style = styles.DarkStyleConfig
	} else {
		style = styles.LightStyleConfig
	}
	noMargin := uint(0)
	style.Document.Margin = &noMargin
	// Tighten internal margins so Glamour output can be aligned by the caller
	// without extra leading space that breaks table/list alignment.
	style.Paragraph.Margin = &noMargin
	style.List.Margin = &noMargin
	style.BlockQuote.Margin = &noMargin
	style.CodeBlock.Margin = &noMargin
	style.Table.Margin = &noMargin
	levelIndent := uint(2)
	style.List.LevelIndent = levelIndent
	// Avoid raw-looking heading markers in rendered output.
	style.H2.Prefix = ""
	style.H3.Prefix = ""
	style.H4.Prefix = ""
	style.H5.Prefix = ""
	style.H6.Prefix = ""
	return style
}

func styleForWidth(width int) ansi.StyleConfig {
	style := baseMarkdownStyle()
	hrWidth := width
	if hrWidth < 3 {
		hrWidth = 3
	}
	style.HorizontalRule.Format = "\n" + strings.Repeat(FigHeavyLine, hrWidth) + "\n"
	return style
}

func (p *rendererPool) get(width int) *glamour.TermRenderer {
	width = normalizeMarkdownRenderWidth(width)

	p.mu.RLock()
	if r, ok := p.items[width]; ok {
		p.mu.RUnlock()
		return r
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if r, ok := p.items[width]; ok {
		return r
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(styleForWidth(width)),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}

	// Limit cache size: evict all if too many entries
	if len(p.items) > 10 {
		p.items = make(map[int]*glamour.TermRenderer)
	}

	p.items[width] = r
	return r
}

// RenderMarkdown renders markdown content for terminal display at the given width.
// Falls back to plain text if the renderer is unavailable.
func RenderMarkdown(content string, width int) string {
	if pool == nil || content == "" {
		return content
	}
	if isPlainTextForFastPath(content) {
		protectedContent, mathSpans := protectMathSpans(content)
		return ansiWrap(restoreMathSpans(protectedContent, mathSpans), width)
	}

	renderer := pool.get(width)
	if renderer == nil {
		return content
	}

	protectedContent, mathSpans := protectMathSpans(content)
	rendered, err := renderer.Render(protectedContent)
	if err != nil {
		return content
	}
	rendered = restoreMathSpans(rendered, mathSpans)

	// Glamour adds leading/trailing newlines; strip only newlines to preserve
	// internal indentation (tables, nested lists, block quotes).
	rendered = strings.Trim(rendered, "\n")

	return rendered
}

type mathSpanReplacement struct {
	token  string
	source string
}

func protectMathSpans(content string) (string, []mathSpanReplacement) {
	var replacements []mathSpanReplacement
	var b strings.Builder
	b.Grow(len(content))

	nextToken := 0
	addSpan := func(source string) {
		token := ""
		for {
			token = "OpenScholarMathToken" + strconv.Itoa(nextToken) + "End"
			nextToken++
			if !strings.Contains(content, token) {
				break
			}
		}
		replacements = append(replacements, mathSpanReplacement{
			token:  token,
			source: source,
		})
		b.WriteString(token)
	}

	for i := 0; i < len(content); {
		switch {
		case content[i] == '`':
			if end := findClosingCodeSpan(content, i); end >= 0 {
				b.WriteString(content[i : end+1])
				i = end + 1
				continue
			}
		case strings.HasPrefix(content[i:], "$$"):
			if end := strings.Index(content[i+2:], "$$"); end >= 0 {
				end += i + 4
				addSpan(content[i:end])
				i = end
				continue
			}
		case strings.HasPrefix(content[i:], `\[`):
			if end := strings.Index(content[i+2:], `\]`); end >= 0 {
				end += i + 4
				addSpan(content[i:end])
				i = end
				continue
			}
		case strings.HasPrefix(content[i:], `\(`):
			if end := strings.Index(content[i+2:], `\)`); end >= 0 {
				end += i + 4
				addSpan(content[i:end])
				i = end
				continue
			}
		case canOpenInlineDollar(content, i):
			if i+1 < len(content) && content[i+1] == '$' {
				break
			}
			if end := findClosingInlineDollar(content, i+1); end >= 0 {
				addSpan(content[i : end+1])
				i = end + 1
				continue
			}
		}

		b.WriteByte(content[i])
		i++
	}

	if len(replacements) == 0 {
		return content, nil
	}
	return b.String(), replacements
}

func restoreMathSpans(content string, replacements []mathSpanReplacement) string {
	for _, replacement := range replacements {
		content = strings.ReplaceAll(content, replacement.token, renderMathSpan(replacement.source))
	}
	return content
}

func renderMathSpan(source string) string {
	return strings.TrimSpace(source)
}

func findClosingCodeSpan(content string, start int) int {
	markerLen := 0
	for start+markerLen < len(content) && content[start+markerLen] == '`' {
		markerLen++
	}
	if markerLen == 0 {
		return -1
	}
	for i := start + markerLen; i < len(content); i++ {
		if content[i] != '`' {
			continue
		}
		runLen := 0
		for i+runLen < len(content) && content[i+runLen] == '`' {
			runLen++
		}
		if runLen == markerLen {
			return i + runLen - 1
		}
		i += runLen - 1
	}
	return -1
}

func findClosingInlineDollar(content string, start int) int {
	for i := start; i < len(content); i++ {
		if content[i] == '\n' || content[i] == '\r' {
			return -1
		}
		if !canCloseInlineDollar(content, i) {
			continue
		}
		return i
	}
	return -1
}

func canOpenInlineDollar(content string, pos int) bool {
	if pos < 0 || pos >= len(content) || content[pos] != '$' || isEscapedMarkdownDelimiter(content, pos) {
		return false
	}
	if pos+1 >= len(content) || content[pos+1] == '$' || content[pos+1] == '\n' || content[pos+1] == '\r' || content[pos+1] == ' ' || content[pos+1] == '\t' {
		return false
	}
	// Avoid treating common currency like "$5" as math.
	if content[pos+1] >= '0' && content[pos+1] <= '9' {
		return false
	}
	return true
}

func canCloseInlineDollar(content string, pos int) bool {
	if pos < 0 || pos >= len(content) || content[pos] != '$' || isEscapedMarkdownDelimiter(content, pos) {
		return false
	}
	if pos+1 < len(content) && content[pos+1] == '$' {
		return false
	}
	if pos == 0 || content[pos-1] == ' ' || content[pos-1] == '\t' || content[pos-1] == '\n' || content[pos-1] == '\r' {
		return false
	}
	return true
}

func isEscapedMarkdownDelimiter(content string, pos int) bool {
	backslashes := 0
	for i := pos - 1; i >= 0 && content[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func isPlainTextForFastPath(content string) bool {
	return !hasMarkdownSyntax(content)
}

func hasHeadingPrefix(line string) bool {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	return i > 0 && i <= 6 && i < len(line) && line[i] == ' '
}

func hasOrderedListPrefix(line string) bool {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(line) && (line[i] == '.' || line[i] == ')') && line[i+1] == ' '
}

func hasMarkdownSyntax(content string) bool {
	if strings.Contains(content, "**") ||
		strings.Contains(content, "__") ||
		strings.Contains(content, "`") ||
		strings.Contains(content, "](") ||
		strings.Contains(content, "![") ||
		strings.Count(content, "*") >= 2 ||
		strings.Count(content, "_") >= 2 {
		return true
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if hasHeadingPrefix(trimmed) ||
			hasThematicBreakLine(line) ||
			strings.HasPrefix(trimmed, ">") ||
			hasFencePrefix(line) ||
			strings.HasPrefix(trimmed, "- ") ||
			strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "+ ") ||
			hasOrderedListPrefix(trimmed) ||
			strings.HasPrefix(trimmed, "|") {
			return true
		}
	}
	return false
}

func hasThematicBreakLine(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}

	leadingSpaces := 0
	for leadingSpaces < len(line) && line[leadingSpaces] == ' ' {
		leadingSpaces++
	}
	// 4-space indentation is code block territory, not thematic break.
	if leadingSpaces > 3 {
		return false
	}

	candidate := strings.TrimRight(line[leadingSpaces:], " \t")
	if candidate == "" {
		return false
	}
	// Reject common non-break constructs.
	if strings.Contains(candidate, "|") {
		return false
	}

	marker := byte(0)
	markerCount := 0
	fields := strings.Fields(candidate)
	for i, field := range fields {
		if len(fields) > 1 && i == 1 && len(fields[0]) == 1 && len(field) > 1 {
			return false
		}
		for i := 0; i < len(field); i++ {
			ch := field[i]
			if ch != '-' && ch != '*' && ch != '_' {
				return false
			}
			if marker == 0 {
				marker = ch
			}
			if ch != marker {
				return false
			}
			markerCount++
		}
	}

	if markerCount < 3 {
		return false
	}
	return true
}

func hasFencePrefix(line string) bool {
	return fenceMarker(line) != ""
}

func fenceMarker(line string) string {
	trimmed, ok := fenceCandidate(line)
	if !ok || trimmed == "" {
		return ""
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return ""
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == ch {
		i++
	}
	if i < 3 {
		return ""
	}
	if ch == '`' && strings.Contains(trimmed[i:], "`") {
		return ""
	}
	return trimmed[:i]
}

func closingFenceMarker(line string) string {
	trimmed, ok := fenceCandidate(line)
	if !ok || trimmed == "" {
		return ""
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return ""
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == ch {
		i++
	}
	if i < 3 || strings.TrimSpace(trimmed[i:]) != "" {
		return ""
	}
	return trimmed[:i]
}

func fenceCandidate(line string) (string, bool) {
	leadingSpaces := 0
	for leadingSpaces < len(line) && line[leadingSpaces] == ' ' {
		leadingSpaces++
	}
	if leadingSpaces > 3 {
		return "", false
	}
	if leadingSpaces < len(line) && line[leadingSpaces] == '\t' {
		return "", false
	}
	return strings.TrimRight(line[leadingSpaces:], " \t"), true
}

func isClosingFenceMarker(openMarker, marker string) bool {
	return openMarker != "" &&
		marker != "" &&
		openMarker[0] == marker[0] &&
		len(marker) >= len(openMarker)
}

func isClosingFenceLine(openMarker, line string) bool {
	return isClosingFenceMarker(openMarker, closingFenceMarker(line))
}

// Ensure ansi package is referenced (StyleConfig types come from it)
var _ ansi.StyleConfig
