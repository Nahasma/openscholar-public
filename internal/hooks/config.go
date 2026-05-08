package hooks

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// HooksConfig is the top-level JSON structure for the hooks field in
// config.json.  It is a slice of HookConfig values.
//
// Example config.json fragment:
//
//	{
//	  "hooks": [
//	    {
//	      "event": "pre_tool_use",
//	      "command": "echo pre",
//	      "timeout": 5,
//	      "if": { "tool": "Bash" }
//	    }
//	  ]
//	}
type HooksConfig []HookConfig

// configFile is the minimal JSON envelope used to extract the hooks field.
type configFile struct {
	Hooks *HooksConfig `json:"hooks"`
}

// LoadFromFile reads a JSON config file at path and returns a Service built
// from the hooks field.
//
//   - If the file cannot be read, an error is returned.
//   - If the file has no "hooks" field, a no-op Service is returned (fail-open).
//   - If the "hooks" field is present but malformed, an error is returned and a
//     warning is logged.
func LoadFromFile(path string) (Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("hooks: read config %q: %w", path, err)
	}
	return parseConfig(data, path)
}

// parseConfig parses raw JSON bytes and builds a Service.
// It is separate from LoadFromFile to facilitate testing.
func parseConfig(data []byte, source string) (Service, error) {
	var envelope configFile
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("hooks: parse config %q: %w", source, err)
	}

	if envelope.Hooks == nil {
		// No hooks field — silently return a no-op service.
		return newService(newHookRunner(nil), nil), nil
	}

	configs := []HookConfig(*envelope.Hooks)

	// Validate each hook entry: command must not be empty.
	valid := make([]HookConfig, 0, len(configs))
	for i, h := range configs {
		if h.Command == "" {
			slog.Warn("hooks: skipping hook with empty command",
				"index", i,
				"event", h.Event,
				"source", source,
			)
			continue
		}
		if h.Event == "" {
			slog.Warn("hooks: skipping hook with empty event",
				"index", i,
				"command", h.Command,
				"source", source,
			)
			continue
		}
		if !ValidEvent(h.Event) {
			slog.Warn("hooks: skipping hook with unknown event",
				"index", i,
				"event", h.Event,
				"command", h.Command,
				"source", source,
			)
			continue
		}
		valid = append(valid, h)
	}

	loader := func() ([]HookConfig, error) {
		svc, err := LoadFromFile(source)
		if err != nil {
			return nil, err
		}
		// Extract the inner runner's hooks via a type assertion on the concrete
		// type.  This avoids exposing the slice directly on the Service
		// interface.
		if impl, ok := svc.(*serviceImpl); ok {
			impl.mu.RLock()
			defer impl.mu.RUnlock()
			cfgs := make([]HookConfig, len(impl.runner.hooks))
			for i, h := range impl.runner.hooks {
				cfgs[i] = h.cfg
			}
			return cfgs, nil
		}
		return nil, nil
	}

	return newService(newHookRunner(valid), loader), nil
}

// New constructs a Service directly from a slice of HookConfig values.
// This is the primary constructor for programmatic usage (e.g. from the app
// config struct after it has been unmarshalled elsewhere).
func New(configs []HookConfig) Service {
	valid := make([]HookConfig, 0, len(configs))
	for _, h := range configs {
		if h.Command == "" || h.Event == "" || !ValidEvent(h.Event) {
			continue
		}
		valid = append(valid, h)
	}
	return newService(newHookRunner(valid), nil)
}
