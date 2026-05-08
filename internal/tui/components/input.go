package components

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// visualLineCount mirrors bubbles/textarea's wrapping rules closely enough for
// height calculations, including word wraps and exact-width cursor rows.
func visualLineCount(value string, textWidth int) int {
	if textWidth < 1 {
		textWidth = 1
	}
	count := 0
	for _, line := range strings.Split(value, "\n") {
		count += textareaWrappedLineCount([]rune(line), textWidth)
	}
	return count
}

func textareaWrappedLineCount(runes []rune, width int) int {
	if width < 1 {
		width = 1
	}

	lines := [][]rune{{}}
	word := []rune{}
	row := 0
	spaces := 0

	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			word = append(word, r)
		}

		if spaces > 0 {
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces > width {
				row++
				lines = append(lines, []rune{})
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], inputSpaces(spaces)...)
			} else {
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], inputSpaces(spaces)...)
			}
			spaces = 0
			word = nil
			continue
		}

		lastCharLen := rw.RuneWidth(word[len(word)-1])
		if uniseg.StringWidth(string(word))+lastCharLen > width {
			if len(lines[row]) > 0 {
				row++
				lines = append(lines, []rune{})
			}
			lines[row] = append(lines[row], word...)
			word = nil
		}
	}

	if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces >= width {
		lines = append(lines, []rune{})
		spaces++
		lines[row+1] = append(lines[row+1], word...)
		lines[row+1] = append(lines[row+1], inputSpaces(spaces)...)
	} else {
		lines[row] = append(lines[row], word...)
		spaces++
		lines[row] = append(lines[row], inputSpaces(spaces)...)
	}

	return len(lines)
}

func inputSpaces(n int) []rune {
	return []rune(strings.Repeat(" ", n))
}

var hintStyle lipgloss.Style

func initInputStyles() {
	hintStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
}

const (
	inputMinHeight = 1
	inputMaxHeight = 8
)

func inputPromptText() string {
	return FigPrompt + " "
}

func inputPromptWidth() int {
	return lipgloss.Width(inputPromptText())
}

func configureInputPrompt(ta *textarea.Model) {
	prompt := inputPromptText()
	continuation := strings.Repeat(" ", inputPromptWidth())
	ta.SetPromptFunc(inputPromptWidth(), func(lineIdx int) string {
		if lineIdx == 0 {
			return prompt
		}
		return continuation
	})
}

// InputModel wraps textarea.Model with border decoration, dynamic height, and history.
type InputModel struct {
	Textarea     textarea.Model
	Width        int
	history      []string
	historyIndex int
	savedInput   string
}

// NewInputModel creates a new InputModel.
func NewInputModel() InputModel {
	ta := textarea.New()
	ta.Placeholder = `What shall we build today?`
	ta.Focus()
	ta.ShowLineNumbers = false
	configureInputPrompt(&ta)
	ta.SetWidth(76)
	ta.SetHeight(inputMinHeight)
	ta.CharLimit = 0
	// Remove default border
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()

	return InputModel{
		Textarea:     ta,
		Width:        80,
		historyIndex: -1,
	}
}

// SetWidth updates the input width.
func (m *InputModel) SetWidth(w int) {
	m.Width = w
	taWidth := w - 4 // prompt gutter with separator frame padding
	if taWidth < 1 {
		taWidth = 1
	}
	m.Textarea.SetWidth(taWidth)
	m.adjustHeight()
}

// Update delegates to the inner textarea and adjusts height dynamically.
func (m *InputModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.Textarea, cmd = m.Textarea.Update(msg)

	// Dynamic height: count lines in current value
	m.adjustHeight()

	return cmd
}

// adjustHeight resizes the textarea based on content, accounting for soft wrapping.
func (m *InputModel) adjustHeight() {
	m.Textarea.SetHeight(m.inputContentHeight())
}

func (m *InputModel) inputContentHeight() int {
	value := m.Textarea.Value()
	logicalLines := strings.Count(value, "\n") + 1
	visual := visualLineCount(value, m.Textarea.Width())
	lines := visual
	if lines < logicalLines {
		lines = logicalLines
	}
	if lines < inputMinHeight {
		lines = inputMinHeight
	}
	if lines > inputMaxHeight {
		lines = inputMaxHeight
	}
	return lines
}

// Value returns the current input value.
func (m *InputModel) Value() string {
	return m.Textarea.Value()
}

// SetValue sets the input value and adjusts height.
func (m *InputModel) SetValue(s string) {
	m.Textarea.SetValue(s)
	m.adjustHeight()
}

// SetValueWithCursor sets the input value and places the cursor at byte offset.
func (m *InputModel) SetValueWithCursor(s string, offset int) {
	m.Textarea.SetValue(s)
	if offset < 0 {
		offset = 0
	}
	if offset > len(s) {
		offset = len(s)
	}
	prefix := s[:offset]
	targetRow := strings.Count(prefix, "\n")
	lastLineStart := strings.LastIndex(prefix, "\n") + 1
	targetCol := len([]rune(prefix[lastLineStart:]))
	for m.Textarea.Line() > targetRow {
		m.Textarea.CursorUp()
	}
	for m.Textarea.Line() < targetRow {
		m.Textarea.CursorDown()
	}
	m.Textarea.SetCursor(targetCol)
	m.adjustHeight()
}

// CursorOffset returns the current cursor offset in bytes within Value().
func (m InputModel) CursorOffset() int {
	value := m.Textarea.Value()
	if value == "" {
		return 0
	}
	lines := strings.Split(value, "\n")
	row := m.Textarea.Line()
	if row < 0 {
		return 0
	}
	if row >= len(lines) {
		return len(value)
	}
	lineInfo := m.Textarea.LineInfo()
	col := lineInfo.StartColumn + lineInfo.ColumnOffset
	lineRunes := []rune(lines[row])
	if col < 0 {
		col = 0
	}
	if col > len(lineRunes) {
		col = len(lineRunes)
	}
	offset := 0
	for i := 0; i < row; i++ {
		offset += len(lines[i]) + 1 // include newline
	}
	offset += len(string(lineRunes[:col]))
	if offset > len(value) {
		return len(value)
	}
	return offset
}

// Reset clears the input and resets height and history index.
func (m *InputModel) Reset() {
	m.Textarea.Reset()
	m.Textarea.SetHeight(inputMinHeight)
	m.historyIndex = -1
}

// Focus sets focus to the textarea.
func (m *InputModel) Focus() tea.Cmd {
	return m.Textarea.Focus()
}

// PushHistory adds an entry to the input history.
func (m *InputModel) PushHistory(text string) {
	if text == "" {
		return
	}
	// Don't add duplicates of the last entry
	if len(m.history) > 0 && m.history[len(m.history)-1] == text {
		return
	}
	m.history = append(m.history, text)
	m.historyIndex = -1
}

// HistoryUp navigates to the previous history entry.
func (m *InputModel) HistoryUp() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIndex == -1 {
		// Save current input before navigating
		m.savedInput = m.Textarea.Value()
		m.historyIndex = len(m.history) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	} else {
		return
	}
	m.Textarea.SetValue(m.history[m.historyIndex])
	m.adjustHeight()
}

// HistoryDown navigates to the next history entry or restores saved input.
func (m *InputModel) HistoryDown() {
	if m.historyIndex == -1 {
		return
	}
	if m.historyIndex < len(m.history)-1 {
		m.historyIndex++
		m.Textarea.SetValue(m.history[m.historyIndex])
	} else {
		// Restore saved input
		m.historyIndex = -1
		m.Textarea.SetValue(m.savedInput)
	}
	m.adjustHeight()
}

// Height returns the total rendered prompt height.
func (m *InputModel) Height() int {
	return m.inputContentHeight() + 2 // top and bottom separator lines
}

// View renders the input prompt between stable separator lines.
func (m InputModel) View() string {
	if m.Width < 20 {
		return m.Textarea.View()
	}

	bStyle := lipgloss.NewStyle().Foreground(Theme.PromptBorder)
	topLine := bStyle.Render(SafeRule(m.Width, 0, "─"))

	taView := m.Textarea.View()
	taLines := strings.Split(taView, "\n")

	// Limit to expected height to prevent textarea from rendering extra lines
	// +1 tolerance for cursor positioning on last line
	expectedHeight := m.Textarea.Height()
	if len(taLines) > expectedHeight+1 {
		taLines = taLines[:expectedHeight]
	}
	for i := range taLines {
		taLines[i] = strings.TrimRight(taLines[i], " \t")
	}

	content := strings.Join(taLines, "\n")
	bottomLine := bStyle.Render(SafeRule(m.Width, 0, "─"))

	return topLine + "\n" + strings.TrimRight(content, "\n") + "\n" + bottomLine
}

// InputFooterParams carries footer state for the prompt area.
type InputFooterParams struct {
	HasText      bool
	IsProcessing bool
	Notice       TransientNotice
	Width        int
}

// RenderInputFooter renders the footer below the input box.
// Footer ownership is intentionally narrow: transient notices and input-local
// shortcuts live here. Session metadata, mode, and runtime telemetry belong to
// the status bar.
func RenderInputFooter(p InputFooterParams) string {
	width := p.Width
	if width < 30 {
		return ""
	}

	dimStyle := lipgloss.NewStyle().Foreground(Theme.TextMuted)
	infoStyle := lipgloss.NewStyle().Foreground(Theme.Info)
	successStyle := lipgloss.NewStyle().Foreground(Theme.Success)
	errorStyle := lipgloss.NewStyle().Foreground(Theme.Danger)

	msg := strings.TrimSpace(p.Notice.Text)
	if msg != "" {
		statusStyle := dimStyle
		switch p.Notice.Kind {
		case NoticeError:
			statusStyle = errorStyle
		case NoticeSuccess:
			statusStyle = successStyle
		case NoticeWarning:
			statusStyle = lipgloss.NewStyle().Foreground(Theme.Warning)
		default:
			statusStyle = infoStyle
		}
		return renderFooterRow("  "+statusStyle.Render(truncateToWidth(msg, width-4, "…")), "", width)
	}

	_ = p.HasText
	_ = p.IsProcessing
	return ""
}

func renderFooterRow(left, right string, width int) string {
	width = TerminalSafeWidth(width)
	if width <= 0 {
		return ""
	}

	left = strings.TrimRight(left, " ")
	right = strings.TrimSpace(right)
	if right == "" {
		return truncateToWidth(left, width, "…")
	}
	if lipgloss.Width(right) >= width {
		return truncateToWidth(right, width, "…")
	}

	rightWidth := lipgloss.Width(right)
	leftMax := width - rightWidth - 1
	if leftMax < 0 {
		leftMax = 0
	}

	left = truncateToWidth(left, leftMax, "…")
	leftWidth := lipgloss.Width(left)
	gap := width - leftWidth - rightWidth
	if gap < 1 {
		left = truncateToWidth(left, max(0, width-rightWidth-1), "…")
		leftWidth = lipgloss.Width(left)
		gap = width - leftWidth - rightWidth
		if gap < 1 {
			gap = 1
		}
	}

	return left + strings.Repeat(" ", gap) + right
}
