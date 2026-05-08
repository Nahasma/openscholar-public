package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TavilySearchBackend executes web searches via the Tavily Search API.
// Tavily natively supports domain allow/block lists.
type TavilySearchBackend struct {
	apiKey string
	client *http.Client
}

// NewTavilySearch creates a Tavily Search backend with the given API key.
func NewTavilySearch(apiKey string) *TavilySearchBackend {
	return &TavilySearchBackend{
		apiKey: apiKey,
		client: newPolicyHTTPClient(6*time.Second, DefaultConfig(), PurposeSearch),
	}
}

// tavilyRequest is the JSON body sent to the Tavily API.
type tavilyRequest struct {
	APIKey            string   `json:"api_key"`
	Query             string   `json:"query"`
	MaxResults        int      `json:"max_results"`
	SearchDepth       string   `json:"search_depth"`
	IncludeAnswer     bool     `json:"include_answer"`
	IncludeRawContent bool     `json:"include_raw_content"`
	IncludeDomains    []string `json:"include_domains,omitempty"`
	ExcludeDomains    []string `json:"exclude_domains,omitempty"`
}

// tavilyResponse is the JSON response from the Tavily API.
type tavilyResponse struct {
	Answer  string         `json:"answer"`
	Results []tavilyResult `json:"results"`
}

// tavilyResult is a single search result from Tavily.
type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// SearchWeb implements SearchProvider by calling the Tavily Search API.
func (b *TavilySearchBackend) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	start := time.Now()

	reqBody := tavilyRequest{
		APIKey:            b.apiKey,
		Query:             query,
		MaxResults:        5,
		SearchDepth:       "basic",
		IncludeAnswer:     false,
		IncludeRawContent: false,
	}
	if len(opts.AllowedDomains) > 0 {
		reqBody.IncludeDomains = opts.AllowedDomains
	}
	if len(opts.BlockedDomains) > 0 {
		reqBody.ExcludeDomains = opts.BlockedDomains
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("marshal request: %w", err),
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("build request: %w", err),
		}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("http request: %w", err),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("read response body: %w", err),
		}
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrAuth,
			Cause:    fmt.Errorf("401 unauthorized: %s", string(body)),
		}
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrRateLimited,
			Cause:    fmt.Errorf("429 rate limited: %s", string(body)),
		}
	case resp.StatusCode >= 500:
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("%d server error: %s", resp.StatusCode, string(body)),
		}
	case resp.StatusCode != http.StatusOK:
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)),
		}
	}

	var parsed tavilyResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("parse response: %w", err),
		}
	}

	if len(parsed.Results) == 0 {
		return nil, &SearchError{
			Provider: "tavily",
			Kind:     SearchErrNoResults,
			Cause:    fmt.Errorf("no results returned for query: %q", query),
		}
	}

	hits := make([]SearchHit, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		hits = append(hits, SearchHit{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}

	// Build summary from the first 3 content fields.
	var parts []string
	for i, r := range parsed.Results {
		if i >= 3 {
			break
		}
		if r.Content != "" {
			parts = append(parts, r.Content)
		}
	}
	summary := strings.Join(parts, "\n\n")

	return &SearchResult{
		Hits:     hits,
		Summary:  summary,
		Duration: time.Since(start).Seconds(),
		Backend:  "tavily",
	}, nil
}

// SupportsFilter reports that Tavily natively supports domain allow/block lists.
func (b *TavilySearchBackend) SupportsFilter() bool { return true }
