package context

import (
	"math"
	"unicode/utf8"

	"github.com/openscholar/openscholar/internal/llm/provider"
	"github.com/openscholar/openscholar/internal/message"
)

type TokenCountEstimate struct {
	TokenCount       int64
	EstimatedDelta   int64
	LastRealUsageIdx int
	HasRealUsage     bool
}

func UsageFromProvider(usage provider.TokenUsage) message.Usage {
	return message.Usage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheCreationInputTokens: usage.CacheCreationTokens,
		CacheReadInputTokens:     usage.CacheReadTokens,
	}
}

func CurrentUsageTokens(usage message.Usage) int64 {
	return nonNegative(usage.InputTokens) +
		nonNegative(usage.CacheReadInputTokens) +
		nonNegative(usage.CacheCreationInputTokens)
}

func FullResponseTokenCount(usage message.Usage) int64 {
	return CurrentUsageTokens(usage) + nonNegative(usage.OutputTokens)
}

func CurrentUsagePercentage(usage message.Usage, contextWindow int64) float64 {
	if contextWindow <= 0 {
		return 0
	}
	pct := float64(CurrentUsageTokens(usage)) / float64(contextWindow) * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// TokenCountWithEstimation computes current context token count with Claude Code style semantics:
// last real usage snapshot + estimated increments after that point.
func TokenCountWithEstimation(history []message.Message) TokenCountEstimate {
	result := TokenCountEstimate{LastRealUsageIdx: -1}

	start := 0
	for i := len(history) - 1; i >= 0; i-- {
		if isRealUsage(history[i].Usage) {
			result.HasRealUsage = true
			result.LastRealUsageIdx = i
			result.TokenCount = CurrentUsageTokens(history[i].Usage)
			start = i + 1
			break
		}
	}

	for i := start; i < len(history); i++ {
		delta := EstimateMessageTokens(history[i])
		result.EstimatedDelta += delta
	}
	result.TokenCount += result.EstimatedDelta
	return result
}

// EstimateMessageTokens provides a conservative heuristic when no provider usage exists.
func EstimateMessageTokens(msg message.Message) int64 {
	var tokens int64
	for _, part := range msg.Parts {
		switch p := part.(type) {
		case message.TextContent:
			tokens += estimateTextTokens(p.Text)
		case message.ReasoningContent:
			tokens += estimateTextTokens(p.Thinking)
		case message.ToolCall:
			tokens += 6 + estimateTextTokens(p.Name) + estimateTextTokens(p.Input)
		case message.ToolResult:
			tokens += 6 + estimateTextTokens(p.Content) + estimateTextTokens(p.Metadata)
		case message.BinaryContent:
			// Binary payload is pre-tokenized by providers; use a bounded fallback.
			tokens += 256
		}
	}
	if tokens == 0 {
		return 0
	}
	return tokens + 4
}

func isRealUsage(usage message.Usage) bool {
	if usage.Estimated {
		return false
	}
	return !usage.IsZero()
}

func estimateTextTokens(text string) int64 {
	if text == "" {
		return 0
	}
	runes := utf8.RuneCountInString(text)
	return int64(math.Ceil(float64(runes) / 4.0))
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
