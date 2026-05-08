package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

// StreamingMarkdownRenderer caches completed paragraph renderings to avoid
// re-rendering stable content on every streaming tick. Only the trailing
// incomplete paragraph is re-rendered each tick.
type StreamingMarkdownRenderer struct {
	// cachedSegments holds the rendered output for each completed paragraph.
	cachedSegments []string
	// cachedSource is the portion of content whose paragraphs are cached.
	cachedSource string
	// width is the render width used to produce the cached results.
	width int
}

// NewStreamingMarkdownRenderer constructs a StreamingMarkdownRenderer.
func NewStreamingMarkdownRenderer() *StreamingMarkdownRenderer {
	return &StreamingMarkdownRenderer{}
}

// Reset clears all cached state. Call this when a message finishes streaming.
func (r *StreamingMarkdownRenderer) Reset() {
	r.cachedSegments = nil
	r.cachedSource = ""
	r.width = 0
}

// Render performs incremental rendering of streaming markdown content.
// Paragraphs separated by \n\n are cached after they are syntactically closed.
// Only the trailing unclosed portion is re-rendered on each call.
//
// A paragraph is considered "cacheable" only when no code fence, blockquote, or
// other block-level construct is left open across the \n\n boundary. This prevents
// code blocks containing blank lines from being incorrectly split and cached.
func (r *StreamingMarkdownRenderer) Render(content string, width int) string {
	if width <= 0 {
		width = 80
	}

	// Split content into paragraphs.
	paragraphs := strings.Split(content, "\n\n")

	// If width changed or the content no longer starts with the cached source,
	// invalidate all cached segments.
	if r.width != width || !strings.HasPrefix(content, r.cachedSource) {
		r.cachedSegments = nil
		r.cachedSource = ""
		r.width = width
	}

	// Determine how many paragraphs are syntactically closed and cacheable.
	// We track code fence state across paragraphs: a paragraph is only cacheable
	// if all code fences opened before or within it are also closed.
	cacheableCount := 0
	openFenceMarker := ""
	for i := 0; i < len(paragraphs)-1; i++ {
		openFenceMarker = trackFenceMarkerState(paragraphs[i], openFenceMarker)
		if openFenceMarker == "" {
			cacheableCount = i + 1
		}
	}

	completeParagraphs := paragraphs[:cacheableCount]
	// The tail is everything from the first uncacheable paragraph onward.
	tailParts := paragraphs[cacheableCount:]
	tail := strings.Join(tailParts, "\n\n")

	// Trim cache if we have more cached than cacheable paragraphs.
	if len(r.cachedSegments) > len(completeParagraphs) {
		r.cachedSegments = r.cachedSegments[:len(completeParagraphs)]
	}

	// Render any newly completed paragraphs that are not yet cached.
	for i := len(r.cachedSegments); i < len(completeParagraphs); i++ {
		seg := completeParagraphs[i]
		rendered := renderSegmentFull(seg, width)
		r.cachedSegments = append(r.cachedSegments, rendered)
	}

	// Update cachedSource to be the joined cacheable paragraphs (with trailing \n\n).
	if len(completeParagraphs) > 0 {
		r.cachedSource = strings.Join(completeParagraphs, "\n\n") + "\n\n"
	} else {
		r.cachedSource = ""
	}

	// Assemble output: cached complete paragraphs + lightweight-rendered tail.
	var sb strings.Builder
	for i, seg := range r.cachedSegments {
		if i > 0 && sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(seg)
		if i < len(r.cachedSegments)-1 && !strings.HasSuffix(seg, "\n") {
			sb.WriteByte('\n')
		}
	}

	// Render the tail with lightweight renderer after patching incomplete syntax.
	if tail != "" {
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		patched := patchIncompleteMarkdown(tail)
		tailRendered := renderSegmentLightweight(patched, width)
		sb.WriteString(tailRendered)
	}

	return strings.TrimRight(sb.String(), "\n")
}

// trackFenceMarkerState scans a paragraph for code fence markers and returns
// the marker for an open fence. Backtick and tilde fences do not close each
// other; a different marker inside an open fence is code content.
func trackFenceMarkerState(paragraph string, openMarker string) string {
	for _, line := range strings.Split(paragraph, "\n") {
		marker := fenceMarker(line)
		if marker == "" {
			continue
		}
		if openMarker == "" {
			openMarker = marker
			continue
		}
		if isClosingFenceLine(openMarker, line) {
			openMarker = ""
		}
	}
	return openMarker
}

// trackFenceState scans a paragraph for code fence markers and returns whether
// any fence remains open. Kept as a small compatibility helper for tests and
// older internal callers.
func trackFenceState(paragraph string, fenceOpen bool) bool {
	openMarker := ""
	if fenceOpen {
		openMarker = "```"
	}
	return trackFenceMarkerState(paragraph, openMarker) != ""
}

// renderSegmentFull renders a complete markdown paragraph using Glamour.
func renderSegmentFull(segment string, width int) string {
	if segment == "" {
		return ""
	}
	rendered := RenderMarkdown(segment, width)
	rendered = strings.TrimSpace(rendered)
	// Strip leading indentation that Glamour may add.
	var lines []string
	for _, line := range strings.Split(rendered, "\n") {
		lines = append(lines, strings.TrimLeft(line, " \t"))
	}
	return strings.Join(lines, "\n")
}

// renderSegmentLightweight renders a potentially incomplete markdown segment
// using simple lipgloss-based styling without invoking Glamour.
func renderSegmentLightweight(segment string, width int) string {
	if segment == "" {
		return ""
	}
	if width <= 0 {
		width = 80
	}

	protectedSegment, mathSpans := protectMathSpans(segment)
	lines := strings.Split(protectedSegment, "\n")
	var sb strings.Builder
	openFenceMarker := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code fence toggle.
		if marker := fenceMarker(line); marker != "" && (openFenceMarker == "" || isClosingFenceLine(openFenceMarker, line)) {
			if openFenceMarker == "" {
				openFenceMarker = marker
			} else {
				openFenceMarker = ""
			}
			rendered := codeBlockFenceStyle.Render(line)
			sb.WriteString(rendered)
			if i < len(lines)-1 {
				sb.WriteByte('\n')
			}
			continue
		}

		// Inside a code block.
		if openFenceMarker != "" {
			wrapped := wrapTextByDisplayWidth(line, width)
			segments := strings.Split(wrapped, "\n")
			for j, segment := range segments {
				sb.WriteString(codeBlockStreamStyle.Render(segment))
				if j < len(segments)-1 || i < len(lines)-1 {
					sb.WriteByte('\n')
				}
			}
			continue
		}

		// Heading: # through ######.
		if markerLen := headingMarkerLen(trimmed); markerLen > 0 {
			headingStyle := lipgloss.NewStyle().Bold(true)
			text := strings.TrimSpace(trimmed[markerLen:])
			wrapped := wrapTextByDisplayWidth(text, width)
			sb.WriteString(headingStyle.Render(wrapped))
			if i < len(lines)-1 {
				sb.WriteByte('\n')
			}
			continue
		}

		if hasThematicBreakLine(line) {
			sb.WriteString(strings.Repeat(FigHeavyLine, normalizeMarkdownRenderWidth(width)))
			if i < len(lines)-1 {
				sb.WriteByte('\n')
			}
			continue
		}

		if strings.HasPrefix(trimmed, "> ") {
			text := renderInlineMarkup(strings.TrimSpace(trimmed[2:]))
			rendered := FigBlockquote + " " + text
			rendered = ansiWrap(rendered, width)
			sb.WriteString(rendered)
			if i < len(lines)-1 {
				sb.WriteByte('\n')
			}
			continue
		}

		if hasTaskListPrefix(trimmed) {
			marker := "[ ]"
			text := strings.TrimSpace(trimmed[5:])
			if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
				marker = "[x]"
				text = strings.TrimSpace(trimmed[6:])
			}
			text = renderInlineMarkup(text)
			rendered := "  - " + marker + " " + text
			rendered = ansiWrap(rendered, width)
			sb.WriteString(rendered)
			if i < len(lines)-1 {
				sb.WriteByte('\n')
			}
			continue
		}

		// List items: -, *, +, and ordered 1./1).
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
			marker := trimmed[:2]
			text := strings.TrimSpace(trimmed[2:])
			text = renderInlineMarkup(text)
			rendered := "  " + marker + text
			// ANSI-safe wrapping: use xansi.Wrap for proper word-break with escape sequences
			rendered = ansiWrap(rendered, width)
			sb.WriteString(rendered)
			if i < len(lines)-1 {
				sb.WriteByte('\n')
			}
			continue
		}
		if prefixLen := orderedListPrefixLen(trimmed); prefixLen > 0 {
			marker := trimmed[:prefixLen]
			text := strings.TrimSpace(trimmed[prefixLen:])
			text = renderInlineMarkup(text)
			rendered := "  " + marker + text
			rendered = ansiWrap(rendered, width)
			sb.WriteString(rendered)
			if i < len(lines)-1 {
				sb.WriteByte('\n')
			}
			continue
		}

		// Regular paragraph line: apply inline markup, then ANSI-safe wrap.
		rendered := renderInlineMarkup(line)
		rendered = ansiWrap(rendered, width)
		sb.WriteString(rendered)
		if i < len(lines)-1 {
			sb.WriteByte('\n')
		}
	}

	return restoreMathSpans(sb.String(), mathSpans)
}

func headingMarkerLen(line string) int {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 || i >= len(line) || line[i] != ' ' {
		return 0
	}
	return i
}

func orderedListPrefixLen(line string) int {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) {
		return 0
	}
	if (line[i] != '.' && line[i] != ')') || line[i+1] != ' ' {
		return 0
	}
	return i + 2
}

func hasTaskListPrefix(line string) bool {
	return strings.HasPrefix(line, "- [ ] ") ||
		strings.HasPrefix(line, "- [x] ") ||
		strings.HasPrefix(line, "- [X] ")
}

// ansiWrap wraps a string that may contain ANSI escape sequences at the given width.
// Uses xansi.Wrap for ANSI-safe word breaking.
func ansiWrap(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	return xansi.Wrap(s, width, "")
}

// renderInlineMarkup applies simple inline lipgloss styling for bold, italic, and inline code.
// This is a best-effort approximation suitable for streaming partial content.
func renderInlineMarkup(s string) string {
	boldStyle := lipgloss.NewStyle().Bold(true)
	italicStyle := lipgloss.NewStyle().Italic(true)
	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236"))

	var result strings.Builder
	i := 0
	runes := []rune(s)
	n := len(runes)

	for i < n {
		// Inline code: `...`
		if runes[i] == '`' {
			end := -1
			for j := i + 1; j < n; j++ {
				if runes[j] == '`' {
					end = j
					break
				}
			}
			if end > i {
				inner := string(runes[i+1 : end])
				result.WriteString(codeStyle.Render(inner))
				i = end + 1
				continue
			}
		}

		// Bold: **...**
		if i+1 < n && runes[i] == '*' && runes[i+1] == '*' {
			end := -1
			for j := i + 2; j+1 < n; j++ {
				if runes[j] == '*' && runes[j+1] == '*' {
					end = j
					break
				}
			}
			if end > i+1 {
				inner := string(runes[i+2 : end])
				result.WriteString(boldStyle.Render(inner))
				i = end + 2
				continue
			}
		}

		// Italic: *...*
		if runes[i] == '*' {
			end := -1
			for j := i + 1; j < n; j++ {
				if runes[j] == '*' {
					end = j
					break
				}
			}
			if end > i {
				inner := string(runes[i+1 : end])
				result.WriteString(italicStyle.Render(inner))
				i = end + 1
				continue
			}
		}

		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}
