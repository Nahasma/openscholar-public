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

// DuckDuckGoBackend 通过 HTML Lite 版本抓取 DuckDuckGo 结果。
// 免费，无需 API key，无官方 API 配额限制。
type DuckDuckGoBackend struct {
	client *http.Client
}

// NewDuckDuckGoSearch 创建一个 DuckDuckGo 搜索后端。
func NewDuckDuckGoSearch() *DuckDuckGoBackend {
	return &DuckDuckGoBackend{
		client: newPolicyHTTPClient(4*time.Second, DefaultConfig(), PurposeSearch),
	}
}

// SearchWeb 实现 SearchProvider 接口，通过抓取 DuckDuckGo HTML Lite 版本执行搜索。
func (b *DuckDuckGoBackend) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	start := time.Now()

	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, &SearchError{
			Provider: "duckduckgo",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("create request: %w", err),
		}
	}
	req.Header.Set("User-Agent", "OpenScholar/2.0")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, &SearchError{
			Provider: "duckduckgo",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("http request: %w", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 || resp.StatusCode == 403 {
		return nil, &SearchError{
			Provider: "duckduckgo",
			Kind:     SearchErrRateLimited,
			Cause:    fmt.Errorf("rate limited: HTTP %d", resp.StatusCode),
		}
	}

	root, err := html.Parse(resp.Body)
	if err != nil {
		return nil, &SearchError{
			Provider: "duckduckgo",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("html parse: %w", err),
		}
	}

	hits := extractDDGResults(root)
	if len(hits) == 0 {
		return nil, &SearchError{
			Provider: "duckduckgo",
			Kind:     SearchErrNoResults,
			Cause:    fmt.Errorf("no results for query: %q", query),
		}
	}

	// 拼接前 3 条 Snippet 作为 Summary
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
		Backend:  "duckduckgo",
	}, nil
}

// SupportsFilter 报告此后端不支持域名过滤。
func (b *DuckDuckGoBackend) SupportsFilter() bool { return false }

// extractDDGResults 从解析后的 HTML 树中提取搜索结果（最多 10 条）。
func extractDDGResults(root *html.Node) []SearchHit {
	var results []SearchHit
	// 临时存储已提取的链接节点，与 snippet 节点配对
	type partialHit struct {
		title   string
		rawHref string
	}
	var partials []partialHit
	var snippets []string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			cls := getAttr(n, "class")

			if n.Data == "a" && strings.Contains(cls, "result__a") {
				href := getAttr(n, "href")
				title := textContent(n)
				partials = append(partials, partialHit{title: title, rawHref: href})
			}

			if strings.Contains(cls, "result__snippet") {
				snippets = append(snippets, textContent(n))
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	maxResults := 10
	count := len(partials)
	if len(snippets) < count {
		count = len(snippets)
	}
	if count > maxResults {
		count = maxResults
	}

	for i := 0; i < count; i++ {
		p := partials[i]
		results = append(results, SearchHit{
			Title:   strings.TrimSpace(p.title),
			URL:     decodeDDGURL(p.rawHref),
			Snippet: strings.TrimSpace(snippets[i]),
		})
	}
	return results
}

// getAttr 从 HTML 节点中获取指定属性值。
func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// textContent 递归提取 HTML 节点的纯文本内容。
func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(textContent(c))
	}
	return sb.String()
}

// decodeDDGURL 从 DuckDuckGo 跳转包装 URL 中提取真实目标 URL。
// DDG 返回格式: //duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com&rut=...
// 如果不是 DDG 包装格式，直接返回原始 href。
func decodeDDGURL(href string) string {
	if !strings.Contains(href, "duckduckgo.com/l/") && !strings.Contains(href, "uddg=") {
		return href
	}

	// 补全协议以便解析
	fullURL := href
	if strings.HasPrefix(href, "//") {
		fullURL = "https:" + href
	}

	parsed, err := url.Parse(fullURL)
	if err != nil {
		return href
	}

	uddg := parsed.Query().Get("uddg")
	if uddg == "" {
		return href
	}

	decoded, err := url.QueryUnescape(uddg)
	if err != nil {
		return uddg
	}
	return decoded
}
