package agent

import (
	"context"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/task"
)

func (a *agent) drainTaskNotifications(ctx context.Context, sessionID string) ([]message.Message, error) {
	if a.taskRegistry == nil || !agentReceivesSubagentNotifications(a.agentName) || !subagentNotificationsEnabled() {
		return nil, nil
	}

	var ready []*task.SubtaskState
	for _, st := range a.taskRegistry.All() {
		sub, ok := st.(*task.SubtaskState)
		if !ok {
			continue
		}
		meta := sub.TaskMeta()
		if sub.ParentSessionID != sessionID || meta.Notified || meta.NotifyClaimed {
			continue
		}
		if meta.Status != task.StatusCompleted && meta.Status != task.StatusFailed && meta.Status != task.StatusCanceled {
			continue
		}
		ready = append(ready, sub)
	}

	if len(ready) == 0 {
		return nil, nil
	}

	sort.Slice(ready, func(i, j int) bool {
		mi := ready[i].TaskMeta()
		mj := ready[j].TaskMeta()
		if mi.StartedAt.Equal(mj.StartedAt) {
			return mi.ID < mj.ID
		}
		return mi.StartedAt.Before(mj.StartedAt)
	})

	maxChars := 6000
	if cfg := config.Get(); cfg != nil {
		maxChars = cfg.SubagentOrchestration.NotificationMaxChars
	}
	if maxChars <= 0 {
		maxChars = 6000
	}

	out := make([]message.Message, 0, len(ready))
	for _, sub := range ready {
		claimed, ok := a.taskRegistry.TryClaimNotification(sub.TaskMeta().ID)
		if !ok {
			continue
		}
		claimedSub, ok := claimed.(*task.SubtaskState)
		if !ok {
			a.taskRegistry.ReleaseNotificationClaim(sub.TaskMeta().ID)
			continue
		}

		body := buildTaskNotificationXML(claimedSub, maxChars)
		notifyMsg, err := a.messages.Create(ctx, sessionID, message.CreateMessageParams{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: body}},
			Meta: map[string]any{
				"subagent_notification": true,
				"task_id":               claimedSub.TaskMeta().ID,
			},
		})
		if err != nil {
			a.taskRegistry.ReleaseNotificationClaim(claimedSub.TaskMeta().ID)
			return out, err
		}
		a.taskRegistry.MarkNotificationDelivered(claimedSub.TaskMeta().ID)
		out = append(out, notifyMsg)
	}
	return out, nil
}

func subagentNotificationsEnabled() bool {
	cfg := config.Get()
	return cfg != nil && cfg.SubagentOrchestration.Enabled
}

func agentReceivesSubagentNotifications(agentName config.AgentName) bool {
	switch agentName {
	case config.AgentCoder, config.AgentLeader, config.AgentCoordinator:
		return true
	default:
		return false
	}
}

func buildTaskNotificationXML(sub *task.SubtaskState, maxChars int) string {
	meta := sub.TaskMeta()
	result, resultTruncated := truncateForNotification(sub.Result, maxChars)
	verifyResult, verifyResultTruncated := truncateForNotification(sub.VerifyResult, maxChars)
	verifyErr, verifyErrTruncated := truncateForNotification(sub.VerifyError, maxChars)

	var b strings.Builder
	b.WriteString("<task-notification>\n")
	writeTag(&b, "task-id", meta.ID)
	writeTag(&b, "agent-type", sub.AgentType)
	writeTag(&b, "status", string(meta.Status))
	writeTag(&b, "summary", sub.Summary)
	writeTag(&b, "model", sub.Model)
	writeTag(&b, "parent-session-id", sub.ParentSessionID)
	writeTag(&b, "child-session-id", sub.ChildSessionID)
	writeTag(&b, "started-at", meta.StartedAt.UTC().Format(time.RFC3339))
	if meta.EndedAt != nil {
		writeTag(&b, "ended-at", meta.EndedAt.UTC().Format(time.RFC3339))
		writeTag(&b, "duration-ms", fmt.Sprintf("%d", meta.EndedAt.Sub(meta.StartedAt).Milliseconds()))
	}
	writeTag(&b, "result", result)
	if sub.ResultTruncated || resultTruncated {
		writeTag(&b, "result-truncated", "true")
	}
	writeTag(&b, "verify-status", string(sub.VerifyStatus))
	writeTag(&b, "verify-verdict", sub.VerifyVerdict)
	if verifyResult != "" {
		writeTag(&b, "verify-result", verifyResult)
	}
	if verifyErr != "" {
		writeTag(&b, "verify-error", verifyErr)
	}
	if verifyResultTruncated || verifyErrTruncated {
		writeTag(&b, "verify-truncated", "true")
	}
	b.WriteString("</task-notification>")
	return b.String()
}

func writeTag(b *strings.Builder, name, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	b.WriteString("  <")
	b.WriteString(name)
	b.WriteString(">")
	b.WriteString(html.EscapeString(value))
	b.WriteString("</")
	b.WriteString(name)
	b.WriteString(">\n")
}

func truncateForNotification(s string, maxChars int) (string, bool) {
	if maxChars <= 0 {
		return strings.TrimSpace(s), false
	}
	trimmed := strings.TrimSpace(s)
	rs := []rune(trimmed)
	if len(rs) <= maxChars {
		return trimmed, false
	}
	return string(rs[:maxChars]), true
}
