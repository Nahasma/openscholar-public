//go:build unix

package cron

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Scheduler manages scheduled tasks: it holds an in-memory cache of tasks,
// coordinates persistence via Storage, ensures single-instance execution via
// PIDLock, and fires OnFireFunc callbacks on schedule.
type Scheduler struct {
	storage   *Storage
	lock      *PIDLock
	onFire    OnFireFunc
	isLoading func() bool

	mu      sync.Mutex
	tasks   []Task
	stopped chan struct{}
}

// NewScheduler creates a Scheduler. lockDir is used for the PID lock file.
// onFire is called (in the tick goroutine) each time a task fires.
// isLoading is queried before firing — if true the tick is skipped.
func NewScheduler(storage *Storage, lockDir string, onFire OnFireFunc, isLoading func() bool) *Scheduler {
	return &Scheduler{
		storage:   storage,
		lock:      NewPIDLock(lockDir),
		onFire:    onFire,
		isLoading: isLoading,
		stopped:   make(chan struct{}),
	}
}

// Start attempts to acquire the PID lock (retrying every 5 s on failure),
// loads persisted tasks into memory, then launches the tick goroutine.
// The goroutine runs until Stop is called or ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		// Keep retrying until the lock is acquired or we are stopped/cancelled.
		for {
			ok, err := s.lock.TryAcquire()
			if err == nil && ok {
				break
			}
			select {
			case <-s.stopped:
				return
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}

		// Load tasks from persistent storage.
		tasks, err := s.storage.Load()
		if err == nil {
			s.mu.Lock()
			s.tasks = tasks
			s.mu.Unlock()
		}

		// Tick loop — check every second.
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopped:
				return
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if s.isLoading() {
					continue
				}
				s.tick(now)
			}
		}
	}()
}

// tick checks all in-memory tasks and fires those whose NextRunAt <= now.
func (s *Scheduler) tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false
	var remaining []Task

	for i := range s.tasks {
		t := s.tasks[i]
		if now.Before(t.NextRunAt) {
			remaining = append(remaining, t)
			continue
		}

		// Fire the task (outside the lock is better for performance, but
		// keeping it inside is simpler and safe for our use-case; callbacks
		// should be non-blocking or fast).
		s.onFire(t)

		lastRun := now
		t.LastRunAt = &lastRun

		if t.Recurring {
			next, err := NextRunTime(t.Spec, now)
			if err == nil {
				t.NextRunAt = next
			}
			remaining = append(remaining, t)
		}
		// Non-recurring tasks are simply not appended — they are removed.

		changed = true
	}

	if !changed {
		return
	}

	s.tasks = remaining

	// Persist only durable tasks (best-effort — log errors are swallowed here
	// because there is no logger dependency in this package).
	s.persistDurable()
}

// Stop closes the stopped channel to signal the tick goroutine to exit, and
// releases the PID lock.
func (s *Scheduler) Stop() {
	select {
	case <-s.stopped:
		// Already stopped — avoid closing a closed channel.
	default:
		close(s.stopped)
	}
	_ = s.lock.Release()
}

// persistDurable saves only Durable tasks to storage. Must be called with mu held.
func (s *Scheduler) persistDurable() {
	var durable []Task
	for _, t := range s.tasks {
		if t.Durable {
			durable = append(durable, t)
		}
	}
	_ = s.storage.Save(durable)
}

// AddTask validates the cron spec, computes the first NextRunAt, persists the
// task (if Durable), and appends it to the in-memory cache.
func (s *Scheduler) AddTask(t Task) error {
	if err := ValidateSpec(t.Spec); err != nil {
		return err
	}

	// Compute NextRunAt from now if not already set.
	if t.NextRunAt.IsZero() {
		next, err := NextRunTime(t.Spec, time.Now())
		if err != nil {
			return err
		}
		t.NextRunAt = next
	}

	// Only persist durable tasks to storage.
	if t.Durable {
		if err := s.storage.AddTask(t); err != nil {
			return err
		}
	}

	s.mu.Lock()
	s.tasks = append(s.tasks, t)
	s.mu.Unlock()

	return nil
}

// RemoveTask removes the task. For durable tasks, storage is updated first;
// if storage fails the in-memory state is left unchanged. Returns an error
// if the task does not exist or storage removal fails.
func (s *Scheduler) RemoveTask(id string) error {
	s.mu.Lock()

	found := false
	isDurable := false
	for _, t := range s.tasks {
		if t.ID == id {
			found = true
			isDurable = t.Durable
			break
		}
	}

	if !found {
		s.mu.Unlock()
		return fmt.Errorf("cron scheduler: task %q not found", id)
	}
	s.mu.Unlock()

	// Persist first — if storage fails, keep in-memory state unchanged.
	if isDurable {
		if err := s.storage.RemoveTask(id); err != nil {
			return fmt.Errorf("cron scheduler: remove durable task: %w", err)
		}
	}

	// Storage succeeded (or non-durable) — now update memory.
	s.mu.Lock()
	filtered := s.tasks[:0]
	for _, t := range s.tasks {
		if t.ID != id {
			filtered = append(filtered, t)
		}
	}
	s.tasks = filtered
	s.mu.Unlock()

	return nil
}

// ListTasks returns a snapshot of the current in-memory task list.
func (s *Scheduler) ListTasks() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := make([]Task, len(s.tasks))
	copy(snapshot, s.tasks)
	return snapshot
}

// IsOwner reports whether this process currently holds the PID lock.
func (s *Scheduler) IsOwner() bool {
	return s.lock.IsOwner()
}
