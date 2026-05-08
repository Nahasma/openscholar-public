//go:build unix

package cron

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── Parser tests ──────────────────────────────────────────────────────────────

func TestNextRunTime_StandardFiveField(t *testing.T) {
	// "0 * * * *" — top of every hour
	from := time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC)
	next, err := NextRunTime("0 * * * *", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextRunTime = %v, want %v", next, want)
	}
}

func TestNextRunTime_WithSeconds(t *testing.T) {
	// "30 * * * * *" — at :30 seconds of every minute (second field = 30)
	from := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	next, err := NextRunTime("30 * * * * *", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 1, 1, 12, 0, 30, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextRunTime = %v, want %v", next, want)
	}
}

func TestNextRunTime_Daily(t *testing.T) {
	// "0 9 * * *" — every day at 09:00
	from := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
	next, err := NextRunTime("0 9 * * *", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 3, 16, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextRunTime = %v, want %v", next, want)
	}
}

func TestNextRunTime_Descriptor(t *testing.T) {
	// @hourly descriptor
	from := time.Date(2024, 1, 1, 5, 30, 0, 0, time.UTC)
	next, err := NextRunTime("@hourly", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 1, 1, 6, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextRunTime = %v, want %v", next, want)
	}
}

func TestNextRunTime_Invalid(t *testing.T) {
	_, err := NextRunTime("not a cron spec", time.Now())
	if err == nil {
		t.Fatal("expected error for invalid spec, got nil")
	}
}

func TestValidateSpec_Valid(t *testing.T) {
	specs := []string{
		"0 * * * *",
		"*/5 * * * *",
		"0 0 * * 1",
		"@daily",
		"@hourly",
		"0 30 9 * * *",
	}
	for _, spec := range specs {
		if err := ValidateSpec(spec); err != nil {
			t.Errorf("ValidateSpec(%q) = %v, want nil", spec, err)
		}
	}
}

func TestValidateSpec_Invalid(t *testing.T) {
	if err := ValidateSpec("bad spec !!!"); err == nil {
		t.Error("expected error for invalid spec")
	}
}

// ─── Storage tests ─────────────────────────────────────────────────────────────

func newTempStorage(t *testing.T) *Storage {
	t.Helper()
	dir := t.TempDir()
	return NewStorage(dir)
}

func TestStorage_LoadEmpty(t *testing.T) {
	s := newTempStorage(t)
	tasks, err := s.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestStorage_SaveAndLoad(t *testing.T) {
	s := newTempStorage(t)
	now := time.Now().UTC().Truncate(time.Second)
	tasks := []Task{
		{ID: "t1", Spec: "0 * * * *", Prompt: "hello", CreatedAt: now, NextRunAt: now.Add(time.Hour), Recurring: true, Durable: true},
		{ID: "t2", Spec: "@daily", Prompt: "daily", CreatedAt: now, NextRunAt: now.Add(24 * time.Hour), Recurring: false, Durable: true},
	}
	if err := s.Save(tasks); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(loaded))
	}
	if loaded[0].ID != "t1" || loaded[1].ID != "t2" {
		t.Errorf("unexpected task IDs: %v %v", loaded[0].ID, loaded[1].ID)
	}
}

func TestStorage_AddTask(t *testing.T) {
	s := newTempStorage(t)
	task := Task{ID: "add1", Spec: "0 * * * *", Prompt: "p", CreatedAt: time.Now().UTC(), NextRunAt: time.Now().UTC()}
	if err := s.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	tasks, _ := s.Load()
	if len(tasks) != 1 || tasks[0].ID != "add1" {
		t.Errorf("unexpected tasks after AddTask: %+v", tasks)
	}
}

func TestStorage_RemoveTask(t *testing.T) {
	s := newTempStorage(t)
	now := time.Now().UTC()
	_ = s.AddTask(Task{ID: "r1", Spec: "0 * * * *", CreatedAt: now, NextRunAt: now})
	_ = s.AddTask(Task{ID: "r2", Spec: "@daily", CreatedAt: now, NextRunAt: now})

	if err := s.RemoveTask("r1"); err != nil {
		t.Fatalf("RemoveTask: %v", err)
	}
	tasks, _ := s.Load()
	if len(tasks) != 1 || tasks[0].ID != "r2" {
		t.Errorf("expected only r2 remaining, got %+v", tasks)
	}
}

func TestStorage_RemoveTask_NotFound(t *testing.T) {
	s := newTempStorage(t)
	if err := s.RemoveTask("nonexistent"); err == nil {
		t.Error("expected error when removing nonexistent task")
	}
}

func TestStorage_UpdateTask(t *testing.T) {
	s := newTempStorage(t)
	now := time.Now().UTC()
	_ = s.AddTask(Task{ID: "u1", Spec: "0 * * * *", Prompt: "old", CreatedAt: now, NextRunAt: now})

	if err := s.UpdateTask("u1", func(tk *Task) { tk.Prompt = "new" }); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	tasks, _ := s.Load()
	if tasks[0].Prompt != "new" {
		t.Errorf("expected prompt=new, got %q", tasks[0].Prompt)
	}
}

func TestStorage_UpdateTask_NotFound(t *testing.T) {
	s := newTempStorage(t)
	if err := s.UpdateTask("ghost", func(_ *Task) {}); err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestStorage_AtomicWrite(t *testing.T) {
	// Verify no .tmp file is left behind after a successful Save.
	s := newTempStorage(t)
	_ = s.Save([]Task{{ID: "x", Spec: "0 * * * *", CreatedAt: time.Now().UTC(), NextRunAt: time.Now().UTC()}})

	tmpPath := s.path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("tmp file should not exist after successful Save, but found: %s", tmpPath)
	}
	if _, err := os.Stat(s.path); err != nil {
		t.Errorf("expected storage file to exist: %v", err)
	}
}

// ─── Lock tests ────────────────────────────────────────────────────────────────

func newTempLock(t *testing.T) *PIDLock {
	t.Helper()
	dir := t.TempDir()
	return NewPIDLock(dir)
}

func TestPIDLock_AcquireRelease(t *testing.T) {
	l := newTempLock(t)

	ok, err := l.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !ok {
		t.Fatal("expected to acquire lock")
	}
	if !l.IsOwner() {
		t.Error("IsOwner should be true after TryAcquire")
	}

	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if l.IsOwner() {
		t.Error("IsOwner should be false after Release")
	}
	if _, err := os.Stat(l.path); !os.IsNotExist(err) {
		t.Error("lock file should not exist after Release")
	}
}

func TestPIDLock_IdempotentAcquire(t *testing.T) {
	l := newTempLock(t)

	ok1, err := l.TryAcquire()
	if err != nil || !ok1 {
		t.Fatalf("first TryAcquire failed: ok=%v err=%v", ok1, err)
	}
	// Acquiring again by the same process should succeed.
	ok2, err := l.TryAcquire()
	if err != nil || !ok2 {
		t.Errorf("second TryAcquire failed: ok=%v err=%v", ok2, err)
	}
	_ = l.Release()
}

func TestPIDLock_StaleLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, lockFileName)

	// Write a PID that is guaranteed to be dead (PID 0 is never a user process).
	if err := os.WriteFile(lockPath, []byte("99999999"), 0o644); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	l := NewPIDLock(dir)
	ok, err := l.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire on stale lock: %v", err)
	}
	if !ok {
		t.Error("expected to acquire stale lock")
	}
	_ = l.Release()
}

func TestPIDLock_ReleaseNoLock(t *testing.T) {
	l := newTempLock(t)
	// Release without acquiring should be a no-op.
	if err := l.Release(); err != nil {
		t.Errorf("Release without lock: %v", err)
	}
}
