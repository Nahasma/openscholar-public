package provider

import (
	"context"
	"reflect"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestAnthropicBuildSystemBlocks_DynamicBlockNotCached(t *testing.T) {
	client := &anthropicClient{providerOptions: providerClientOptions{
		systemMessage: "fallback",
		SystemBlocks: []SystemBlock{
			{Text: "static", IsDynamic: false},
			{Text: "dynamic", IsDynamic: true},
		},
	}}

	blocks := client.buildSystemBlocks(context.Background())
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if reflect.DeepEqual(blocks[0].CacheControl, anthropic.CacheControlEphemeralParam{}) {
		t.Fatal("expected static block cache_control")
	}
	if !reflect.DeepEqual(blocks[1].CacheControl, anthropic.CacheControlEphemeralParam{}) {
		t.Fatal("expected dynamic block without cache_control")
	}
}

func TestAnthropicBuildSystemBlocks_RequestPromptOverridesSourceAndFallback(t *testing.T) {
	client := &anthropicClient{providerOptions: providerClientOptions{
		systemMessage: "fallback",
		SystemPromptSource: func() SystemPrompt {
			return SystemPrompt{Message: "source"}
		},
	}}
	ctx := WithRequestSystemPrompt(context.Background(), SystemPrompt{Message: "request"})
	blocks := client.buildSystemBlocks(ctx)
	if len(blocks) != 1 {
		t.Fatalf("expected one system block, got %d", len(blocks))
	}
	if blocks[0].Text != "request" {
		t.Fatalf("expected request message, got %q", blocks[0].Text)
	}
}
