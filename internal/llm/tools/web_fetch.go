package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/llm/web"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

type webFetchTool struct {
	permissions permission.Service
	runtime     *web.Runtime
}

// NewWebFetchTool creates the WebFetch tool backed by a Web Runtime.
func NewWebFetchTool(perms permission.Service, runtime *web.Runtime) BaseTool {
	return &webFetchTool{permissions: perms, runtime: runtime}
}

func (t *webFetchTool) Info() ToolInfo {
	return ToolInfo{
		Name: "WebFetch",
		Description: `Fetch content from a URL and process it with an AI prompt.
- Takes a URL and a prompt describing what to extract
- Fetches the page, converts HTML to markdown, then summarises with a small model
- Use for reading documentation, articles, or web page content
- URL must be a fully-formed valid URL (http/https)
- HTTP URLs are automatically upgraded to HTTPS
- Results may be summarised if content is very large
- 15-minute cache for repeated fetches of the same URL
- Cross-host redirects are reported back; you should re-fetch with the new URL`,
		MaxResultBytes: 100 * 1024, // 100KB — aligned with CC
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch content from",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "The prompt describing what information to extract from the page",
				},
			},
			"required": []string{"url", "prompt"},
		},
	}
}

func (t *webFetchTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	// Parse input
	var input struct {
		URL    string `json:"url"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(call.Input), &input); err != nil {
		return ToolResponse{Content: "Error: invalid input JSON: " + err.Error(), IsError: true}, nil
	}

	// Validate URL
	if strings.TrimSpace(input.URL) == "" {
		return ToolResponse{Content: "Error: missing url", IsError: true}, nil
	}
	parsed, err := url.Parse(input.URL)
	if err != nil {
		return ToolResponse{Content: fmt.Sprintf("Error: invalid URL %q: %s", web.RedactURL(input.URL), err), IsError: true}, nil
	}

	// Permission check: pre-approved domains skip permission
	hostname := parsed.Hostname()
	isPreapproved := web.IsPreapprovedHost(hostname, parsed.Path)

	if !isPreapproved && t.permissions != nil {
		if !t.permissions.Request(permission.CreatePermissionRequest{
			ToolName:    "WebFetch",
			Description: fmt.Sprintf("抓取网页: %s", hostname),
			Action:      "web_fetch",
			Path:        "domain:" + hostname,
		}) {
			return ToolResponse{Content: "Permission denied for WebFetch on " + hostname, IsError: true}, nil
		}
	}

	// Execute fetch
	result, redirect, err := t.runtime.Fetch(ctx, input.URL, input.Prompt)
	if err != nil {
		var fetchErr *web.FetchError
		if errors.As(err, &fetchErr) {
			meta := map[string]any{
				"tool":          "WebFetch",
				"provider":      fetchErr.Provider,
				"source":        fetchErr.Source,
				"error_kind":    string(fetchErr.Kind),
				"recoverable":   fetchErr.Recoverable,
				"status_code":   fetchErr.StatusCode,
				"progress_kind": "none",
			}
			return WithResponseMetadata(ToolResponse{
				Content: fmt.Sprintf("WebFetch failed (%s).", fetchErr.Kind),
				IsError: true,
			}, meta), nil
		}
		if meta, ok := web.PolicyMetadataFromError(err); ok {
			return WithResponseMetadata(ToolResponse{Content: "Fetch failed: " + err.Error(), IsError: true}, meta), nil
		}
		return ToolResponse{Content: "Fetch failed: " + err.Error(), IsError: true}, nil
	}

	// Handle cross-host redirect
	if redirect != nil {
		statusText := "Found"
		switch redirect.StatusCode {
		case 301:
			statusText = "Moved Permanently"
		case 307:
			statusText = "Temporary Redirect"
		case 308:
			statusText = "Permanent Redirect"
		}
		content := fmt.Sprintf(`REDIRECT DETECTED: The URL redirects to a different host.

Original URL: %s
Redirect URL: %s
Status: %d %s

To complete your request, I need to fetch content from the redirected URL. Please use WebFetch again with these parameters:
- url: %q
- prompt: %q`, web.RedactURL(redirect.OriginalURL), web.RedactURL(redirect.RedirectURL), redirect.StatusCode, statusText, web.RedactURL(redirect.RedirectURL), input.Prompt)

		return ToolResponse{Content: content}, nil
	}

	// Build metadata
	canonicalURL := web.CanonicalCacheURL(web.RedactURL(result.URL))
	contentClass := classifyWebFetchContent(result.URL, result.ContentType, result.Content, result.Bytes)
	promptClass := classifyWebFetchPrompt(input.Prompt)
	targetKey := "webfetch:" + canonicalURL
	durable := isDurableWebFetchContent(contentClass)
	evidenceKeys := []string{"url:" + canonicalURL}
	lowValueReason := ""
	if !durable {
		lowValueReason = "repeated_target"
	}
	meta, _ := json.Marshal(map[string]any{
		"bytes":                result.Bytes,
		"code":                 result.Code,
		"status_code":          result.Code,
		"code_text":            result.CodeText,
		"content_type":         result.ContentType,
		"duration_seconds":     result.Duration,
		"cache_hit":            result.CacheHit,
		"url":                  web.RedactURL(result.URL),
		"tool":                 "WebFetch",
		"provider":             "http",
		"source":               web.RedactURL(result.URL),
		"progress_kind":        "fetched_page",
		"target_key":           targetKey,
		"canonical_url":        canonicalURL,
		"content_class":        contentClass,
		"prompt_class":         promptClass,
		"evidence_keys":        evidenceKeys,
		"outcome_hash":         webFetchOutcomeHash(canonicalURL, contentClass, result.Content),
		"low_value_reason":     lowValueReason,
		"durable_progress":     durable,
		"public_summary":       result.Content,
		"proxy_mode":           result.Policy.ProxyMode,
		"proxy_used":           result.Policy.ProxyUsed,
		"proxy_endpoint_class": result.Policy.ProxyEndpointClass,
		"resolved_ip_class":    result.Policy.ResolvedIPClass,
		"policy_decision":      result.Policy.PolicyDecision,
		"fake_ip_allowed":      result.Policy.FakeIPAllowed,
	})

	observation := fmt.Sprintf("Fetched %s (%d %s, %d bytes, content_type=%s).", web.RedactURL(result.URL), result.Code, result.CodeText, result.Bytes, result.ContentType)
	return ToolResponse{Content: observation, Metadata: string(meta)}, nil
}

func classifyWebFetchPrompt(prompt string) string {
	p := strings.ToLower(strings.TrimSpace(prompt))
	switch {
	case strings.Contains(p, "continue") || strings.Contains(p, "start from") || strings.Contains(p, "从第"):
		return "continue_same_target"
	case strings.Contains(p, "list") || strings.Contains(p, "列出"):
		return "list_extract"
	default:
		return "extract"
	}
}

func classifyWebFetchContent(rawURL, contentType, content string, bytes int) string {
	lurl := strings.ToLower(rawURL)
	lct := strings.ToLower(contentType)
	if strings.Contains(lct, "pdf") || strings.HasSuffix(lurl, ".pdf") {
		return "pdf"
	}
	if strings.Contains(lurl, "-abstract-conference.html") || strings.Contains(lurl, "/abs/") || strings.Contains(lurl, "doi.org/") {
		return "paper_page"
	}
	if isNeurIPSYearListURL(rawURL) || strings.Contains(lurl, "openreview.net/group") || strings.Contains(lurl, "openreview.net/search") {
		return "list_page"
	}
	if bytes > 1024*1024 && strings.Count(content, "](") >= 30 {
		return "list_page"
	}
	if strings.Contains(lurl, "?q=") || strings.Contains(lurl, "&q=") || strings.Contains(lurl, "?query=") {
		return "search_page"
	}
	return "doc_page"
}

func isNeurIPSYearListURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "paper_files" || parts[1] != "paper" {
		return false
	}
	if len(parts[2]) != 4 {
		return false
	}
	for _, r := range parts[2] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isDurableWebFetchContent(contentClass string) bool {
	switch contentClass {
	case "paper_page", "doc_page", "pdf":
		return true
	default:
		return false
	}
}

func webFetchOutcomeHash(canonicalURL, contentClass, summary string) string {
	h := sha256.Sum256([]byte(canonicalURL + "|" + contentClass + "|" + compactInline(summary, 240)))
	return fmt.Sprintf("sha256:%x", h[:])
}
