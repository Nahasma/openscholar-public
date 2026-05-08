package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SearXNGBackend 通过自建 SearXNG 实例进行搜索。
// 完全免费，无 API key，无用量限制。
type SearXNGBackend struct {
	baseURL string
	client  *http.Client
}

// NewSearXNGSearch 创建一个 SearXNG 搜索后端。
// baseURL 为自建 SearXNG 实例地址，如 "http://localhost:8080"。
func NewSearXNGSearch(baseURL string) *SearXNGBackend {
	return &SearXNGBackend{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  newPolicyHTTPClient(5*time.Second, DefaultConfig(), PurposeSearch),
	}
}

// searxngResponse 是 SearXNG JSON 响应的结构。
type searxngResponse struct {
	Query   string          `json:"query"`
	Results []searxngResult `json:"results"`
}

type searxngResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Engine  string `json:"engine"`
}

// SearchWeb 实现 SearchProvider 接口，通过 SearXNG JSON API 执行搜索。
func (b *SearXNGBackend) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	start := time.Now()

	apiURL := fmt.Sprintf("%s/search?q=%s&format=json&categories=general",
		b.baseURL, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, &SearchError{
			Provider: "searxng",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("create request: %w", err),
		}
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, &SearchError{
			Provider: "searxng",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("http request: %w", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return nil, &SearchError{
			Provider: "searxng",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("server error: HTTP %d", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &SearchError{
			Provider: "searxng",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("read body: %w", err),
		}
	}

	var data searxngResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, &SearchError{
			Provider: "searxng",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("json parse: %w", err),
		}
	}

	if len(data.Results) == 0 {
		return nil, &SearchError{
			Provider: "searxng",
			Kind:     SearchErrNoResults,
			Cause:    fmt.Errorf("no results for query: %q", query),
		}
	}

	maxResults := 10
	if len(data.Results) < maxResults {
		maxResults = len(data.Results)
	}

	hits := make([]SearchHit, 0, maxResults)
	for i := 0; i < maxResults; i++ {
		r := data.Results[i]
		hits = append(hits, SearchHit{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}

	// 拼接前 3 条结果的 content 作为 Summary
	var summaryParts []string
	for i := 0; i < 3 && i < len(hits); i++ {
		if hits[i].Snippet != "" {
			summaryParts = append(summaryParts, hits[i].Snippet)
		}
	}

	return &SearchResult{
		Hits:     hits,
		Summary:  strings.Join(summaryParts, "\n\n"),
		Duration: time.Since(start).Seconds(),
		Backend:  "searxng",
	}, nil
}

// SupportsFilter 报告此后端不支持域名过滤（通过查询改写 + 后过滤实现）。
func (b *SearXNGBackend) SupportsFilter() bool { return false }
