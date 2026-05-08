package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Unified gutter constants matching Claude Code's layout.
// These define the prefix columns used by all block renderers.
const (
	gutterIndent = "  " // 2-space base indent for all blocks
)

var (
	// Computed widths — initialized on first use via ensureGutterWidths().
	gutterWidthsReady bool
	messageDotWidth   int // width of "  ⏺ " (indent + dot + space)
	responsePfxWidth  int // width of "  ⎿  " (indent + connector + 2 spaces)
)

func ensureGutterWidths() {
	if gutterWidthsReady {
		return
	}
	gutterWidthsReady = true
	messageDotWidth = lipgloss.Width(gutterIndent+FigBlackCircle) + 1 // "  ⏺" + " "
	responsePfxWidth = lipgloss.Width(gutterIndent+FigConnector) + 2  // "  ⎿" + "  "
}

// MessagePrefixWidth returns the width of the message dot prefix "  ⏺ ".
func MessagePrefixWidth() int {
	ensureGutterWidths()
	return messageDotWidth
}

// ResponsePrefixWidth returns the width of the response connector prefix "  ⎿  ".
func ResponsePrefixWidth() int {
	ensureGutterWidths()
	return responsePfxWidth
}

// BodyWidth returns the available width for body content after the message dot prefix.
// Use this when the content will have a ⏺ dot on the first line.
func BodyWidth(totalWidth int) int {
	ensureGutterWidths()
	safeTotal := TerminalSafeWidth(totalWidth)
	w := safeTotal - messageDotWidth
	if w < 1 {
		return 1
	}
	return w
}

// ContinuationBodyWidth returns the available width for continuation content
// (no dot prefix, just 2-space indent). Use this for no-dot markdown blocks.
func ContinuationBodyWidth(totalWidth int) int {
	safeTotal := TerminalSafeWidth(totalWidth)
	w := safeTotal - len(gutterIndent) // 2-space indent only
	if w < 1 {
		return 1
	}
	return w
}

// ResponseBodyWidth returns the available width for response content after "  ⎿  ".
func ResponseBodyWidth(totalWidth int) int {
	ensureGutterWidths()
	safeTotal := TerminalSafeWidth(totalWidth)
	w := safeTotal - responsePfxWidth
	if w < 1 {
		return 1
	}
	return w
}

// ComposeMessageRow formats a message row with a dot prefix on the first line
// and continuation alignment on subsequent lines.
//
//	"  ⏺ first line content"
//	"     continuation line"
func ComposeMessageRow(dot, body string, totalWidth int) string {
	ensureGutterWidths()

	lines := strings.Split(body, "\n")
	continuation := strings.Repeat(" ", messageDotWidth)

	var sb strings.Builder
	for i, line := range lines {
		if i == 0 {
			sb.WriteString(gutterIndent + dot + " " + line)
		} else {
			sb.WriteString(continuation + line)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// ComposeResponseRow formats a response row with the ⎿ connector prefix.
// If inResponse is true, the content is already inside a response and the
// connector is not added (prevents nesting).
//
//	"  ⎿  response content"
//	"      continuation"
func ComposeResponseRow(body string, totalWidth int, inResponse bool) string {
	ensureGutterWidths()

	body = strings.TrimRight(body, "\n")
	if strings.TrimSpace(body) == "" {
		return ""
	}

	// Already inside a response container — pass through.
	if inResponse {
		return body
	}

	// Safety: detect existing connector prefix to prevent nesting.
	// This handles cases where inResponse isn't propagated through the full chain yet.
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, FigConnector) {
			return body
		}
		break
	}

	lines := strings.Split(body, "\n")
	prefix := gutterIndent + connectorStyle.Render(FigConnector) + "  "
	continuation := strings.Repeat(" ", responsePfxWidth)

	var sb strings.Builder
	for i, line := range lines {
		if i == 0 {
			sb.WriteString(prefix + line)
		} else {
			sb.WriteString(continuation + line)
		}
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// ComposeContinuation formats continuation lines with the standard indent
// (no dot, no connector). Used for multi-line markdown after the first line.
//
//	"  continuation content"
func ComposeContinuation(line string) string {
	return gutterIndent + line
}
