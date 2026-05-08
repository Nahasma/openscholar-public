//go:build unix

package cron

import (
	"testing"
	"time"
)

// ─── Helpers ───────────────────────────────────────────────────────────────────

func newTestScheduler(t *testing.T, onFire OnFireFunc, isLoading func() bool) *Scheduler {
	t.Helper()
	dir := t.TempDir()
	storage := NewStorage(dir)
	return NewScheduler(storage, dir, onFire, isLoading)
}

func neverLoading() bool { return false }

// waitChan waits for a value on ch or times out after d.
func waitChan[T any](t *testing.T, ch <-chan T, d time.Duration) (T, bool) {
	t.Helper()
	select {
	case v := <-ch:
		return v, true
	case <-time.After(d):
		var zero T
		return zero, false
	}
}

// ─── Tests ─────────────────────────────────────────────────────────────────────

func TestScheduler_StartStop(t *testing.T) {
	s := newTestScheduler(t, func(_ Task) {}, neverLoading)
	s.Start(t.Context())

	// Give the goroutine a moment to acquire the lock.
	time.Sleep(50 * time.Millisecond)
	if !s.IsOwner() {
		t.Error("scheduler should own the lock after Start")
	}

	s.Stop()

	// After Stop the lock should be released.
	if s.IsOwner() {
		t.Error("scheduler should not own the lock after Stop")
	}
}

func TestScheduler_AddAndFire(t *testing.T) {
	fired := make(chan Task, 1)
	s := newTestScheduler(t, func(tk Task) { fired <- tk }, neverLoading)

	s.Start(t.Context())
	time.Sleep(50 * time.Millisecond) // let the lock be acquired

	task := Task{
		ID:        "fire-me",
		Spec:      "* * * * *",
		Prompt:    "hello",
		CreatedAt: time.Now(),
		NextRunAt: time.Now().Add(-1 * time.Second), // already due
		Recurring: false,
		Durable:   true,
	}
	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	got, ok := waitChan(t, fired, 3*time.Second)
	if !ok {
		t.Fatal("task did not fire within timeout")
	}
	if got.ID != "fire-me" {
		t.Errorf("fired task ID = %q, want %q", got.ID, "fire-me")
	}

	s.Stop()
}

func TestScheduler_RecurringReschedule(t *testing.T) {
	fired := make(chan Task, 10)
	s := newTestScheduler(t, func(tk Task) { fired <- tk }, neverLoading)

	s.Start(t.Context())
	time.Sleep(50 * time.Millisecond)

	task := Task{
		ID:        "recurring",
		Spec:      "* * * * *",
		Prompt:    "repeat",
		CreatedAt: time.Now(),
		NextRunAt: time.Now().Add(-1 * time.Second),
		Recurring: true,
		Durable:   true,
	}
	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Wait for the first fire.
	_, ok := waitChan(t, fired, 3*time.Second)
	if !ok {
		t.Fatal("recurring task did not fire")
	}

	// After firing, the task should still be in the list (rescheduled).
	time.Sleep(50 * time.Millisecond)
	tasks := s.ListTasks()
	found := false
	for _, tk := range tasks {
		if tk.ID == "recurring" {
			found = true
			break
		}
	}
	if !found {
		t.Error("recurring task should still be in list after firing")
	}

	s.Stop()
}

func TestScheduler_OneShotRemoval(t *testing.T) {
	fired := make(chan Task, 1)
	s := newTestScheduler(t, func(tk Task) { fired <- tk }, neverLoading)

	s.Start(t.Context())
	time.Sleep(50 * time.Millisecond)

	task := Task{
		ID:        "one-shot",
		Spec:      "* * * * *",
		Prompt:    "once",
		CreatedAt: time.Now(),
		NextRunAt: time.Now().Add(-1 * time.Second),
		Recurring: false,
	}
	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	_, ok := waitChan(t, fired, 3*time.Second)
	if !ok {
		t.Fatal("one-shot task did not fire")
	}

	// After firing the task should be gone.
	time.Sleep(50 * time.Millisecond)
	tasks := s.ListTasks()
	for _, tk := range tasks {
		if tk.ID == "one-shot" {
			t.Error("one-shot task should be removed after firing")
		}
	}

	s.Stop()
}

func TestScheduler_IsLoadingSkips(t *testing.T) {
	fired := make(chan Task, 1)
	loading := true
	isLoading := func() bool { return loading }

	s := newTestScheduler(t, func(tk Task) { fired <- tk }, isLoading)

	s.Start(t.Context())
	time.Sleep(50 * time.Millisecond)

	task := Task{
		ID:        "skip-while-loading",
		Spec:      "* * * * *",
		Prompt:    "skipped",
		CreatedAt: time.Now(),
		NextRunAt: time.Now().Add(-1 * time.Second),
		Recurring: false,
	}
	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Task should NOT fire while isLoading returns true.
	select {
	case tk := <-fired:
		t.Errorf("task fired while loading: %v", tk.ID)
	case <-time.After(2 * time.Second):
		// Expected: no fire.
	}

	s.Stop()
}

func TestScheduler_ListTasks(t *testing.T) {
	s := newTestScheduler(t, func(_ Task) {}, neverLoading)

	now := time.Now()
	tasks := []Task{
		{ID: "a", Spec: "* * * * *", CreatedAt: now, NextRunAt: now.Add(time.Hour)},
		{ID: "b", Spec: "@daily", CreatedAt: now, NextRunAt: now.Add(24 * time.Hour)},
	}
	for _, tk := range tasks {
		if err := s.AddTask(tk); err != nil {
			t.Fatalf("AddTask(%s): %v", tk.ID, err)
		}
	}

	list := s.ListTasks()
	if len(list) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(list))
	}

	// Verify the snapshot is independent (mutation should not affect cache).
	list[0].ID = "mutated"
	cached := s.ListTasks()
	if cached[0].ID == "mutated" {
		t.Error("ListTasks should return a copy, not a reference")
	}
}

func TestScheduler_RemoveTask(t *testing.T) {
	s := newTestScheduler(t, func(_ Task) {}, neverLoading)

	now := time.Now()
	task := Task{ID: "del", Spec: "* * * * *", CreatedAt: now, NextRunAt: now.Add(time.Hour)}
	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := s.RemoveTask("del"); err != nil {
		t.Fatalf("RemoveTask: %v", err)
	}

	tasks := s.ListTasks()
	for _, tk := range tasks {
		if tk.ID == "del" {
			t.Error("removed task still present in list")
		}
	}
}

func TestScheduler_AddTask_InvalidSpec(t *testing.T) {
	s := newTestScheduler(t, func(_ Task) {}, neverLoading)

	task := Task{
		ID:        "bad",
		Spec:      "not a cron spec !!!",
		CreatedAt: time.Now(),
	}
	if err := s.AddTask(task); err == nil {
		t.Error("expected error for invalid spec, got nil")
	}

	// Task should not be in the list.
	tasks := s.ListTasks()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after failed AddTask, got %d", len(tasks))
	}
}

func TestScheduler_NonDurableNotPersisted(t *testing.T) {
	dir := t.TempDir()
	storage := NewStorage(dir)
	s := NewScheduler(storage, dir, func(_ Task) {}, neverLoading)

	now := time.Now()
	// Add a non-durable task — it should be in memory but not on disk.
	ephemeral := Task{
		ID:        "ephemeral",
		Spec:      "* * * * *",
		Prompt:    "temp",
		CreatedAt: now,
		NextRunAt: now.Add(time.Hour),
		Recurring: false,
		Durable:   false,
	}
	if err := s.AddTask(ephemeral); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// In-memory list should contain it.
	if len(s.ListTasks()) != 1 {
		t.Fatalf("expected 1 task in memory, got %d", len(s.ListTasks()))
	}

	// Storage should have no tasks (non-durable was not persisted).
	persisted, err := storage.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(persisted) != 0 {
		t.Errorf("expected 0 persisted tasks, got %d", len(persisted))
	}

	// Add a durable task — it should be persisted.
	durable := Task{
		ID:        "durable",
		Spec:      "@daily",
		Prompt:    "keep",
		CreatedAt: now,
		NextRunAt: now.Add(24 * time.Hour),
		Recurring: true,
		Durable:   true,
	}
	if err := s.AddTask(durable); err != nil {
		t.Fatalf("AddTask durable: %v", err)
	}

	persisted, err = storage.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(persisted) != 1 || persisted[0].ID != "durable" {
		t.Errorf("expected only durable task persisted, got %+v", persisted)
	}
}
