package agent

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/message"
)

func TestNormalizeResumeHistoryForProvider_DropsInvalidInterruptedToolCalls(t *testing.T) {
	in := []message.Message{
		{
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.ToolCall{ID: "ok", Name: "Read", Input: `{"path":"a"}`, Finished: true},
				message.ToolCall{ID: "bad", Name: "Read", Input: `not-json`, Finished: false},
			},
		},
		{
			Role:  message.Tool,
			Parts: []message.ContentPart{message.ToolResult{ToolCallID: "ok", Name: "Read", Content: "ok"}},
		},
	}
	out := normalizeResumeHistoryForProvider(in)
	if len(out) != 2 {
		t.Fatalf("expected assistant/tool pair")
	}
	if got := len(out[0].ToolCalls()); got != 1 {
		t.Fatalf("expected one valid tool call after normalization, got %d", got)
	}
}

func TestNormalizeResumeHistoryForProvider_DropsUnresolvedToolPairs(t *testing.T) {
	in := []message.Message{
		{
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "checking"},
				message.ToolCall{ID: "missing", Name: "Read", Input: `{"path":"a"}`, Finished: true},
			},
		},
		{
			Role:  message.Tool,
			Parts: []message.ContentPart{message.ToolResult{ToolCallID: "orphan", Name: "Read", Content: "result"}},
		},
	}
	out := normalizeResumeHistoryForProvider(in)
	if len(out) != 1 {
		t.Fatalf("expected assistant text message only, got %d messages", len(out))
	}
	if got := len(out[0].ToolCalls()); got != 0 {
		t.Fatalf("expected unresolved tool call to be dropped, got %d", got)
	}
}

func TestNormalizeResumeHistoryForProvider_DropsInterleavedToolResults(t *testing.T) {
	in := []message.Message{
		{
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "checking"},
				message.ToolCall{ID: "tc-1", Name: "Read", Input: `{"path":"a"}`, Finished: true},
			},
		},
		{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: "interrupt"}},
		},
		{
			Role:  message.Tool,
			Parts: []message.ContentPart{message.ToolResult{ToolCallID: "tc-1", Name: "Read", Content: "late"}},
		},
	}
	out := normalizeResumeHistoryForProvider(in)
	if len(out) != 2 {
		t.Fatalf("expected assistant text and user message, got %d", len(out))
	}
	if got := len(out[0].ToolCalls()); got != 0 {
		t.Fatalf("expected interleaved tool call to be dropped, got %d", got)
	}
	if out[1].Role != message.User {
		t.Fatalf("expected user message to remain second, got %s", out[1].Role)
	}
}
