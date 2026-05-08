package tui

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/message"
)

func TestUpdateMessages_ReplacesAssistantStreamUpdatesByID(t *testing.T) {
	created := message.Message{ID: "assistant-1", Role: message.Assistant}
	messages := updateMessages(nil, created)

	messages = updateMessages(messages, message.Message{
		ID:   "assistant-1",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "好的"},
		},
	})
	messages = updateMessages(messages, message.Message{
		ID:   "assistant-1",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "好的，用 WebSearch 来搜。"},
		},
	})

	if len(messages) != 1 {
		t.Fatalf("same assistant stream ID should replace in place, got %d messages", len(messages))
	}
	if got := messages[0].Content().Text; got != "好的，用 WebSearch 来搜。" {
		t.Fatalf("same-ID update kept content %q", got)
	}
}

func TestUpdateMessages_AppendsDifferentIDs(t *testing.T) {
	messages := updateMessages(nil, message.Message{ID: "assistant-1", Role: message.Assistant})
	messages = updateMessages(messages, message.Message{ID: "assistant-2", Role: message.Assistant})

	if len(messages) != 2 {
		t.Fatalf("different assistant IDs should append, got %d messages", len(messages))
	}
}
