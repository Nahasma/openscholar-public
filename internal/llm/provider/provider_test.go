package provider

import (
	"context"
	"testing"
)

func TestIsPromptTooLong(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "openai max context", err: testStatusError{code: 400, msg: "This model's maximum context length is 8192 tokens."}, want: true},
		{name: "anthropic prompt too long", err: testStatusError{code: 400, msg: "prompt is too long: 210000 tokens > 200000"}, want: true},
		{name: "generic context length exceeded", err: testStatusError{code: 400, msg: "context length exceeded"}, want: true},
		{name: "other bad request", err: testStatusError{code: 400, msg: "invalid api key"}, want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPromptTooLong(tc.err); got != tc.want {
				t.Fatalf("IsPromptTooLong() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveSystemPrompt_Order(t *testing.T) {
	options := providerClientOptions{
		systemMessage: "fallback",
		SystemBlocks:  []SystemBlock{{Text: "fallback-block"}},
		SystemPromptSource: func() SystemPrompt {
			return SystemPrompt{Message: "source", Blocks: []SystemBlock{{Text: "source-block"}}}
		},
	}
	requested := SystemPrompt{Message: "request", Blocks: []SystemBlock{{Text: "request-block"}}}
	ctx := WithRequestSystemPrompt(context.Background(), requested)

	if got := resolveSystemPrompt(ctx, options); got.Message != "request" {
		t.Fatalf("request prompt should win, got %q", got.Message)
	}

	noRequest := resolveSystemPrompt(context.Background(), options)
	if noRequest.Message != "source" {
		t.Fatalf("source prompt should be second priority, got %q", noRequest.Message)
	}

	options.SystemPromptSource = nil
	fallback := resolveSystemPrompt(context.Background(), options)
	if fallback.Message != "fallback" {
		t.Fatalf("fallback prompt should be last priority, got %q", fallback.Message)
	}
}
