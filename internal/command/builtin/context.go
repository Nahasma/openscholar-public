package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/llm/agent"
)

type contextCmd struct{}

func (c *contextCmd) Name() string        { return "context" }
func (c *contextCmd) Description() string { return "Show runtime context usage and breakdown" }

func (c *contextCmd) Execute(ctx command.Context) command.Result {
	if ctx.SessionID == "" {
		return command.Result{Output: "No active session."}
	}
	if ctx.App == nil || ctx.App.CoderAgent == nil {
		return command.Result{Output: "Context analysis is unavailable."}
	}

	execCtx := ctx.ExecContext
	if execCtx == nil {
		execCtx = context.Background()
	}
	report, err := ctx.App.CoderAgent.AnalyzeContext(execCtx, ctx.SessionID)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("Failed to analyze context: %v", err)}
	}
	return command.Result{Output: formatContextReport(report)}
}

func formatContextReport(report agent.ContextReport) string {
	var sb strings.Builder

	sb.WriteString("Current Usage:\n")
	sb.WriteString(fmt.Sprintf("  Context window:    %s\n", formatTokenCount(report.ContextWindow)))
	sb.WriteString(fmt.Sprintf("  Last API input:    %s (%d%% used, %d%% remaining)\n",
		formatTokenCount(report.CurrentUsageTokens),
		report.CurrentUsagePct,
		report.RemainingPct,
	))
	if report.CountedInputTokens > 0 {
		label := "estimated"
		if report.CountedPrecisely {
			label = "precise"
		}
		sb.WriteString(fmt.Sprintf("  API view now:      %s (%s)\n",
			formatTokenCount(report.CountedInputTokens),
			label,
		))
	}
	thresholdLabel := "estimated"
	if !report.ThresholdEstimated {
		thresholdLabel = "precise"
	}
	sb.WriteString(fmt.Sprintf("  Threshold count:   %s (%s)\n",
		formatTokenCount(report.ThresholdTokenCount),
		thresholdLabel,
	))
	sb.WriteString(fmt.Sprintf("  API view size:     %d messages, %d active tools\n",
		report.APIMessageCount,
		report.ActiveToolCount,
	))

	sb.WriteString("\nCumulative Totals:\n")
	sb.WriteString(fmt.Sprintf("  Prompt:            %s\n", formatTokenCount(report.CumulativePrompt)))
	sb.WriteString(fmt.Sprintf("  Completion:        %s\n", formatTokenCount(report.CumulativeCompletion)))
	if report.CumulativeCacheRead > 0 || report.CumulativeCacheWrite > 0 {
		sb.WriteString(fmt.Sprintf("  Cache read:        %s\n", formatTokenCount(report.CumulativeCacheRead)))
		sb.WriteString(fmt.Sprintf("  Cache write:       %s\n", formatTokenCount(report.CumulativeCacheWrite)))
	}
	sb.WriteString(fmt.Sprintf("  Cost:              $%.4f\n", report.CumulativeCostUSD))

	if len(report.Analysis.ByCategory) > 0 {
		sb.WriteString("\nCategory Breakdown:\n")
		for _, slice := range report.Analysis.ByCategory {
			sb.WriteString(fmt.Sprintf("  %-18s %8s  (%d item)\n",
				string(slice.Category),
				formatTokenCount(int64(slice.Tokens)),
				slice.Count,
			))
		}
	}

	if tools := agent.SortedToolBreakdown(report.Analysis.ByTool); len(tools) > 0 {
		sb.WriteString("\nTool Breakdown:\n")
		for _, tool := range tools {
			total := int64(tool.InputTokens + tool.ResultTokens)
			sb.WriteString(fmt.Sprintf("  %-18s %8s  (%d call)\n",
				tool.Name,
				formatTokenCount(total),
				tool.CallCount,
			))
		}
	}

	if len(report.Suggestions) > 0 {
		sb.WriteString("\nSuggestions:\n")
		for _, suggestion := range report.Suggestions {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", suggestion.Title, suggestion.Detail))
		}
	}

	return sb.String()
}
