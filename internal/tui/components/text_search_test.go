package components

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/message"
)

func TestFindMatches_Basic(t *testing.T) {
	messages := []message.Message{
		{Parts: []message.ContentPart{message.TextContent{Text: "hello world"}}},
		{Parts: []message.ContentPart{message.TextContent{Text: "hello again"}}},
	}

	matches := FindMatches(messages, "hello")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].MessageIdx != 0 || matches[1].MessageIdx != 1 {
		t.Error("unexpected message indices")
	}
}

func TestFindMatches_CaseInsensitive(t *testing.T) {
	messages := []message.Message{
		{Parts: []message.ContentPart{message.TextContent{Text: "Hello World"}}},
	}

	matches := FindMatches(messages, "hello")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFindMatches_MultipleInSameMessage(t *testing.T) {
	messages := []message.Message{
		{Parts: []message.ContentPart{message.TextContent{Text: "foo bar foo baz foo"}}},
	}

	matches := FindMatches(messages, "foo")
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
}

func TestFindMatches_EmptyQuery(t *testing.T) {
	messages := []message.Message{
		{Parts: []message.ContentPart{message.TextContent{Text: "hello"}}},
	}

	matches := FindMatches(messages, "")
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches for empty query, got %d", len(matches))
	}
}

func TestFindMatches_ToolResult(t *testing.T) {
	messages := []message.Message{
		{Parts: []message.ContentPart{message.ToolResult{Content: "error: file not found"}}},
	}

	matches := FindMatches(messages, "error")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match in tool result, got %d", len(matches))
	}
}

func TestFindMatches_NoMatch(t *testing.T) {
	messages := []message.Message{
		{Parts: []message.ContentPart{message.TextContent{Text: "hello world"}}},
	}

	matches := FindMatches(messages, "xyz")
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestTextSearch_Visibility(t *testing.T) {
	m := NewTextSearch()
	if m.Visible() {
		t.Fatal("expected hidden initially")
	}
	m.Show()
	if !m.Visible() {
		t.Fatal("expected visible after Show")
	}
	m.Hide()
	if m.Visible() {
		t.Fatal("expected hidden after Hide")
	}
}

func TestFindMatches_Preview(t *testing.T) {
	text := "This is a long text with some keyword inside it that we want to search for"
	messages := []message.Message{
		{Parts: []message.ContentPart{message.TextContent{Text: text}}},
	}

	matches := FindMatches(messages, "keyword")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Preview == "" {
		t.Error("expected non-empty preview")
	}
}

func TestTextSearch_SetWidthClampsInputWidth(t *testing.T) {
	m := NewTextSearch()
	m.SetWidth(2)
	if m.input.Width != 1 {
		t.Fatalf("input width = %d, want 1", m.input.Width)
	}
}
