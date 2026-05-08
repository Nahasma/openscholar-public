package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openscholar/openscholar/internal/message"
)

func TestMapOpenAIFinishReason(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		hasToolCalls bool
		want         message.FinishReason
	}{
		{name: "stop", raw: "stop", want: message.FinishReasonEndTurn},
		{name: "tool_calls", raw: "tool_calls", want: message.FinishReasonToolUse},
		{name: "length", raw: "length", want: message.FinishReasonMaxTokens},
		{name: "empty_with_tools", raw: "", hasToolCalls: true, want: message.FinishReasonToolUse},
		{name: "empty_without_tools", raw: "", want: message.FinishReasonEndTurn},
		{name: "unknown_with_tools", raw: "unexpected", hasToolCalls: true, want: message.FinishReasonToolUse},
		{name: "unknown_without_tools", raw: "unexpected", want: message.FinishReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapOpenAIFinishReason(tt.raw, tt.hasToolCalls); got != tt.want {
				t.Fatalf("mapOpenAIFinishReason(%q, %v) = %q, want %q", tt.raw, tt.hasToolCalls, got, tt.want)
			}
		})
	}
}

func TestOpenAICountTokensUnsupported(t *testing.T) {
	c := &openaiClient{}
	_, err := c.countTokens(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected unsupported error")
	}
	if !IsCountTokensUnsupported(err) {
		t.Fatalf("expected ErrCountTokensUnsupported, got: %v", err)
	}
}

func TestOpenAIUsage(t *testing.T) {
	tests := []struct {
		name         string
		rawUsage     string
		want         TokenUsage
		wantHasUsage bool
	}{
		{
			name: "with cached prompt tokens",
			rawUsage: `{
				"prompt_tokens": 120,
				"completion_tokens": 30,
				"total_tokens": 150,
				"prompt_tokens_details": {"cached_tokens": 20}
			}`,
			want: TokenUsage{
				InputTokens:     100,
				OutputTokens:    30,
				CacheReadTokens: 20,
			},
			wantHasUsage: true,
		},
		{
			name: "without prompt token details",
			rawUsage: `{
				"prompt_tokens": 120,
				"completion_tokens": 30,
				"total_tokens": 150
			}`,
			want: TokenUsage{
				InputTokens:  120,
				OutputTokens: 30,
			},
			wantHasUsage: true,
		},
		{
			name: "cached tokens capped at zero input",
			rawUsage: `{
				"prompt_tokens": 10,
				"completion_tokens": 5,
				"total_tokens": 15,
				"prompt_tokens_details": {"cached_tokens": 20}
			}`,
			want: TokenUsage{
				InputTokens:     0,
				OutputTokens:    5,
				CacheReadTokens: 20,
			},
			wantHasUsage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var usage openai.CompletionUsage
			if err := json.Unmarshal([]byte(tt.rawUsage), &usage); err != nil {
				t.Fatalf("unmarshal usage: %v", err)
			}

			got, hasUsage := openAIUsage(usage)
			if hasUsage != tt.wantHasUsage {
				t.Fatalf("openAIUsage hasUsage=%v, want %v", hasUsage, tt.wantHasUsage)
			}
			if got != tt.want {
				t.Fatalf("openAIUsage usage=%+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOpenAIUsage_NoFields(t *testing.T) {
	got, hasUsage := openAIUsage(openai.CompletionUsage{})
	if hasUsage {
		t.Fatal("expected hasUsage=false for empty usage")
	}
	if got != (TokenUsage{}) {
		t.Fatalf("expected empty usage for empty input, got %+v", got)
	}
}

func TestOpenAIIncludeUsageUnsupported(t *testing.T) {
	if !openAIIncludeUsageUnsupported(testStatusError{
		code: 400,
		msg:  "unknown parameter stream_options.include_usage",
	}) {
		t.Fatal("expected include_usage unsupported to be detected")
	}

	if openAIIncludeUsageUnsupported(testStatusError{
		code: 500,
		msg:  "server error include_usage",
	}) {
		t.Fatal("expected 500 not to be treated as include_usage unsupported")
	}
}

func TestJoinSystemBlocks(t *testing.T) {
	joined := joinSystemBlocks([]SystemBlock{
		{Text: "A"},
		{Text: ""},
		{Text: "B"},
	})
	if joined != "A\n\nB" {
		t.Fatalf("joinSystemBlocks() = %q", joined)
	}
}

type testStatusError struct {
	code int
	msg  string
}

func (e testStatusError) Error() string   { return fmt.Sprintf("%d: %s", e.code, e.msg) }
func (e testStatusError) StatusCode() int { return e.code }
