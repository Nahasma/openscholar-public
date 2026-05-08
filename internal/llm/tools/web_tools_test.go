package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/llm/web"
)

// TestWebSearchTool_Registration verifies the tool registers and reports availability.
func TestWebSearchTool_Registration(t *testing.T) {
	// Without provider → unavailable
	rt := web.NewRuntime(nil, nil, web.DefaultConfig())
	tool := NewWebSearchTool(nil, rt)

	info := tool.Info()
	if info.Name != "WebSearch" {
		t.Errorf("name = %q, want WebSearch", info.Name)
	}

	if checker, ok := tool.(AvailabilityChecker); ok {
		avail, reason := checker.Available()
		if avail {
			t.Error("should be unavailable without search provider")
		}
		t.Logf("Unavailable reason: %s", reason)
	}

	// With mock provider → available
	rt2 := web.NewRuntime(&mockSearch{}, nil, web.DefaultConfig())
	tool2 := NewWebSearchTool(nil, rt2)
	if checker, ok := tool2.(AvailabilityChecker); ok {
		avail, _ := checker.Available()
		if !avail {
			t.Error("should be available with search provider")
		}
	}
}

// TestWebSearchTool_Run verifies the tool executes correctly with a mock provider.
func TestWebSearchTool_Run(t *testing.T) {
	mock := &mockSearch{
		result: &web.SearchResult{
			Hits:    []web.SearchHit{{Title: "Test", URL: "https://1.1.1.1/test"}},
			Summary: "Test result",
			Backend: "mock",
		},
	}
	rt := web.NewRuntime(mock, nil, web.DefaultConfig())
	tool := NewWebSearchTool(nil, rt) // nil perms = skip permission check

	input, _ := json.Marshal(map[string]string{"query": "test query"})
	resp, err := tool.Run(context.Background(), ToolCall{
		ID:    "test-1",
		Name:  "WebSearch",
		Input: string(input),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("tool error: %s", resp.Content)
	}

	t.Logf("WebSearch output:\n%s", resp.Content)
	t.Logf("WebSearch metadata: %s", resp.Metadata)

	if resp.Content == "" {
		t.Error("expected non-empty content")
	}
	if resp.Metadata == "" {
		t.Error("expected non-empty metadata")
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &meta); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if meta["provider"] != "mock" || meta["source"] != "mock" {
		t.Fatalf("expected provider/source=mock, got provider=%v source=%v", meta["provider"], meta["source"])
	}
	if _, ok := meta["public_summary"]; !ok {
		t.Fatal("expected public_summary in metadata")
	}
	if meta["query_key"] == "" {
		t.Fatal("expected query_key in metadata")
	}
	if _, ok := meta["outcome_hash"]; !ok {
		t.Fatal("expected outcome_hash in metadata")
	}
	if _, ok := meta["evidence_keys"]; !ok {
		t.Fatal("expected evidence_keys in metadata")
	}
	if arr, ok := meta["sources"].([]any); !ok || len(arr) == 0 {
		t.Fatalf("expected non-empty sources, got %#v", meta["sources"])
	}
}

func TestWebSearchTool_RunFailureIncludesAttemptsMetadata(t *testing.T) {
	mock := &mockSearch{
		err: &web.SearchFailureError{
			Message: "all search backends failed",
			Attempts: []web.ProviderAttempt{
				{Name: "duckduckgo", Success: false, Error: "temporary"},
				{Name: "openai", Success: false, Error: "skipped: quota exhausted"},
			},
			Cause: fmt.Errorf("timeout"),
		},
	}
	rt := web.NewRuntime(mock, nil, web.DefaultConfig())
	tool := NewWebSearchTool(nil, rt)

	input, _ := json.Marshal(map[string]string{"query": "latest gopher news"})
	resp, err := tool.Run(context.Background(), ToolCall{Input: string(input)})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected error response, got success: %s", resp.Content)
	}

	var meta map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &meta); err != nil {
		t.Fatalf("invalid metadata JSON: %v", err)
	}
	if meta["query"] != "latest gopher news" {
		t.Fatalf("unexpected query metadata: %v", meta["query"])
	}
	if meta["query_key"] == "" {
		t.Fatalf("expected query_key metadata, got: %v", meta)
	}
	if _, ok := meta["attempts"]; !ok {
		t.Fatal("expected attempts key in metadata")
	}
}

func TestWebSearchTool_RunFailureIncludesPolicyMetadata(t *testing.T) {
	mock := &mockSearch{
		err: &web.PolicyError{
			Message: "blocked",
			Metadata: web.PolicyMetadata{
				ProxyMode:          "always",
				ProxyUsed:          true,
				ProxyEndpointClass: "public",
				ResolvedIPClass:    "private",
				PolicyDecision:     "blocked_target_dns_resolution",
				FakeIPAllowed:      false,
			},
		},
	}
	rt := web.NewRuntime(mock, nil, web.DefaultConfig())
	tool := NewWebSearchTool(nil, rt)

	input, _ := json.Marshal(map[string]string{"query": "test"})
	resp, err := tool.Run(context.Background(), ToolCall{Input: string(input)})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !resp.IsError {
		t.Fatal("expected error response")
	}

	var meta map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &meta); err != nil {
		t.Fatalf("invalid metadata JSON: %v", err)
	}
	if meta["policy_decision"] != "blocked_target_dns_resolution" {
		t.Fatalf("unexpected policy_decision: %v", meta["policy_decision"])
	}
	if _, ok := meta["attempts"]; !ok {
		t.Fatal("expected attempts key in metadata")
	}
}

// TestWebSearchTool_Validation tests input validation.
func TestWebSearchTool_Validation(t *testing.T) {
	rt := web.NewRuntime(&mockSearch{}, nil, web.DefaultConfig())
	tool := NewWebSearchTool(nil, rt)

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
	}{
		{"empty query", map[string]any{"query": ""}, true},
		{"both filters", map[string]any{"query": "test", "allowed_domains": []string{"a.com"}, "blocked_domains": []string{"b.com"}}, true},
		{"valid", map[string]any{"query": "test"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := json.Marshal(tt.input)
			resp, _ := tool.Run(context.Background(), ToolCall{Input: string(input)})
			if resp.IsError != tt.wantErr {
				t.Errorf("IsError = %v, want %v; content: %s", resp.IsError, tt.wantErr, resp.Content)
			}
		})
	}
}

// TestWebFetchTool_Run verifies WebFetch with cached content.
func TestWebFetchTool_Run(t *testing.T) {
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return "Summary of the page", nil
	}
	rt := web.NewRuntime(nil, mockLLM, web.DefaultConfig())

	// Pre-populate cache so we don't hit the network
	testURL := "https://1.1.1.1/doc/effective_go"
	rt.GetCache().Set(web.CanonicalCacheURL(testURL), &web.CacheEntry{
		Content:     "# Effective Go\n\nGo programming guide.",
		ContentType: "text/html",
		Bytes:       100,
		Code:        200,
		CodeText:    "OK",
	})

	tool := NewWebFetchTool(nil, rt)

	info := tool.Info()
	if info.Name != "WebFetch" {
		t.Errorf("name = %q, want WebFetch", info.Name)
	}

	input, _ := json.Marshal(map[string]string{
		"url":    testURL,
		"prompt": "summarize the key points",
	})
	resp, err := tool.Run(context.Background(), ToolCall{
		ID:    "test-2",
		Name:  "WebFetch",
		Input: string(input),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("tool error: %s", resp.Content)
	}

	t.Logf("WebFetch output: %s", resp.Content)
	t.Logf("WebFetch metadata: %s", resp.Metadata)

	if resp.Content == "" {
		t.Error("expected non-empty content")
	}

	// Verify metadata
	var meta map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &meta); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if meta["cache_hit"] != true {
		t.Error("expected cache_hit=true in metadata")
	}
	if meta["target_key"] == "" || meta["canonical_url"] == "" {
		t.Fatalf("expected target_key/canonical_url metadata, got: %v", meta)
	}
	if meta["content_class"] != "doc_page" {
		t.Fatalf("expected doc_page content_class, got: %v", meta["content_class"])
	}
	if meta["durable_progress"] != true {
		t.Fatalf("expected durable_progress=true for doc_page, got: %v", meta["durable_progress"])
	}
}

func TestWebFetchTool_ListPageMarkedLowValueNonDurable(t *testing.T) {
	rt := web.NewRuntime(nil, func(ctx context.Context, prompt string) (string, error) { return "summary", nil }, web.DefaultConfig())
	testURL := "https://papers.nips.cc/paper_files/paper/2024"
	rt.GetCache().Set(web.CanonicalCacheURL(testURL), &web.CacheEntry{
		Content:     strings.Repeat("[x](https://example.com)\n", 40),
		ContentType: "text/html",
		Bytes:       3 * 1024 * 1024,
		Code:        200,
		CodeText:    "OK",
	})
	tool := NewWebFetchTool(nil, rt)
	input, _ := json.Marshal(map[string]string{"url": testURL, "prompt": "continue from paper 9"})
	resp, err := tool.Run(context.Background(), ToolCall{Input: string(input)})
	if err != nil || resp.IsError {
		t.Fatalf("run failed: err=%v content=%s", err, resp.Content)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &meta); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if meta["content_class"] != "list_page" {
		t.Fatalf("expected list_page, got %v", meta["content_class"])
	}
	if meta["durable_progress"] != false {
		t.Fatalf("expected durable_progress=false for list_page, got %v", meta["durable_progress"])
	}
	if meta["prompt_class"] == "" || meta["outcome_hash"] == "" || meta["evidence_keys"] == nil {
		t.Fatalf("expected prompt_class/outcome_hash/evidence_keys, got %v", meta)
	}
}

func TestWebFetchTool_NeurIPSPaperPageMarkedDurable(t *testing.T) {
	rt := web.NewRuntime(nil, func(ctx context.Context, prompt string) (string, error) { return "summary", nil }, web.DefaultConfig())
	testURL := "https://papers.nips.cc/paper_files/paper/2024/hash-Abstract-Conference.html"
	rt.GetCache().Set(web.CanonicalCacheURL(testURL), &web.CacheEntry{
		Content:     "# Paper\n\nAbstract.",
		ContentType: "text/html",
		Bytes:       512,
		Code:        200,
		CodeText:    "OK",
	})
	tool := NewWebFetchTool(nil, rt)
	input, _ := json.Marshal(map[string]string{"url": testURL, "prompt": "extract abstract"})
	resp, err := tool.Run(context.Background(), ToolCall{Input: string(input)})
	if err != nil || resp.IsError {
		t.Fatalf("run failed: err=%v content=%s", err, resp.Content)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &meta); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if meta["content_class"] != "paper_page" || meta["durable_progress"] != true {
		t.Fatalf("expected durable paper_page metadata, got %v", meta)
	}
}

// TestWebFetchTool_InvalidURL verifies URL validation.
func TestWebFetchTool_InvalidURL(t *testing.T) {
	rt := web.NewRuntime(nil, nil, web.DefaultConfig())
	tool := NewWebFetchTool(nil, rt)

	input, _ := json.Marshal(map[string]string{
		"url":    "not-a-url",
		"prompt": "test",
	})
	resp, _ := tool.Run(context.Background(), ToolCall{Input: string(input)})
	if !resp.IsError {
		t.Error("expected error for invalid URL")
	}
	t.Logf("Error: %s", resp.Content)
}

func TestWebFetchTool_BlocksSecretURL(t *testing.T) {
	rt := web.NewRuntime(nil, nil, web.DefaultConfig())
	tool := NewWebFetchTool(nil, rt)

	input, _ := json.Marshal(map[string]string{
		"url":    "https://1.1.1.1/path?token=super-secret",
		"prompt": "test",
	})
	resp, _ := tool.Run(context.Background(), ToolCall{Input: string(input)})
	if !resp.IsError {
		t.Fatal("expected secret URL to be blocked")
	}
	if resp.Content == "" || resp.Content == "https://1.1.1.1/path?token=super-secret" {
		t.Fatalf("unexpected response content: %s", resp.Content)
	}
}

func TestWebFetchTool_ValidationFailedMetadata(t *testing.T) {
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return "<function_call>{}</function_call>", nil
	}
	rt := web.NewRuntime(nil, mockLLM, web.DefaultConfig())
	testURL := "https://1.1.1.1/doc"
	rt.GetCache().Set(web.CanonicalCacheURL(testURL), &web.CacheEntry{
		Content:     "content",
		ContentType: "text/html",
		Bytes:       10,
		Code:        200,
		CodeText:    "OK",
	})
	tool := NewWebFetchTool(nil, rt)
	input, _ := json.Marshal(map[string]string{"url": testURL, "prompt": "summarize"})
	resp, err := tool.Run(context.Background(), ToolCall{Input: string(input)})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected error response, got: %s", resp.Content)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &meta); err != nil {
		t.Fatalf("invalid metadata JSON: %v", err)
	}
	if meta["error_kind"] != string(web.FetchErrValidation) {
		t.Fatalf("error_kind = %v, want %s", meta["error_kind"], web.FetchErrValidation)
	}
}

// TestDeferredRegistry_WebTools verifies web tools appear in the deferred registry.
func TestDeferredRegistry_WebTools(t *testing.T) {
	mock := &mockSearch{}
	mockLLM := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	rt := web.NewRuntime(mock, mockLLM, web.DefaultConfig())

	webTools := registerWebTools(ToolDeps{WebRuntime: rt})

	names := make([]string, len(webTools))
	for i, tool := range webTools {
		names[i] = tool.Info().Name
	}
	t.Logf("Registered web tools: %v", names)

	if len(webTools) != 2 {
		t.Fatalf("expected 2 web tools, got %d: %v", len(webTools), names)
	}

	foundSearch, foundFetch := false, false
	for _, tool := range webTools {
		switch tool.Info().Name {
		case "WebSearch":
			foundSearch = true
		case "WebFetch":
			foundFetch = true
		}
	}
	if !foundSearch {
		t.Error("WebSearch not registered")
	}
	if !foundFetch {
		t.Error("WebFetch not registered")
	}
}

// TestDeferredRegistry_WebSearchAvailabilityFollowsRuntime verifies WebSearch stays
// registered while availability follows runtime provider reloads.
func TestDeferredRegistry_WebSearchAvailabilityFollowsRuntime(t *testing.T) {
	rt := web.NewRuntime(nil, nil, web.DefaultConfig())
	webTools := registerWebTools(ToolDeps{WebRuntime: rt})

	var searchTool AvailabilityChecker
	for _, tool := range webTools {
		if tool.Info().Name == "WebSearch" {
			var ok bool
			searchTool, ok = tool.(AvailabilityChecker)
			if !ok {
				t.Fatal("WebSearch should implement AvailabilityChecker")
			}
		}
	}
	if searchTool == nil {
		t.Fatal("WebSearch should be registered even without initial search provider")
	}
	if len(webTools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(webTools))
	}
	if available, _ := searchTool.Available(); available {
		t.Fatal("WebSearch should be unavailable before a search provider is set")
	}
	rt.SetSearchProvider(&mockSearch{})
	if available, reason := searchTool.Available(); !available {
		t.Fatalf("WebSearch should become available after provider reload: %s", reason)
	}
}

// --- mock ---

type mockSearch struct {
	result *web.SearchResult
	err    error
}

func (m *mockSearch) SearchWeb(ctx context.Context, query string, opts web.SearchOptions) (*web.SearchResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(opts.AllowedDomains) > 0 || len(opts.BlockedDomains) > 0 {
		return nil, fmt.Errorf("mock does not support filters")
	}
	if m.result != nil {
		return m.result, nil
	}
	return &web.SearchResult{Summary: "mock: " + query, Backend: "mock"}, nil
}

func (m *mockSearch) SupportsFilter() bool { return false }
