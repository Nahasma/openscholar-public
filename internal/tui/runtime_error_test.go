package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/llm/agent"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/tui/components"
)

func TestRuntimeError_NotAppendedToInlineErrors(t *testing.T) {
	m := newTestModel()
	m.sessionID = "sess-1"
	next, _ := m.Update(pubsub.Event[agent.AgentEvent]{
		Type:    pubsub.CreatedEvent,
		Payload: agent.AgentEvent{SessionID: "sess-1", Error: errors.New("provider=minimax baseURL=https://api.minimax.io/anthropic")},
	})
	got := next.(Model)
	if len(got.chat.inlineErrors) != 0 {
		t.Fatalf("inlineErrors should stay empty, got %d", len(got.chat.inlineErrors))
	}
	if got.status.notice.Kind != components.NoticeError {
		t.Fatalf("notice kind=%v want error", got.status.notice.Kind)
	}
	if strings.Contains(got.status.notice.Text, "baseURL=") {
		t.Fatalf("notice should stay summary-only, got %q", got.status.notice.Text)
	}
	if !strings.Contains(got.status.runtimeErr.Detail, "baseURL=") {
		t.Fatalf("detail should keep full context, got %q", got.status.runtimeErr.Detail)
	}
}

func TestTogglePrimaryExpansion_PrefersRuntimeError(t *testing.T) {
	m := newTestModel()
	m.state = stateChat
	m.status.notice = components.TransientNotice{Kind: components.NoticeError, Text: "short"}
	m.status.runtimeErr = RuntimeErrorState{Summary: "short", Detail: "full", Expanded: false}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	got := *(next.(*Model))
	if !got.status.runtimeErr.Expanded {
		t.Fatal("expected runtime error detail expanded")
	}
}

func TestRuntimeErrorClearedOnSuccessfulDone(t *testing.T) {
	m := newTestModel()
	m.sessionID = "sess-1"
	m.status.runtimeErr = RuntimeErrorState{Summary: "short", Detail: "full", Expanded: true}

	next, _ := m.Update(pubsub.Event[agent.AgentEvent]{
		Type:    pubsub.CreatedEvent,
		Payload: agent.AgentEvent{SessionID: "sess-1", Done: true, TerminalReason: agent.ReasonCompleted},
	})
	got := next.(Model)
	if got.status.runtimeErr.Detail != "" || got.status.runtimeErr.Summary != "" || got.status.runtimeErr.Expanded {
		t.Fatalf("runtime error should be cleared on successful done: %+v", got.status.runtimeErr)
	}
}

func TestRuntimeErrorClearedOnManualCompactStart(t *testing.T) {
	m := newTestModel()
	m.sessionID = "sess-1"
	m.app = &app.App{CoderAgent: newLayoutTestAgent()}
	m.status.runtimeErr = RuntimeErrorState{Summary: "short", Detail: "full", Expanded: true}

	next, _ := m.startManualCompact("")
	got := next.(Model)
	if got.status.runtimeErr.Detail != "" || got.status.runtimeErr.Summary != "" || got.status.runtimeErr.Expanded {
		t.Fatalf("runtime error should be cleared on manual compact start: %+v", got.status.runtimeErr)
	}
}
