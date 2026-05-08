package hooks

import (
	"context"
	"sync"
)

// Service manages lifecycle hook execution.
//
// Callers are responsible for nil-checking before calling methods.
// A nil Service is not safe to call; callers should guard with:
//
//	if svc != nil { svc.Run(ctx, event, input) }
type Service interface {
	// Run executes blocking hooks first (in order), then fires async hooks.
	Run(ctx context.Context, event Event, input Input) error

	// RunBlocking executes only blocking (synchronous) hooks for the event.
	RunBlocking(ctx context.Context, event Event, input Input) error

	// RunAsync fires only async hooks for the event (fire-and-forget).
	RunAsync(ctx context.Context, event Event, input Input)

	// Reload re-reads the hook configuration from its source.
	Reload() error
}

// serviceImpl is the concrete implementation of Service.
type serviceImpl struct {
	mu     sync.RWMutex
	runner *hookRunner
	loader func() ([]HookConfig, error) // called by Reload
}

// newService constructs a Service from a pre-loaded runner and a reload
// function. Pass nil for loader to disable runtime reloading.
func newService(runner *hookRunner, loader func() ([]HookConfig, error)) Service {
	return &serviceImpl{
		runner: runner,
		loader: loader,
	}
}

// Run implements Service.
func (s *serviceImpl) Run(ctx context.Context, event Event, input Input) error {
	if skip(ctx, event) {
		return nil
	}
	childCtx := withDepth(ctx) // increment depth for any hooks spawned by this execution

	s.mu.RLock()
	r := s.runner
	s.mu.RUnlock()

	if err := r.runBlocking(childCtx, event, input); err != nil {
		return err
	}
	r.runAsync(childCtx, event, input)
	return nil
}

// RunBlocking implements Service.
func (s *serviceImpl) RunBlocking(ctx context.Context, event Event, input Input) error {
	if skip(ctx, event) {
		return nil
	}
	childCtx := withDepth(ctx)

	s.mu.RLock()
	r := s.runner
	s.mu.RUnlock()

	return r.runBlocking(childCtx, event, input)
}

// RunAsync implements Service.
func (s *serviceImpl) RunAsync(ctx context.Context, event Event, input Input) {
	if skip(ctx, event) {
		return
	}
	childCtx := withDepth(ctx)

	s.mu.RLock()
	r := s.runner
	s.mu.RUnlock()

	r.runAsync(childCtx, event, input)
}

// Reload implements Service.
func (s *serviceImpl) Reload() error {
	if s.loader == nil {
		return nil
	}
	configs, err := s.loader()
	if err != nil {
		return err
	}

	r := newHookRunner(configs)

	s.mu.Lock()
	s.runner = r
	s.mu.Unlock()

	return nil
}

// withDepth increments the hook depth counter stored in ctx.
func withDepth(ctx context.Context) context.Context {
	depth, _ := ctx.Value(hookDepthKey{}).(int)
	return context.WithValue(ctx, hookDepthKey{}, depth+1)
}

// currentDepth returns the current hook depth from ctx (0 if not set).
func currentDepth(ctx context.Context) int {
	depth, _ := ctx.Value(hookDepthKey{}).(int)
	return depth
}

// skip returns true when the hook should not be executed due to recursion
// protection. Tool-related events are blocked when depth >= 1, unless the
// event is in the allowlist.
func skip(ctx context.Context, event Event) bool {
	if depthAllowlistEvents[event] {
		return false
	}
	if !toolHookEvents[event] {
		return false
	}
	return currentDepth(ctx) >= 1
}
