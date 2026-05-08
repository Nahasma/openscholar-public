package tools

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/task"
)

func TestWaitForChildTaskNotificationsUsesRegistryEvents(t *testing.T) {
	reg := task.NewRegistry()
	defer reg.Shutdown()

	var ready atomic.Bool
	var running atomic.Bool
	running.Store(true)
	done := make(chan struct{})
	errCh := make(chan error, 1)
	readyCh := make(chan bool, 1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		got, err := waitForChildTaskNotificationsByRegistry(ctx, reg, "parent-1", func(string) (bool, bool) {
			return ready.Load(), running.Load()
		})
		if err != nil {
			errCh <- err
			return
		}
		readyCh <- got
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	ready.Store(true)
	running.Store(false)
	reg.Register(&task.BaseState{M: task.Meta{
		ID:        "child-1",
		SessionID: "parent-1",
		Kind:      task.KindAgent,
		Status:    task.StatusCompleted,
		StartedAt: time.Now(),
	}})

	select {
	case err := <-errCh:
		t.Fatalf("wait returned error: %v", err)
	case got := <-readyCh:
		if !got {
			t.Fatal("expected ready=true")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("wait did not wake from registry event")
	}

	select {
	case <-done:
	default:
		t.Fatal("wait goroutine did not finish")
	}
}

func TestWaitForChildTaskNotificationsReturnsFalseWhenNoChildrenRunning(t *testing.T) {
	reg := task.NewRegistry()
	defer reg.Shutdown()

	got, err := waitForChildTaskNotificationsByRegistry(context.Background(), reg, "parent-1", func(string) (bool, bool) {
		return false, false
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Fatal("expected ready=false")
	}
}
