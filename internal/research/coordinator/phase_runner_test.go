package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type mockPhaseExecutor struct {
	calls int
	err   error
}

func (m *mockPhaseExecutor) RunPhase(context.Context, *PipelineRef, *PhaseRef) error {
	m.calls++
	return m.err
}

func TestPhaseRunner_RunSuccess(t *testing.T) {
	executor := &mockPhaseExecutor{}
	published := make([]string, 0, 2)
	runner := NewPhaseRunner(executor, func(_ context.Context, payload string) error {
		published = append(published, payload)
		return nil
	})

	err := runner.Run(context.Background(), PhaseRunRequest{
		TaskID:    "task-1",
		Pipeline:  &PipelineRef{ID: "pipe-1"},
		Phase:     &PhaseRef{ID: "p1", Name: "Literature", Order: 1},
		NextPhase: &PhaseRef{ID: "p2", Name: "Method", Order: 2},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
	if len(published) != 2 {
		t.Fatalf("published notifications = %d, want 2", len(published))
	}
	if !strings.Contains(published[0], `type="phase-started"`) {
		t.Fatalf("first notification is not phase-started: %s", published[0])
	}
	if !strings.Contains(published[1], `type="phase-completed"`) {
		t.Fatalf("second notification is not phase-completed: %s", published[1])
	}
	if !strings.Contains(published[1], `<next-phase`) {
		t.Fatalf("phase-completed notification missing next-phase: %s", published[1])
	}
}

func TestPhaseRunner_RunFailure(t *testing.T) {
	execErr := errors.New("executor failed")
	executor := &mockPhaseExecutor{err: execErr}
	published := make([]string, 0, 2)
	runner := NewPhaseRunner(executor, func(_ context.Context, payload string) error {
		published = append(published, payload)
		return nil
	})

	err := runner.Run(context.Background(), PhaseRunRequest{
		Pipeline: &PipelineRef{ID: "pipe-1"},
		Phase:    &PhaseRef{ID: "p1", Name: "Experiment", Order: 3},
	})
	if !errors.Is(err, execErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, execErr)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
	if len(published) != 2 {
		t.Fatalf("published notifications = %d, want 2", len(published))
	}
	if !strings.Contains(published[1], `type="phase-failed"`) {
		t.Fatalf("failure notification is not phase-failed: %s", published[1])
	}
}

func TestPhaseRunner_RunStartPublishError(t *testing.T) {
	executor := &mockPhaseExecutor{}
	runner := NewPhaseRunner(executor, func(_ context.Context, _ string) error {
		return errors.New("publish failed")
	})

	err := runner.Run(context.Background(), PhaseRunRequest{
		Pipeline: &PipelineRef{ID: "pipe-1"},
		Phase:    &PhaseRef{ID: "p1", Name: "Experiment", Order: 3},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want publish error")
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestPhaseRunner_RunValidation(t *testing.T) {
	runner := NewPhaseRunner(&mockPhaseExecutor{}, nil)

	err := runner.Run(context.Background(), PhaseRunRequest{Phase: &PhaseRef{ID: "p1"}})
	if err == nil || !strings.Contains(err.Error(), "pipeline") {
		t.Fatalf("expected pipeline validation error, got %v", err)
	}

	err = runner.Run(context.Background(), PhaseRunRequest{Pipeline: &PipelineRef{ID: "pipe-1"}})
	if err == nil || !strings.Contains(err.Error(), "phase") {
		t.Fatalf("expected phase validation error, got %v", err)
	}
}

func TestPhaseRunner_NotifyCheckpoint(t *testing.T) {
	executor := &mockPhaseExecutor{}
	var got string
	runner := NewPhaseRunner(executor, func(_ context.Context, payload string) error {
		got = payload
		return nil
	})

	err := runner.NotifyCheckpoint(context.Background(), "task-chk", &PhaseRef{ID: "p2", Name: "Method", Order: 2}, "")
	if err != nil {
		t.Fatalf("NotifyCheckpoint() error = %v, want nil", err)
	}
	if !strings.Contains(got, `type="checkpoint"`) {
		t.Fatalf("checkpoint notification missing type: %s", got)
	}
	if !strings.Contains(got, `status="checkpoint"`) {
		t.Fatalf("checkpoint notification missing status: %s", got)
	}
}
