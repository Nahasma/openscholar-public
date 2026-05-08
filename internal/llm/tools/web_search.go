package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/llm/web"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

type webSearchTool struct {
	permissions permission.Service
	runtime     *web.Runtime
}

// NewWebSearchTool creates the WebSearch tool backed by a Web Runtime.
func NewWebSearchTool(perms permission.Service, runtime *web.Runtime) BaseTool {
	return &webSearchTool{permissions: perms, runtime: runtime}
}

func (t *webSearchTool) Info() ToolInfo {
	return ToolInfo{
		Name: "WebSearch",
		Description: `Search the web for current information and return results with sources.
- Provides up-to-date information beyond the knowledge cutoff
- Returns search result links and AI-generated summary
- Use for recent events, documentation, or current data
- For broad/long research, prefer delegating batches to Task agent_type=research and return only compressed evidence to the parent
- Domain filtering supported (allowed_domains / blocked_domains, mutually exclusive)
- ALWAYS include sources in your response using markdown hyperlinks`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query to execute",
				},
				"allowed_domains": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Only include results from these domains",
				},
				"blocked_domains": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Never include results from these domains",
				},
			},
			"required": []string{"query"},
		},
	}
}

// Available implements AvailabilityChecker.
func (t *webSearchTool) Available() (bool, string) {
	if t.runtime == nil || !t.runtime.HasSearchProvider() {
		return false, "WebSearch requires a provider with web search capability (Anthropic or OpenAI)"
	}
	return true, ""
}

func (t *webSearchTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	// Parse input
	var input struct {
		Query          string   `json:"query"`
		AllowedDomains []string `json:"allowed_domains"`
		BlockedDomains []string `json:"blocked_domains"`
	}
	if err := json.Unmarshal([]byte(call.Input), &input); err != nil {
		return ToolResponse{Content: "Error: invalid input JSON: " + err.Error(), IsError: true}, nil
	}

	// Validate
	if strings.TrimSpace(input.Query) == "" {
		return ToolResponse{Content: "Error: missing query", IsError: true}, nil
	}
	if len(input.AllowedDomains) > 0 && len(input.BlockedDomains) > 0 {
		return ToolResponse{Content: "Error: cannot specify both allowed_domains and blocked_domains", IsError: true}, nil
	}

	// Note: domain filter handling is now done by SearchRouter internally
	// (native filter / query rewrite + post-filter depending on backend)

	// Permission check
	if t.permissions != nil {
		if !t.permissions.Request(permission.CreatePermissionRequest{
			ToolName:    "WebSearch",
			Description: fmt.Sprintf("搜索互联网: %s", input.Query),
			Action:      "web_search",
		}) {
			return ToolResponse{Content: "Permission denied for WebSearch", IsError: true}, nil
		}
	}

	// Execute search
	result, err := t.runtime.Search(ctx, input.Query, web.SearchOptions{
		AllowedDomains: input.AllowedDomains,
		BlockedDomains: input.BlockedDomains,
	})
	if err != nil {
		var attempts []web.ProviderAttempt
		var failureErr *web.SearchFailureError
		retryAfter := ""
		errorKind := "search_failed"
		recoverable := true
		provider := ""
		source := ""
		var searchErr *web.SearchError
		if errors.As(err, &searchErr) {
			errorKind = string(searchErr.Kind)
			provider = searchErr.Provider
			source = searchErr.Provider
			recoverable = searchErr.Kind != web.SearchErrAuth
			if searchErr.RetryAfter > 0 {
				retryAfter = searchErr.RetryAfter.String()
			}
		}
		if errors.As(err, &failureErr) {
			attempts = failureErr.Attempts
		}
		errorMeta := map[string]any{
			"tool":          "WebSearch",
			"query":         input.Query,
			"query_key":     webSearchQueryKey(input.Query, input.AllowedDomains, input.BlockedDomains),
			"attempts":      attempts,
			"error_kind":    errorKind,
			"provider":      provider,
			"source":        source,
			"recoverable":   recoverable,
			"retry_after":   retryAfter,
			"progress_kind": "none",
		}
		if policyMeta, ok := web.PolicyMetadataFromError(err); ok {
			errorMeta["proxy_mode"] = policyMeta.ProxyMode
			errorMeta["proxy_used"] = policyMeta.ProxyUsed
			errorMeta["proxy_endpoint_class"] = policyMeta.ProxyEndpointClass
			errorMeta["resolved_ip_class"] = policyMeta.ResolvedIPClass
			errorMeta["policy_decision"] = policyMeta.PolicyDecision
			errorMeta["fake_ip_allowed"] = policyMeta.FakeIPAllowed
		}
		return WithResponseMetadata(ToolResponse{Content: "WebSearch failed.", IsError: true}, errorMeta), nil
	}

	content := fmt.Sprintf("WebSearch found %d results for %q via %s.", len(result.Hits), input.Query, result.Backend)
	if strings.TrimSpace(result.Summary) != "" {
		content += " " + compactInline(result.Summary, 240)
	}

	// Build metadata
	const maxSources = 5
	sources := make([]map[string]string, 0, min(len(result.Hits), maxSources))
	evidenceKeys := make([]string, 0, min(len(result.Hits), maxSources))
	for i, hit := range result.Hits {
		if i >= maxSources {
			break
		}
		redactedURL := web.RedactURL(hit.URL)
		sources = append(sources, map[string]string{
			"title":   hit.Title,
			"url":     redactedURL,
			"snippet": compactInline(hit.Snippet, 180),
		})
		evidenceKeys = append(evidenceKeys, "url:"+redactedURL)
	}
	queryKey := webSearchQueryKey(input.Query, input.AllowedDomains, input.BlockedDomains)
	meta, _ := json.Marshal(map[string]any{
		"provider":             result.Backend,
		"source":               result.Backend,
		"backend":              result.Backend,
		"duration_seconds":     result.Duration,
		"hit_count":            len(result.Hits),
		"evidence_keys":        evidenceKeys,
		"outcome_hash":         webSearchOutcomeHash(input.Query, result.Hits),
		"sources":              sources,
		"public_summary":       compactInline(result.Summary, 320),
		"query":                input.Query,
		"query_key":            queryKey,
		"attempts":             result.Attempts,
		"tool":                 "WebSearch",
		"progress_kind":        "search_page",
		"proxy_mode":           result.Policy.ProxyMode,
		"proxy_used":           result.Policy.ProxyUsed,
		"proxy_endpoint_class": result.Policy.ProxyEndpointClass,
		"resolved_ip_class":    result.Policy.ResolvedIPClass,
		"policy_decision":      result.Policy.PolicyDecision,
		"fake_ip_allowed":      result.Policy.FakeIPAllowed,
	})

	return ToolResponse{Content: content, Metadata: string(meta)}, nil
}

func compactInline(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return ""
	}
	if max <= 0 {
		max = 200
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func webSearchQueryKey(query string, allowedDomains, blockedDomains []string) string {
	q := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(query)), " "))
	if q == "" {
		q = "-"
	}
	allow := normalizedDomainList(allowedDomains)
	block := normalizedDomainList(blockedDomains)
	return fmt.Sprintf("search:web:%s|allow:%s|block:%s", q, strings.Join(allow, ","), strings.Join(block, ","))
}

func webSearchOutcomeHash(query string, hits []web.SearchHit) string {
	h := sha256.New()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(query))))
	const max = 8
	for i, hit := range hits {
		if i >= max {
			break
		}
		h.Write([]byte("|" + web.RedactURL(hit.URL) + "|" + strings.TrimSpace(hit.Title)))
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func normalizedDomainList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}
