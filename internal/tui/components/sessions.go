package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/session"
)

var (
	sessionSelectedStyle lipgloss.Style
	sessionNormalStyle   lipgloss.Style
	sessionTitleStyle    lipgloss.Style
	sessionMetaStyle     lipgloss.Style
	sessionHintStyle     lipgloss.Style
)

func initSessionStyles() {
	sessionSelectedStyle = lipgloss.NewStyle().
		Reverse(true)
	sessionNormalStyle = lipgloss.NewStyle().
		Foreground(ColorGrayBright)
	sessionTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBrandPurple)
	sessionMetaStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
	sessionHintStyle = lipgloss.NewStyle().
		Foreground(ColorGrayDim)
}

// RenderSessionBrowser renders a session list browser overlay.
func RenderSessionBrowser(sessions []session.Session, selectedIdx, scrollOffset, width, height int) string {
	if width < 40 {
		width = 40
	}

	innerWidth := width - 6
	if innerWidth < 30 {
		innerWidth = 30
	}

	var sb strings.Builder

	bStyle := lipgloss.NewStyle().Foreground(ColorGrayDim)

	// Top border with title
	title := " Sessions "
	topLen := innerWidth + 2
	titleStart := 2
	top := "╭" + strings.Repeat("─", titleStart) + title + strings.Repeat("─", topLen-titleStart-len(title)) + "╮"
	sb.WriteString(bStyle.Render(top) + "\n")

	// Calculate visible rows (leave room for borders + hint)
	visibleRows := height - 4
	if visibleRows < 3 {
		visibleRows = 3
	}

	if len(sessions) == 0 {
		emptyMsg := "  No sessions found"
		pad := innerWidth + 2 - lipgloss.Width(emptyMsg)
		if pad < 0 {
			pad = 0
		}
		sb.WriteString(bStyle.Render("│") + emptyMsg + strings.Repeat(" ", pad) + bStyle.Render("│") + "\n")
	} else {
		// Adjust scroll offset to keep selected item visible
		if selectedIdx < scrollOffset {
			scrollOffset = selectedIdx
		}
		if selectedIdx >= scrollOffset+visibleRows {
			scrollOffset = selectedIdx - visibleRows + 1
		}

		end := scrollOffset + visibleRows
		if end > len(sessions) {
			end = len(sessions)
		}

		for i := scrollOffset; i < end; i++ {
			sess := sessions[i]
			title := sess.Title
			if title == "" {
				title = "(untitled)"
			}
			title = truncateDisplay(title, innerWidth-25)

			timeStr := relativeTime(sess.UpdatedAt)
			meta := fmt.Sprintf(" %d msgs · %s", sess.MessageCount, timeStr)

			line := "  " + title + sessionMetaStyle.Render(meta)
			lineWidth := lipgloss.Width(line)
			rightPad := innerWidth + 2 - lineWidth
			if rightPad < 0 {
				rightPad = 0
			}
			paddedLine := line + strings.Repeat(" ", rightPad)

			if i == selectedIdx {
				sb.WriteString(bStyle.Render("│") + sessionSelectedStyle.Render(paddedLine) + bStyle.Render("│") + "\n")
			} else {
				sb.WriteString(bStyle.Render("│") + sessionNormalStyle.Render(paddedLine) + bStyle.Render("│") + "\n")
			}
		}
	}

	// Hint line
	sb.WriteString(bStyle.Render("│") + strings.Repeat(" ", innerWidth+2) + bStyle.Render("│") + "\n")
	hint := sessionHintStyle.Render("  enter resume · n new · d delete · esc cancel")
	hintPad := innerWidth + 2 - lipgloss.Width(hint)
	if hintPad < 0 {
		hintPad = 0
	}
	sb.WriteString(bStyle.Render("│") + hint + strings.Repeat(" ", hintPad) + bStyle.Render("│") + "\n")

	// Bottom border
	bottom := "╰" + strings.Repeat("─", innerWidth+2) + "╯"
	sb.WriteString(bStyle.Render(bottom))

	return sb.String()
}

// relativeTime formats a unix timestamp as a relative time string.
func relativeTime(ts int64) string {
	if ts == 0 {
		return "unknown"
	}
	dur := time.Since(time.Unix(ts, 0))
	switch {
	case dur < time.Minute:
		return "just now"
	case dur < time.Hour:
		m := int(dur.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case dur < 24*time.Hour:
		h := int(dur.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		d := int(dur.Hours() / 24)
		if d == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", d)
	}
}
