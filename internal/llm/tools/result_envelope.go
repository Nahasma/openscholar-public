package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ResultEnvelope struct {
	Tool            string              `json:"tool"`
	Status          string              `json:"status"`
	Action          string              `json:"action,omitempty"`
	ErrorKind       string              `json:"error_kind,omitempty"`
	Provider        string              `json:"provider,omitempty"`
	Source          string              `json:"source,omitempty"`
	RetryAfter      string              `json:"retry_after,omitempty"`
	StatusCode      int                 `json:"status_code,omitempty"`
	Recoverable     bool                `json:"recoverable,omitempty"`
	ProgressKind    string              `json:"progress_kind,omitempty"`
	DurableProgress bool                `json:"durable_progress"`
	TargetKey       string              `json:"target_key,omitempty"`
	CanonicalURL    string              `json:"canonical_url,omitempty"`
	ContentClass    string              `json:"content_class,omitempty"`
	PromptClass     string              `json:"prompt_class,omitempty"`
	QueryKey        string              `json:"query_key,omitempty"`
	EvidenceKeys    []string            `json:"evidence_keys,omitempty"`
	OutcomeHash     string              `json:"outcome_hash,omitempty"`
	LowValueReason  string              `json:"low_value_reason,omitempty"`
	CandidateCount  int                 `json:"candidate_count"`
	HitCount        int                 `json:"hit_count"`
	Attempts        any                 `json:"attempts,omitempty"`
	PublicSummary   string              `json:"public_summary"`
	Sources         []map[string]string `json:"sources,omitempty"`
	DataRef         string              `json:"data_ref,omitempty"`
}

func CompactToolResponse(toolName string, resp ToolResponse) ToolResponse {
	if !shouldCompactToolResponse(toolName) {
		return resp
	}
	envelope := ResultEnvelope{
		Tool:          strings.TrimSpace(toolName),
		Status:        "ok",
		PublicSummary: compactSummary(resp.Content),
	}
	if resp.IsError {
		envelope.Status = "error"
	}
	if md := parseResponseMetadata(resp.Metadata); md != nil {
		if v, _ := md["action"].(string); strings.TrimSpace(v) != "" {
			envelope.Action = strings.TrimSpace(v)
		}
		if v, _ := md["error_kind"].(string); strings.TrimSpace(v) != "" {
			envelope.ErrorKind = strings.TrimSpace(v)
		}
		if v, _ := md["provider"].(string); strings.TrimSpace(v) != "" {
			envelope.Provider = strings.TrimSpace(v)
		}
		if v, _ := md["source"].(string); strings.TrimSpace(v) != "" {
			envelope.Source = strings.TrimSpace(v)
		}
		if v, _ := md["retry_after"].(string); strings.TrimSpace(v) != "" {
			envelope.RetryAfter = strings.TrimSpace(v)
		}
		if v, ok := intMetadata(md, "status_code"); ok {
			envelope.StatusCode = v
		}
		if v, _ := md["recoverable"].(bool); v {
			envelope.Recoverable = true
		}
		if v, _ := md["progress_kind"].(string); strings.TrimSpace(v) != "" {
			envelope.ProgressKind = strings.TrimSpace(v)
		}
		if v, _ := md["durable_progress"].(bool); v {
			envelope.DurableProgress = true
		}
		if v, _ := md["target_key"].(string); strings.TrimSpace(v) != "" {
			envelope.TargetKey = strings.TrimSpace(v)
		}
		if v, _ := md["canonical_url"].(string); strings.TrimSpace(v) != "" {
			envelope.CanonicalURL = strings.TrimSpace(v)
		}
		if v, _ := md["content_class"].(string); strings.TrimSpace(v) != "" {
			envelope.ContentClass = strings.TrimSpace(v)
		}
		if v, _ := md["prompt_class"].(string); strings.TrimSpace(v) != "" {
			envelope.PromptClass = strings.TrimSpace(v)
		}
		if v, _ := md["query_key"].(string); strings.TrimSpace(v) != "" {
			envelope.QueryKey = strings.TrimSpace(v)
		}
		envelope.EvidenceKeys = stringSliceMetadata(md["evidence_keys"])
		if v, _ := md["outcome_hash"].(string); strings.TrimSpace(v) != "" {
			envelope.OutcomeHash = strings.TrimSpace(v)
		}
		if v, _ := md["low_value_reason"].(string); strings.TrimSpace(v) != "" {
			envelope.LowValueReason = strings.TrimSpace(v)
		}
		if v, ok := intMetadata(md, "candidate_count"); ok {
			envelope.CandidateCount = v
		}
		if v, ok := intMetadata(md, "hit_count"); ok {
			envelope.HitCount = v
		}
		if v, ok := md["attempts"]; ok {
			envelope.Attempts = v
		}
		if v, _ := md["data_ref"].(string); strings.TrimSpace(v) != "" {
			envelope.DataRef = strings.TrimSpace(v)
		}
		if v, _ := md["public_summary"].(string); strings.TrimSpace(v) != "" {
			envelope.PublicSummary = compactSummary(v)
		}
		envelope.Sources = sourcesMetadata(md["sources"])
	}
	if envelope.ErrorKind == "" && resp.IsError {
		envelope.ErrorKind = "tool_error"
	}
	b, err := json.Marshal(envelope)
	if err != nil {
		return ToolResponse{
			Type:    resp.Type,
			IsError: resp.IsError,
			Content: fmt.Sprintf(`{"tool":%q,"status":"%s","public_summary":%q}`, envelope.Tool, envelope.Status, envelope.PublicSummary),
		}
	}
	resp.Content = string(b)
	return resp
}

func shouldCompactToolResponse(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "websearch", "webfetch", "scholarsearch":
		return true
	case "kbquery", "kbsearch":
		return true
	default:
		return false
	}
}

func compactSummary(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return "No additional details."
	}
	const max = 800
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

func parseResponseMetadata(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		return nil
	}
	return md
}

func intMetadata(md map[string]any, key string) (int, bool) {
	v, ok := md[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func sourcesMetadata(raw any) []map[string]string {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]string, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		title, _ := m["title"].(string)
		u, _ := m["url"].(string)
		if strings.TrimSpace(title) == "" && strings.TrimSpace(u) == "" {
			continue
		}
		out = append(out, map[string]string{
			"title":   strings.TrimSpace(title),
			"url":     strings.TrimSpace(u),
			"snippet": strings.TrimSpace(stringValue(m["snippet"])),
		})
	}
	return out
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func stringSliceMetadata(raw any) []string {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		s, ok := v.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}
