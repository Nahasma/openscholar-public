package doctor

import (
	"fmt"
	"strings"
)

// FormatReport formats check results into a human-readable diagnostic report.
func FormatReport(checks []Check) string {
	var sb strings.Builder

	sb.WriteString("OpenScholar Environment Check\n")
	sb.WriteString("==============================\n")

	issues := 0
	for _, c := range checks {
		icon := "[OK]"
		switch c.Status {
		case StatusWarn:
			icon = "[--]"
		case StatusFail:
			icon = "[!!]"
			issues++
		}

		// Right-pad name with dots for alignment
		name := c.Name
		padding := 24 - len(name)
		if padding < 2 {
			padding = 2
		}
		dots := strings.Repeat(".", padding)

		fmt.Fprintf(&sb, "%s %s %s %s\n", icon, name, dots, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(&sb, "     -> %s\n", c.Fix)
		}
	}

	sb.WriteString("\n")
	if issues == 0 {
		sb.WriteString("All checks passed.\n")
	} else {
		fmt.Fprintf(&sb, "%d issue(s) found. Run the suggested commands to resolve.\n", issues)
	}

	return sb.String()
}

// Warnings extracts user-facing warning messages from check results.
// Used for TUI startup notifications.
func Warnings(checks []Check) []string {
	var warnings []string
	for _, c := range checks {
		if c.Status == StatusFail && c.Fix != "" {
			warnings = append(warnings, fmt.Sprintf("%s: %s -> %s", c.Name, c.Detail, c.Fix))
		}
	}
	return warnings
}
