package web

import (
	"testing"
)

func TestRewriteQueryWithFilters(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		allowed []string
		blocked []string
		want    string
	}{
		{
			name:  "no filters",
			query: "NeurIPS 2026",
			want:  "NeurIPS 2026",
		},
		{
			name:    "single allowed",
			query:   "NeurIPS 2026",
			allowed: []string{"arxiv.org"},
			want:    "NeurIPS 2026 site:arxiv.org",
		},
		{
			name:    "multiple allowed",
			query:   "transformer",
			allowed: []string{"arxiv.org", "openreview.net"},
			want:    "transformer site:arxiv.org OR site:openreview.net",
		},
		{
			name:    "single blocked",
			query:   "Go tutorial",
			blocked: []string{"w3schools.com"},
			want:    "Go tutorial -site:w3schools.com",
		},
		{
			name:    "multiple blocked",
			query:   "python",
			blocked: []string{"w3schools.com", "geeksforgeeks.org"},
			want:    "python -site:w3schools.com -site:geeksforgeeks.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RewriteQueryWithFilters(tt.query, tt.allowed, tt.blocked)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPostFilterHits(t *testing.T) {
	hits := []SearchHit{
		{Title: "arxiv paper", URL: "https://arxiv.org/abs/1234"},
		{Title: "python docs", URL: "https://docs.python.org/3/library"},
		{Title: "w3schools", URL: "https://www.w3schools.com/python"},
		{Title: "github", URL: "https://github.com/foo"},
	}

	t.Run("allowed filter", func(t *testing.T) {
		result := PostFilterHits(hits, []string{"arxiv.org", "python.org"}, nil)
		if len(result) != 2 {
			t.Fatalf("expected 2 hits, got %d", len(result))
		}
		if result[0].URL != "https://arxiv.org/abs/1234" {
			t.Errorf("unexpected first hit: %s", result[0].URL)
		}
		if result[1].URL != "https://docs.python.org/3/library" {
			t.Errorf("unexpected second hit: %s", result[1].URL)
		}
	})

	t.Run("blocked filter", func(t *testing.T) {
		result := PostFilterHits(hits, nil, []string{"w3schools.com"})
		if len(result) != 3 {
			t.Fatalf("expected 3 hits, got %d", len(result))
		}
		for _, h := range result {
			if h.URL == "https://www.w3schools.com/python" {
				t.Error("blocked hit should be removed")
			}
		}
	})

	t.Run("no filters", func(t *testing.T) {
		result := PostFilterHits(hits, nil, nil)
		if len(result) != 4 {
			t.Fatalf("expected 4 hits, got %d", len(result))
		}
	})

	t.Run("subdomain matching", func(t *testing.T) {
		result := PostFilterHits(hits, []string{"python.org"}, nil)
		if len(result) != 1 {
			t.Fatalf("expected 1 hit (docs.python.org), got %d", len(result))
		}
		if result[0].URL != "https://docs.python.org/3/library" {
			t.Errorf("expected docs.python.org, got %s", result[0].URL)
		}
	})
}

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://www.example.com/path", "example.com"},
		{"https://DOCS.Python.org", "docs.python.org"},
		{"http://WWW.Google.COM", "google.com"},
		{"arxiv.org", "arxiv.org"},
		{"www.example.com", "example.com"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeDomain(tt.input)
			if got != tt.want {
				t.Errorf("normalizeDomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
