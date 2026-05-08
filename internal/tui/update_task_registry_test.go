package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/task"
)

func TestUpdate_TaskRegistryEvent_RefreshesBackgroundTaskCount(t *testing.T) {
	reg := task.NewRegistry()
	now := time.Now().UTC()
	reg.Register(&task.BaseState{M: task.Meta{
		ID:             "run-1",
		SessionID:      "sess-1",
		Kind:           task.KindAgent,
		Status:         task.StatusRunning,
		StartedAt:      now,
		IsBackgrounded: true,
	}})
	reg.Register(&task.BaseState{M: task.Meta{
		ID:             "other-session",
		SessionID:      "sess-2",
		Kind:           task.KindAgent,
		Status:         task.StatusRunning,
		StartedAt:      now,
		IsBackgrounded: true,
	}})
	reg.Register(&task.BaseState{M: task.Meta{
		ID:             "done-1",
		SessionID:      "sess-1",
		Kind:           task.KindAgent,
		Status:         task.StatusCompleted,
		StartedAt:      now.Add(-1 * time.Minute),
		EndedAt:        ptrTime(now),
		IsBackgrounded: true,
	}})

	m := New(context.Background(), &app.App{TaskRegistry: reg}, false, nil)
	m.sessionID = "sess-1"
	updated, _ := m.Update(pubsub.Event[task.RegistryEvent]{
		Type: pubsub.UpdatedEvent,
		Payload: task.RegistryEvent{
			Action: "updated",
			TaskID: "run-1",
			Kind:   task.KindAgent,
			Status: task.StatusRunning,
		},
	})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.status.bgTaskCount != 1 {
		t.Fatalf("expected bgTaskCount=1, got %d", got.status.bgTaskCount)
	}

	// Drive another event after marking running task finished.
	reg.Mutate("run-1", func(s task.State) {
		meta := s.TaskMeta()
		meta.Status = task.StatusCompleted
		meta.EndedAt = ptrTime(now.Add(2 * time.Minute))
	})
	updated, _ = got.Update(pubsub.Event[task.RegistryEvent]{Type: pubsub.UpdatedEvent, Payload: task.RegistryEvent{TaskID: "run-1"}})
	got, _ = updated.(Model)
	if got.status.bgTaskCount != 0 {
		t.Fatalf("expected bgTaskCount=0, got %d", got.status.bgTaskCount)
	}
}

func TestUpdate_LoadSessionMsg_RefreshesBackgroundTaskCount(t *testing.T) {
	reg := task.NewRegistry()
	now := time.Now().UTC()
	reg.Register(&task.BaseState{M: task.Meta{
		ID:             "sess-1-task",
		SessionID:      "sess-1",
		Kind:           task.KindAgent,
		Status:         task.StatusRunning,
		StartedAt:      now,
		IsBackgrounded: true,
	}})

	m := New(context.Background(), &app.App{TaskRegistry: reg}, false, nil)
	m.status.bgTaskCount = 7

	updated, _ := m.Update(loadSessionMsg{
		session:      session.Session{ID: "sess-1"},
		toolMessages: map[string]message.Message{},
	})
	got := updated.(Model)
	if got.status.bgTaskCount != 1 {
		t.Fatalf("expected sess-1 bgTaskCount=1, got %d", got.status.bgTaskCount)
	}

	updated, _ = got.Update(loadSessionMsg{
		session:      session.Session{ID: "sess-2"},
		toolMessages: map[string]message.Message{},
	})
	got = updated.(Model)
	if got.status.bgTaskCount != 0 {
		t.Fatalf("expected sess-2 bgTaskCount=0, got %d", got.status.bgTaskCount)
	}
}

func TestUpdate_TaskRegistryEvent_DoesNotShowOtherTasksWithoutSession(t *testing.T) {
	reg := task.NewRegistry()
	now := time.Now().UTC()
	reg.Register(&task.BaseState{M: task.Meta{
		ID:             "other-session-task",
		SessionID:      "sess-1",
		Kind:           task.KindAgent,
		Status:         task.StatusRunning,
		StartedAt:      now,
		IsBackgrounded: true,
	}})

	m := New(context.Background(), &app.App{TaskRegistry: reg}, false, nil)
	m.sessionID = ""
	m.status.bgTaskCount = 0

	updated, _ := m.Update(pubsub.Event[task.RegistryEvent]{Type: pubsub.UpdatedEvent, Payload: task.RegistryEvent{TaskID: "other-session-task"}})
	got := updated.(Model)
	if got.status.bgTaskCount != 0 {
		t.Fatalf("expected no-session bgTaskCount=0, got %d", got.status.bgTaskCount)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

var _ tea.Model = Model{}
