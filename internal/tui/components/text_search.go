package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/message"
)

// SearchMatch represents a text match in the chat history.
type SearchMatch struct {
	MessageIdx int    // index in the message list
	PartIndex  int    // index of the content part
	Offset     int    // character offset within the part text
	Length     int    // match length
	Preview    string // surrounding text preview
}

// TextSearchModel provides Ctrl+F text search within chat messages.
type TextSearchModel struct {
	input   textinput.Model
	matches []SearchMatch
	current int // index of currently highlighted match
	visible bool
	width   int
}

// NewTextSearch creates a new text search component.
func NewTextSearch() TextSearchModel {
	ti := textinput.New()
	ti.Placeholder = "search chat..."
	ti.CharLimit = 256

	return TextSearchModel{
		input: ti,
	}
}

// Show makes the search bar visible.
func (m *TextSearchModel) Show() tea.Cmd {
	m.visible = true
	m.current = 0
	m.input.SetValue("")
	m.matches = nil
	return m.input.Focus()
}

// Hide closes the search bar and clears matches.
func (m *TextSearchModel) Hide() {
	m.visible = false
	m.input.Blur()
	m.matches = nil
	m.current = 0
}

// HideKeepMatches closes the search bar but preserves matches and current index.
// This allows vim n/N to continue navigating after the search UI is dismissed.
func (m *TextSearchModel) HideKeepMatches() {
	m.visible = false
	m.input.Blur()
}

// HasMatches reports whether there are search matches to navigate,
// regardless of whether the search UI is visible.
func (m TextSearchModel) HasMatches() bool {
	return len(m.matches) > 0
}

// NextMatch advances to the next match. Returns the match and true if available.
func (m *TextSearchModel) NextMatch() (SearchMatch, bool) {
	if len(m.matches) == 0 {
		return SearchMatch{}, false
	}
	m.current = (m.current + 1) % len(m.matches)
	return m.matches[m.current], true
}

// PrevMatch moves to the previous match. Returns the match and true if available.
func (m *TextSearchModel) PrevMatch() (SearchMatch, bool) {
	if len(m.matches) == 0 {
		return SearchMatch{}, false
	}
	m.current--
	if m.current < 0 {
		m.current = len(m.matches) - 1
	}
	return m.matches[m.current], true
}

// Visible returns whether the search bar is shown.
func (m TextSearchModel) Visible() bool {
	return m.visible
}

// SetWidth sets the rendering width.
func (m *TextSearchModel) SetWidth(w int) {
	if w < 0 {
		w = 0
	}
	m.width = w
	inputWidth := w - 20 // leave room for match count
	if inputWidth < 1 {
		inputWidth = 1
	}
	m.input.Width = inputWidth
}

// Query returns the current search query.
func (m TextSearchModel) Query() string {
	return m.input.Value()
}

// CurrentMatch returns the currently highlighted match.
func (m TextSearchModel) CurrentMatch() (SearchMatch, bool) {
	if m.current >= 0 && m.current < len(m.matches) {
		return m.matches[m.current], true
	}
	return SearchMatch{}, false
}

// MatchCount returns total matches.
func (m TextSearchModel) MatchCount() int {
	return len(m.matches)
}

// CurrentIndex returns the 1-based index of the current match.
func (m TextSearchModel) CurrentIndex() int {
	if len(m.matches) == 0 {
		return 0
	}
	return m.current + 1
}

// Update handles key events.
func (m TextSearchModel) Update(msg tea.Msg) (TextSearchModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Next match
			if len(m.matches) > 0 {
				m.current = (m.current + 1) % len(m.matches)
			}
			return m, nil
		case tea.KeyEsc:
			m.Hide()
			return m, nil
		}
		// Shift+Enter: previous match (Alt as approximation)
		if msg.Alt && msg.Type == tea.KeyEnter {
			if len(m.matches) > 0 {
				m.current--
				if m.current < 0 {
					m.current = len(m.matches) - 1
				}
			}
			return m, nil
		}
	}

	// Update text input
	prevQuery := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	// Re-search if query changed
	if m.input.Value() != prevQuery {
		m.current = 0
	}

	return m, cmd
}

// Search executes a search across messages and updates matches.
func (m *TextSearchModel) Search(messages []message.Message) {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	m.matches = nil
	m.current = 0

	if query == "" {
		return
	}

	m.matches = FindMatches(messages, query)
}

// FindMatches searches through messages for query matches.
func FindMatches(messages []message.Message, query string) []SearchMatch {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	var matches []SearchMatch

	for msgIdx, msg := range messages {
		for partIdx, part := range msg.Parts {
			var text string
			switch p := part.(type) {
			case message.TextContent:
				text = p.Text
			case message.ReasoningContent:
				text = p.Thinking
			case message.ToolResult:
				text = p.Content
			default:
				continue
			}

			lower := strings.ToLower(text)
			offset := 0
			for {
				idx := strings.Index(lower[offset:], query)
				if idx < 0 {
					break
				}
				absOffset := offset + idx

				// Extract preview (surrounding context)
				previewStart := absOffset - 30
				if previewStart < 0 {
					previewStart = 0
				}
				previewEnd := absOffset + len(query) + 30
				if previewEnd > len(text) {
					previewEnd = len(text)
				}

				matches = append(matches, SearchMatch{
					MessageIdx: msgIdx,
					PartIndex:  partIdx,
					Offset:     absOffset,
					Length:     len(query),
					Preview:    text[previewStart:previewEnd],
				})

				offset = absOffset + len(query)
			}
		}
	}

	return matches
}

var (
	searchBarStyle   = lipgloss.NewStyle().Foreground(ColorBlue)
	searchCountStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// View renders the search bar.
func (m TextSearchModel) View() string {
	if !m.visible {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(searchBarStyle.Render("Find: "))
	sb.WriteString(m.input.View())

	if m.input.Value() != "" {
		count := itoa(len(m.matches))
		if len(m.matches) > 0 {
			sb.WriteString(searchCountStyle.Render("  " + itoa(m.current+1) + "/" + count))
		} else {
			sb.WriteString(searchCountStyle.Render("  0/" + count))
		}
	}

	return sb.String()
}
