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

// BingSearchBackend 通过抓取 Bing HTML 页面进行搜索。
// 免费无需 API Key，使用国际版 Bing。
type BingSearchBackend struct {
	client *http.Client
}

// NewBingSearch 创建一个 Bing 搜索后端。
func NewBingSearch() *BingSearchBackend {
	return &BingSearchBackend{
		client: newPolicyHTTPClient(5*time.Second, DefaultConfig(), PurposeSearch),
	}
}

// SearchWeb 实现 SearchProvider 接口，通过抓取 Bing HTML 执行搜索。
func (b *BingSearchBackend) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	start := time.Now()

	// ensearch=1 forces international Bing (English results)
	searchURL := "https://www.bing.com/search?q=" + url.QueryEscape(query) + "&ensearch=1"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, &SearchError{
			Provider: "bing",
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
			Provider: "bing",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("http request: %w", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 || resp.StatusCode == 403 {
		return nil, &SearchError{
			Provider: "bing",
			Kind:     SearchErrRateLimited,
			Cause:    fmt.Errorf("rate limited: HTTP %d", resp.StatusCode),
		}
	}
	if resp.StatusCode >= 500 {
		return nil, &SearchError{
			Provider: "bing",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("server error: HTTP %d", resp.StatusCode),
		}
	}

	root, err := html.Parse(resp.Body)
	if err != nil {
		return nil, &SearchError{
			Provider: "bing",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("html parse: %w", err),
		}
	}

	hits := extractBingResults(root)
	if len(hits) == 0 {
		return nil, &SearchError{
			Provider: "bing",
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
		Backend:  "bing",
	}, nil
}

// SupportsFilter 报告此后端不支持域名过滤。
func (b *BingSearchBackend) SupportsFilter() bool { return false }

// extractBingResults 从 Bing HTML 树中提取搜索结果（最多 10 条）。
//
// Bing HTML 结构:
//
//	<li class="b_algo">
//	  <h2><a href="https://example.com">Title</a></h2>
//	  <div class="b_caption"><p>Snippet...</p></div>
//	</li>
func extractBingResults(root *html.Node) []SearchHit {
	var hits []SearchHit

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(hits) >= 10 {
			return
		}

		if n.Type == html.ElementNode {
			cls := getAttr(n, "class")

			// Each organic result is in <li class="b_algo">
			if n.Data == "li" && strings.Contains(cls, "b_algo") {
				hit := extractSingleBingResult(n)
				if hit.URL != "" && hit.Title != "" {
					hits = append(hits, hit)
				}
				return
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return hits
}

// extractSingleBingResult extracts title, URL, and snippet from a single <li class="b_algo">.
func extractSingleBingResult(li *html.Node) SearchHit {
	var hit SearchHit

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Title + URL: <h2><a href="...">Title</a></h2>
			if n.Data == "h2" && hit.Title == "" {
				if a := findChildElement(n, "a"); a != nil {
					hit.URL = getAttr(a, "href")
					hit.Title = strings.TrimSpace(textContent(a))
				}
				return
			}

			cls := getAttr(n, "class")

			// Snippet: <div class="b_caption"><p>...</p></div>
			// or <p class="b_lineclamp...">
			if n.Data == "p" && hit.Snippet == "" {
				if strings.Contains(cls, "b_lineclamp") || isInsideBCaption(n) {
					hit.Snippet = strings.TrimSpace(textContent(n))
					return
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(li)
	return hit
}

// findChildElement finds the first child element with the given tag name.
func findChildElement(parent *html.Node, tag string) *html.Node {
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

// isInsideBCaption checks whether n is inside a div with class "b_caption".
func isInsideBCaption(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "div" {
			cls := getAttr(p, "class")
			if strings.Contains(cls, "b_caption") {
				return true
			}
		}
	}
	return false
}
