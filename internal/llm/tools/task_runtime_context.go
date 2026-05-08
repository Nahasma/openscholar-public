package tools

import (
	"context"
	"strings"
)

type taskRuntimeOverrideContextKey struct{}

var taskRuntimeOverrideCtxKey = taskRuntimeOverrideContextKey{}

// TaskRuntimeOverride carries request-scoped runtime hints for a Task worker run.
type TaskRuntimeOverride struct {
	Model        string
	AllowedTools []string
}

// WithTaskRuntimeOverride attaches task-scoped runtime hints to ctx.
func WithTaskRuntimeOverride(ctx context.Context, runtime TaskRuntimeOverride) context.Context {
	runtime.Model = strings.TrimSpace(runtime.Model)
	runtime.AllowedTools = normalizeToolNames(runtime.AllowedTools)
	if runtime.Model == "" && len(runtime.AllowedTools) == 0 {
		return ctx
	}
	return context.WithValue(ctx, taskRuntimeOverrideCtxKey, runtime)
}

func normalizeToolNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// TaskRuntimeOverrideFromContext returns task-scoped runtime hints stored on ctx.
func TaskRuntimeOverrideFromContext(ctx context.Context) (TaskRuntimeOverride, bool) {
	runtime, ok := ctx.Value(taskRuntimeOverrideCtxKey).(TaskRuntimeOverride)
	if !ok {
		return TaskRuntimeOverride{}, false
	}
	return runtime, true
}
