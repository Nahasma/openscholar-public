package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestFetch_BasicHTML(t *testing.T) {
	// Use a mock LLM that just echoes
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return "Summary: Test Page with Hello world content", nil
	}

	rt := NewRuntime(nil, mockLLM, DefaultConfig())

	// We can't use the TLS server directly with ValidateWebURL (it blocks localhost).
	// Instead test the summariseContent path directly.
	content := "# Test Page\n\nHello world"
	result, err := rt.summariseContent(context.Background(), "https://example.com/test", content, "text/html", "summarize", 100, 200, "OK")
	if err != nil {
		t.Fatalf("summariseContent error: %v", err)
	}
	if !strings.Contains(result.Content, "Summary") {
		t.Errorf("expected summary in content, got: %s", result.Content)
	}
	if result.Code != 200 {
		t.Errorf("code = %d, want 200", result.Code)
	}
}

func TestFetch_CacheHit(t *testing.T) {
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return "cached summary", nil
	}

	rt := NewRuntime(nil, mockLLM, DefaultConfig())

	// Pre-populate cache
	rt.cache.Set(CanonicalCacheURL("https://1.1.1.1/cached"), &CacheEntry{
		Content:     "# Cached Content",
		ContentType: "text/html",
		Bytes:       50,
		Code:        200,
		CodeText:    "OK",
	})

	result, redirect, err := rt.Fetch(context.Background(), "https://1.1.1.1/cached", "summarize")
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if redirect != nil {
		t.Fatal("unexpected redirect")
	}
	if !result.CacheHit {
		t.Error("expected cache hit")
	}
}

func TestFetch_RedirectSameHost(t *testing.T) {
	original := "https://example.com/old"
	redirect := "https://example.com/new"
	if !IsPermittedRedirect(original, redirect) {
		t.Error("same-host redirect should be permitted")
	}
}

func TestFetch_RedirectCrossHost(t *testing.T) {
	if IsPermittedRedirect("https://example.com/page", "https://evil.com/trap") {
		t.Error("cross-host redirect should NOT be permitted")
	}
}

func TestFetch_InvalidURL(t *testing.T) {
	rt := NewRuntime(nil, nil, DefaultConfig())
	_, _, err := rt.Fetch(context.Background(), "not-a-url", "summarize")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestFetch_PrivateIP(t *testing.T) {
	rt := NewRuntime(nil, nil, DefaultConfig())
	_, _, err := rt.Fetch(context.Background(), "https://192.168.1.1/admin", "summarize")
	if err == nil {
		t.Error("expected error for private IP")
	}
}

func TestFetchHTTPStatusErrorClassification(t *testing.T) {
	tests := []struct {
		name        string
		code        int
		wantKind    FetchErrorKind
		recoverable bool
	}{
		{name: "not found", code: http.StatusNotFound, wantKind: FetchErrNotFound},
		{name: "forbidden", code: http.StatusForbidden, wantKind: FetchErrForbidden},
		{name: "server", code: http.StatusInternalServerError, wantKind: FetchErrServer, recoverable: true},
		{name: "other", code: http.StatusTeapot, wantKind: FetchErrHTTPStatus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fetchHTTPStatusError(tt.code, http.StatusText(tt.code), "https://example.com")
			if err.Kind != tt.wantKind {
				t.Fatalf("kind = %s, want %s", err.Kind, tt.wantKind)
			}
			if err.Recoverable != tt.recoverable {
				t.Fatalf("recoverable = %v, want %v", err.Recoverable, tt.recoverable)
			}
		})
	}
}

func TestFetch_CachedHTTPErrorDoesNotCallSummarizer(t *testing.T) {
	calls := 0
	rt := NewRuntime(nil, func(ctx context.Context, prompt string) (string, error) {
		calls++
		return "should not run", nil
	}, DefaultConfig())

	testURL := "https://1.1.1.1/missing"
	rt.cache.Set(CanonicalCacheURL(testURL), &CacheEntry{
		Content:     "<html>not found</html>",
		ContentType: "text/html",
		Bytes:       22,
		Code:        http.StatusNotFound,
		CodeText:    http.StatusText(http.StatusNotFound),
	})

	_, _, err := rt.Fetch(context.Background(), testURL, "summarize")
	if err == nil {
		t.Fatal("expected cached HTTP error")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected FetchError, got %T %v", err, err)
	}
	if fetchErr.Kind != FetchErrNotFound {
		t.Fatalf("kind = %s, want %s", fetchErr.Kind, FetchErrNotFound)
	}
	if calls != 0 {
		t.Fatalf("summarizer calls = %d, want 0", calls)
	}
}

func TestClassifyFetchBodyReadErrorEOF(t *testing.T) {
	err := classifyFetchBodyReadError(io.ErrUnexpectedEOF, http.StatusOK, "https://example.com")
	if err.Kind != FetchErrEOF {
		t.Fatalf("kind = %s, want %s", err.Kind, FetchErrEOF)
	}
}

func TestFetch_FakeIPDNSAllowedWhenProxyUsed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Proxy.Mode = "explicit"
	cfg.Proxy.URL = "http://127.0.0.1:7890"
	rt := NewRuntime(nil, nil, cfg)
	rt.urlPolicy = rt.urlPolicy.withResolver(&fakeResolver{
		ips: map[string][]net.IPAddr{
			"fake.example.com": {{IP: net.ParseIP("198.18.0.10")}},
		},
		err: map[string]error{},
	})

	testURL := "https://fake.example.com/doc"
	rt.cache.Set(CanonicalCacheURL(testURL), &CacheEntry{
		Content:     "# Cached",
		ContentType: "text/markdown",
		Bytes:       8,
		Code:        200,
		CodeText:    "OK",
	})

	result, redirect, err := rt.Fetch(context.Background(), testURL, "summarize")
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if redirect != nil {
		t.Fatal("unexpected redirect")
	}
	if !result.Policy.ProxyUsed {
		t.Fatalf("expected proxy metadata, got %+v", result.Policy)
	}
	if result.Policy.ResolvedIPClass != "fake_ip" || !result.Policy.FakeIPAllowed {
		t.Fatalf("expected fake-ip allowed metadata, got %+v", result.Policy)
	}
}

func TestFetch_FakeIPDNSBlockedWithoutProxyEvenWithCache(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Proxy.Mode = "direct"
	rt := NewRuntime(nil, nil, cfg)
	rt.urlPolicy = rt.urlPolicy.withResolver(&fakeResolver{
		ips: map[string][]net.IPAddr{
			"fake.example.com": {{IP: net.ParseIP("198.18.0.10")}},
		},
		err: map[string]error{},
	})

	testURL := "https://fake.example.com/doc"
	rt.cache.Set(CanonicalCacheURL(testURL), &CacheEntry{
		Content:     "# Cached",
		ContentType: "text/markdown",
		Bytes:       8,
		Code:        200,
		CodeText:    "OK",
	})

	if _, _, err := rt.Fetch(context.Background(), testURL, "summarize"); err == nil {
		t.Fatal("expected fake-ip DNS target to be blocked without proxy")
	}
}

func TestRuntimeSetLLMCaller(t *testing.T) {
	rt := NewRuntime(nil, nil, DefaultConfig())
	if rt.GetLLMCaller() != nil {
		t.Fatal("expected nil caller initially")
	}
	llm := func(ctx context.Context, prompt string) (string, error) { return "ok", nil }
	rt.SetLLMCaller(llm)
	if rt.GetLLMCaller() == nil {
		t.Fatal("expected caller to be set")
	}
}

func TestRuntimeSetSearchProvider(t *testing.T) {
	rt := NewRuntime(nil, nil, DefaultConfig())
	if rt.GetSearchProvider() != nil {
		t.Fatal("expected nil search provider initially")
	}
	provider := &testSearchProvider{}
	rt.SetSearchProvider(provider)
	if rt.GetSearchProvider() != provider {
		t.Fatal("expected search provider to be replaced")
	}
}

func TestSummariseContentRejectsPseudoToolCallXML(t *testing.T) {
	rt := NewRuntime(nil, func(ctx context.Context, prompt string) (string, error) {
		return "<tool_call>{\"name\":\"WebSearch\"}</tool_call>", nil
	}, DefaultConfig())

	_, err := rt.summariseContent(context.Background(), "https://example.com", "abc", "text/html", "summarize", 3, 200, "OK")
	if err == nil {
		t.Fatal("expected validation error")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected FetchError, got %T", err)
	}
	if fetchErr.Kind != FetchErrValidation {
		t.Fatalf("kind = %s, want %s", fetchErr.Kind, FetchErrValidation)
	}
}

func TestSummariseContentRejectsPseudoToolCallFencedJSON(t *testing.T) {
	rt := NewRuntime(nil, func(ctx context.Context, prompt string) (string, error) {
		return "```json\n{\"tool_calls\":[{\"name\":\"WebSearch\"}]}\n```", nil
	}, DefaultConfig())

	_, err := rt.summariseContent(context.Background(), "https://example.com", "abc", "text/html", "summarize", 3, 200, "OK")
	if err == nil {
		t.Fatal("expected validation error")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected FetchError, got %T", err)
	}
	if fetchErr.Kind != FetchErrValidation {
		t.Fatalf("kind = %s, want %s", fetchErr.Kind, FetchErrValidation)
	}
}

func TestSummariseContentRejectsPseudoToolCallProseJSON(t *testing.T) {
	rt := NewRuntime(nil, func(ctx context.Context, prompt string) (string, error) {
		return "Here is the call: {\"function_call\":{\"name\":\"WebSearch\"}}", nil
	}, DefaultConfig())

	_, err := rt.summariseContent(context.Background(), "https://example.com", "abc", "text/html", "summarize", 3, 200, "OK")
	if err == nil {
		t.Fatal("expected validation error")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected FetchError, got %T", err)
	}
	if fetchErr.Kind != FetchErrValidation {
		t.Fatalf("kind = %s, want %s", fetchErr.Kind, FetchErrValidation)
	}
}

type testSearchProvider struct{}

func (p *testSearchProvider) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	return &SearchResult{}, nil
}

func (p *testSearchProvider) SupportsFilter() bool {
	return false
}

func TestSummariseContentRejectsPseudoToolCallJSON(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"tool_calls": []map[string]any{{"name": "WebSearch"}},
	})
	rt := NewRuntime(nil, func(ctx context.Context, prompt string) (string, error) {
		return string(payload), nil
	}, DefaultConfig())

	_, err := rt.summariseContent(context.Background(), "https://example.com", "abc", "text/html", "summarize", 3, 200, "OK")
	if err == nil {
		t.Fatal("expected validation error")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected FetchError, got %T", err)
	}
	if fetchErr.Kind != FetchErrValidation {
		t.Fatalf("kind = %s, want %s", fetchErr.Kind, FetchErrValidation)
	}
}
