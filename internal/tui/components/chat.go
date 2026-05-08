package components

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/Nahasma/openscholar-public/internal/message"
)

var (
	userPrefixStyle      lipgloss.Style
	assistantPrefixStyle lipgloss.Style
	toolDoneStyle        lipgloss.Style
	toolRunningStyle     lipgloss.Style
	toolErrorStyle       lipgloss.Style
	errorStyle           lipgloss.Style
	connectorStyle       lipgloss.Style
	thinkingStyle        lipgloss.Style
	thinkingLabelStyle   lipgloss.Style
	userContentStyle     lipgloss.Style
	toolNameStyle        lipgloss.Style
	toolParamStyle       lipgloss.Style
	toolResultMutedStyle lipgloss.Style

	codeBlockFenceStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243"))
	codeBlockStreamStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("236"))
)

// RenderCache caches Glamour rendering results for completed messages.
type RenderCache struct {
	mu    sync.RWMutex
	items map[string]string
}

func NewRenderCache() *RenderCache {
	return &RenderCache{items: make(map[string]string)}
}

func (c *RenderCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.items[key]
	return v, ok
}

func (c *RenderCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = value
}

func renderCacheKey(msgID string, contentLen int, width int) string {
	return fmt.Sprintf("%s:%d:%d", msgID, contentLen, width)
}

func initChatStyles() {
	// Base styles — used by shared renderers (dialog, progress line, summary, inline expand)
	userPrefixStyle = lipgloss.NewStyle().
		Foreground(ColorGrayUser)
	assistantPrefixStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBrandPurple)
	toolDoneStyle = lipgloss.NewStyle().
		Foreground(ColorGreen)
	toolRunningStyle = lipgloss.NewStyle().
		Foreground(ColorOrange)
	toolErrorStyle = lipgloss.NewStyle().
		Foreground(ColorRed)
	errorStyle = lipgloss.NewStyle().
		Foreground(ColorRed)
	connectorStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
	thinkingStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium).
		Italic(true)
	thinkingLabelStyle = lipgloss.NewStyle().
		Foreground(ColorBrandPurple).
		Bold(true)
	if IsDarkTheme {
		userContentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("16")).
			Background(lipgloss.Color("255")).
			PaddingLeft(1).
			PaddingRight(1)
	} else {
		userContentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("16")).
			PaddingLeft(1).
			PaddingRight(1)
	}
	toolNameStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorWhite)
	toolParamStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
	toolResultMutedStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)

	// V2 styles — Theme-token based, used by V2 renderers
	userPromptStyleV2 = lipgloss.NewStyle().Foreground(Theme.User)
	systemLineStyleV2 = lipgloss.NewStyle().Foreground(Theme.System)
	thinkingPrefixStyleV2 = lipgloss.NewStyle().Foreground(Theme.TextMuted).Italic(true)
	userPrefixStyleV2 = lipgloss.NewStyle().Foreground(Theme.User)
	assistantPrefixStyleV2 = lipgloss.NewStyle().Foreground(Theme.Assistant).Bold(true)
	thinkingLabelStyleV2 = lipgloss.NewStyle(). // dimColor + italic
							Foreground(ColorGrayMedium).Italic(true)
	userContentStyleV2 = lipgloss.NewStyle().
		Foreground(Theme.TextPrimary).
		Background(Theme.UserBackground).
		PaddingRight(1)
	toolNameStyleV2 = lipgloss.NewStyle().Bold(true) // bold, default foreground
}

// InlineError represents an error displayed inline in chat.
type InlineError struct {
	Text      string
	Timestamp int64
}

// GetToolAction returns a brief action verb for a running tool (exported for tui.go).
func GetToolAction(name string) string {
	return GetPresenter(name).RunningLabel()
}

// getToolResultSummary generates a one-line summary for a completed tool call.
func getToolResultSummary(tc message.ToolCall, toolResults map[string]message.Message) string {
	result, ok := toolResultForCall(tc.ID, toolResults)
	if !ok {
		return ""
	}

	if result.IsError {
		errText := strings.ReplaceAll(result.Content, "\n", " ")
		errText = truncateDisplay(errText, 80)
		return "Error: " + errText
	}

	lines := strings.Split(result.Content, "\n")
	lineCount := len(lines)

	switch tc.Name {
	case "Write":
		// Extract file path and count lines written
		var params map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &params); err == nil {
			if fp, ok := params["file_path"].(string); ok {
				// Count lines from the content param if available
				contentLines := lineCount
				if c, ok := params["content"].(string); ok {
					contentLines = len(strings.Split(c, "\n"))
				}
				return fmt.Sprintf("+%d lines → %s", contentLines, filepath.Base(fp))
			}
		}
		return fmt.Sprintf("+%d lines written", lineCount)
	case "Edit":
		// Count added/removed lines from diff content
		addCount, removeCount := 0, 0
		for _, l := range lines {
			if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
				addCount++
			} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
				removeCount++
			}
		}
		// Extract file path from params
		var filePart string
		var params map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &params); err == nil {
			if fp, ok := params["file_path"].(string); ok {
				filePart = filepath.Base(fp)
			}
		}
		var editParts []string
		if addCount > 0 {
			editParts = append(editParts, fmt.Sprintf("+%d", addCount))
		}
		if removeCount > 0 {
			editParts = append(editParts, fmt.Sprintf("-%d", removeCount))
		}
		if len(editParts) > 0 {
			summary := strings.Join(editParts, " / ")
			if filePart != "" {
				return summary + " lines → " + filePart
			}
			return summary + " lines"
		}
		if filePart != "" {
			return "edited " + filePart
		}
		return fmt.Sprintf("applied edit (%d lines)", lineCount)
	case "View", "Read":
		return fmt.Sprintf("read %d lines", lineCount)
	case "Bash":
		// Extract exit code from result metadata if encoded
		exitCode := 0
		content := result.Content
		// Check for exit code prefix pattern "exit:N\n..."
		if strings.HasPrefix(content, "exit:") {
			if newline := strings.IndexByte(content, '\n'); newline > 0 {
				codeStr := content[5:newline]
				if n, err := fmt.Sscanf(codeStr, "%d", &exitCode); n == 1 && err == nil {
					content = content[newline+1:]
					lines = strings.Split(content, "\n")
					lineCount = len(lines)
				}
			}
		}
		if strings.TrimSpace(content) == "" {
			if exitCode != 0 {
				return fmt.Sprintf("exit %d (no output)", exitCode)
			}
			return "(no output)"
		}
		outputLines := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				outputLines++
			}
		}
		if exitCode != 0 {
			return fmt.Sprintf("exit %d · %d lines", exitCode, outputLines)
		}
		return fmt.Sprintf("exit 0 · %d lines", outputLines)
	case "Glob":
		// Count non-empty lines as matched files
		fileCount := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				fileCount++
			}
		}
		return fmt.Sprintf("%d files matched", fileCount)
	case "Grep":
		// Count file hits and line hits from grep output
		fileSet := make(map[string]struct{})
		hitLines := 0
		for _, l := range lines {
			if strings.TrimSpace(l) == "" {
				continue
			}
			hitLines++
			// grep output typically "file:line:content"
			if colon := strings.IndexByte(l, ':'); colon > 0 {
				fileSet[l[:colon]] = struct{}{}
			}
		}
		if len(fileSet) > 0 {
			return fmt.Sprintf("%d files · %d matches", len(fileSet), hitLines)
		}
		return fmt.Sprintf("%d matches", hitLines)
	case "Task":
		// Try to extract title from params
		var params map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &params); err == nil {
			title, _ := params["title"].(string)
			status, _ := params["status"].(string)
			if title != "" {
				if status != "" {
					return fmt.Sprintf("%s · %s", truncateDisplay(title, 40), status)
				}
				return truncateDisplay(title, 60)
			}
		}
		if strings.TrimSpace(result.Content) == "" {
			return "task completed"
		}
		return fmt.Sprintf("task done · %d lines", lineCount)
	default:
		if lineCount > 1 {
			return fmt.Sprintf("%d lines", lineCount)
		}
		firstLine := strings.TrimSpace(lines[0])
		firstLine = truncateDisplay(firstLine, 80)
		return firstLine
	}
}

// renderToolResult renders the tool call result below the tool header (expanded view).
func renderToolResult(
	sb *strings.Builder,
	tc message.ToolCall,
	toolResults map[string]message.Message,
	expanded map[string]bool,
	width int,
) {
	result, ok := toolResultForCall(tc.ID, toolResults)
	if !ok {
		return
	}
	if result.Content == "" {
		return
	}

	contentWidth := width - 8

	switch tc.Name {
	case "Edit":
		renderDiffResult(sb, result.Content, contentWidth)
	case "Write":
		renderWritePreview(sb, tc.Input, result.Content, contentWidth)
	case "View":
		// Extract file_path for language detection
		var viewLang string
		var viewParams map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &viewParams); err == nil {
			if fp, ok := viewParams["file_path"].(string); ok {
				viewLang = extToLang(filepath.Ext(fp))
			}
		}
		renderCodeResult(sb, result.Content, contentWidth, viewLang)
	case "Bash":
		renderCodeResult(sb, result.Content, contentWidth)
	case "Glob", "Grep":
		renderCodeResult(sb, result.Content, contentWidth)
	default:
		if result.IsError {
			errText := strings.ReplaceAll(result.Content, "\n", " ")
			errText = truncateDisplay(errText, 80)
			sb.WriteString("     " + toolErrorStyle.Render("✗ "+errText) + "\n")
		} else {
			renderCodeResult(sb, result.Content, contentWidth)
		}
	}
}

// renderDiffResult renders a unified diff with foreground-only colors for add/remove lines.
func renderDiffResult(sb *strings.Builder, content string, maxWidth int) {
	lineNumStyle := lipgloss.NewStyle().Foreground(ColorGrayDim)
	diffAddStyle := lipgloss.NewStyle().Foreground(ColorDiffAddFg)
	diffRemoveStyle := lipgloss.NewStyle().Foreground(ColorDiffDelFg)
	headerStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)
	contextLineStyle := lipgloss.NewStyle().Foreground(ColorGrayBright)
	indent := "     "

	lines := strings.Split(content, "\n")

	// Parse @@ hunk headers to track line numbers
	oldLineNum := 0
	newLineNum := 0

	// contentWidth is the available width for text after indent + line number + "  " prefix
	contentWidth := maxWidth - 8

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "@@"):
			if parts := parseHunkHeader(line); parts != nil {
				oldLineNum = parts[0]
				newLineNum = parts[1]
			}
			sb.WriteString(indent + headerStyle.Render(truncateStr(line, maxWidth)) + "\n")
		case strings.HasPrefix(line, "+"):
			lineText := line[1:]
			numStr := lineNumStyle.Render(fmt.Sprintf("%4d ", newLineNum))
			wrapped := wrapLine(lineText, contentWidth)
			for i, seg := range wrapped {
				prefix := "+ "
				if i > 0 {
					prefix = "  "
				}
				if i == 0 {
					sb.WriteString(indent + numStr + diffAddStyle.Render(prefix+seg) + "\n")
				} else {
					sb.WriteString(indent + "     " + diffAddStyle.Render(prefix+seg) + "\n")
				}
			}
			newLineNum++
		case strings.HasPrefix(line, "-"):
			lineText := line[1:]
			numStr := lineNumStyle.Render(fmt.Sprintf("%4d ", oldLineNum))
			wrapped := wrapLine(lineText, contentWidth)
			for i, seg := range wrapped {
				prefix := "- "
				if i > 0 {
					prefix = "  "
				}
				if i == 0 {
					sb.WriteString(indent + numStr + diffRemoveStyle.Render(prefix+seg) + "\n")
				} else {
					sb.WriteString(indent + "     " + diffRemoveStyle.Render(prefix+seg) + "\n")
				}
			}
			oldLineNum++
		default:
			numStr := lineNumStyle.Render(fmt.Sprintf("%4d ", newLineNum))
			wrapped := wrapLine(line, contentWidth)
			for i, seg := range wrapped {
				if i == 0 {
					sb.WriteString(indent + numStr + contextLineStyle.Render("  "+seg) + "\n")
				} else {
					sb.WriteString(indent + "     " + contextLineStyle.Render("  "+seg) + "\n")
				}
			}
			oldLineNum++
			newLineNum++
		}
	}
}

// renderCompactDiff renders a compact diff preview showing only changed lines (max 8 lines).
func renderCompactDiff(sb *strings.Builder, tc message.ToolCall, toolResults map[string]message.Message, width int) {
	result, ok := toolResultForCall(tc.ID, toolResults)
	if !ok {
		return
	}
	if result.Content == "" {
		return
	}

	compactAddStyle := lipgloss.NewStyle().Foreground(ColorDiffAddFg).Background(ColorDiffAddBg)
	compactRemoveStyle := lipgloss.NewStyle().Foreground(ColorDiffDelFg).Background(ColorDiffDelBg)
	indent := "     "
	maxLines := 8
	contentWidth := width - 10 // account for indent + "+ " prefix

	// Collect only changed lines
	lines := strings.Split(result.Content, "\n")
	var changedLines []string
	for _, line := range lines {
		if (strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++")) ||
			(strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---")) {
			changedLines = append(changedLines, line)
		}
	}

	showCount := len(changedLines)
	if showCount > maxLines {
		showCount = maxLines
	}

	for i := 0; i < showCount; i++ {
		line := changedLines[i]
		text := line[1:] // remove +/- prefix from content
		if len([]rune(text)) > contentWidth {
			text = string([]rune(text)[:contentWidth])
		}
		if strings.HasPrefix(line, "+") {
			sb.WriteString(indent + compactAddStyle.Render("+ "+text) + "\n")
		} else {
			sb.WriteString(indent + compactRemoveStyle.Render("- "+text) + "\n")
		}
	}

	if len(changedLines) > maxLines {
		remaining := len(changedLines) - maxLines
		sb.WriteString(indent +
			toolResultMutedStyle.Render(fmt.Sprintf("+%d lines (ctrl+o to expand)", remaining)) + "\n")
	}
}

// renderCompactOutput renders a compact preview of plain-text tool output (max 8 lines).
func renderCompactOutput(sb *strings.Builder, tc message.ToolCall, toolResults map[string]message.Message, width int) {
	// Skip compact preview for Task — subagent output is too verbose for inline display
	if tc.Name == "Task" {
		return
	}
	result, ok := toolResultForCall(tc.ID, toolResults)
	if !ok {
		return
	}

	indent := "     "
	maxLines := 8
	contentWidth := width - 10

	// Skip empty content and errors (already shown in summary line)
	if strings.TrimSpace(result.Content) == "" || result.IsError {
		return
	}

	lines := strings.Split(strings.TrimRight(result.Content, "\n"), "\n")

	// Skip single-line content — already shown in summary
	if len(lines) <= 1 {
		return
	}

	showCount := len(lines)
	if showCount > maxLines {
		showCount = maxLines
	}

	for i := 0; i < showCount; i++ {
		line := lines[i]
		if len([]rune(line)) > contentWidth {
			line = string([]rune(line)[:contentWidth])
		}
		sb.WriteString(indent + toolResultMutedStyle.Render(line) + "\n")
	}

	if len(lines) > maxLines {
		remaining := len(lines) - maxLines
		sb.WriteString(indent +
			toolResultMutedStyle.Render(fmt.Sprintf("… +%d lines (ctrl+o to expand)", remaining)) + "\n")
	}
}

// parseHunkHeader parses @@ -old,count +new,count @@ and returns [oldStart, newStart].
func parseHunkHeader(line string) []int {
	// Find the @@ markers
	if !strings.HasPrefix(line, "@@") {
		return nil
	}
	parts := strings.SplitN(line, "@@", 3)
	if len(parts) < 2 {
		return nil
	}
	header := strings.TrimSpace(parts[1])
	// header is like "-10,5 +10,6"
	fields := strings.Fields(header)
	oldStart, newStart := 1, 1
	for _, f := range fields {
		if strings.HasPrefix(f, "-") {
			fmt.Sscanf(f, "-%d", &oldStart)
		} else if strings.HasPrefix(f, "+") {
			fmt.Sscanf(f, "+%d", &newStart)
		}
	}
	return []int{oldStart, newStart}
}

// renderWritePreview renders a preview of written file content with language hint.
func renderWritePreview(sb *strings.Builder, input string, resultContent string, maxWidth int) {
	indent := "     "

	// Extract file path from input to determine language
	var lang string
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err == nil {
		if fp, ok := params["file_path"].(string); ok {
			ext := filepath.Ext(fp)
			lang = extToLang(ext)
		}
	}

	// Try to get the actual content from input params (file_content field)
	var content string
	if params != nil {
		if fc, ok := params["file_content"].(string); ok && fc != "" {
			content = fc
		} else if fc, ok := params["content"].(string); ok && fc != "" {
			content = fc
		}
	}
	if content == "" {
		content = resultContent
	}

	if lang != "" {
		sb.WriteString(indent + toolResultMutedStyle.Render("```"+lang) + "\n")
	}

	// Apply syntax highlighting
	displayContent := content
	if lang != "" {
		displayContent = HighlightCode(content, lang, maxWidth)
	}
	contPad := indent + "  " // continuation indent
	for _, line := range strings.Split(displayContent, "\n") {
		wrapped := wrapLine(line, maxWidth)
		for i, seg := range wrapped {
			if i == 0 {
				sb.WriteString(indent + seg + "\n")
			} else {
				sb.WriteString(contPad + seg + "\n")
			}
		}
	}

	if lang != "" {
		sb.WriteString(indent + toolResultMutedStyle.Render("```") + "\n")
	}
}

// extToLang maps file extension to language name for code block display.
func extToLang(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".sh", ".bash":
		return "bash"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".md":
		return "markdown"
	case ".tex":
		return "latex"
	case ".sql":
		return "sql"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".c":
		return "c"
	case ".cpp", ".cc":
		return "cpp"
	case ".h":
		return "c"
	}
	return ""
}

// renderCodeResult renders code/text content with line limit and optional syntax highlighting.
func renderCodeResult(sb *strings.Builder, content string, maxWidth int, lang ...string) {
	indent := "     "

	// Apply syntax highlighting if language is provided
	highlighted := content
	if len(lang) > 0 && lang[0] != "" {
		highlighted = HighlightCode(content, lang[0], maxWidth)
	}

	contPad := indent + "  "
	for _, line := range strings.Split(highlighted, "\n") {
		wrapped := wrapLine(line, maxWidth)
		for i, seg := range wrapped {
			if i == 0 {
				sb.WriteString(indent + seg + "\n")
			} else {
				sb.WriteString(contPad + seg + "\n")
			}
		}
	}
}

// getToolDisplayName returns a human-readable tool name.
func getToolDisplayName(name string) string {
	return GetPresenter(name).DisplayName()
}

// extractToolParamsSummary extracts a short summary from tool input params.
// Delegates to ToolPresenter.FormatParams() as the single source of truth.
func extractToolParamsSummary(toolName, input string) string {
	if input == "" {
		return ""
	}
	// Use presenter as primary source
	if result := GetPresenter(toolName).FormatParams(input); result != "" {
		return result
	}
	// Fallback for unrecognized tools or empty presenter result
	params := parseInputJSON(input)
	if params != nil {
		return extractBestParamValue(params)
	}
	s := strings.ReplaceAll(input, "\n", " ")
	return truncateDisplay(s, 60)
}

// extractBestParamValue extracts the most meaningful value from params for display.
func extractBestParamValue(params map[string]any) string {
	priorities := []string{"query", "description", "file_path", "name", "command", "prompt", "input", "url", "pattern", "question"}
	for _, key := range priorities {
		if v, ok := params[key].(string); ok && v != "" {
			if key == "file_path" {
				return truncatePathMiddle(v, 60)
			}
			return truncateDisplay(v, 60)
		}
	}
	// Last resort: show param key names instead of raw JSON
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 3 {
		keys = keys[:3]
		keys = append(keys, "...")
	}
	if len(keys) > 0 {
		return strings.Join(keys, ", ")
	}
	return ""
}

// truncateToWidth truncates a string so its display width fits within maxWidth,
// appending ellipsis if truncated. Uses xansi for ANSI-escape-aware truncation.
func truncateToWidth(s string, maxWidth int, ellipsis string) string {
	if maxWidth <= 0 {
		return s
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	return xansi.Truncate(s, maxWidth, ellipsis)
}

func truncateStr(s string, maxLen int) string {
	return truncateDisplay(s, maxLen)
}

// wrapLine wraps a long line into multiple visual lines at maxWidth.
// Returns a slice of wrapped segments.
func wrapLine(s string, maxWidth int) []string {
	return wrapDisplay(s, maxWidth)
}

// streamRenderers maps msgID to a per-message StreamingMarkdownRenderer.
var streamRenderers sync.Map // msgID -> *StreamingMarkdownRenderer

// CleanupStreamRenderer removes the streaming renderer for a finished message.
func CleanupStreamRenderer(msgID string) {
	streamRenderers.Delete(msgID)
}

// renderStreamingMarkdown renders streaming content using incremental paragraph caching.
// Only the last (incomplete) paragraph is re-rendered on each tick; completed paragraphs
// are served from the per-message StreamingMarkdownRenderer cache.
func renderStreamingMarkdown(content string, msgID string, width int, _ *RenderCache) string {
	if width <= 0 {
		width = 80
	}

	// Obtain or create the per-message renderer.
	v, _ := streamRenderers.LoadOrStore(msgID, NewStreamingMarkdownRenderer())
	renderer := v.(*StreamingMarkdownRenderer)

	// Delegate to incremental renderer.
	rendered := renderer.Render(content, width)

	// Strip leading indentation to match our own indent scheme.
	var cleanLines []string
	for _, line := range strings.Split(rendered, "\n") {
		cleanLines = append(cleanLines, strings.TrimLeft(line, " \t"))
	}
	return strings.Join(cleanLines, "\n")
}

// patchIncompleteMarkdown closes unclosed markdown constructs to help rendering.
// Handles: ```/~~~ code fences, ** bold markers, ` inline code.
func patchIncompleteMarkdown(content string) string {
	openFenceMarker := ""
	for _, line := range strings.Split(content, "\n") {
		marker := fenceMarker(line)
		if marker == "" {
			continue
		}
		if openFenceMarker == "" {
			openFenceMarker = marker
			continue
		}
		if isClosingFenceLine(openFenceMarker, line) {
			openFenceMarker = ""
		}
	}
	if openFenceMarker != "" {
		content += "\n" + openFenceMarker
	}

	// If there are no open code fences, also patch inline ** and `.
	// (Inside a code fence these markers are not meaningful.)
	if openFenceMarker == "" {
		inlineContent := contentOutsideFencedBlocks(content)

		// Count ** occurrences (each "**" token counts as one marker).
		boldCount := strings.Count(inlineContent, "**")
		if boldCount%2 != 0 {
			content += "**"
		}

		// Count lone backticks (not part of ```).
		backtickCount := strings.Count(inlineContent, "`")
		if backtickCount%2 != 0 {
			content += "`"
		}
	}

	return content
}

func contentOutsideFencedBlocks(content string) string {
	var outside strings.Builder
	openFenceMarker := ""
	for _, line := range strings.Split(content, "\n") {
		marker := fenceMarker(line)
		if openFenceMarker == "" {
			if marker != "" {
				openFenceMarker = marker
				continue
			}
			if outside.Len() > 0 {
				outside.WriteByte('\n')
			}
			outside.WriteString(line)
			continue
		}
		if isClosingFenceLine(openFenceMarker, line) {
			openFenceMarker = ""
		}
	}
	return outside.String()
}

// truncateToDisplayWidth truncates a string by display width (CJK-aware).
func truncateToDisplayWidth(line string, maxWidth int) string {
	if runewidth.StringWidth(line) <= maxWidth {
		return line
	}
	w := 0
	for i, r := range line {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxWidth-1 {
			return line[:i] + "…"
		}
		w += rw
	}
	return line
}

// ── V2 message rendering (feature-gated by VisualV2) ────────────────────────

var (
	userPromptStyleV2     lipgloss.Style
	systemLineStyleV2     lipgloss.Style
	thinkingPrefixStyleV2 lipgloss.Style

	// CC visual alignment styles (V2 only)
	userPrefixStyleV2      lipgloss.Style
	assistantPrefixStyleV2 lipgloss.Style
	thinkingLabelStyleV2   lipgloss.Style
	userContentStyleV2     lipgloss.Style
	toolNameStyleV2        lipgloss.Style
)

// wrapTextByDisplayWidth wraps text by display width (CJK-aware).
func wrapTextByDisplayWidth(line string, maxWidth int) string {
	if maxWidth <= 0 || runewidth.StringWidth(line) <= maxWidth {
		return line
	}
	var sb strings.Builder
	w := 0
	for _, r := range line {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxWidth {
			sb.WriteByte('\n')
			w = 0
		}
		sb.WriteRune(r)
		w += rw
	}
	return sb.String()
}
