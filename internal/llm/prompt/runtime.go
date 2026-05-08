package prompt

import (
	"fmt"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/config"
)

type RuntimePrompt struct {
	SystemMessage string
	Blocks        []PromptBlock
}

func BuildAgentPromptRuntime(agentName config.AgentName, now time.Time) RuntimePrompt {
	blocks := buildAgentPromptBlocksAt(agentName, now)
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return RuntimePrompt{
		SystemMessage: strings.Join(parts, "\n\n"),
		Blocks:        blocks,
	}
}

func FormatRuntimeClockContext(agentName config.AgentName, now time.Time) string {
	n := normalizeRuntimeNow(now)
	local := n.Format("2006-01-02 15:04") + " " + zoneOffsetText(n) + " (" + n.Location().String() + ")"
	utc := n.UTC().Format("2006-01-02T15:04Z")

	if agentName == config.AgentSummarizer {
		return "\n\n# Runtime Context\n" +
			"- Summary generated at: " + local + "\n" +
			"- Do not rewrite relative dates in historical messages into absolute dates unless the messages themselves include timestamps.\n"
	}

	switch agentName {
	case config.AgentCoder, config.AgentGeneral, config.AgentExplore, config.AgentLeader, config.AgentPlan, config.AgentVerify, config.AgentCoordinator:
		return "\n\n# Runtime Context\n" +
			"- Request local time: " + local + "\n" +
			"- Request UTC time: " + utc + "\n" +
			"- Interpret relative dates such as \"now\", \"today\", \"yesterday\", \"tomorrow\", \"this week\", and \"recent\" against the request local time unless the user provides another timezone or date.\n" +
			"- This timestamp is only a time reference, not evidence of current facts. For facts that may have changed, use WebSearch/WebFetch; for recent academic literature, use ScholarSearch. If tools are unavailable or permission is denied, state that the current fact could not be verified.\n"
	default:
		return ""
	}
}

func normalizeRuntimeNow(now time.Time) time.Time {
	return now.Round(0).Truncate(time.Minute)
}

func zoneOffsetText(now time.Time) string {
	_, offsetSeconds := now.Zone()
	offset := offsetSeconds / 60
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, offset/60, offset%60)
}
