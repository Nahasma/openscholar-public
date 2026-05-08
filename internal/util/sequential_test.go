package util

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSequentialRunner_SameKeyIsSerial verifies that two concurrent calls with
// the same key are serialized (they never overlap).
func TestSequentialRunner_SameKeyIsSerial(t *testing.T) {
	r := NewSequentialRunner[string]()

	var concurrent int64 // number of goroutines currently inside fn
	var maxConcurrent int64

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := r.Do(context.Background(), "same-key", func(ctx context.Context) error {
				n := atomic.AddInt64(&concurrent, 1)
				// Record maximum observed concurrency.
				for {
					old := atomic.LoadInt64(&maxConcurrent)
					if n <= old || atomic.CompareAndSwapInt64(&maxConcurrent, old, n) {
						break
					}
				}
				// Hold the "lock" briefly to make overlap observable if serialization is broken.
				time.Sleep(2 * time.Millisecond)
				atomic.AddInt64(&concurrent, -1)
				return nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()

	if maxConcurrent != 1 {
		t.Errorf("expected max concurrent = 1 for same key, got %d", maxConcurrent)
	}
}

// TestSequentialRunner_DifferentKeysParallel verifies that two different keys
// can run their fn concurrently.
func TestSequentialRunner_DifferentKeysParallel(t *testing.T) {
	r := NewSequentialRunner[string]()

	// Use a barrier to ensure both goroutines enter fn at the same time.
	ready := make(chan struct{})
	var inside int64

	bothInside := make(chan struct{}, 1)

	run := func(key string) {
		_ = r.Do(context.Background(), key, func(ctx context.Context) error {
			atomic.AddInt64(&inside, 1)
			// Signal that we are inside.
			select {
			case ready <- struct{}{}:
			default:
			}
			// Wait until the peer is also inside (max 2 seconds).
			deadline := time.After(2 * time.Second)
			for atomic.LoadInt64(&inside) < 2 {
				select {
				case <-deadline:
					return nil
				default:
					time.Sleep(time.Millisecond)
				}
			}
			select {
			case bothInside <- struct{}{}:
			default:
			}
			atomic.AddInt64(&inside, -1)
			return nil
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); run("key-A") }()
	go func() { defer wg.Done(); run("key-B") }()

	wg.Wait()

	select {
	case <-bothInside:
		// Both were inside simultaneously — correct.
	default:
		t.Error("different keys should run concurrently but they did not overlap")
	}
}

// TestSequentialRunner_ContextCancelBeforeLock verifies that a context already
// cancelled before calling Do is respected immediately.
func TestSequentialRunner_ContextCancelBeforeLock(t *testing.T) {
	r := NewSequentialRunner[string]()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Do

	err := r.Do(ctx, "cancel-key", func(ctx context.Context) error {
		t.Error("fn should not be called when context is already cancelled")
		return nil
	})

	if err == nil {
		t.Error("expected context error, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestSequentialRunner_ContextCancelWhileWaiting verifies that a context
// cancelled while waiting for the per-key lock is respected.
func TestSequentialRunner_ContextCancelWhileWaiting(t *testing.T) {
	r := NewSequentialRunner[string]()

	// Hold the key lock for a long time.
	holding := make(chan struct{})
	released := make(chan struct{})
	go func() {
		_ = r.Do(context.Background(), "block-key", func(ctx context.Context) error {
			close(holding)
			time.Sleep(500 * time.Millisecond)
			return nil
		})
		close(released)
	}()

	// Wait until the holder is inside.
	<-holding

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := r.Do(ctx, "block-key", func(ctx context.Context) error {
		t.Error("fn should not be called after context timeout")
		return nil
	})

	if err == nil {
		t.Error("expected context error, got nil")
	}

	// Wait for the holding goroutine to finish so we don't leak goroutines.
	<-released
}

// TestSequentialRunner_FnErrorPropagated verifies that errors returned by fn
// are correctly propagated to the caller.
func TestSequentialRunner_FnErrorPropagated(t *testing.T) {
	r := NewSequentialRunner[int]()

	sentinel := context.DeadlineExceeded // reuse a known error value
	err := r.Do(context.Background(), 42, func(ctx context.Context) error {
		return sentinel
	})

	if err != sentinel {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// TestSequentialRunner_MultipleKeys verifies independent tracking for multiple keys.
func TestSequentialRunner_MultipleKeys(t *testing.T) {
	r := NewSequentialRunner[string]()

	keys := []string{
		"magicdoc:/path/to/file",
		"session-memory:sess-001",
		"checkpoint:sess-001",
		"magicdoc:/other/path",
	}

	var wg sync.WaitGroup
	for _, k := range keys {
		k := k
		wg.Add(3)
		for i := 0; i < 3; i++ {
			go func() {
				defer wg.Done()
				err := r.Do(context.Background(), k, func(ctx context.Context) error {
					time.Sleep(time.Millisecond)
					return nil
				})
				if err != nil {
					t.Errorf("key %q: unexpected error: %v", k, err)
				}
			}()
		}
	}
	wg.Wait()
}
