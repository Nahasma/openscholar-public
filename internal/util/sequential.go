package util

import (
	"context"
	"sync"
)

// SequentialRunner ensures concurrent write operations on the same resource are serialized.
// Key naming conventions: "magicdoc:<path>", "session-memory:<sessionID>", "checkpoint:<sessionID>"
type SequentialRunner[K comparable] interface {
	Do(ctx context.Context, key K, fn func(context.Context) error) error
}

type sequentialRunner[K comparable] struct {
	mu    sync.Mutex
	locks map[K]*sync.Mutex
}

// NewSequentialRunner creates a new SequentialRunner that serializes operations
// per key while allowing different keys to run in parallel.
func NewSequentialRunner[K comparable]() SequentialRunner[K] {
	return &sequentialRunner[K]{
		locks: make(map[K]*sync.Mutex),
	}
}

// Do executes fn while holding the per-key mutex, ensuring serial execution
// for the same key. Different keys can run concurrently.
// It respects context cancellation both before acquiring the lock and before
// executing fn.
func (r *sequentialRunner[K]) Do(ctx context.Context, key K, fn func(context.Context) error) error {
	// Check context before we even try to acquire the lock.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Retrieve or create the per-key mutex under the outer lock.
	r.mu.Lock()
	km, ok := r.locks[key]
	if !ok {
		km = &sync.Mutex{}
		r.locks[key] = km
	}
	r.mu.Unlock()

	// Acquire the per-key lock.  We do this in a goroutine so we can also
	// respect context cancellation while waiting.
	locked := make(chan struct{})
	go func() {
		km.Lock()
		close(locked)
	}()

	select {
	case <-ctx.Done():
		// We were cancelled while waiting for the key lock.  We must still
		// drain the lock goroutine: wait until it has locked, then immediately
		// unlock so that other waiters can proceed.
		<-locked
		km.Unlock()
		return ctx.Err()
	case <-locked:
		// We now hold the per-key lock.
	}

	defer km.Unlock()

	// One more cancellation check before running the actual work.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return fn(ctx)
}
