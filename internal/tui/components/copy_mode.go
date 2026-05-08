package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// CopyModeBlock represents a code block for copy mode selection.
type CopyModeBlock struct {
	Language string
	Content  string
}

// RenderCopyModeOverlay renders a dialog showing numbered code blocks for selection.
func RenderCopyModeOverlay(blocks []CopyModeBlock, width int) string {
	if width < 30 {
		width = 30
	}

	innerWidth := width - 4
	if innerWidth < 30 {
		innerWidth = 30
	}

	var sb strings.Builder

	// Top border with title
	title := " Copy Code Block "
	topLen := innerWidth + 2
	titleStart := 2
	top := "╭" + strings.Repeat("─", titleStart) + title + strings.Repeat("─", topLen-titleStart-len(title)) + "╮"
	sb.WriteString(helpBorderStyle.Render(top) + "\n")

	// Empty line
	sb.WriteString(helpBorderStyle.Render("│") + strings.Repeat(" ", innerWidth+2) + helpBorderStyle.Render("│") + "\n")

	for i, block := range blocks {
		if i >= 9 {
			break // max 9 blocks (keys 1-9)
		}

		// Block header: number + language
		lang := block.Language
		if lang == "" {
			lang = "text"
		}
		header := fmt.Sprintf(" %d) %s", i+1, lang)
		headerStyled := helpKeyStyle.Render(header)
		headerWidth := lipgloss.Width(headerStyled)
		headerPad := innerWidth + 2 - headerWidth
		if headerPad < 0 {
			headerPad = 0
		}
		sb.WriteString(helpBorderStyle.Render("│") + headerStyled + strings.Repeat(" ", headerPad) + helpBorderStyle.Render("│") + "\n")

		// Preview: first 2 lines of content
		previewLines := strings.Split(block.Content, "\n")
		maxPreview := 2
		if len(previewLines) < maxPreview {
			maxPreview = len(previewLines)
		}
		for j := 0; j < maxPreview; j++ {
			line := previewLines[j]
			line = truncateDisplay(line, innerWidth-2)
			preview := helpDescStyle.Render("   " + line)
			previewWidth := lipgloss.Width(preview)
			previewPad := innerWidth + 2 - previewWidth
			if previewPad < 0 {
				previewPad = 0
			}
			sb.WriteString(helpBorderStyle.Render("│") + preview + strings.Repeat(" ", previewPad) + helpBorderStyle.Render("│") + "\n")
		}
		if len(previewLines) > maxPreview {
			more := helpDescStyle.Render(fmt.Sprintf("   ... +%d lines", len(previewLines)-maxPreview))
			moreWidth := lipgloss.Width(more)
			morePad := innerWidth + 2 - moreWidth
			if morePad < 0 {
				morePad = 0
			}
			sb.WriteString(helpBorderStyle.Render("│") + more + strings.Repeat(" ", morePad) + helpBorderStyle.Render("│") + "\n")
		}
	}

	// Empty line
	sb.WriteString(helpBorderStyle.Render("│") + strings.Repeat(" ", innerWidth+2) + helpBorderStyle.Render("│") + "\n")

	// Hint line
	hint := helpDescStyle.Render("  1-9: select  a: all  esc: cancel")
	hintPad := innerWidth + 2 - lipgloss.Width(hint)
	if hintPad < 0 {
		hintPad = 0
	}
	sb.WriteString(helpBorderStyle.Render("│") + hint + strings.Repeat(" ", hintPad) + helpBorderStyle.Render("│") + "\n")

	// Bottom border
	bottom := "╰" + strings.Repeat("─", innerWidth+2) + "╯"
	sb.WriteString(helpBorderStyle.Render(bottom))

	return sb.String()
}
