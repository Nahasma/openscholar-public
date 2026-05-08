package agent

import (
	"context"
	"strings"
)

// triggerMemoryCapture runs generic memory extraction asynchronously for
// user-authored turn text. It is best-effort and intentionally detached from
// the request context so cancellation does not suppress persistence work.
func (a *agent) triggerMemoryCapture(sessionID, content string) {
	if a.memoryService == nil {
		return
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(content) == "" {
		return
	}
	go func() {
		_ = a.memoryService.ExecuteSkills(context.Background(), sessionID, content)
	}()
}
