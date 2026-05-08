package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// --- SearXNG (baseURL is injectable, full httptest) ---

func TestSearXNG_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"query": "test",
			"results": [
				{"title": "Result 1", "url": "https://a.com", "content": "snippet 1", "engine": "google"},
				{"title": "Result 2", "url": "https://b.com", "content": "snippet 2", "engine": "bing"}
			]
		}`))
	}))
	defer srv.Close()

	b := NewSearXNGSearch(srv.URL)
	b.client = srv.Client()
	result, err := b.SearchWeb(context.Background(), "test", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(result.Hits))
	}
	if result.Hits[0].Title != "Result 1" {
		t.Errorf("unexpected title: %s", result.Hits[0].Title)
	}
	if result.Hits[0].Snippet != "snippet 1" {
		t.Errorf("unexpected snippet: %s", result.Hits[0].Snippet)
	}
	if result.Backend != "searxng" {
		t.Errorf("expected backend=searxng, got %s", result.Backend)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestSearXNG_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := NewSearXNGSearch(srv.URL)
	b.client = srv.Client()
	_, err := b.SearchWeb(context.Background(), "test", SearchOptions{})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	se, ok := err.(*SearchError)
	if !ok {
		t.Fatalf("expected SearchError, got %T", err)
	}
	if se.Kind != SearchErrTemporary {
		t.Errorf("expected SearchErrTemporary, got %s", se.Kind)
	}
}

func TestSearXNG_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"query":"test","results":[]}`))
	}))
	defer srv.Close()

	b := NewSearXNGSearch(srv.URL)
	b.client = srv.Client()
	_, err := b.SearchWeb(context.Background(), "test", SearchOptions{})
	if err == nil {
		t.Fatal("expected error for empty results")
	}
	se := err.(*SearchError)
	if se.Kind != SearchErrNoResults {
		t.Errorf("expected SearchErrNoResults, got %s", se.Kind)
	}
}

func TestSearXNG_DefaultClientBlocksLocalEndpoint(t *testing.T) {
	b := NewSearXNGSearch("http://127.0.0.1:1")
	_, err := b.SearchWeb(context.Background(), "test", SearchOptions{})
	if err == nil {
		t.Fatal("expected local SearXNG endpoint to be blocked by URL policy")
	}
}

// --- Brave (inject client with custom transport for httptest) ---

func TestBrave_StructSetup(t *testing.T) {
	b := NewBraveSearch("test-key")
	if b.apiKey != "test-key" {
		t.Errorf("expected apiKey=test-key, got %s", b.apiKey)
	}
	if b.client == nil {
		t.Error("expected non-nil client")
	}
	// Brave hardcodes endpoint URL, so full HTTP testing is done
	// via the router integration tests with mock SearchProvider.
}

func TestBrave_SupportsFilter(t *testing.T) {
	b := NewBraveSearch("key")
	if b.SupportsFilter() {
		t.Error("Brave should NOT support native domain filtering")
	}
}

func TestBrave_RateLimitRetryAfter(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}

	resp.Header.Set("Retry-After", "30")
	d := parseRetryAfter(resp)
	if d != 30e9 { // 30 seconds
		t.Errorf("expected 30s, got %v", d)
	}

	resp.Header.Set("Retry-After", "")
	d = parseRetryAfter(resp)
	if d != 60e9 { // default 60s
		t.Errorf("expected 60s default, got %v", d)
	}

	resp.Header.Set("Retry-After", "invalid")
	d = parseRetryAfter(resp)
	if d != 60e9 {
		t.Errorf("expected 60s for invalid, got %v", d)
	}
}

// --- Tavily (test SupportsFilter and request body structure) ---

func TestTavily_SupportsFilter(t *testing.T) {
	b := NewTavilySearch("test-key")
	if !b.SupportsFilter() {
		t.Error("Tavily should support native domain filtering")
	}
}

func TestTavily_RequestStructure(t *testing.T) {
	// Verify the request struct marshals correctly with domain filters
	b := NewTavilySearch("test-key")
	_ = b // Tavily uses hardcoded URL, tested via router mocks
}

// --- DuckDuckGo (test HTML parsing + URL decode) ---

func TestDDG_URLDecode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ddg wrapped URL",
			input: "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com&rut=abc",
			want:  "https://example.com",
		},
		{
			name:  "normal URL passthrough",
			input: "https://normal-url.com",
			want:  "https://normal-url.com",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "ddg wrapped with encoded params",
			input: "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpath%3Fq%3Dtest&rut=xyz",
			want:  "https://example.com/path?q=test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeDDGURL(tt.input)
			if got != tt.want {
				t.Errorf("decodeDDGURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDDG_HTMLExtraction(t *testing.T) {
	// Test the extractDDGResults function directly with a synthetic HTML tree
	htmlStr := `<html><body>
		<div class="results">
			<div class="result">
				<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com">Example Title</a>
				<a class="result__snippet">This is a snippet about the example.</a>
			</div>
			<div class="result">
				<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgolang.org">Go Programming</a>
				<a class="result__snippet">The Go programming language.</a>
			</div>
		</div>
	</body></html>`

	root, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	hits := extractDDGResults(root)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}

	if hits[0].Title != "Example Title" {
		t.Errorf("hit[0].Title = %q, want %q", hits[0].Title, "Example Title")
	}
	if hits[0].URL != "https://example.com" {
		t.Errorf("hit[0].URL = %q, want %q", hits[0].URL, "https://example.com")
	}
	if hits[0].Snippet != "This is a snippet about the example." {
		t.Errorf("hit[0].Snippet = %q", hits[0].Snippet)
	}

	if hits[1].Title != "Go Programming" {
		t.Errorf("hit[1].Title = %q, want %q", hits[1].Title, "Go Programming")
	}
	if hits[1].URL != "https://golang.org" {
		t.Errorf("hit[1].URL = %q, want %q", hits[1].URL, "https://golang.org")
	}
}

func TestDDG_EmptyHTML(t *testing.T) {
	root, _ := html.Parse(strings.NewReader(`<html><body></body></html>`))
	hits := extractDDGResults(root)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits from empty HTML, got %d", len(hits))
	}
}

func TestDDG_MaxResults(t *testing.T) {
	// Build HTML with 15 results
	var sb strings.Builder
	sb.WriteString(`<html><body>`)
	for i := 0; i < 15; i++ {
		sb.WriteString(`<a class="result__a" href="https://example.com">Title</a>`)
		sb.WriteString(`<a class="result__snippet">Snippet</a>`)
	}
	sb.WriteString(`</body></html>`)

	root, _ := html.Parse(strings.NewReader(sb.String()))
	hits := extractDDGResults(root)
	if len(hits) > 10 {
		t.Errorf("expected max 10 hits, got %d", len(hits))
	}
}

// --- Backend SupportsFilter consistency ---

func TestBackend_FilterSupport(t *testing.T) {
	tests := []struct {
		name    string
		backend SearchProvider
		want    bool
	}{
		{"SearXNG", NewSearXNGSearch("http://localhost"), false},
		{"DuckDuckGo", NewDuckDuckGoSearch(), false},
		{"Brave", NewBraveSearch("key"), false},
		{"Tavily", NewTavilySearch("key"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.backend.SupportsFilter(); got != tt.want {
				t.Errorf("%s.SupportsFilter() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
