package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

const defaultTimeout = 10 // seconds

// compiledHook is a HookConfig that has been validated and its condition built.
type compiledHook struct {
	cfg       HookConfig
	condition Condition
	fired     bool
}

// hookRunner executes compiled hooks.
type hookRunner struct {
	mu    sync.Mutex
	hooks []compiledHook
}

// newHookRunner creates a hookRunner from a slice of HookConfig values.
func newHookRunner(configs []HookConfig) *hookRunner {
	compiled := make([]compiledHook, 0, len(configs))
	for _, cfg := range configs {
		compiled = append(compiled, compiledHook{
			cfg:       cfg,
			condition: buildCondition(cfg.If),
		})
	}
	return &hookRunner{hooks: compiled}
}

// runBlocking executes all matching non-async hooks for the given event
// sequentially, returning the first error encountered.
func (r *hookRunner) runBlocking(ctx context.Context, event Event, input Input) error {
	for i := range r.hooks {
		cfg, ok := r.nextHook(i, event, input, false)
		if !ok {
			continue
		}
		if err := execHook(ctx, cfg, input); err != nil {
			return fmt.Errorf("hook %q failed: %w", cfg.Command, err)
		}
	}
	return nil
}

// runAsync fires all matching async hooks for the given event in goroutines.
// Errors are logged but not returned.
func (r *hookRunner) runAsync(ctx context.Context, event Event, input Input) {
	for i := range r.hooks {
		cfg, ok := r.nextHook(i, event, input, true)
		if !ok {
			continue
		}
		cfgCopy := cfg
		go func() {
			if err := execHook(ctx, cfgCopy, input); err != nil {
				slog.Warn("async hook error",
					"event", event,
					"command", cfgCopy.Command,
					"error", err,
				)
			}
		}()
	}
}

func (r *hookRunner) nextHook(index int, event Event, input Input, async bool) (HookConfig, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.hooks) {
		return HookConfig{}, false
	}
	h := &r.hooks[index]
	if h.cfg.Event != event || h.cfg.Async != async || !h.condition.Match(input) {
		return HookConfig{}, false
	}
	if h.cfg.Once && h.fired {
		return HookConfig{}, false
	}
	if h.cfg.Once {
		h.fired = true
	}
	return h.cfg, true
}

// execHook runs a single hook command, writing the JSON-encoded Input to its
// stdin. The command runs under /bin/sh -c to support shell features.
func execHook(ctx context.Context, cfg HookConfig, input Input) error {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	payload, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal input: %w", err)
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cfg.Command)
	cmd.Stdin = bytes.NewReader(payload)

	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%w: %s", err, string(out))
		}
		return err
	}
	return nil
}
