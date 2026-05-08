package web

import (
	"net/url"
	"strings"
)

// RewriteQueryWithFilters 在查询中追加 site: / -site: 操作符。
// 适用于 Brave/SearXNG/DuckDuckGo。
//
// 示例:
//
//	RewriteQueryWithFilters("NeurIPS 2026", ["arxiv.org"], nil)
//	→ "NeurIPS 2026 site:arxiv.org"
//
//	RewriteQueryWithFilters("Go tutorial", nil, ["w3schools.com"])
//	→ "Go tutorial -site:w3schools.com"
//
//	多个 allowed: "query site:a.com OR site:b.com"
//	多个 blocked: "query -site:a.com -site:b.com"
func RewriteQueryWithFilters(query string, allowed, blocked []string) string {
	if len(allowed) == 0 && len(blocked) == 0 {
		return query
	}

	var parts []string
	parts = append(parts, query)

	if len(allowed) > 0 {
		siteTerms := make([]string, 0, len(allowed))
		for _, domain := range allowed {
			d := normalizeDomain(domain)
			if d == "" {
				d = domain
			}
			siteTerms = append(siteTerms, "site:"+d)
		}
		if len(siteTerms) == 1 {
			parts = append(parts, siteTerms[0])
		} else {
			parts = append(parts, strings.Join(siteTerms, " OR "))
		}
	}

	if len(blocked) > 0 {
		for _, domain := range blocked {
			d := normalizeDomain(domain)
			if d == "" {
				d = domain
			}
			parts = append(parts, "-site:"+d)
		}
	}

	return strings.Join(parts, " ")
}

// PostFilterHits 对搜索结果做严格域名过滤。
// 移除 URL 不在 allowed 列表中的结果，或 URL 在 blocked 列表中的结果。
// 域名比较忽略大小写，并剥离 scheme、www 前缀。
// 用 strings.HasSuffix 支持子域名匹配 (如 allowed=["python.org"] 应匹配 "docs.python.org")
func PostFilterHits(hits []SearchHit, allowed, blocked []string) []SearchHit {
	if len(allowed) == 0 && len(blocked) == 0 {
		return hits
	}

	// Normalize the filter lists once.
	normalizedAllowed := make([]string, 0, len(allowed))
	for _, d := range allowed {
		nd := normalizeDomain(d)
		if nd == "" {
			nd = strings.ToLower(strings.TrimPrefix(d, "www."))
		}
		normalizedAllowed = append(normalizedAllowed, nd)
	}

	normalizedBlocked := make([]string, 0, len(blocked))
	for _, d := range blocked {
		nd := normalizeDomain(d)
		if nd == "" {
			nd = strings.ToLower(strings.TrimPrefix(d, "www."))
		}
		normalizedBlocked = append(normalizedBlocked, nd)
	}

	result := make([]SearchHit, 0, len(hits))
	for _, hit := range hits {
		hitDomain := normalizeDomain(hit.URL)

		// Check blocked list first: skip if blocked.
		if isMatchedByDomainList(hitDomain, normalizedBlocked) {
			continue
		}

		// If allowed list is set, only keep hits matching it.
		if len(normalizedAllowed) > 0 && !isMatchedByDomainList(hitDomain, normalizedAllowed) {
			continue
		}

		result = append(result, hit)
	}

	return result
}

// isMatchedByDomainList checks whether hitDomain matches any domain in the list.
// Uses HasSuffix to support subdomain matching.
func isMatchedByDomainList(hitDomain string, list []string) bool {
	for _, d := range list {
		// Exact match or subdomain match: hitDomain ends with "."+d or equals d.
		if hitDomain == d || strings.HasSuffix(hitDomain, "."+d) {
			return true
		}
	}
	return false
}

// normalizeDomain 归一化域名：小写 + 去 www.
// If rawURL looks like a plain domain (no scheme), it handles it directly.
func normalizeDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	// If no scheme is present, add one so url.Parse works correctly.
	toParse := rawURL
	if !strings.Contains(rawURL, "://") {
		toParse = "https://" + rawURL
	}

	parsed, err := url.Parse(toParse)
	if err != nil || parsed.Host == "" {
		// Fallback: treat rawURL itself as the domain.
		host := rawURL
		host = strings.ToLower(host)
		host = strings.TrimPrefix(host, "www.")
		return host
	}

	host := parsed.Hostname() // strips port
	host = strings.ToLower(host)
	host = strings.TrimPrefix(host, "www.")
	return host
}
