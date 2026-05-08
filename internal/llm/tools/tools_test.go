package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTextResponse(t *testing.T) {
	resp := NewTextResponse("hello")
	assert.Equal(t, ToolResponseTypeText, resp.Type)
	assert.Equal(t, "hello", resp.Content)
	assert.False(t, resp.IsError)
}

func TestNewTextErrorResponse(t *testing.T) {
	resp := NewTextErrorResponse("error occurred")
	assert.Equal(t, ToolResponseTypeText, resp.Type)
	assert.Equal(t, "error occurred", resp.Content)
	assert.True(t, resp.IsError)
}

func TestWithResponseMetadata(t *testing.T) {
	resp := NewTextResponse("content")
	metadata := map[string]string{"key": "value"}
	enriched := WithResponseMetadata(resp, metadata)
	assert.Contains(t, enriched.Metadata, `"key":"value"`)
}

func TestWithResponseMetadata_Nil(t *testing.T) {
	resp := NewTextResponse("content")
	enriched := WithResponseMetadata(resp, nil)
	assert.Equal(t, "", enriched.Metadata)
}

func TestValidateArtifactFilenameRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../outside", "figures/outside", `figures\outside`, "/tmp/outside", ".", "..", "bad*name"} {
		if err := validateArtifactFilename(name); err == nil {
			t.Fatalf("expected invalid artifact filename %q", name)
		}
	}
	if err := validateArtifactFilename("figure_1"); err != nil {
		t.Fatalf("expected valid artifact filename: %v", err)
	}
}

func TestIsResearchMode(t *testing.T) {
	ctx := context.Background()
	assert.False(t, IsResearchMode(ctx))

	ctx = context.WithValue(ctx, ResearchModeContextKey, true)
	assert.True(t, IsResearchMode(ctx))

	ctx = context.WithValue(ctx, ResearchModeContextKey, false)
	assert.False(t, IsResearchMode(ctx))
}

func TestGetContextValues(t *testing.T) {
	ctx := context.Background()
	sid, mid := GetContextValues(ctx)
	assert.Equal(t, "", sid)
	assert.Equal(t, "", mid)

	ctx = context.WithValue(ctx, SessionIDContextKey, "sess-123")
	sid, mid = GetContextValues(ctx)
	assert.Equal(t, "sess-123", sid)
	assert.Equal(t, "", mid)

	ctx = context.WithValue(ctx, MessageIDContextKey, "msg-456")
	sid, mid = GetContextValues(ctx)
	assert.Equal(t, "sess-123", sid)
	assert.Equal(t, "msg-456", mid)
}
