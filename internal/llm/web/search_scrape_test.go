package web

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// --- Startpage HTML extraction ---

func TestStartpage_HTMLExtraction(t *testing.T) {
	htmlStr := `<html><body>
		<div class="w-gl__result">
			<a class="w-gl__result-title" href="https://example.com"><h3>Example Title</h3></a>
			<p class="w-gl__description">This is a snippet about example.</p>
		</div>
		<div class="w-gl__result">
			<a class="w-gl__result-title" href="https://golang.org/doc"><h3>Go Documentation</h3></a>
			<p class="w-gl__description">Official Go programming docs.</p>
		</div>
	</body></html>`

	root, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	hits := extractStartpageResults(root)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}

	if hits[0].Title != "Example Title" {
		t.Errorf("hit[0].Title = %q, want %q", hits[0].Title, "Example Title")
	}
	if hits[0].URL != "https://example.com" {
		t.Errorf("hit[0].URL = %q, want %q", hits[0].URL, "https://example.com")
	}
	if hits[0].Snippet != "This is a snippet about example." {
		t.Errorf("hit[0].Snippet = %q", hits[0].Snippet)
	}

	if hits[1].Title != "Go Documentation" {
		t.Errorf("hit[1].Title = %q", hits[1].Title)
	}
	if hits[1].URL != "https://golang.org/doc" {
		t.Errorf("hit[1].URL = %q", hits[1].URL)
	}
}

func TestStartpage_EmptyHTML(t *testing.T) {
	root, _ := html.Parse(strings.NewReader(`<html><body></body></html>`))
	hits := extractStartpageResults(root)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits from empty HTML, got %d", len(hits))
	}
}

func TestStartpage_MaxResults(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<html><body>`)
	for i := 0; i < 15; i++ {
		sb.WriteString(`<div class="w-gl__result">`)
		sb.WriteString(`<a class="w-gl__result-title" href="https://example.com"><h3>Title</h3></a>`)
		sb.WriteString(`<p class="w-gl__description">Snippet</p>`)
		sb.WriteString(`</div>`)
	}
	sb.WriteString(`</body></html>`)

	root, _ := html.Parse(strings.NewReader(sb.String()))
	hits := extractStartpageResults(root)
	if len(hits) > 10 {
		t.Errorf("expected max 10 hits, got %d", len(hits))
	}
}

func TestStartpage_SupportsFilter(t *testing.T) {
	b := NewStartpageSearch()
	if b.SupportsFilter() {
		t.Error("Startpage should NOT support native domain filtering")
	}
}

// --- Bing HTML extraction ---

func TestBing_HTMLExtraction(t *testing.T) {
	htmlStr := `<html><body>
		<ol id="b_results">
			<li class="b_algo">
				<h2><a href="https://example.com">Example Result</a></h2>
				<div class="b_caption"><p>A snippet about the example result.</p></div>
			</li>
			<li class="b_algo">
				<h2><a href="https://golang.org">Go Language</a></h2>
				<div class="b_caption"><p>Go is an open source programming language.</p></div>
			</li>
		</ol>
	</body></html>`

	root, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	hits := extractBingResults(root)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}

	if hits[0].Title != "Example Result" {
		t.Errorf("hit[0].Title = %q, want %q", hits[0].Title, "Example Result")
	}
	if hits[0].URL != "https://example.com" {
		t.Errorf("hit[0].URL = %q, want %q", hits[0].URL, "https://example.com")
	}
	if hits[0].Snippet != "A snippet about the example result." {
		t.Errorf("hit[0].Snippet = %q", hits[0].Snippet)
	}

	if hits[1].Title != "Go Language" {
		t.Errorf("hit[1].Title = %q", hits[1].Title)
	}
}

func TestBing_EmptyHTML(t *testing.T) {
	root, _ := html.Parse(strings.NewReader(`<html><body></body></html>`))
	hits := extractBingResults(root)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits from empty HTML, got %d", len(hits))
	}
}

func TestBing_MaxResults(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<html><body><ol>`)
	for i := 0; i < 15; i++ {
		sb.WriteString(`<li class="b_algo">`)
		sb.WriteString(`<h2><a href="https://example.com">Title</a></h2>`)
		sb.WriteString(`<div class="b_caption"><p>Snippet</p></div>`)
		sb.WriteString(`</li>`)
	}
	sb.WriteString(`</ol></body></html>`)

	root, _ := html.Parse(strings.NewReader(sb.String()))
	hits := extractBingResults(root)
	if len(hits) > 10 {
		t.Errorf("expected max 10 hits, got %d", len(hits))
	}
}

func TestBing_SupportsFilter(t *testing.T) {
	b := NewBingSearch()
	if b.SupportsFilter() {
		t.Error("Bing should NOT support native domain filtering")
	}
}

func TestBing_BLineclampSnippet(t *testing.T) {
	// Test alternative snippet format with b_lineclamp class
	htmlStr := `<html><body>
		<li class="b_algo">
			<h2><a href="https://example.com">Title</a></h2>
			<p class="b_lineclamp2 b_algoSlug">Alternative snippet format.</p>
		</li>
	</body></html>`

	root, _ := html.Parse(strings.NewReader(htmlStr))
	hits := extractBingResults(root)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Snippet != "Alternative snippet format." {
		t.Errorf("hit[0].Snippet = %q", hits[0].Snippet)
	}
}

// --- Router integration ---

func TestRouter_ZeroConfigIncludesNewBackends(t *testing.T) {
	r := BuildSearchRouter(DefaultConfig(), nil, nil, "")

	found := map[string]bool{}
	for _, b := range r.backends {
		found[b.Name] = true
	}

	if !found["startpage"] {
		t.Error("zero config should include Startpage")
	}
	if !found["bing"] {
		t.Error("zero config should include Bing")
	}
	if !found["duckduckgo"] {
		t.Error("zero config should include DuckDuckGo")
	}
}

func TestRouter_StartpagePriorityBetweenDDGAndLLM(t *testing.T) {
	r := BuildSearchRouter(DefaultConfig(), nil, nil, "")

	var ddgPri, spPri, bingPri int
	for _, b := range r.backends {
		switch b.Name {
		case "duckduckgo":
			ddgPri = b.Priority
		case "startpage":
			spPri = b.Priority
		case "bing":
			bingPri = b.Priority
		}
	}

	if spPri <= ddgPri {
		t.Errorf("startpage priority (%d) should be > duckduckgo (%d)", spPri, ddgPri)
	}
	if bingPri <= ddgPri {
		t.Errorf("bing priority (%d) should be > duckduckgo (%d)", bingPri, ddgPri)
	}
}
