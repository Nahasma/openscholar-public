package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// StartpageBackend 通过抓取 Startpage HTML 页面进行搜索。
// Startpage 是 Google 结果的隐私代理，免费无需 API Key。
type StartpageBackend struct {
	client *http.Client
}

// NewStartpageSearch 创建一个 Startpage 搜索后端。
func NewStartpageSearch() *StartpageBackend {
	return &StartpageBackend{
		client: newPolicyHTTPClient(5*time.Second, DefaultConfig(), PurposeSearch),
	}
}

// SearchWeb 实现 SearchProvider 接口，通过抓取 Startpage HTML 执行搜索。
func (b *StartpageBackend) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	start := time.Now()

	searchURL := "https://www.startpage.com/sp/search?query=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, &SearchError{
			Provider: "startpage",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("create request: %w", err),
		}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenScholar/2.0)")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, &SearchError{
			Provider: "startpage",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("http request: %w", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 || resp.StatusCode == 403 {
		return nil, &SearchError{
			Provider: "startpage",
			Kind:     SearchErrRateLimited,
			Cause:    fmt.Errorf("rate limited: HTTP %d", resp.StatusCode),
		}
	}
	if resp.StatusCode >= 500 {
		return nil, &SearchError{
			Provider: "startpage",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("server error: HTTP %d", resp.StatusCode),
		}
	}

	root, err := html.Parse(resp.Body)
	if err != nil {
		return nil, &SearchError{
			Provider: "startpage",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("html parse: %w", err),
		}
	}

	hits := extractStartpageResults(root)
	if len(hits) == 0 {
		return nil, &SearchError{
			Provider: "startpage",
			Kind:     SearchErrNoResults,
			Cause:    fmt.Errorf("no results for query: %q", query),
		}
	}

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
		Backend:  "startpage",
	}, nil
}

// SupportsFilter 报告此后端不支持域名过滤。
func (b *StartpageBackend) SupportsFilter() bool { return false }

// extractStartpageResults 从 Startpage HTML 树中提取搜索结果（最多 10 条）。
//
// Startpage HTML 结构:
//
//	<div class="w-gl__result">
//	  <a class="w-gl__result-title" href="https://example.com"><h3>Title</h3></a>
//	  <p class="w-gl__description">Snippet...</p>
//	</div>
func extractStartpageResults(root *html.Node) []SearchHit {
	var hits []SearchHit

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(hits) >= 10 {
			return
		}

		if n.Type == html.ElementNode {
			cls := getAttr(n, "class")

			// Startpage wraps each result in a div with class containing "w-gl__result"
			if n.Data == "div" && strings.Contains(cls, "w-gl__result") && !strings.Contains(cls, "w-gl__results") {
				hit := extractSingleStartpageResult(n)
				if hit.URL != "" && hit.Title != "" {
					hits = append(hits, hit)
				}
				return // don't recurse into this result div
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return hits
}

// extractSingleStartpageResult extracts title, URL, and snippet from a single result div.
func extractSingleStartpageResult(div *html.Node) SearchHit {
	var hit SearchHit

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			cls := getAttr(n, "class")

			// Title link: <a class="w-gl__result-title" href="..."><h3>Title</h3></a>
			if n.Data == "a" && strings.Contains(cls, "result-title") {
				hit.URL = getAttr(n, "href")
				hit.Title = strings.TrimSpace(textContent(n))
			}

			// Snippet: <p class="w-gl__description">...</p>
			if n.Data == "p" && strings.Contains(cls, "description") {
				hit.Snippet = strings.TrimSpace(textContent(n))
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(div)
	return hit
}
