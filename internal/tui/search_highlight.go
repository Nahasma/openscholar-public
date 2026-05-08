package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

var (
	// searchMatchStyle highlights all search matches (yellow background).
	searchMatchStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("178")).
			Foreground(lipgloss.Color("0"))
	// searchCurrentStyle highlights the currently selected match (orange background).
	searchCurrentStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("208")).
				Foreground(lipgloss.Color("0"))
)

// searchHighlightInfo describes which match is "current" so it can be styled differently.
type searchHighlightInfo struct {
	Query        string // lowercased search query
	CurrentLine  int    // content line index of current match (-1 if unknown)
	CurrentCol   int    // visual column of current match start (-1 if unknown)
}

// applySearchHighlight scans rendered content lines for query matches and
// applies highlight styling. The current match gets a distinct style.
// This operates on the same ANSI-decorated content lines that
// applySelectionHighlight works on, so it must be ANSI-safe.
func applySearchHighlight(lines []string, info searchHighlightInfo) []string {
	if info.Query == "" {
		return lines
	}

	lowerQuery := strings.ToLower(info.Query)
	result := make([]string, len(lines))
	copy(result, lines)

	for i, line := range lines {
		plain := xansi.Strip(line)
		lowerPlain := strings.ToLower(plain)

		// Collect all match positions (in rune-based offsets within plain text)
		type matchPos struct {
			runeStart int
			runeEnd   int
		}
		var matches []matchPos

		offset := 0
		plainRunes := []rune(lowerPlain)
		queryRunes := []rune(lowerQuery)
		queryRuneLen := len(queryRunes)

		for offset <= len(plainRunes)-queryRuneLen {
			idx := runeIndex(plainRunes[offset:], queryRunes)
			if idx < 0 {
				break
			}
			absIdx := offset + idx
			matches = append(matches, matchPos{
				runeStart: absIdx,
				runeEnd:   absIdx + queryRuneLen,
			})
			offset = absIdx + queryRuneLen
		}

		if len(matches) == 0 {
			continue
		}

		// Convert rune positions to visual cell positions
		origRunes := []rune(plain)
		lineWidth := xansi.StringWidth(line)

		// Build highlighted line by splitting at match boundaries
		var sb strings.Builder
		prevVisual := 0

		for _, m := range matches {
			// Calculate visual start/end from rune positions
			visualStart := runeOffsetToVisual(origRunes, m.runeStart)
			visualEnd := runeOffsetToVisual(origRunes, m.runeEnd)

			if visualStart >= lineWidth {
				continue
			}
			if visualEnd > lineWidth {
				visualEnd = lineWidth
			}

			// Write segment before this match
			if prevVisual < visualStart {
				sb.WriteString(xansi.Cut(line, prevVisual, visualStart))
			}

			// Choose style: current match vs other matches
			style := searchMatchStyle
			if i == info.CurrentLine && visualStart == info.CurrentCol {
				style = searchCurrentStyle
			}

			// Extract matched segment, strip ANSI, apply highlight
			matched := xansi.Cut(line, visualStart, visualEnd)
			sb.WriteString(style.Render(xansi.Strip(matched)))

			prevVisual = visualEnd
		}

		// Write remaining segment after last match
		if prevVisual < lineWidth {
			sb.WriteString(xansi.Cut(line, prevVisual, lineWidth))
		}

		result[i] = sb.String()
	}

	return result
}

// runeIndex finds the first occurrence of needle in haystack (rune slices).
// Returns -1 if not found.
func runeIndex(haystack, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	if len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// buildSearchHighlightInfo constructs search highlight metadata from the current
// text search state. Returns a zero-query info if search is inactive.
func (m *Model) buildSearchHighlightInfo() searchHighlightInfo {
	query := m.search.textSearch.Query()
	if query == "" && !m.search.textSearch.HasMatches() {
		return searchHighlightInfo{}
	}
	// If search UI is hidden but matches are preserved (vim n/N), use the query
	// from the match data.
	if query == "" {
		return searchHighlightInfo{}
	}

	info := searchHighlightInfo{
		Query:       query,
		CurrentLine: -1,
		CurrentCol:  -1,
	}

	// We don't track exact content line positions for the current match
	// because the match coordinates (MessageIdx/Offset) don't directly map
	// to rendered content line numbers. The yellow highlighting for all
	// matches is still valuable visual feedback.
	return info
}

// runeOffsetToVisual converts a rune offset in plain text to a visual cell position.
func runeOffsetToVisual(runes []rune, runeOffset int) int {
	if runeOffset <= 0 {
		return 0
	}
	if runeOffset >= len(runes) {
		return runewidth.StringWidth(string(runes))
	}
	return runewidth.StringWidth(string(runes[:runeOffset]))
}
