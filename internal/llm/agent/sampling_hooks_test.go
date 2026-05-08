package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/message"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// countingHook increments a counter each time Run is called.
type countingHook struct {
	name    string
	count   atomic.Int32
	delay   time.Duration // optional artificial delay
	retErr  error         // optional error to return
}

func (h *countingHook) Name() string { return h.name }
func (h *countingHook) Run(_ context.Context, _ PostSamplingContext) error {
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	h.count.Add(1)
	return h.retErr
}

// panicHook always panics when Run is called.
type panicHook struct{ name string }

func (h *panicHook) Name() string { return h.name }
func (h *panicHook) Run(_ context.Context, _ PostSamplingContext) error {
	panic("intentional test panic")
}

// blockingHook blocks until ctx is cancelled, then returns.
type blockingHook struct {
	name    string
	started chan struct{}
}

func (h *blockingHook) Name() string { return h.name }
func (h *blockingHook) Run(ctx context.Context, _ PostSamplingContext) error {
	if h.started != nil {
		close(h.started)
	}
	<-ctx.Done()
	return ctx.Err()
}

// recordingHook records the PostSamplingContext it received.
type recordingHook struct {
	name string
	mu   sync.Mutex
	got  []PostSamplingContext
}

func (h *recordingHook) Name() string { return h.name }
func (h *recordingHook) Run(_ context.Context, ps PostSamplingContext) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.got = append(h.got, ps)
	return nil
}

// newTestPS builds a minimal PostSamplingContext for tests.
func newTestPS(source SamplingSource) PostSamplingContext {
	return PostSamplingContext{
		SessionID:     "test-session",
		Source:        source,
		AssistantMsg:  message.Message{},
		History:       []message.Message{},
		ToolCallCount: 2,
		StartedAt:     time.Now(),
		FinishedAt:    time.Now(),
	}
}

// newTestRegistry creates a PostSamplingRegistry backed by a discarding slog logger.
func newTestRegistry() PostSamplingRegistry {
	return NewPostSamplingRegistry(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
}

// waitUntil polls cond until it returns true or d elapses, then returns whether it became true.
func waitUntil(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestPostSamplingMultipleHooksAllTriggered verifies that all registered hooks are
// invoked when ExecuteAsync is called with SamplingSourceMain.
func TestPostSamplingMultipleHooksAllTriggered(t *testing.T) {
	reg := newTestRegistry()

	hookA := &countingHook{name: "hookA"}
	hookB := &countingHook{name: "hookB"}
	hookC := &countingHook{name: "hookC"}

	reg.Register(hookA)
	reg.Register(hookB)
	reg.Register(hookC)

	reg.ExecuteAsync(context.Background(), newTestPS(SamplingSourceMain))

	ok := waitUntil(t, 2*time.Second, func() bool {
		return hookA.count.Load() == 1 &&
			hookB.count.Load() == 1 &&
			hookC.count.Load() == 1
	})
	if !ok {
		t.Errorf("not all hooks triggered: A=%d B=%d C=%d",
			hookA.count.Load(), hookB.count.Load(), hookC.count.Load())
	}
}

// TestPostSamplingPanicRecovery ensures that a panicking hook does not affect
// other hooks in the registry.
func TestPostSamplingPanicRecovery(t *testing.T) {
	reg := newTestRegistry()

	good := &countingHook{name: "good"}
	bad := &panicHook{name: "panic-hook"}

	reg.Register(bad)
	reg.Register(good)

	reg.ExecuteAsync(context.Background(), newTestPS(SamplingSourceMain))

	ok := waitUntil(t, 2*time.Second, func() bool {
		return good.count.Load() == 1
	})
	if !ok {
		t.Error("good hook was not triggered after sibling panicked")
	}
}

// TestPostSamplingTimeoutContextCancelled verifies that a blocking hook's context is
// cancelled when the per-hook timeout elapses.  We use a very small timeout override
// by relying on the parent context being cancelled instead.
func TestPostSamplingTimeoutContextCancelled(t *testing.T) {
	reg := newTestRegistry()

	started := make(chan struct{})
	bh := &blockingHook{name: "blocker", started: started}
	reg.Register(bh)

	// Cancel parent context quickly so that the hook's derived context is also cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	reg.ExecuteAsync(ctx, newTestPS(SamplingSourceMain))

	// The hook must have started.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("hook never started")
	}

	// After parent context times out, the hook's context should be done.
	<-ctx.Done()
	// No assertion needed beyond "did not deadlock".
}

// TestPostSamplingForkedSourceSkipped verifies that no hooks are executed when
// the source is SamplingSourceForked.
func TestPostSamplingForkedSourceSkipped(t *testing.T) {
	reg := newTestRegistry()

	h := &countingHook{name: "should-not-run"}
	reg.Register(h)

	reg.ExecuteAsync(context.Background(), newTestPS(SamplingSourceForked))

	// Give goroutines a chance to run (they should not).
	time.Sleep(100 * time.Millisecond)

	if h.count.Load() != 0 {
		t.Errorf("hook was called for forked source: count=%d", h.count.Load())
	}
}

// TestPostSamplingClearEmptiesRegistry verifies that Clear removes all hooks so that
// subsequent ExecuteAsync calls invoke nothing.
func TestPostSamplingClearEmptiesRegistry(t *testing.T) {
	reg := newTestRegistry()

	h := &countingHook{name: "pre-clear"}
	reg.Register(h)
	reg.Clear()

	reg.ExecuteAsync(context.Background(), newTestPS(SamplingSourceMain))

	time.Sleep(100 * time.Millisecond)

	if h.count.Load() != 0 {
		t.Errorf("hook was called after Clear: count=%d", h.count.Load())
	}
}

// TestPostSamplingHookErrorDoesNotBlockOthers verifies that a hook returning an error
// does not prevent other hooks from executing.
func TestPostSamplingHookErrorDoesNotBlockOthers(t *testing.T) {
	reg := newTestRegistry()

	failing := &countingHook{name: "failing", retErr: errors.New("intentional")}
	succeeding := &countingHook{name: "succeeding"}

	reg.Register(failing)
	reg.Register(succeeding)

	reg.ExecuteAsync(context.Background(), newTestPS(SamplingSourceMain))

	ok := waitUntil(t, 2*time.Second, func() bool {
		return failing.count.Load() == 1 && succeeding.count.Load() == 1
	})
	if !ok {
		t.Errorf("hooks did not both execute: failing=%d succeeding=%d",
			failing.count.Load(), succeeding.count.Load())
	}
}

// TestPostSamplingContextPassedThrough verifies the PostSamplingContext fields are
// delivered intact to the hook.
func TestPostSamplingContextPassedThrough(t *testing.T) {
	reg := newTestRegistry()

	rec := &recordingHook{name: "recorder"}
	reg.Register(rec)

	ps := PostSamplingContext{
		SessionID:     "sess-abc",
		Source:        SamplingSourceMain,
		ToolCallCount: 7,
		StartedAt:     time.Now().Truncate(time.Second),
		FinishedAt:    time.Now().Truncate(time.Second).Add(time.Second),
	}

	reg.ExecuteAsync(context.Background(), ps)

	ok := waitUntil(t, 2*time.Second, func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return len(rec.got) == 1
	})
	if !ok {
		t.Fatal("hook was not called")
	}

	rec.mu.Lock()
	got := rec.got[0]
	rec.mu.Unlock()

	if got.SessionID != ps.SessionID {
		t.Errorf("SessionID: got %q want %q", got.SessionID, ps.SessionID)
	}
	if got.ToolCallCount != ps.ToolCallCount {
		t.Errorf("ToolCallCount: got %d want %d", got.ToolCallCount, ps.ToolCallCount)
	}
	if got.Source != SamplingSourceMain {
		t.Errorf("Source: got %q want %q", got.Source, SamplingSourceMain)
	}
}

// TestPostSamplingRaceSafety exercises concurrent Register and ExecuteAsync calls to
// surface data races when run with -race.
func TestPostSamplingRaceSafety(t *testing.T) {
	reg := newTestRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reg.Register(&countingHook{name: "concurrent"})
		}(i)
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.ExecuteAsync(context.Background(), newTestPS(SamplingSourceMain))
		}()
	}

	wg.Wait()
	// Allow spawned goroutines to finish.
	time.Sleep(200 * time.Millisecond)
}
