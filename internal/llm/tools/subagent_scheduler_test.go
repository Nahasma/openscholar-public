package tools

import (
	"context"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/config"
)

func TestSubagentSchedulerWriteSetConflictSerializes(t *testing.T) {
	s := newSubagentScheduler(&config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true}})
	ctx := context.Background()
	prof := SubagentProfile{ID: "general", AgentType: "general", CanWriteFiles: true}

	lease1, err := s.Acquire(ctx, "parent-1", prof, []string{"internal/a"})
	if err != nil {
		t.Fatalf("acquire1 failed: %v", err)
	}
	defer lease1.Release()

	acquired := make(chan struct{})
	go func() {
		lease2, err := s.Acquire(ctx, "parent-1", prof, []string{"internal/a/file.go"})
		if err != nil {
			return
		}
		lease2.Release()
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("conflicting write set should block")
	case <-time.After(80 * time.Millisecond):
	}

	lease1.Release()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("expected acquire after release")
	}
}

func TestSubagentSchedulerContextCancelWhileWaiting(t *testing.T) {
	s := newSubagentScheduler(&config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true}})
	prof := SubagentProfile{ID: "general", AgentType: "general", CanWriteFiles: true}
	lease1, err := s.Acquire(context.Background(), "parent-1", prof, []string{"docs"})
	if err != nil {
		t.Fatalf("acquire1 failed: %v", err)
	}
	defer lease1.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = s.Acquire(ctx, "parent-1", prof, []string{"docs/guide.md"})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestSubagentSchedulerProfileConcurrencyLimit(t *testing.T) {
	s := newSubagentScheduler(&config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true}})
	prof := SubagentProfile{ID: "verify", AgentType: "verify"}

	l1, err := s.Acquire(context.Background(), "parent-2", prof, nil)
	if err != nil {
		t.Fatalf("acquire1 failed: %v", err)
	}
	defer l1.Release()
	l2, err := s.Acquire(context.Background(), "parent-2", prof, nil)
	if err != nil {
		t.Fatalf("acquire2 failed: %v", err)
	}
	defer l2.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = s.Acquire(ctx, "parent-2", prof, nil)
	if err == nil {
		t.Fatal("expected verify profile concurrency limit")
	}
}

func TestSubagentSchedulerUsesConfiguredGlobalLimit(t *testing.T) {
	s := newSubagentScheduler(&config.Config{SubagentOrchestration: config.SubagentOrchestrationConfig{Enabled: true, GlobalMaxConcurrent: 1}})

	l1, err := s.Acquire(context.Background(), "parent-3", SubagentProfile{ID: "general", AgentType: "general"}, nil)
	if err != nil {
		t.Fatalf("acquire1 failed: %v", err)
	}
	defer l1.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = s.Acquire(ctx, "parent-3", SubagentProfile{ID: "explore", AgentType: "explore"}, nil)
	if err == nil {
		t.Fatal("expected configured global concurrency limit")
	}
}
