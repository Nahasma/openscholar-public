package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/command"
)

type costCmd struct {
	app *app.App
}

func (c *costCmd) Name() string        { return "cost" }
func (c *costCmd) Description() string { return "Show token usage and cost for current session" }

func (c *costCmd) Execute(ctx command.Context) command.Result {
	if ctx.SessionID == "" {
		return command.Result{Output: "No active session."}
	}

	model := c.app.CoderAgent.Model()
	state := c.app.CoderAgent.CostState()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Model: %s\n", model.Name))

	if len(state.ByModel) > 0 {
		sb.WriteString("\nPer-Model Usage:\n")
		for _, mu := range state.ByModel {
			sb.WriteString(fmt.Sprintf("  %s\n", mu.ModelID))
			sb.WriteString(fmt.Sprintf("    Input:       %s\n", formatTokenCount(mu.InputTokens)))
			sb.WriteString(fmt.Sprintf("    Output:      %s\n", formatTokenCount(mu.OutputTokens)))
			if mu.CacheReadTokens > 0 || mu.CacheWriteTokens > 0 {
				sb.WriteString(fmt.Sprintf("    Cache read:  %s\n", formatTokenCount(mu.CacheReadTokens)))
				sb.WriteString(fmt.Sprintf("    Cache write: %s\n", formatTokenCount(mu.CacheWriteTokens)))
			}
			sb.WriteString(fmt.Sprintf("    Cost:        %s\n", formatCost(mu.Cost, mu.CostKnown)))
		}
	}

	sb.WriteString("\nSession Total:\n")
	if len(state.ByModel) > 0 {
		var totalInput, totalOutput, totalCacheRead, totalCacheWrite int64
		for _, mu := range state.ByModel {
			totalInput += mu.InputTokens
			totalOutput += mu.OutputTokens
			totalCacheRead += mu.CacheReadTokens
			totalCacheWrite += mu.CacheWriteTokens
		}
		promptEffective := totalInput + totalCacheRead + totalCacheWrite
		sb.WriteString(fmt.Sprintf("  Prompt (effective): %s\n", formatTokenCount(promptEffective)))
		sb.WriteString(fmt.Sprintf("  Completion:         %s\n", formatTokenCount(totalOutput)))
		sb.WriteString(fmt.Sprintf("  Total cost:         %s\n", formatCost(state.Total, state.CostKnown)))
	} else {
		sess, err := ctx.App.Sessions.Get(context.Background(), ctx.SessionID)
		if err != nil {
			return command.Result{Output: fmt.Sprintf("Failed to get session: %v", err)}
		}
		sb.WriteString(fmt.Sprintf("  Prompt (effective): %s\n", formatTokenCount(sess.PromptTokens)))
		sb.WriteString(fmt.Sprintf("  Completion:         %s\n", formatTokenCount(sess.CompletionTokens)))
		sb.WriteString(fmt.Sprintf("  Total cost:         %s\n", formatCost(sess.Cost, model.CostKnown)))
	}

	return command.Result{Output: sb.String()}
}

func formatCost(cost float64, known bool) string {
	if !known {
		if cost > 0 {
			return fmt.Sprintf("unknown (known subtotal $%.4f)", cost)
		}
		return "unknown"
	}
	return fmt.Sprintf("$%.4f", cost)
}

// formatTokenCount formats a token count with comma separators.
func formatTokenCount(n int64) string {
	if n < 0 {
		return "-" + formatTokenCount(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}
