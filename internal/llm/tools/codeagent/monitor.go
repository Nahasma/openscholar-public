package codeagent

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// StreamEvent represents a parsed event from the provider's streaming JSON output.
type StreamEvent struct {
	Type      string    `json:"type"`
	Tool      string    `json:"tool,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// StreamMonitor watches a streaming JSON output for activity and tool calls.
type StreamMonitor struct {
	mu           sync.Mutex
	LastActivity time.Time
	IdleTimeout  time.Duration
	ToolCalls    []StreamEvent
	Warnings     []string
}

// NewStreamMonitor creates a StreamMonitor with the given idle timeout.
func NewStreamMonitor(idleTimeout time.Duration) *StreamMonitor {
	return &StreamMonitor{
		LastActivity: time.Now(),
		IdleTimeout:  idleTimeout,
	}
}

// ProcessStream reads newline-delimited JSON events from reader and updates the monitor state.
// Each line is expected to be a JSON object with at least a "type" field.
// This method blocks until the reader is exhausted or closed.
func (m *StreamMonitor) ProcessStream(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event StreamEvent
		if err := json.Unmarshal(line, &event); err != nil {
			// Non-JSON line: still counts as activity.
			m.mu.Lock()
			m.LastActivity = time.Now()
			m.mu.Unlock()
			continue
		}

		event.Timestamp = time.Now()

		m.mu.Lock()
		m.LastActivity = event.Timestamp
		if event.Type == "tool_use" || event.Type == "tool_call" {
			m.ToolCalls = append(m.ToolCalls, event)
		}
		m.mu.Unlock()
	}
}

// IsIdle returns true if no activity has been seen within the idle timeout window.
func (m *StreamMonitor) IsIdle() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.Since(m.LastActivity) > m.IdleTimeout
}
