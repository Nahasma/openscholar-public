package agent

// SuggestionSeverity indicates how urgent an optimization suggestion is.
type SuggestionSeverity string

const (
	SeverityInfo     SuggestionSeverity = "info"
	SeverityWarning  SuggestionSeverity = "warning"
	SeverityCritical SuggestionSeverity = "critical"
)

// ContextSuggestion is a single actionable optimization recommendation.
type ContextSuggestion struct {
	Code        string
	Severity    SuggestionSeverity
	Title       string
	Detail      string
	ActionLabel string
	TokensSaved int
}

// SuggestionEngine generates optimization suggestions from a ContextAnalysis.
type SuggestionEngine interface {
	Suggest(analysis ContextAnalysis, contextWindow int, autoCompactEnabled bool) []ContextSuggestion
}

type suggestionEngine struct{}

// NewSuggestionEngine returns a new SuggestionEngine.
func NewSuggestionEngine() SuggestionEngine {
	return &suggestionEngine{}
}

func (e *suggestionEngine) Suggest(
	analysis ContextAnalysis,
	contextWindow int,
	autoCompactEnabled bool,
) []ContextSuggestion {
	var suggestions []ContextSuggestion

	inputTokens := analysis.ActualInputTokens
	if inputTokens == 0 {
		inputTokens = analysis.EstimatedTokens
	}

	// Rule 1: near_capacity — context window > 80% full
	if contextWindow > 0 && inputTokens > int(float64(contextWindow)*0.8) {
		tokensSaved := inputTokens - int(float64(contextWindow)*0.5) // rough estimate
		if tokensSaved < 0 {
			tokensSaved = 0
		}
		suggestions = append(suggestions, ContextSuggestion{
			Code:        "near_capacity",
			Severity:    SeverityWarning,
			Title:       "Context window nearly full",
			Detail:      "The context is using more than 80% of the available context window. Consider compacting the conversation to free up space.",
			ActionLabel: "/compact",
			TokensSaved: tokensSaved,
		})
	}

	// Rule 2: large_tool_results — any single tool's result tokens > 20000
	for toolName, stats := range analysis.ByTool {
		if stats.ResultTokens > 20000 {
			suggestions = append(suggestions, ContextSuggestion{
				Code:        "large_tool_results",
				Severity:    SeverityInfo,
				Title:       "Large tool results",
				Detail:      "Tool \"" + toolName + "\" produced large results. Consider using offset/limit parameters to reduce the amount of data returned.",
				ActionLabel: "Use offset/limit",
				TokensSaved: stats.ResultTokens - 5000,
			})
		}
	}

	// Rule 3: read_bloat — same tool called more than 3 times
	for toolName, stats := range analysis.ByTool {
		if stats.CallCount > 3 {
			suggestions = append(suggestions, ContextSuggestion{
				Code:        "read_bloat",
				Severity:    SeverityInfo,
				Title:       "Repeated file reads",
				Detail:      "Tool \"" + toolName + "\" was called " + itoa(stats.CallCount) + " times. Consider caching results or reading only once.",
				ActionLabel: "Cache reads",
				TokensSaved: (stats.CallCount - 1) * (stats.InputTokens / stats.CallCount),
			})
		}
	}

	// Rule 4: memory_bloat — memory category > 5000 tokens
	for _, slice := range analysis.ByCategory {
		if slice.Category == ContextMemory && slice.Tokens > 5000 {
			suggestions = append(suggestions, ContextSuggestion{
				Code:        "memory_bloat",
				Severity:    SeverityInfo,
				Title:       "Large memory context",
				Detail:      "The memory context is consuming a large portion of the context window. Consider trimming stale memory entries.",
				ActionLabel: "Trim memory",
				TokensSaved: slice.Tokens - 2000,
			})
		}
	}

	// Rule 5: autocompact_disabled — auto-compact is turned off
	if !autoCompactEnabled {
		suggestions = append(suggestions, ContextSuggestion{
			Code:        "autocompact_disabled",
			Severity:    SeverityWarning,
			Title:       "Auto-compact is disabled",
			Detail:      "Auto-compact is currently disabled. Enable it to automatically manage context size before the window fills up.",
			ActionLabel: "Enable auto-compact",
			TokensSaved: 0,
		})
	}

	// Rule 6: compact_tool_results — raw tool output has been omitted from context.
	if analysis.CompactToolResults > 0 {
		detail := "Tool output is represented as compact observations"
		if len(analysis.DataRefs) > 0 {
			detail += "; raw output is available through data refs such as " + analysis.DataRefs[0]
		}
		if len(analysis.ArtifactPaths) > 0 {
			detail += "; artifacts include " + analysis.ArtifactPaths[0]
		}
		suggestions = append(suggestions, ContextSuggestion{
			Code:        "compact_tool_results",
			Severity:    SeverityInfo,
			Title:       "Raw tool output omitted",
			Detail:      detail + ".",
			ActionLabel: "Use debug refs",
			TokensSaved: 0,
		})
	}

	return suggestions
}

// itoa converts an int to a string without importing strconv (keeps the file minimal).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
