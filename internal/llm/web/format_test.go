package web

import (
	"strings"
	"testing"
)

func TestHTMLToMarkdown(t *testing.T) {
	html := `<h1>Hello</h1><p>World <strong>bold</strong></p>`
	md, err := HTMLToMarkdown(html)
	if err != nil {
		t.Fatalf("HTMLToMarkdown error: %v", err)
	}
	if !strings.Contains(md, "Hello") {
		t.Errorf("expected markdown to contain 'Hello', got: %s", md)
	}
	if !strings.Contains(md, "**bold**") {
		t.Errorf("expected markdown to contain '**bold**', got: %s", md)
	}
}

func TestTruncateContent(t *testing.T) {
	short := "hello"
	if got := TruncateContent(short, 100); got != short {
		t.Errorf("short content should not be truncated, got: %s", got)
	}

	long := strings.Repeat("x", 200)
	got := TruncateContent(long, 100)
	if len(got) <= 100 {
		t.Error("truncated content should include the truncation notice")
	}
	if !strings.Contains(got, "[Content truncated") {
		t.Error("expected truncation notice")
	}
	// First 100 chars should be preserved
	if got[:100] != long[:100] {
		t.Error("first 100 chars should be preserved")
	}
}

func TestFormatSearchOutput(t *testing.T) {
	result := &SearchResult{
		Hits: []SearchHit{
			{Title: "Go Docs", URL: "https://go.dev"},
			{Title: "Rust Book", URL: "https://doc.rust-lang.org"},
		},
		Summary:  "Go and Rust are popular languages.",
		Duration: 1.5,
		Backend:  "anthropic",
	}

	output := FormatSearchOutput("golang vs rust", result)

	if !strings.Contains(output, `"golang vs rust"`) {
		t.Error("expected query in output")
	}
	if !strings.Contains(output, "Go and Rust") {
		t.Error("expected summary in output")
	}
	if !strings.Contains(output, "go.dev") {
		t.Error("expected URL in output")
	}
	if !strings.Contains(output, "REMINDER") {
		t.Error("expected REMINDER in output")
	}
}

func TestFormatSearchOutput_NoResults(t *testing.T) {
	result := &SearchResult{Hits: nil, Summary: "No results found."}
	output := FormatSearchOutput("nonexistent query", result)

	if !strings.Contains(output, "No links found") {
		t.Error("expected 'No links found' for empty hits")
	}
}

func TestMakeSecondaryModelPrompt_Preapproved(t *testing.T) {
	prompt := MakeSecondaryModelPrompt("# Hello", "summarize this", true)
	if !strings.Contains(prompt, "code examples") {
		t.Error("preapproved should allow code examples")
	}
	if strings.Contains(prompt, "125-character") {
		t.Error("preapproved should NOT have quote length restriction")
	}
}

func TestMakeSecondaryModelPrompt_NonPreapproved(t *testing.T) {
	prompt := MakeSecondaryModelPrompt("# Hello", "summarize this", false)
	if !strings.Contains(prompt, "125-character") {
		t.Error("non-preapproved should enforce 125-char quote limit")
	}
}
