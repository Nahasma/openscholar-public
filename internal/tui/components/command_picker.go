package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// CommandItem represents a command for the picker display.
type CommandItem struct {
	Name         string
	Description  string
	ArgumentHint string
	WhenToUse    string
	Source       string
}

var (
	pickerNormalStyle   lipgloss.Style
	pickerSelectedStyle lipgloss.Style
	pickerHintStyle     lipgloss.Style
)

func initCommandPickerStyles() {
	pickerNormalStyle = lipgloss.NewStyle().
		Foreground(ColorGrayBright)
	pickerSelectedStyle = lipgloss.NewStyle().
		Foreground(ColorWhite).
		Background(ColorGrayDark)
	pickerHintStyle = lipgloss.NewStyle().
		Foreground(ColorGrayDim)
}

// RenderCommandPicker renders a list of matching commands below the input box.
func RenderCommandPicker(commands []CommandItem, selectedIdx, width int) string {
	if len(commands) == 0 {
		return ""
	}
	safeWidth := TerminalSafeWidth(width)
	if safeWidth <= 0 {
		return ""
	}

	maxShow := 8
	if len(commands) < maxShow {
		maxShow = len(commands)
	}

	// Sliding window around selectedIdx
	start := 0
	if selectedIdx >= maxShow {
		start = selectedIdx - maxShow + 1
	}
	end := start + maxShow
	if end > len(commands) {
		end = len(commands)
		start = end - maxShow
		if start < 0 {
			start = 0
		}
	}
	visible := commands[start:end]

	// Calculate column widths
	maxNameLen := 0
	for _, cmd := range visible {
		nameLen := displayWidth(cmd.Name) + 1 // +1 for /
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
	}

	var sb strings.Builder
	for i, cmd := range visible {
		actualIdx := start + i
		name := "/" + cmd.Name + strings.Repeat(" ", max(0, maxNameLen-displayWidth(cmd.Name)))
		desc := strings.TrimSpace(cmd.Description)
		if cmd.ArgumentHint != "" {
			desc = strings.TrimSpace(desc + " " + cmd.ArgumentHint)
		}
		meta := ""
		if cmd.WhenToUse != "" {
			meta = cmd.WhenToUse
		}
		if cmd.Source != "" {
			if meta != "" {
				meta += " · "
			}
			meta += cmd.Source
		}
		line := fmt.Sprintf("  %s  %s", name, desc)
		if meta != "" {
			line += "  [" + meta + "]"
		}

		// Truncate to width
		line = truncateDisplay(line, safeWidth-2)

		if actualIdx == selectedIdx {
			// Pad to full width for selected highlight
			padded := line + strings.Repeat(" ", max(0, safeWidth-displayWidth(line)))
			sb.WriteString(pickerSelectedStyle.Render(padded) + "\n")
		} else {
			sb.WriteString(pickerNormalStyle.Render(line) + "\n")
		}
	}

	sb.WriteString("  " + pickerHintStyle.Render("↑↓ navigate  ⏎ select  esc cancel") + "\n")

	return sb.String()
}
