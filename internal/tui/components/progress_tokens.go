package components

import "fmt"

// SmoothTokenStep calculates the next displayed token count,
// smoothly catching up to the target value.
// displayed: current displayed value.
// target: actual token count to catch up to.
// Returns the new displayed value (always <= target).
func SmoothTokenStep(displayed, target int) int {
	gap := target - displayed

	if gap < 0 {
		return target
	}
	if gap == 0 {
		return displayed
	}

	var increment int
	switch {
	case gap < 20:
		increment = gap / 7
		if increment < 1 {
			increment = 1
		}
	case gap < 70:
		increment = 3
	case gap < 200:
		increment = gap * 15 / 100
		if increment < 8 {
			increment = 8
		}
	default:
		increment = 50
	}

	result := displayed + increment
	if result > target {
		return target
	}
	return result
}

// FormatTokenLabel formats a token count for display in the progress rail.
// CC source: SpinnerAnimationRow.tsx uses formatNumber() which produces compact
// notation (e.g. "1.3k") then renders "↓ {count} tokens".
// count: token count to format.
// width: available terminal width.
func FormatTokenLabel(count int, width int) string {
	if width < 48 {
		return ""
	}

	numStr := formatCompact(count)
	return fmt.Sprintf("↓ %s tokens", numStr)
}

// formatCompact formats a number using compact notation matching CC's formatNumber().
// CC source: ref/claude-code/src/utils/format.ts
// Examples: 900 → "900", 1321 → "1.3k", 15000 → "15k"
func formatCompact(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	k := float64(n) / 1000.0
	s := fmt.Sprintf("%.1fk", k)
	// Remove trailing .0 like CC does: "1.0k" → "1k"
	if len(s) > 3 && s[len(s)-3:len(s)-1] == ".0" {
		return s[:len(s)-3] + "k"
	}
	return s
}
