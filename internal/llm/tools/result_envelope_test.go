package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompactToolResponsePreservesLocalToolOutput(t *testing.T) {
	resp := NewTextResponse("line 1\nline 2\nline 3")
	got := CompactToolResponse("View", resp)
	if got.Content != resp.Content {
		t.Fatalf("local tool output should stay unmodified, got %q", got.Content)
	}
}

func TestCompactToolResponseWebSearchIncludesSources(t *testing.T) {
	meta := map[string]any{
		"provider":      "brave",
		"source":        "brave",
		"progress_kind": "search_page",
		"sources": []map[string]string{
			{"title": "Paper", "url": "https://example.com/paper", "snippet": "short abstract"},
		},
		"public_summary": "compact web summary",
	}
	resp := WithResponseMetadata(NewTextResponse("WebSearch found 1 results."), meta)
	got := CompactToolResponse("WebSearch", resp)

	var env ResultEnvelope
	if err := json.Unmarshal([]byte(got.Content), &env); err != nil {
		t.Fatalf("compact response should be JSON: %v", err)
	}
	if env.Status != "ok" || env.Provider != "brave" || env.ProgressKind != "search_page" {
		t.Fatalf("unexpected envelope metadata: %#v", env)
	}
	if len(env.Sources) != 1 || env.Sources[0]["url"] != "https://example.com/paper" {
		t.Fatalf("expected compact sources in envelope, got %#v", env.Sources)
	}
	if env.Sources[0]["snippet"] != "short abstract" {
		t.Fatalf("expected snippet in envelope sources, got %#v", env.Sources[0])
	}
	if env.PublicSummary != "compact web summary" {
		t.Fatalf("expected metadata public summary, got %q", env.PublicSummary)
	}
}

func TestCompactToolResponseWebFetchUsesPublicSummary(t *testing.T) {
	longSummary := strings.Repeat("summary ", 200)
	resp := WithResponseMetadata(NewTextResponse("Fetched https://example.com."), map[string]any{
		"provider":       "http",
		"source":         "https://example.com",
		"progress_kind":  "fetched_page",
		"public_summary": longSummary,
	})
	got := CompactToolResponse("WebFetch", resp)

	var env ResultEnvelope
	if err := json.Unmarshal([]byte(got.Content), &env); err != nil {
		t.Fatalf("compact response should be JSON: %v", err)
	}
	if !strings.Contains(env.PublicSummary, "summary") {
		t.Fatalf("expected public summary in envelope, got %#v", env)
	}
	if len([]rune(env.PublicSummary)) > 800 {
		t.Fatalf("expected capped summary, got %d runes", len([]rune(env.PublicSummary)))
	}
}

func TestCompactToolResponseIncludesExpandedMetadataFields(t *testing.T) {
	resp := WithResponseMetadata(NewTextResponse("ok"), map[string]any{
		"target_key":       "webfetch:https://example.com/paper/1",
		"canonical_url":    "https://example.com/paper/1",
		"content_class":    "paper_page",
		"prompt_class":     "extract",
		"query_key":        "search:web:test",
		"evidence_keys":    []string{"url:https://example.com/paper/1"},
		"outcome_hash":     "sha256:abc",
		"low_value_reason": "",
		"candidate_count":  3,
		"hit_count":        5,
	})
	got := CompactToolResponse("WebFetch", resp)
	var env ResultEnvelope
	if err := json.Unmarshal([]byte(got.Content), &env); err != nil {
		t.Fatalf("compact response should be JSON: %v", err)
	}
	if env.TargetKey == "" || env.CanonicalURL == "" || env.ContentClass == "" || env.QueryKey == "" || env.OutcomeHash == "" {
		t.Fatalf("expected expanded metadata fields in envelope: %#v", env)
	}
	if env.CandidateCount != 3 || env.HitCount != 5 || len(env.EvidenceKeys) != 1 {
		t.Fatalf("expected count/evidence fields in envelope: %#v", env)
	}
}

func TestCompactToolResponsePreservesAttemptsAndZeroValues(t *testing.T) {
	resp := WithResponseMetadata(NewTextErrorResponse("failed"), map[string]any{
		"progress_kind":    "none",
		"durable_progress": false,
		"candidate_count":  0,
		"hit_count":        0,
		"attempts": []map[string]any{
			{"name": "duckduckgo", "success": false},
		},
	})
	got := CompactToolResponse("WebSearch", resp)
	if !strings.Contains(got.Content, `"durable_progress":false`) {
		t.Fatalf("expected explicit false durable_progress, got %s", got.Content)
	}
	if !strings.Contains(got.Content, `"candidate_count":0`) || !strings.Contains(got.Content, `"hit_count":0`) {
		t.Fatalf("expected explicit zero counts, got %s", got.Content)
	}
	if !strings.Contains(got.Content, `"attempts"`) {
		t.Fatalf("expected attempts in compact envelope, got %s", got.Content)
	}
}

func TestCompactToolResponseKBSearchIncludesAnswerAndSources(t *testing.T) {
	resp := WithResponseMetadata(NewTextResponse("raw kb output"), map[string]any{
		"provider":         "kb",
		"source":           "kb",
		"progress_kind":    "kb_answer",
		"durable_progress": true,
		"public_summary":   "answer text",
		"sources": []map[string]string{
			{"title": "Paper A", "url": "kb://paper-a#node-1", "snippet": "pp. 1-2"},
		},
	})
	got := CompactToolResponse("KBSearch", resp)

	var env ResultEnvelope
	if err := json.Unmarshal([]byte(got.Content), &env); err != nil {
		t.Fatalf("compact response should be JSON: %v", err)
	}
	if env.Tool != "KBSearch" || env.PublicSummary != "answer text" || env.ProgressKind != "kb_answer" || !env.DurableProgress {
		t.Fatalf("unexpected KB envelope: %#v", env)
	}
	if len(env.Sources) != 1 || env.Sources[0]["url"] != "kb://paper-a#node-1" {
		t.Fatalf("expected KB sources in envelope, got %#v", env.Sources)
	}
}
