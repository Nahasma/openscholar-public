package components

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// CCSpinnerChars: forward+reverse cycle (Claude Code style).
// Source: ref/claude-code/src/components/Spinner/utils.ts getDefaultCharacters()
// CC uses ['·','✢','✳','✶','✻','✽'] + reverse = 12 frames.
var CCSpinnerChars = []string{"·", "✢", "✳", "✶", "✻", "✽", "✽", "✻", "✶", "✳", "✢", "·"}

// SpinnerGlyph returns the CC-style spinner character for the given frame.
//
//   - frame: current animation frame (incremented each 100ms tick).
//   - stalledPct: 0.0-1.0 stalled intensity for color interpolation.
//   - reducedMotion: if true, returns a static ● with dim/bright cycle.
func SpinnerGlyph(frame int, stalledPct float64, reducedMotion bool) string {
	if reducedMotion {
		// Static dot with dim/bright alternation every 10 frames.
		color := StalledColor(stalledPct)
		style := lipgloss.NewStyle().Foreground(color)
		if frame%20 < 10 {
			// dim phase — use muted color
			style = lipgloss.NewStyle().Foreground(Theme.TextMuted)
		}
		return style.Render("●")
	}

	// CC source: SpinnerGlyph.tsx uses SPINNER_FRAMES[frame % length]
	glyph := CCSpinnerChars[frame%len(CCSpinnerChars)]
	color := StalledColor(stalledPct)
	return lipgloss.NewStyle().Foreground(color).Render(glyph)
}

// ShimmerIndex calculates the glimmer position for shimmer animation.
//
//   - frame: current animation frame (incremented each 100ms tick).
//   - textLen: number of runes in the text.
//   - reverse: true for tool-use/streaming (right-to-left sweep).
//
// Returns -1 if textLen <= 0.
func ShimmerIndex(frame int, textLen int, reverse bool) int {
	if textLen <= 0 {
		return -1
	}
	cycle := textLen + 10
	pos := frame % cycle
	if reverse {
		return textLen + 5 - pos
	}
	return pos
}

// RenderShimmerText renders text with a 3-cell sweeping highlight.
//
//   - text: the string to render.
//   - glimmerIndex: position of the highlight center (-1 = disabled).
//   - baseColor: normal text color.
//   - shimmerColor: highlight color for the center char and adjacent chars.
//
// The center character (glimmerIndex) is rendered with shimmerColor + Bold.
// Adjacent characters (±1) are rendered with shimmerColor (no Bold).
// All other characters use baseColor.
// When glimmerIndex < 0 or >= len([]rune(text)), all characters use baseColor.
func RenderShimmerText(text string, glimmerIndex int, baseColor, shimmerColor lipgloss.TerminalColor) string {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return ""
	}

	// Determine whether shimmer is active.
	shimmerActive := glimmerIndex >= 0 && glimmerIndex < n

	baseStyle := lipgloss.NewStyle().Foreground(baseColor)
	adjStyle := lipgloss.NewStyle().Foreground(shimmerColor)
	centerStyle := lipgloss.NewStyle().Foreground(shimmerColor).Bold(true)

	var sb strings.Builder
	for i, r := range runes {
		ch := string(r)
		if !shimmerActive {
			sb.WriteString(baseStyle.Render(ch))
			continue
		}
		switch i {
		case glimmerIndex:
			sb.WriteString(centerStyle.Render(ch))
		case glimmerIndex - 1, glimmerIndex + 1:
			sb.WriteString(adjStyle.Render(ch))
		default:
			sb.WriteString(baseStyle.Render(ch))
		}
	}
	return sb.String()
}

// StalledColor interpolates between accent and danger color based on stalled intensity.
//
//   - intensity: 0.0 = accent color, 1.0 = danger color.
//
// For TrueColor terminals: performs hex RGB linear interpolation.
// For ANSI256/16: returns accent if intensity < 0.5, danger otherwise.
func StalledColor(intensity float64) lipgloss.TerminalColor {
	// Clamp intensity to [0, 1].
	if intensity < 0 {
		intensity = 0
	} else if intensity > 1 {
		intensity = 1
	}

	profile := lipgloss.ColorProfile()

	// TrueColor: perform RGB lerp.
	if profile == termenv.TrueColor {
		accentHex := string(Theme.Accent.TrueColor)
		dangerHex := string(Theme.Danger.TrueColor)

		ar, ag, ab, aOk := parseHexColor(accentHex)
		dr, dg, db, dOk := parseHexColor(dangerHex)
		if aOk && dOk {
			r := uint8(float64(ar) + intensity*(float64(dr)-float64(ar)))
			g := uint8(float64(ag) + intensity*(float64(dg)-float64(ag)))
			b := uint8(float64(ab) + intensity*(float64(db)-float64(ab)))
			return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", r, g, b))
		}
	}

	// ANSI256 / ANSI16: threshold switch.
	if intensity < 0.5 {
		return Theme.Accent
	}
	return Theme.Danger
}

// PhaseColor returns the semantic color for each processing phase.
//
//   - 0 (Idle):        TextMuted
//   - 1 (Thinking):    Accent
//   - 2 (ToolRunning): Info
//   - 3 (Streaming):   Accent
//   - 4 (Compacting):  Warning
func PhaseColor(phase uint8) lipgloss.TerminalColor {
	switch phase {
	case PhaseIdle:
		return Theme.TextMuted
	case PhaseThinking:
		return Theme.Accent
	case PhaseToolRunning:
		return Theme.Info
	case PhaseStreaming:
		return Theme.Accent
	case PhaseCompacting:
		return Theme.Warning
	default:
		return Theme.TextMuted
	}
}

// parseHexColor parses a CSS hex color string ("#RRGGBB") into RGB components.
// Returns false if the string is not a valid 7-character hex color.
func parseHexColor(hex string) (r, g, b uint8, ok bool) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, 0, 0, false
	}
	rv, err1 := strconv.ParseUint(hex[1:3], 16, 8)
	gv, err2 := strconv.ParseUint(hex[3:5], 16, 8)
	bv, err3 := strconv.ParseUint(hex[5:7], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return uint8(rv), uint8(gv), uint8(bv), true
}
