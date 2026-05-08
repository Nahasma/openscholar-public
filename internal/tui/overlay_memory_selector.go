package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/tui/components"
)

const memorySelectorMaxVisible = 10

// MemoryFileEntry represents a memory file in the selector.
type MemoryFileEntry struct {
	RelPath   string    // e.g. "user_preferences.md"
	Type      string    // "user", "feedback", "project", "reference"
	UpdatedAt time.Time
	IsActive  bool // whether this file is injected into current prompt
}

// MemorySelectorChoice is the result data from the overlay.
type MemorySelectorChoice struct {
	Action   string // "view", "delete"
	FilePath string
}

// MemorySelectorOverlay implements Overlay for memory file browsing.
type MemorySelectorOverlay struct {
	entries []MemoryFileEntry
	idx     int
	scroll  int
}

// NewMemorySelectorOverlay creates a new memory selector overlay.
func NewMemorySelectorOverlay(entries []MemoryFileEntry) *MemorySelectorOverlay {
	return &MemorySelectorOverlay{
		entries: entries,
	}
}

func (o *MemorySelectorOverlay) ID() string        { return "memory-selector" }
func (o *MemorySelectorOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *MemorySelectorOverlay) BlocksInput() bool { return true }

func (o *MemorySelectorOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		return o, &OverlayResult{Action: "dismiss"}, nil

	case tea.KeyUp:
		if o.idx > 0 {
			o.idx--
		}
		if o.idx < o.scroll {
			o.scroll = o.idx
		}
		return o, nil, nil

	case tea.KeyDown:
		if o.idx < len(o.entries)-1 {
			o.idx++
		}
		if o.idx >= o.scroll+memorySelectorMaxVisible {
			o.scroll = o.idx - memorySelectorMaxVisible + 1
		}
		return o, nil, nil

	case tea.KeyEnter:
		if len(o.entries) > 0 && o.idx < len(o.entries) {
			entry := o.entries[o.idx]
			return o, &OverlayResult{
				Action: "view",
				Data:   MemorySelectorChoice{Action: "view", FilePath: entry.RelPath},
			}, nil
		}
		return o, nil, nil
	}

	switch keyMsg.String() {
	case "j":
		keyMsg.Type = tea.KeyDown
		return o.Update(keyMsg)

	case "k":
		keyMsg.Type = tea.KeyUp
		return o.Update(keyMsg)

	case "q":
		return o, &OverlayResult{Action: "dismiss"}, nil

	case "d":
		if len(o.entries) > 0 && o.idx < len(o.entries) {
			entry := o.entries[o.idx]
			return o, &OverlayResult{
				Action: "delete",
				Data:   MemorySelectorChoice{Action: "delete", FilePath: entry.RelPath},
			}, nil
		}
	}

	return o, nil, nil
}

func (o *MemorySelectorOverlay) View(width, height int) string {
	theme := components.Theme

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderSub).
		Padding(0, 1)

	// Calculate inner width (border takes 2 cols each side + padding 1 each side)
	innerWidth := width - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	var lines []string

	// Blank line at top
	lines = append(lines, "")

	if len(o.entries) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)
		lines = append(lines, emptyStyle.Render("  No memory files found."))
		lines = append(lines, "")
	} else {
		// Visible window
		end := o.scroll + memorySelectorMaxVisible
		if end > len(o.entries) {
			end = len(o.entries)
		}

		for i := o.scroll; i < end; i++ {
			entry := o.entries[i]
			line := o.renderEntry(entry, i == o.idx, innerWidth)
			lines = append(lines, line)
		}
	}

	// Blank line before hints
	lines = append(lines, "")

	// Key hints
	hintStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)
	lines = append(lines, hintStyle.Render("  [j/k] navigate  [Enter] view  [d] delete"))
	lines = append(lines, hintStyle.Render("  [Esc] close"))

	body := strings.Join(lines, "\n")

	// Title for the border
	titleStyle := lipgloss.NewStyle().
		Foreground(theme.TextPrimary).
		Bold(true)
	title := titleStyle.Render(" Memory Files ")

	boxWidth := innerWidth + 4 // account for padding
	if boxWidth < 30 {
		boxWidth = 30
	}

	box := borderStyle.
		Width(boxWidth).
		Render(body)

	// Inject title into the top border line
	box = injectBorderTitle(box, title)

	// Center horizontally
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(box)
}

// renderEntry renders a single memory file entry row.
func (o *MemorySelectorOverlay) renderEntry(entry MemoryFileEntry, selected bool, innerWidth int) string {
	theme := components.Theme

	// Prefix: selected indicator
	var prefix string
	if selected {
		prefix = lipgloss.NewStyle().
			Foreground(theme.Accent).
			Bold(true).
			Render(components.FigExpand + " ")
	} else {
		prefix = "  "
	}

	// File name styled
	// Type label with semantic color
	typeColor := typeColor(entry.Type)
	typeLabel := lipgloss.NewStyle().Foreground(typeColor).Render(entry.Type)

	// Active marker
	activeMarker := ""
	if entry.IsActive {
		activeMarker = lipgloss.NewStyle().
			Foreground(theme.Success).
			Render(" " + components.FigBullet)
	}

	// Timestamp
	timeStr := entry.UpdatedAt.Format("2006-01-02 15:04")
	timeLabel := lipgloss.NewStyle().Foreground(theme.TextMuted).Render(timeStr)

	// Build right-aligned section: type + time
	right := fmt.Sprintf("%s  %s", typeLabel, timeLabel)

	// Available width for filename (after prefix and right section)
	rightLen := lipgloss.Width(right)
	prefixLen := lipgloss.Width(prefix)
	nameMaxWidth := innerWidth - prefixLen - rightLen - 2
	if nameMaxWidth < 8 {
		nameMaxWidth = 8
	}

	// Truncate name if needed (rune-aware to handle non-ASCII)
	rawName := entry.RelPath
	nameRunes := []rune(rawName)
	if len(nameRunes) > nameMaxWidth {
		rawName = string(nameRunes[:nameMaxWidth-1]) + "…"
	}
	rawName += activeMarker
	nameStyled := lipgloss.NewStyle().Foreground(theme.TextPrimary)
	if selected {
		nameStyled = nameStyled.Bold(true)
	}
	nameRendered := nameStyled.Render(rawName)

	// Pad between name and right section
	nameLen := lipgloss.Width(nameRendered)
	pad := innerWidth - prefixLen - nameLen - rightLen
	if pad < 1 {
		pad = 1
	}
	padding := strings.Repeat(" ", pad)

	return prefix + nameRendered + padding + right
}

// typeColor returns the semantic color for a given memory file type.
func typeColor(t string) lipgloss.CompleteColor {
	switch t {
	case "user":
		return components.Theme.Info
	case "feedback":
		return components.Theme.Warning
	case "project":
		return components.Theme.Success
	case "reference":
		return components.Theme.TextMuted
	default:
		return components.Theme.TextMuted
	}
}

// injectBorderTitle replaces the top border line's leading "─" characters to
// inject a title string, matching the lipgloss rounded border pattern.
func injectBorderTitle(box, title string) string {
	lines := strings.SplitN(box, "\n", 2)
	if len(lines) < 2 {
		return box
	}
	topLine := lines[0]

	// Find first "─" after the opening corner and inject the title there.
	// Rounded border top: "╭─────────╮"
	// We want:            "╭─ Title ─╮"
	runes := []rune(topLine)
	if len(runes) < 3 {
		return box
	}

	titleRunes := []rune(title)
	titleLen := len(titleRunes)

	// Find insertion point: after first corner character (index 0)
	insertAt := 1
	availableLen := len(runes) - 2 // exclude both corners
	if titleLen > availableLen {
		return box // no room, skip
	}

	newTop := make([]rune, 0, len(runes))
	newTop = append(newTop, runes[0]) // opening corner
	newTop = append(newTop, titleRunes...)
	remaining := availableLen - titleLen
	for i := 0; i < remaining; i++ {
		newTop = append(newTop, runes[insertAt]) // replicate border char
	}
	newTop = append(newTop, runes[len(runes)-1]) // closing corner

	return string(newTop) + "\n" + lines[1]
}
