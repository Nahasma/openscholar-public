package agent

import (
	"context"
	"time"

	"github.com/openscholar/openscholar/internal/hooks"
)

func (a *agent) runPolicyHookAsync(ctx context.Context, event hooks.Event, input hooks.Input) {
	if a == nil || a.hookService == nil {
		return
	}
	if input.Timestamp.IsZero() {
		input.Timestamp = time.Now()
	}
	a.hookService.RunAsync(ctx, event, input)
}

func (a *agent) emitBudgetExhaustedHook(ctx context.Context, sessionID string, reason TerminalReason) {
	a.runPolicyHookAsync(ctx, hooks.BudgetExhausted, hooks.Input{
		SessionID:    sessionID,
		ErrorKind:    string(reason),
		ProgressKind: "none",
		Timestamp:    time.Now(),
	})
}
