package task

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// helper: create a *BaseState with sensible defaults.
func newState(id string, kind Kind, status Status) *BaseState {
	return &BaseState{M: Meta{
		ID:     id,
		Label:  "test-" + id,
		Kind:   kind,
		Status: status,
	}}
}

// helper: set EndedAt to t and return the state (for GC tests).
func withEndedAt(s *BaseState, t time.Time) *BaseState {
	s.M.EndedAt = &t
	return s
}

// ---- basic CRUD --------------------------------------------------------

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	s := newState("t1", KindAgent, StatusRunning)
	r.Register(s)

	got, ok := r.Get("t1")
	if !ok {
		t.Fatal("expected task to be present")
	}
	if got.TaskMeta().ID != "t1" {
		t.Fatalf("unexpected id: %s", got.TaskMeta().ID)
	}
}

func TestGetMissing(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected miss for unknown id")
	}
}

func TestUpdate(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	r.Register(newState("t2", KindHook, StatusPending))
	r.Update("t2", func(m *Meta) {
		m.Status = StatusRunning
	})

	s, _ := r.Get("t2")
	if s.TaskMeta().Status != StatusRunning {
		t.Fatalf("expected Running, got %s", s.TaskMeta().Status)
	}
}

func TestUpdateMissingIsNoop(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	// Must not panic.
	r.Update("ghost", func(m *Meta) { m.Status = StatusFailed })
}

func TestMutateStateFields(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	s := NewSubtaskState(Meta{
		ID:     "mut-1",
		Label:  "task",
		Kind:   KindAgent,
		Status: StatusRunning,
	}, "parent-1", "child-1", "general", "summary", "", "none")
	r.Register(s)

	r.Mutate("mut-1", func(state State) {
		sub, ok := state.(*SubtaskState)
		if !ok {
			t.Fatalf("expected SubtaskState, got %T", state)
		}
		sub.Result = "done"
		sub.TaskMeta().Status = StatusCompleted
	})

	got, ok := r.Get("mut-1")
	if !ok {
		t.Fatal("expected task to exist")
	}
	sub, ok := got.(*SubtaskState)
	if !ok {
		t.Fatalf("expected SubtaskState, got %T", got)
	}
	if sub.Result != "done" {
		t.Fatalf("expected result updated, got %q", sub.Result)
	}
	if sub.TaskMeta().Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", sub.TaskMeta().Status)
	}
}

func TestGetCloneDetachesSubtaskWriteSet(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	s := NewSubtaskState(Meta{
		ID:     "clone-1",
		Label:  "task",
		Kind:   KindAgent,
		Status: StatusRunning,
	}, "parent-1", "child-1", "general", "summary", "", "none")
	s.WriteSet = []string{"a.txt"}
	r.Register(s)

	got, ok := r.Get("clone-1")
	if !ok {
		t.Fatal("expected task to exist")
	}
	sub, ok := got.(*SubtaskState)
	if !ok {
		t.Fatalf("expected SubtaskState, got %T", got)
	}
	sub.WriteSet[0] = "b.txt"

	got2, _ := r.Get("clone-1")
	sub2 := got2.(*SubtaskState)
	if sub2.WriteSet[0] != "a.txt" {
		t.Fatalf("expected registry state unchanged, got %q", sub2.WriteSet[0])
	}
}

func TestRemove(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	r.Register(newState("t3", KindCron, StatusCompleted))
	r.Remove("t3")

	_, ok := r.Get("t3")
	if ok {
		t.Fatal("expected task to be absent after Remove")
	}
}

func TestRemoveMissingIsNoop(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	// Must not panic.
	r.Remove("ghost")
}

// ---- All / BackgroundTasks ---------------------------------------------

func TestAll(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	for i := 0; i < 5; i++ {
		r.Register(newState(fmt.Sprintf("id%d", i), KindAgent, StatusRunning))
	}

	all := r.All()
	if len(all) != 5 {
		t.Fatalf("expected 5, got %d", len(all))
	}
}

func TestBackgroundTasks(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	// Only this one should appear in BackgroundTasks.
	bg := &BaseState{M: Meta{
		ID:             "bg1",
		Kind:           KindDream,
		Status:         StatusRunning,
		IsBackgrounded: true,
	}}
	r.Register(bg)

	// Running but not backgrounded.
	r.Register(newState("fg1", KindAgent, StatusRunning))
	// Backgrounded but completed.
	done := &BaseState{M: Meta{
		ID:             "bg2",
		Kind:           KindCron,
		Status:         StatusCompleted,
		IsBackgrounded: true,
	}}
	r.Register(done)

	bgs := r.BackgroundTasks()
	if len(bgs) != 1 {
		t.Fatalf("expected 1 background task, got %d", len(bgs))
	}
	if bgs[0].TaskMeta().ID != "bg1" {
		t.Fatalf("unexpected id: %s", bgs[0].TaskMeta().ID)
	}
}

func TestBackgroundTasksForSession(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	r.Register(&BaseState{M: Meta{
		ID:             "sess-1-base",
		SessionID:      "sess-1",
		Kind:           KindCron,
		Status:         StatusRunning,
		IsBackgrounded: true,
	}})
	r.Register(NewSubtaskState(Meta{
		ID:             "sess-1-sub",
		SessionID:      "sess-1",
		Kind:           KindAgent,
		Status:         StatusRunning,
		IsBackgrounded: true,
	}, "sess-1", "child-1", "general", "", "", "none"))
	r.Register(NewSubtaskState(Meta{
		ID:             "sess-2-sub",
		SessionID:      "sess-2",
		Kind:           KindAgent,
		Status:         StatusRunning,
		IsBackgrounded: true,
	}, "sess-2", "child-2", "general", "", "", "none"))

	bgs := r.BackgroundTasksForSession("sess-1")
	if len(bgs) != 2 {
		t.Fatalf("expected 2 session background tasks, got %d", len(bgs))
	}
	for _, bg := range bgs {
		if bg.TaskMeta().ID == "sess-2-sub" {
			t.Fatal("unexpected task from another session")
		}
	}
}

func TestNotificationClaimIsAtomicAndRetryable(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	endedAt := time.Now()
	r.Register(NewSubtaskState(Meta{
		ID:             "notify-1",
		SessionID:      "parent-1",
		Kind:           KindAgent,
		Status:         StatusCompleted,
		StartedAt:      endedAt.Add(-time.Second),
		EndedAt:        &endedAt,
		IsBackgrounded: true,
	}, "parent-1", "child-1", "general", "", "", "none"))

	const workers = 20
	claims := make(chan State, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if claimed, ok := r.TryClaimNotification("notify-1"); ok {
				claims <- claimed
			}
		}()
	}
	wg.Wait()
	close(claims)
	if len(claims) != 1 {
		t.Fatalf("expected exactly one notification claim, got %d", len(claims))
	}

	r.ReleaseNotificationClaim("notify-1")
	if _, ok := r.TryClaimNotification("notify-1"); !ok {
		t.Fatal("expected claim to be retryable after release")
	}
	r.MarkNotificationDelivered("notify-1")
	if _, ok := r.TryClaimNotification("notify-1"); ok {
		t.Fatal("delivered notification should not be claimable")
	}
}

// ---- GC ----------------------------------------------------------------

func TestGC_RemovesEligible(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	old := time.Now().Add(-10 * time.Minute)

	// Eligible: completed + notified + old EndedAt.
	eligible := withEndedAt(newState("e1", KindAgent, StatusCompleted), old)
	eligible.M.Notified = true
	r.Register(eligible)

	// Not eligible: not notified.
	notNotified := withEndedAt(newState("e2", KindAgent, StatusCompleted), old)
	notNotified.M.Notified = false
	r.Register(notNotified)

	// Not eligible: EndedAt too recent.
	recent := withEndedAt(newState("e3", KindAgent, StatusCompleted), time.Now().Add(-1*time.Minute))
	recent.M.Notified = true
	r.Register(recent)

	// Not eligible: still running.
	running := newState("e4", KindAgent, StatusRunning)
	r.Register(running)

	// Failed + notified + old → also eligible.
	failedOld := withEndedAt(newState("e5", KindAgent, StatusFailed), old)
	failedOld.M.Notified = true
	r.Register(failedOld)

	// Canceled + notified + old → also eligible.
	canceledOld := withEndedAt(newState("e6", KindAgent, StatusCanceled), old)
	canceledOld.M.Notified = true
	r.Register(canceledOld)

	r.GC(time.Now())

	if _, ok := r.Get("e1"); ok {
		t.Error("e1 should have been GC'd")
	}
	if _, ok := r.Get("e5"); ok {
		t.Error("e5 should have been GC'd")
	}
	if _, ok := r.Get("e6"); ok {
		t.Error("e6 should have been GC'd")
	}
	for _, id := range []string{"e2", "e3", "e4"} {
		if _, ok := r.Get(id); !ok {
			t.Errorf("%s should still be present", id)
		}
	}
}

func TestGC_NilEndedAt(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	// Completed + notified but EndedAt is nil → must not be GC'd.
	s := newState("noend", KindAgent, StatusCompleted)
	s.M.Notified = true
	r.Register(s)

	r.GC(time.Now())

	if _, ok := r.Get("noend"); !ok {
		t.Error("task without EndedAt should not be GC'd")
	}
}

// ---- Subscribe / events ------------------------------------------------

func TestSubscribe_RegisterEvent(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := r.Subscribe(ctx)

	s := newState("ev1", KindToolAsync, StatusPending)
	r.Register(s)

	select {
	case evt := <-ch:
		if evt.Payload.Action != "registered" {
			t.Fatalf("expected 'registered', got %q", evt.Payload.Action)
		}
		if evt.Payload.TaskID != "ev1" {
			t.Fatalf("unexpected task id: %s", evt.Payload.TaskID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for registered event")
	}
}

func TestSubscribe_UpdateEvent(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Register(newState("ev2", KindAgent, StatusPending))

	ch := r.Subscribe(ctx)

	r.Update("ev2", func(m *Meta) { m.Status = StatusRunning })

	select {
	case evt := <-ch:
		if evt.Payload.Action != "updated" {
			t.Fatalf("expected 'updated', got %q", evt.Payload.Action)
		}
		if evt.Payload.Status != StatusRunning {
			t.Fatalf("expected Running status in event, got %s", evt.Payload.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for updated event")
	}
}

func TestSubscribe_RemoveEvent(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Register(newState("ev3", KindCron, StatusCompleted))

	ch := r.Subscribe(ctx)

	r.Remove("ev3")

	select {
	case evt := <-ch:
		if evt.Payload.Action != "removed" {
			t.Fatalf("expected 'removed', got %q", evt.Payload.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for removed event")
	}
}

func TestSubscribe_ChannelClosedOnContextCancel(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	ch := r.Subscribe(ctx)
	cancel()

	// Drain any buffered events then wait for closure.
	timeout := time.After(time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed as expected
			}
		case <-timeout:
			t.Fatal("channel not closed after context cancel")
		}
	}
}

// ---- Concurrent safety -------------------------------------------------

func TestConcurrentRegisterUpdateRemove(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	const workers = 50
	const tasks = 20

	var wg sync.WaitGroup

	// Concurrent registrations.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < tasks; i++ {
				id := fmt.Sprintf("w%d-t%d", w, i)
				r.Register(newState(id, KindAgent, StatusPending))
			}
		}(w)
	}
	wg.Wait()

	// Concurrent updates.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < tasks; i++ {
				id := fmt.Sprintf("w%d-t%d", w, i)
				r.Update(id, func(m *Meta) { m.Status = StatusRunning })
			}
		}(w)
	}
	wg.Wait()

	// Concurrent reads.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			_ = r.All()
			_ = r.BackgroundTasks()
		}(w)
	}
	wg.Wait()

	// Concurrent removes.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < tasks; i++ {
				id := fmt.Sprintf("w%d-t%d", w, i)
				r.Remove(id)
			}
		}(w)
	}
	wg.Wait()

	if len(r.All()) != 0 {
		t.Fatalf("expected empty registry after all removes, got %d", len(r.All()))
	}
}

func TestConcurrentGC(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	old := time.Now().Add(-10 * time.Minute)

	// Register a bunch of GC-eligible tasks.
	const n = 100
	for i := 0; i < n; i++ {
		s := withEndedAt(newState(fmt.Sprintf("gc%d", i), KindCron, StatusCompleted), old)
		s.M.Notified = true
		r.Register(s)
	}

	// Run GC concurrently from multiple goroutines.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.GC(time.Now())
		}()
	}
	wg.Wait()

	if len(r.All()) != 0 {
		t.Fatalf("expected all GC-eligible tasks removed, got %d", len(r.All()))
	}
}

func TestStartGC(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()

	old := time.Now().Add(-10 * time.Minute)

	s := withEndedAt(newState("sgc1", KindAgent, StatusCompleted), old)
	s.M.Notified = true
	r.Register(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.StartGC(ctx, 10*time.Millisecond)

	// Wait up to 1s for the GC to pick up the task.
	deadline := time.After(time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("StartGC did not remove eligible task within 1s")
		case <-time.After(20 * time.Millisecond):
			if _, ok := r.Get("sgc1"); !ok {
				return // GC worked
			}
		}
	}
}

// ---- Shutdown ----------------------------------------------------------

func TestShutdownIdempotent(t *testing.T) {
	r := NewRegistry()
	// Calling Shutdown twice must not panic.
	r.Shutdown()
	r.Shutdown()
}
