package components

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openscholar/openscholar/internal/config"
)

var (
	brandIconStyle       lipgloss.Style
	welcomeTitleStyle    lipgloss.Style
	welcomeTextStyle     lipgloss.Style
	welcomeHintStyle     lipgloss.Style
	welcomeCmdStyle      lipgloss.Style
	welcomeMetaStyle     lipgloss.Style
	welcomeTipLabelStyle lipgloss.Style
	welcomeKeyStyle      lipgloss.Style
	welcomeDescStyle     lipgloss.Style
)

func initWelcomeStyles() {
	brandIconStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBrandPurple)
	welcomeTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorWhite)
	welcomeTextStyle = lipgloss.NewStyle().
		Foreground(ColorGrayBright)
	welcomeHintStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
	welcomeCmdStyle = lipgloss.NewStyle().
		Italic(true).
		Foreground(ColorBrandPurple)
	welcomeMetaStyle = lipgloss.NewStyle().
		Foreground(ColorGrayDim)
	welcomeTipLabelStyle = lipgloss.NewStyle().
		Foreground(ColorBrandPurple)
	welcomeKeyStyle = lipgloss.NewStyle().
		Foreground(ColorGrayBright)
	welcomeDescStyle = lipgloss.NewStyle().
		Foreground(ColorGrayMedium)
}

// RenderWelcome renders a Claude Code–style welcome banner with two-column layout.
func RenderWelcome(modelName string, width int) string {
	if width < 20 {
		width = 20
	}

	// Compact single-column layout for narrow terminals
	if width < 50 {
		return renderCompactWelcome(modelName, width)
	}

	bStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)

	// Pixel brain mascot
	logoLines := []string{
		brandIconStyle.Render("    ▄▄▄▄▄▄▄"),
		brandIconStyle.Render("  ▄█▀▀▀▀▀▀▀█▄"),
		brandIconStyle.Render("  ███▄██▄████"),
		brandIconStyle.Render("  ████████████"),
		brandIconStyle.Render("  ▀███▄▄▄███▀"),
		brandIconStyle.Render("    ▀▀    ▀▀"),
	}

	// Width formula:
	// Border row:  "╭" + dashes(width-2) + "╮"  = width chars
	// Content row: "│" + " " + left(leftColWidth) + " │ " + right(rightColWidth) + " " + "│"
	//            = 1 + 1 + leftColWidth + 3 + rightColWidth + 1 + 1
	//            = leftColWidth + rightColWidth + 7
	// Therefore:  leftColWidth + rightColWidth = width - 7

	contentWidth := width - 7 // total chars available for left+right columns
	if contentWidth < 30 {
		return renderCompactWelcome(modelName, width)
	}
	leftColWidth := contentWidth * 2 / 5
	rightColWidth := contentWidth - leftColWidth

	// Clamp minimums while keeping sum invariant
	if leftColWidth < 18 {
		leftColWidth = 18
		rightColWidth = contentWidth - leftColWidth
	}
	if rightColWidth < 20 {
		rightColWidth = 20
		leftColWidth = contentWidth - rightColWidth
	}

	// Build left column lines
	cwd := config.WorkingDirectory()
	modelDisplay := truncateToWidth(modelName, leftColWidth-2, "…")
	cwdDisplay := truncateToWidth(shortenPath(cwd), leftColWidth-2, "…")

	var leftLines []string
	leftLines = append(leftLines, "")
	leftLines = append(leftLines, welcomeTitleStyle.Render("  Welcome to OpenScholar!"))
	leftLines = append(leftLines, "")
	for _, ll := range logoLines {
		leftLines = append(leftLines, ll)
	}
	leftLines = append(leftLines, "")
	leftLines = append(leftLines, welcomeMetaStyle.Render("  Model · ")+welcomeHintStyle.Render(modelDisplay))
	leftLines = append(leftLines, welcomeMetaStyle.Render("  "+cwdDisplay))
	leftLines = append(leftLines, "")

	// Build right column lines
	var rightLines []string
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, welcomeTipLabelStyle.Render("Tips for getting started"))
	rightLines = append(rightLines, welcomeTextStyle.Render("1. Ask a question or give instruction"))
	rightLines = append(rightLines, welcomeTextStyle.Render("2. Use ")+welcomeCmdStyle.Render("/help")+welcomeTextStyle.Render(" for available commands"))
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, welcomeHintStyle.Render(strings.Repeat("─", rightColWidth)))
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, welcomeTipLabelStyle.Render("Shortcuts"))
	rightLines = append(rightLines, formatShortcut("shift+tab", "cycle mode", rightColWidth))
	rightLines = append(rightLines, formatShortcut("esc", "cancel request", rightColWidth))
	rightLines = append(rightLines, formatShortcut("pgup/pgdn", "scroll", rightColWidth))
	rightLines = append(rightLines, "")

	// Equalize line counts
	for len(leftLines) < len(rightLines) {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < len(leftLines) {
		rightLines = append(rightLines, "")
	}

	var sb strings.Builder

	// Border: "╭" + dashes(width-2) + "╮"
	borderDashes := width - 2
	titleText := " OpenScholar "
	titleStart := 3
	if borderDashes < len(titleText)+titleStart {
		titleStart = 0
	}
	remainDashes := borderDashes - titleStart - len(titleText)
	if remainDashes < 0 {
		remainDashes = 0
	}
	topBorder := "╭" + strings.Repeat("─", titleStart) + titleText + strings.Repeat("─", remainDashes) + "╮"
	sb.WriteString(bStyle.Render(topBorder))
	sb.WriteString("\n")

	// Content rows: "│" + " " + left + " │ " + right + " " + "│"
	sep := bStyle.Render(" │ ")
	for i := 0; i < len(leftLines); i++ {
		left := padToWidth(leftLines[i], leftColWidth)
		right := padToWidth(rightLines[i], rightColWidth)
		sb.WriteString(bStyle.Render("│") + " " + left + sep + right + " " + bStyle.Render("│"))
		sb.WriteString("\n")
	}

	// Bottom border: "╰" + dashes(width-2) + "╯"
	bottomBorder := "╰" + strings.Repeat("─", borderDashes) + "╯"
	sb.WriteString(bStyle.Render(bottomBorder))

	return sb.String()
}

// renderCompactWelcome renders a compact single-column welcome for narrow terminals.
func renderCompactWelcome(modelName string, width int) string {
	bStyle := lipgloss.NewStyle().Foreground(ColorGrayMedium)

	// Border row:  "╭" + dashes(width-2) + "╮"  = width chars
	// Content row: "│" + content + "│"
	//   content must be exactly (width-2) chars wide
	borderDashes := width - 2
	if borderDashes < 1 {
		borderDashes = 1
	}
	innerWidth := borderDashes // chars available inside "│" and "│"

	var sb strings.Builder

	topBorder := "╭" + strings.Repeat("─", borderDashes) + "╮"
	sb.WriteString(bStyle.Render(topBorder) + "\n")

	modelDisplay := truncateToWidth(modelName, innerWidth-2, "…")
	cwdDisplay := truncateToWidth(shortenPath(config.WorkingDirectory()), innerWidth-2, "…")

	lines := []string{
		"",
		welcomeTitleStyle.Render("  Welcome!"),
		"",
		brandIconStyle.Render("    ▄▄▄▄▄▄▄"),
		"",
		welcomeMetaStyle.Render("  " + modelDisplay),
		welcomeMetaStyle.Render("  " + cwdDisplay),
		"",
		welcomeHintStyle.Render("  /help for commands"),
		"",
	}

	for _, line := range lines {
		w := lipgloss.Width(line)
		pad := innerWidth - w
		if pad < 0 {
			pad = 0
		}
		sb.WriteString(bStyle.Render("│") + line + strings.Repeat(" ", pad) + bStyle.Render("│") + "\n")
	}

	bottomBorder := "╰" + strings.Repeat("─", borderDashes) + "╯"
	sb.WriteString(bStyle.Render(bottomBorder))
	return sb.String()
}

// formatShortcut formats a shortcut key-description pair.
func formatShortcut(key, desc string, _ int) string {
	return welcomeKeyStyle.Render(key) + "  " + welcomeDescStyle.Render(desc)
}

// padToWidth pads a styled string to a given visual width using ANSI-aware measurement.
func padToWidth(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// shortenPath shortens a path by replacing home directory with ~.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// HeaderBarParams holds the data needed to render the condensed header bar.
type HeaderBarParams struct {
	Version   string // e.g. "2.0.1" or "dev"
	ModelName string // e.g. "Claude Sonnet 4"
	Provider  string // e.g. "Anthropic"
	Width     int    // terminal width
}

// Mini pixel-brain mascot (3-row condensed version of the 6-row original).
var miniMascotLines = []string{
	" ▄█▀██▀█▄ ",
	" ▀███▄███▀",
	"   ▀▀  ▀▀ ",
}

const miniMascotWidth = 11 // visual width of widest mascot line

// RenderHeaderBar renders a condensed Claude Code–style header bar:
//
//	▄█▀██▀█▄   OpenScholar v2.0.1
//	▀███▄███▀  Claude Sonnet 4 · Anthropic
//	  ▀▀  ▀▀   ~/Documents/Dev/openscholar
func RenderHeaderBar(p HeaderBarParams) string {
	if p.Width < 20 {
		p.Width = 20
	}

	cwd := shortenPath(config.WorkingDirectory())
	gap := "  " // 2-space gap between mascot and text

	// Build the 3 text lines
	versionSuffix := ""
	if p.Version != "" {
		versionSuffix = " v" + p.Version
	}
	line1 := welcomeTitleStyle.Render("OpenScholar") + welcomeMetaStyle.Render(versionSuffix)

	line2 := welcomeHintStyle.Render(p.ModelName)
	if p.Provider != "" {
		line2 += welcomeMetaStyle.Render(" · " + p.Provider)
	}

	line3 := welcomeMetaStyle.Render(cwd)

	// Narrow terminal: skip mascot, just render text lines with indent
	if p.Width < 40 {
		textWidth := p.Width - 2
		cwdDisplay := truncateToWidth(cwd, textWidth, "…")

		// Rebuild text lines with truncation
		modelDisplay := truncateToWidth(p.ModelName, textWidth, "…")
		line2 = welcomeHintStyle.Render(modelDisplay)
		line3 = welcomeMetaStyle.Render(cwdDisplay)
		return "  " + line1 + "\n" + "  " + line2 + "\n" + "  " + line3
	}

	// Truncate text lines to available width
	textWidth := p.Width - miniMascotWidth - len(gap)
	if textWidth < 15 {
		textWidth = 15
	}

	// Rebuild with truncation
	modelDisplay := truncateToWidth(p.ModelName, textWidth-2, "…")
	providerSuffix := ""
	if p.Provider != "" {
		providerSuffix = " · " + p.Provider
	}
	modelFull := modelDisplay + providerSuffix
	if lipgloss.Width(modelFull) > textWidth {
		modelDisplay = truncateToWidth(p.ModelName, textWidth-lipgloss.Width(providerSuffix), "…")
		modelFull = modelDisplay + providerSuffix
	}
	line2 = welcomeHintStyle.Render(modelDisplay)
	if p.Provider != "" {
		line2 += welcomeMetaStyle.Render(" · " + p.Provider)
	}

	cwdDisplay := truncateToWidth(cwd, textWidth, "…")
	line3 = welcomeMetaStyle.Render(cwdDisplay)

	// Assemble rows: indent + mascot (padded) + gap + text
	// 2-space indent aligns with message content ("  ⏺ ...")
	indent := "  "
	var sb strings.Builder
	textLines := []string{line1, line2, line3}
	for i, mascotLine := range miniMascotLines {
		mascot := brandIconStyle.Render(mascotLine)
		mascot = padToWidth(mascot, miniMascotWidth)
		sb.WriteString(indent + mascot + gap + textLines[i] + "\n")
	}

	return sb.String()
}
