package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HistorySearchModel provides a Ctrl+R style history search dialog.
type HistorySearchModel struct {
	input     textinput.Model
	entries   []string // deduplicated, most recent first
	matches   []string
	cursor    int
	visible   bool
	maxHeight int
	width     int
}

// NewHistorySearch creates a new history search component.
func NewHistorySearch() HistorySearchModel {
	ti := textinput.New()
	ti.Placeholder = "search history..."
	ti.CharLimit = 256

	return HistorySearchModel{
		input:     ti,
		maxHeight: 10,
	}
}

// SetEntries sets the history entries (most recent first, pre-deduplicated).
func (m *HistorySearchModel) SetEntries(entries []string) {
	// Deduplicate: keep only first occurrence (most recent)
	seen := make(map[string]bool)
	var deduped []string
	for _, e := range entries {
		trimmed := strings.TrimSpace(e)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		deduped = append(deduped, trimmed)
	}
	m.entries = deduped
	m.filterMatches()
}

// Show makes the search dialog visible and focuses the input.
func (m *HistorySearchModel) Show() tea.Cmd {
	m.visible = true
	m.cursor = 0
	m.input.SetValue("")
	m.filterMatches()
	return m.input.Focus()
}

// Hide closes the search dialog.
func (m *HistorySearchModel) Hide() {
	m.visible = false
	m.input.Blur()
}

// Visible returns whether the dialog is shown.
func (m HistorySearchModel) Visible() bool {
	return m.visible
}

// SetWidth sets the rendering width.
func (m *HistorySearchModel) SetWidth(w int) {
	if w < 0 {
		w = 0
	}
	m.width = w
	inputWidth := w - 4
	if inputWidth < 1 {
		inputWidth = 1
	}
	m.input.Width = inputWidth
}

// Selected returns the currently selected entry, if any.
func (m HistorySearchModel) Selected() (string, bool) {
	if m.cursor >= 0 && m.cursor < len(m.matches) {
		return m.matches[m.cursor], true
	}
	return "", false
}

// Update handles key events for the search dialog.
func (m HistorySearchModel) Update(msg tea.Msg) (HistorySearchModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case tea.KeyDown:
			if m.cursor < len(m.matches)-1 {
				m.cursor++
			}
			return m, nil
		case tea.KeyEnter:
			// Selection handled by caller via Selected()
			return m, nil
		case tea.KeyEsc:
			m.Hide()
			return m, nil
		}
	}

	// Update text input
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.filterMatches()
	// Reset cursor when query changes
	m.cursor = 0

	return m, cmd
}

// filterMatches filters entries by the current query.
func (m *HistorySearchModel) filterMatches() {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if query == "" {
		m.matches = m.entries
		return
	}

	var matches []string
	for _, e := range m.entries {
		if strings.Contains(strings.ToLower(e), query) {
			matches = append(matches, e)
		}
	}
	m.matches = matches
}

var (
	historyTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(ColorBlue)
	historySelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorGreen)
	historyItemStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// View renders the history search dialog.
func (m HistorySearchModel) View() string {
	if !m.visible {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(historyTitleStyle.Render("  History Search (Ctrl+R)"))
	sb.WriteByte('\n')
	sb.WriteString("  ")
	sb.WriteString(m.input.View())
	sb.WriteByte('\n')

	if len(m.matches) == 0 {
		sb.WriteString(historyItemStyle.Render("  no matches"))
		sb.WriteByte('\n')
	} else {
		visible := m.matches
		if len(visible) > m.maxHeight {
			visible = visible[:m.maxHeight]
		}
		for i, entry := range visible {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
				sb.WriteString(historySelectedStyle.Render(prefix + truncate(entry, m.width-4)))
			} else {
				sb.WriteString(historyItemStyle.Render(prefix + truncate(entry, m.width-4)))
			}
			sb.WriteByte('\n')
		}
		if len(m.matches) > m.maxHeight {
			remaining := len(m.matches) - m.maxHeight
			sb.WriteString(historyItemStyle.Render("  ... and " + itoa(remaining) + " more"))
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// truncate shortens a string to fit within maxLen display columns.
func truncate(s string, maxLen int) string {
	return truncateDisplay(s, maxLen)
}

// itoa converts int to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	if neg {
		result = append([]byte{'-'}, result...)
	}
	return string(result)
}
