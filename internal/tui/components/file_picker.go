package components

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/openscholar/openscholar/internal/picker"
)

// RenderFilePicker renders a file selection menu in the same style as the command picker.
func RenderFilePicker(items []picker.FileEntry, selectedIdx, width int) string {
	if len(items) == 0 {
		return ""
	}
	safeWidth := TerminalSafeWidth(width)
	if safeWidth <= 0 {
		return ""
	}

	// Ensure styles are initialized
	initCommandPickerStyles()

	maxShow := 8
	if len(items) < maxShow {
		maxShow = len(items)
	}

	var sb strings.Builder
	for i := 0; i < maxShow; i++ {
		entry := items[i]
		icon := "  "
		name := entry.RelPath
		if entry.IsDir {
			icon = "📁"
			name += "/"
		} else {
			icon = fileIcon(filepath.Ext(entry.RelPath))
		}

		line := fmt.Sprintf("  %s %s", icon, name)

		// Truncate to width
		line = truncateDisplay(line, safeWidth-2)

		if i == selectedIdx {
			padded := line + strings.Repeat(" ", max(0, safeWidth-displayWidth(line)))
			sb.WriteString(pickerSelectedStyle.Render(padded) + "\n")
		} else {
			sb.WriteString(pickerNormalStyle.Render(line) + "\n")
		}
	}

	sb.WriteString("  " + pickerHintStyle.Render("↑↓ navigate  ⏎ select  esc cancel") + "\n")
	return sb.String()
}

// fileIcon returns a simple icon based on file extension.
func fileIcon(ext string) string {
	switch strings.ToLower(ext) {
	case ".pdf":
		return "📄"
	case ".tex":
		return "📝"
	case ".bib":
		return "📚"
	case ".py":
		return "🐍"
	case ".go":
		return "🔧"
	case ".md":
		return "📖"
	case ".docx", ".doc":
		return "📃"
	case ".png", ".jpg", ".jpeg", ".svg":
		return "🖼️"
	default:
		return "  "
	}
}
