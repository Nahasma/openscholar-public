package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Phase status constants for PipelineProgressData.
const (
	PhaseProgressPending   uint8 = 0
	PhaseProgressRunning   uint8 = 1
	PhaseProgressCompleted uint8 = 2
	PhaseProgressFailed    uint8 = 3
)

// PipelineProgressData holds the state needed to render the research pipeline progress bar.
type PipelineProgressData struct {
	Phases       []PhaseProgressItem
	CurrentPhase int    // 0-based index of current phase
	CurrentLabel string // e.g. "Experiment"
	WorkerStatus string // e.g. "running code generation"
}

// PhaseProgressItem represents a single phase in the research pipeline.
type PhaseProgressItem struct {
	Name   string
	Status uint8 // 0=pending, 1=running, 2=completed, 3=failed
}

// pipelineProgressStyles holds lazily-initialized styles for the pipeline progress bar.
var (
	ppCompletedStyle lipgloss.Style
	ppRunningStyle   lipgloss.Style
	ppPendingStyle   lipgloss.Style
	ppFailedStyle    lipgloss.Style
	ppConnectorStyle lipgloss.Style
	ppInfoStyle      lipgloss.Style
	ppStylesReady    bool
)

func ensurePipelineProgressStyles() {
	if ppStylesReady {
		return
	}
	ppStylesReady = true

	ppCompletedStyle = lipgloss.NewStyle().Foreground(Theme.Success)
	ppRunningStyle = lipgloss.NewStyle().Foreground(Theme.Accent)
	ppPendingStyle = lipgloss.NewStyle().Foreground(Theme.TextMuted)
	ppFailedStyle = lipgloss.NewStyle().Foreground(Theme.Danger)
	ppConnectorStyle = lipgloss.NewStyle().Foreground(Theme.BorderSub)
	ppInfoStyle = lipgloss.NewStyle().Foreground(Theme.TextMuted)
}

// phaseSymbol returns the Unicode symbol for the given phase status.
func phaseSymbol(status uint8) string {
	switch status {
	case PhaseProgressCompleted:
		return FigDiamondFill
	case PhaseProgressRunning:
		return FigDiamondOpen
	case PhaseProgressFailed:
		return FigCross
	default:
		return FigCircleOpen
	}
}

// phaseStyle returns the lipgloss style for the given phase status.
func phaseStyle(status uint8) lipgloss.Style {
	switch status {
	case PhaseProgressCompleted:
		return ppCompletedStyle
	case PhaseProgressRunning:
		return ppRunningStyle
	case PhaseProgressFailed:
		return ppFailedStyle
	default:
		return ppPendingStyle
	}
}

// abbreviate returns the first 3 runes of s.
func abbreviate(s string) string {
	runes := []rune(s)
	if len(runes) <= 3 {
		return s
	}
	return string(runes[:3])
}

// renderPhaseChain builds the phase chain string (unstyled width-measurable).
func renderPhaseChain(phases []PhaseProgressItem, connector string, abbrev bool) string {
	var parts []string
	for _, ph := range phases {
		sym := phaseStyle(ph.Status).Render(phaseSymbol(ph.Status))
		name := ph.Name
		if abbrev {
			name = abbreviate(name)
		}
		label := phaseStyle(ph.Status).Render(name)
		parts = append(parts, sym+" "+label)
	}
	return strings.Join(parts, connector)
}

// truncateToWidthRW truncates s to fit within maxWidth using runewidth.
func truncateToWidthRW(s string, maxWidth int) string {
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	return runewidth.Truncate(s, maxWidth-1, "…")
}

// RenderPipelineProgress renders the research pipeline progress bar.
//
// Wide terminal (≥80):
//
//	◆ Literature ─ ◆ Design ─ ◇ Experiment ─ ○ Writing ─ ○ Review
//	Phase 3/5: Experiment ∙ Worker: running code generation
//
// Medium terminal (60-79): single line with abbreviated phase names.
// Narrow terminal (<60): single line with just the current phase info.
func RenderPipelineProgress(data PipelineProgressData, width int) string {
	if len(data.Phases) == 0 {
		return ""
	}

	ensurePipelineProgressStyles()
	const indent = "  "
	connector := ppConnectorStyle.Render(" ─ ")
	total := len(data.Phases)
	current := data.CurrentPhase + 1 // 1-based for display

	availWidth := width - 2 // subtract indent

	// Try wide format first (≥80), then medium (≥60), then narrow
	if width >= 80 {
		line1 := renderPhaseChain(data.Phases, connector, false)
		if lipgloss.Width(line1) <= availWidth {
			phaseInfo := fmt.Sprintf("Phase %d/%d: %s", current, total, data.CurrentLabel)
			var line2 string
			if data.WorkerStatus != "" {
				line2 = phaseInfo + " " + FigBullet + " Worker: " + data.WorkerStatus
				line2 = ppInfoStyle.Render(truncateToWidthRW(line2, availWidth))
			} else {
				line2 = ppInfoStyle.Render(phaseInfo)
			}
			return indent + line1 + "\n" + indent + line2 + "\n"
		}
		// Fall through to medium if wide overflows
	}

	if width >= 60 {
		line1 := renderPhaseChain(data.Phases, connector, true)
		if lipgloss.Width(line1) <= availWidth {
			return indent + line1 + "\n"
		}
		// Fall through to narrow if medium overflows
	}

	// Narrow fallback: just current phase info, width-protected
	phaseInfo := fmt.Sprintf("Phase %d/%d: %s", current, total, data.CurrentLabel)
	phaseInfo = truncateToWidthRW(phaseInfo, availWidth)
	return indent + ppInfoStyle.Render(phaseInfo) + "\n"
}
