package tools

import (
	"context"
	"testing"
)

func TestResearchSessionIDUsesCurrentSessionByDefault(t *testing.T) {
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-current")
	if got := ResearchSessionID(ctx); got != "sess-current" {
		t.Fatalf("ResearchSessionID = %q, want %q", got, "sess-current")
	}
}

func TestResearchSessionIDPrefersRootSessionContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-child")
	ctx = context.WithValue(ctx, ResearchRootSessionContextKey, "sess-parent")
	if got := ResearchSessionID(ctx); got != "sess-parent" {
		t.Fatalf("ResearchSessionID = %q, want %q", got, "sess-parent")
	}
}
