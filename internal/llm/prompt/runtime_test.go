package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/config"
)

func TestFormatRuntimeClockContext_FullAgents(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2026, 5, 7, 14, 32, 45, 123, loc)
	got := FormatRuntimeClockContext(config.AgentCoder, now)

	for _, want := range []string{
		"# Runtime Context",
		"Request local time: 2026-05-07 14:32 UTC+08:00 (Asia/Shanghai)",
		"Request UTC time: 2026-05-07T06:32Z",
		"WebSearch/WebFetch",
		"ScholarSearch",
		"not evidence of current facts",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected runtime context to contain %q, got: %s", want, got)
		}
	}
	if strings.Contains(got, "14:32:45") {
		t.Fatalf("runtime context must not contain second precision: %s", got)
	}
}

func TestFormatRuntimeClockContext_Summarizer(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2026, 5, 7, 14, 32, 45, 0, loc)
	got := FormatRuntimeClockContext(config.AgentSummarizer, now)
	if !strings.Contains(got, "Summary generated at: 2026-05-07 14:32 UTC+08:00 (Asia/Shanghai)") {
		t.Fatalf("expected summarizer generated-at context, got: %s", got)
	}
	if strings.Contains(got, "WebSearch/WebFetch") || strings.Contains(got, "ScholarSearch") {
		t.Fatalf("summarizer context should not force tool usage rules, got: %s", got)
	}
}

func TestBuildAgentPromptRuntime_TitleHasNoRuntimeContext(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 32, 45, 0, time.UTC)
	rt := BuildAgentPromptRuntime(config.AgentTitle, now)
	if strings.Contains(rt.SystemMessage, "# Runtime Context") {
		t.Fatalf("title prompt should not include runtime context: %s", rt.SystemMessage)
	}
}

func TestBuildAgentPromptRuntime_MessageMatchesBlocks(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2026, 5, 7, 14, 32, 45, 0, loc)
	rt := BuildAgentPromptRuntime(config.AgentGeneral, now)
	parts := make([]string, 0, len(rt.Blocks))
	for _, b := range rt.Blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	if got, want := rt.SystemMessage, strings.Join(parts, "\n\n"); got != want {
		t.Fatalf("system message should be joined blocks\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestBuildAgentPromptRuntime_StaticCacheKeyStableAcrossNow(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	rt1 := BuildAgentPromptRuntime(config.AgentGeneral, time.Date(2026, 5, 7, 14, 32, 10, 0, loc))
	rt2 := BuildAgentPromptRuntime(config.AgentGeneral, time.Date(2026, 5, 7, 14, 33, 10, 0, loc))
	if len(rt1.Blocks) < 2 || len(rt2.Blocks) < 2 {
		t.Fatalf("expected static+dynamic blocks, got %d and %d", len(rt1.Blocks), len(rt2.Blocks))
	}
	if rt1.Blocks[0].CacheKey == "" || rt2.Blocks[0].CacheKey == "" {
		t.Fatal("expected first static block to have cache key")
	}
	if rt1.Blocks[0].CacheKey != rt2.Blocks[0].CacheKey {
		t.Fatalf("static cache key should not change across now: %s != %s", rt1.Blocks[0].CacheKey, rt2.Blocks[0].CacheKey)
	}
	if !rt1.Blocks[len(rt1.Blocks)-1].IsDynamic || !rt2.Blocks[len(rt2.Blocks)-1].IsDynamic {
		t.Fatal("expected runtime block to be dynamic")
	}
}
