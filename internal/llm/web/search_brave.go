package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BraveSearchBackend executes web searches via the Brave Search API.
type BraveSearchBackend struct {
	apiKey string
	client *http.Client
}

// NewBraveSearch creates a Brave Search backend with the given API key.
func NewBraveSearch(apiKey string) *BraveSearchBackend {
	return &BraveSearchBackend{
		apiKey: apiKey,
		client: newPolicyHTTPClient(5*time.Second, DefaultConfig(), PurposeSearch),
	}
}

// braveWebResponse is the top-level JSON response from the Brave Search API.
type braveWebResponse struct {
	Web struct {
		Results []braveResult `json:"results"`
	} `json:"web"`
}

// braveResult is a single search result from Brave.
type braveResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Description   string   `json:"description"`
	ExtraSnippets []string `json:"extra_snippets"`
}

// parseRetryAfter parses the Retry-After header from an HTTP response.
// Returns the duration in seconds; defaults to 60s if the header is missing or invalid.
func parseRetryAfter(resp *http.Response) time.Duration {
	val := resp.Header.Get("Retry-After")
	if val == "" {
		return 60 * time.Second
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil || secs <= 0 {
		return 60 * time.Second
	}
	return time.Duration(secs * float64(time.Second))
}

// SearchWeb implements SearchProvider by calling the Brave Search API.
func (b *BraveSearchBackend) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	start := time.Now()

	endpoint := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(query) + "&count=10"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &SearchError{
			Provider: "brave",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("build request: %w", err),
		}
	}
	req.Header.Set("X-Subscription-Token", b.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, &SearchError{
			Provider: "brave",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("http request: %w", err),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &SearchError{
			Provider: "brave",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("read response body: %w", err),
		}
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, &SearchError{
			Provider: "brave",
			Kind:     SearchErrAuth,
			Cause:    fmt.Errorf("401 unauthorized: %s", string(body)),
		}
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &SearchError{
			Provider:   "brave",
			Kind:       SearchErrRateLimited,
			RetryAfter: parseRetryAfter(resp),
			Cause:      fmt.Errorf("429 rate limited: %s", string(body)),
		}
	case resp.StatusCode >= 500:
		return nil, &SearchError{
			Provider: "brave",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("%d server error: %s", resp.StatusCode, string(body)),
		}
	case resp.StatusCode != http.StatusOK:
		return nil, &SearchError{
			Provider: "brave",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body)),
		}
	}

	var parsed braveWebResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, &SearchError{
			Provider: "brave",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("parse response: %w", err),
		}
	}

	results := parsed.Web.Results
	if len(results) > 10 {
		results = results[:10]
	}

	if len(results) == 0 {
		return nil, &SearchError{
			Provider: "brave",
			Kind:     SearchErrNoResults,
			Cause:    fmt.Errorf("no results returned for query: %q", query),
		}
	}

	hits := make([]SearchHit, 0, len(results))
	for _, r := range results {
		hits = append(hits, SearchHit{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}

	// Build summary from the first 3 descriptions.
	var parts []string
	for i, r := range results {
		if i >= 3 {
			break
		}
		if r.Description != "" {
			parts = append(parts, r.Description)
		}
	}
	summary := strings.Join(parts, "\n\n")

	return &SearchResult{
		Hits:     hits,
		Summary:  summary,
		Duration: time.Since(start).Seconds(),
		Backend:  "brave",
	}, nil
}

// SupportsFilter reports that Brave does not natively support domain filtering.
// Domain filters are handled via query rewrite + post-filter.
func (b *BraveSearchBackend) SupportsFilter() bool { return false }
