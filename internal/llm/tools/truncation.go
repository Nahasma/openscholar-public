package tools

func truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	half := maxLen / 2
	return output[:half] + "\n\n... [truncated] ...\n\n" + output[len(output)-half:]
}
