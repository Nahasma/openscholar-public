package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/config"
)

func TestResolveDownloadDir(t *testing.T) {
	// No explicit workspace in context → ValidateWorkspacePath is a no-op,
	// so all paths are allowed. Tests focus on correct path resolution.
	tests := []struct {
		name        string
		destination string
		researchDir string
		expected    string // exact expected path
	}{
		{
			name:     "default non-research",
			expected: filepath.Join(config.DataDirectory(), "papers"),
		},
		{
			name:        "default research mode",
			researchDir: "/tmp/research-topic-123",
			expected:    "/tmp/research-topic-123/papers",
		},
		{
			name:        "relative destination non-research",
			destination: "collected-papers",
			expected:    filepath.Join(config.WorkingDirectory(), "collected-papers"),
		},
		{
			name:        "relative destination research mode",
			destination: "custom-dir",
			researchDir: "/tmp/research-topic-123",
			expected:    "/tmp/research-topic-123/custom-dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.researchDir != "" {
				ctx = context.WithValue(ctx, ResearchWorkDirContextKey, tt.researchDir)
			}

			result, err := resolveDownloadDir(ctx, tt.destination)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("resolveDownloadDir() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResolveDownloadDir_BoundaryValidation(t *testing.T) {
	// With explicit workspace set, absolute paths outside workspace should be rejected.
	workspace := t.TempDir()
	ctx := context.WithValue(context.Background(), WorkspaceDirContextKey, workspace)

	// Path inside workspace should succeed
	insideDir := filepath.Join(workspace, "my-papers")
	result, err := resolveDownloadDir(ctx, insideDir)
	if err != nil {
		t.Errorf("expected success for path inside workspace, got: %v", err)
	}
	if result != insideDir {
		t.Errorf("resolveDownloadDir() = %q, want %q", result, insideDir)
	}

	// Absolute path outside workspace should be rejected
	_, err = resolveDownloadDir(ctx, "/tmp/outside-workspace")
	if err == nil {
		t.Error("expected error for absolute path outside workspace, got nil")
	}
}

func TestResolveDownloadDir_DefaultWithWorkspaceContext(t *testing.T) {
	// When workspace context is set and no destination given,
	// the default data directory path should still work
	// (ValidateWorkspacePath only blocks paths outside workspace).
	// Without explicit destination, default goes to DataDirectory()/papers
	// which may be outside the workspace — but that's the expected behavior
	// because the default data dir IS part of the project structure.
	ctx := context.Background()
	// No workspace context → ValidateWorkspacePath is no-op → default always works
	result, err := resolveDownloadDir(ctx, "")
	if err != nil {
		t.Fatalf("default path without workspace context should succeed, got: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %q", result)
	}
	if filepath.Base(result) != "papers" {
		t.Errorf("expected path ending in 'papers', got %q", result)
	}
}

func TestScholarSearchInfo_HasDestination(t *testing.T) {
	tool := NewScholarSearchTool(nil)
	info := tool.Info()
	props, ok := info.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map in Parameters")
	}
	if _, ok := props["destination"]; !ok {
		t.Error("expected 'destination' in tool parameters")
	}
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func withScholarClients(t *testing.T, apiRT rtFunc, dlRT rtFunc) {
	t.Helper()
	oldAPI := scholarHTTPClient
	oldDL := scholarDownloadHTTPClient
	scholarHTTPClient = &http.Client{Transport: apiRT}
	scholarDownloadHTTPClient = &http.Client{Transport: dlRT}
	t.Cleanup(func() {
		scholarHTTPClient = oldAPI
		scholarDownloadHTTPClient = oldDL
		scholarRateMu.Lock()
		scholarLastRequest = time.Time{}
		scholarCooldownUntil = time.Time{}
		scholarRateMu.Unlock()
		scholarHTTPStateMu.Lock()
		scholarCooldownBy = map[string]time.Time{}
		scholarCacheByKey = map[string]scholarCacheEntry{}
		scholarHTTPStateMu.Unlock()
	})
}

func withDefaultHTTPClient(t *testing.T, rt rtFunc) {
	t.Helper()
	old := http.DefaultClient
	oldScholar := scholarHTTPClient
	http.DefaultClient = &http.Client{Transport: rt}
	scholarHTTPClient = &http.Client{Transport: rt}
	t.Cleanup(func() {
		http.DefaultClient = old
		scholarHTTPClient = oldScholar
		scholarHTTPStateMu.Lock()
		scholarCooldownBy = map[string]time.Time{}
		scholarCacheByKey = map[string]scholarCacheEntry{}
		scholarHTTPStateMu.Unlock()
	})
}

func newJSONResp(status int, body string, headers map[string]string) *http.Response {
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       ioNopCloser(strings.NewReader(body)),
	}
}

type nopCloser struct{ *strings.Reader }

func (nopCloser) Close() error                { return nil }
func ioNopCloser(r *strings.Reader) nopCloser { return nopCloser{Reader: r} }

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) { return 0, fmt.Errorf("read failed") }
func (failingReadCloser) Close() error             { return nil }

func TestScholarSearch_SearchEmitsCandidateID(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Multi-Agent Planning","year":2026,"venue":"NeurIPS","abstract":"multi-agent systems","externalIds":{"ArXiv":"2601.00001"},"openAccessPdf":{"url":"https://example.test/a.pdf"}}]}`, nil), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-1")
	resp, err := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"multi-agent","limit":1}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !strings.Contains(resp.Content, "CandidateID:") {
		t.Fatalf("search output missing CandidateID: %s", resp.Content)
	}
	if len(tool.candidates["sess-1"]) != 1 {
		t.Fatalf("expected candidate stored")
	}
}

func TestScholarSearch_DownloadBlocksUnseenID(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Multi-Agent Planning","year":2026,"venue":"NeurIPS","abstract":"multi-agent systems","externalIds":{"ArXiv":"2601.00001"},"openAccessPdf":{"url":"https://example.test/a.pdf"}}]}`, nil), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-2")
	_, _ = tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"multi-agent","limit":1}`})
	dir := t.TempDir()
	resp, err := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","id":"ArXiv:2508.10923","destination":%q}`, dir)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !resp.IsError || !strings.Contains(resp.Content, "blocked") {
		t.Fatalf("expected blocked unseen id, got: %s", resp.Content)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no files written")
	}
}

func TestScholarSearch_DownloadAllowsCandidateIDAndManifest(t *testing.T) {
	var dlCount atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Multi-Agent Planning","year":2026,"venue":"NeurIPS","abstract":"multi-agent systems","externalIds":{"ArXiv":"2601.00001"},"openAccessPdf":{"url":"https://example.test/a.pdf"}}]}`, nil), nil
	}, func(r *http.Request) (*http.Response, error) {
		dlCount.Add(1)
		return &http.Response{StatusCode: 200, Body: ioNopCloser(strings.NewReader("%PDF-1.4 test"))}, nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-3")
	_, _ = tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"multi-agent","limit":1}`})
	cid := tool.candidates["sess-3"][0].CandidateID
	dir := t.TempDir()
	resp, err := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","candidate_id":%q,"destination":%q}`, cid, dir)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError || !strings.Contains(resp.Content, "CandidateID: "+cid) {
		t.Fatalf("expected successful download, got: %s", resp.Content)
	}
	if dlCount.Load() != 1 {
		t.Fatalf("expected one download request")
	}
	manifest := filepath.Join(dir, ".openscholar-downloads.jsonl")
	data, err := os.ReadFile(manifest)
	if err != nil || !strings.Contains(string(data), cid) {
		t.Fatalf("manifest missing candidate id")
	}
}

func TestScholarSearch_DownloadBlocksExistingFileWithoutManifestMatch(t *testing.T) {
	var dlCount atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Multi-Agent Planning","year":2026,"venue":"NeurIPS","abstract":"multi-agent systems","externalIds":{"ArXiv":"2601.00001"},"openAccessPdf":{"url":"https://example.test/a.pdf"}}]}`, nil), nil
	}, func(r *http.Request) (*http.Response, error) {
		dlCount.Add(1)
		return &http.Response{StatusCode: 200, Body: ioNopCloser(strings.NewReader("%PDF-1.4 test"))}, nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-existing")
	_, _ = tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"multi-agent","limit":1}`})
	candidate := tool.candidates["sess-existing"][0]
	dir := t.TempDir()
	existingPath := filepath.Join(dir, downloadFileName(candidate))
	if err := os.WriteFile(existingPath, []byte("stale unrelated file"), 0o644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}
	resp, err := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","candidate_id":%q,"destination":%q}`, candidate.CandidateID, dir)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !resp.IsError || !strings.Contains(resp.Content, "manifest does not verify") {
		t.Fatalf("expected manifest verification block, got: %s", resp.Content)
	}
	if dlCount.Load() != 0 {
		t.Fatalf("download should not start when existing file is unverified")
	}
}

func TestScholarSearch_DownloadAllowsUniqueSeenRawArxivID(t *testing.T) {
	var dlCount atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Multi-Agent Planning","year":2026,"venue":"NeurIPS","abstract":"multi-agent systems","externalIds":{"ArXiv":"2601.00001"},"openAccessPdf":{"url":"https://example.test/a.pdf"}}]}`, nil), nil
	}, func(r *http.Request) (*http.Response, error) {
		dlCount.Add(1)
		return &http.Response{StatusCode: 200, Body: ioNopCloser(strings.NewReader("%PDF-1.4 test"))}, nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-raw-id")
	_, _ = tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"multi-agent","limit":1}`})
	dir := t.TempDir()
	resp, err := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","id":"2601.00001","destination":%q}`, dir)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError {
		t.Fatalf("expected raw arxiv id to resolve to seen candidate, got: %s", resp.Content)
	}
	if dlCount.Load() != 1 {
		t.Fatalf("expected one download request")
	}
}

func TestScholarSearch_DownloadRejectsYearAndTopicMismatch(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Quantum Walks","year":2025,"venue":"QIP","abstract":"spin magnetization","externalIds":{"ArXiv":"2501.00001"},"openAccessPdf":{"url":"https://example.test/a.pdf"}}]}`, nil), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-4")
	_, _ = tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"multi-agent","limit":1}`})
	cid := tool.candidates["sess-4"][0].CandidateID
	resp, _ := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","candidate_id":%q,"year_min":2026,"topic":"multi-agent systems"}`, cid)})
	if !resp.IsError || !strings.Contains(resp.Content, "year 2025 < year_min 2026") || !strings.Contains(resp.Content, "topic does not match") {
		t.Fatalf("expected year/topic rejection, got: %s", resp.Content)
	}
}

func TestScholarSearch_DryRunDoesNotWrite(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Multi-Agent Planning","year":2026,"venue":"NeurIPS","abstract":"multi-agent systems","externalIds":{"ArXiv":"2601.00001"},"openAccessPdf":{"url":"https://example.test/a.pdf"}}]}`, nil), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-5")
	_, _ = tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"multi-agent","limit":1}`})
	cid := tool.candidates["sess-5"][0].CandidateID
	dir := t.TempDir()
	resp, _ := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","candidate_id":%q,"dry_run":true,"destination":%q}`, cid, dir)})
	if resp.IsError || !strings.Contains(resp.Content, "Dry run OK") {
		t.Fatalf("expected dry run response, got: %s", resp.Content)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("dry run should not write files")
	}
}

func TestScholarSearch_SemanticScholar429Metadata(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(429, `{"error":"rate limit"}`, map[string]string{"Retry-After": "7"}), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","source":"semantic_scholar","query":"vision language model","limit":3,"offset":2}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected error response")
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata is not valid json: %v", err)
	}
	if md["error_kind"] != "rate_limited" || md["provider"] != "semantic_scholar" || md["tool"] != "ScholarSearch" {
		t.Fatalf("unexpected metadata: %v", md)
	}
	if md["action"] != "search" || md["query_key"] != "search:semantic_scholar:vision_language_model:2:3" {
		t.Fatalf("unexpected query metadata: %v", md)
	}
}

func TestScholarSearch_CooldownMetadataAndNoNetwork(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return newJSONResp(200, `{}`, nil), nil
	}, nil)
	scholarRateMu.Lock()
	scholarCooldownUntil = time.Now().Add(20 * time.Second)
	scholarRateMu.Unlock()
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","source":"semantic_scholar","query":"test query","limit":1}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("expected no network calls during cooldown")
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata is not valid json: %v", err)
	}
	if md["error_kind"] != "provider_cooldown" || md["source"] != "semantic_scholar" {
		t.Fatalf("unexpected metadata: %v", md)
	}
	if md["provider"] != "semantic_scholar" || md["progress_kind"] != "none" || md["query_key"] == "" {
		t.Fatalf("expected provider/progress/query metadata, got: %v", md)
	}
}

func TestScholarSearch_WithSourceSuggestionsProviderFallback(t *testing.T) {
	md := withSourceSuggestions(map[string]any{
		"source":        "arxiv",
		"error_kind":    "temporary_failure",
		"query_key":     "search:arxiv:test:0:5",
		"progress_kind": "",
	})
	if md["provider"] != "arxiv" {
		t.Fatalf("expected provider fallback to source, got: %v", md["provider"])
	}
	if md["progress_kind"] != "none" {
		t.Fatalf("expected none progress for error metadata, got: %v", md["progress_kind"])
	}
}

func TestScholarSearchSuccessResponseAddsCandidateEvidence(t *testing.T) {
	resp := scholarSearchSuccessResponse("CandidateID: cand_1\nTitle: Stable One\nDOI: 10.1234/abc\nCandidateID: cand_2\nTitle: Stable Two\nArXiv: 2501.00001\n", map[string]any{"source": "arxiv"}, "search", "arxiv", "multi agent", 5, 0)
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if md["provider"] != "arxiv" || md["candidate_count"].(float64) != 2 {
		t.Fatalf("expected provider and candidate_count metadata, got: %v", md)
	}
	keys, ok := md["evidence_keys"].([]any)
	if !ok || len(keys) != 2 {
		t.Fatalf("expected candidate evidence keys, got: %v", md["evidence_keys"])
	}
	for _, key := range keys {
		if strings.HasPrefix(key.(string), "candidate_id:") {
			t.Fatalf("candidate_id should not be used as durable evidence: %v", keys)
		}
	}
}

func TestStableScholarEvidenceKeysFromContentDoesNotUseCandidateID(t *testing.T) {
	content := "CandidateID: cand_random\nTitle: A Repeatable Paper\nPDF: https://example.com/p.pdf\n"
	first := stableScholarEvidenceKeysFromContent(content, 5)
	second := stableScholarEvidenceKeysFromContent(strings.ReplaceAll(content, "cand_random", "cand_other"), 5)
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("expected stable evidence keys, got %v / %v", first, second)
	}
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatalf("candidate id changes should not change evidence keys: %v vs %v", first, second)
	}
}

func TestScholarSearch_DownloadAllowUnverifiedRateLimitMetadata(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(429, `{"error":"rate limit"}`, map[string]string{"Retry-After": "9"}), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"download","id":"p1","allow_unverified":true}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected error response")
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata is not valid json: %v", err)
	}
	if md["error_kind"] != "rate_limited" || md["action"] != "download" || md["provider"] != "semantic_scholar" {
		t.Fatalf("unexpected metadata: %v", md)
	}
}

func TestScholarSearch_OpenAlexNotBlockedBySemanticCooldown(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{}`, nil), nil
	}, nil)
	withDefaultHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Host, "openalex.org") {
			t.Fatalf("unexpected host: %s", r.URL.Host)
		}
		return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W1","title":"T","publication_year":2024,"cited_by_count":2,"doi":"https://doi.org/10.1/t","authorships":[{"author":{"display_name":"A"}}],"primary_location":{"source":{"display_name":"V","type":"journal"}}}],"meta":{"count":1}}`, nil), nil
	})
	scholarRateMu.Lock()
	scholarCooldownUntil = time.Now().Add(20 * time.Second)
	scholarRateMu.Unlock()
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","source":"openalex","query":"vlm","limit":1}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError {
		t.Fatalf("expected openalex to bypass semantic cooldown: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "OpenAlex Search") {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
	if strings.Contains(resp.Content, "Abstract:") || strings.Contains(resp.Content, "BibTeX:") {
		t.Fatalf("explicit OpenAlex output should be compact: %s", resp.Content)
	}
}

func TestScholarSearch_ExplicitCrossRefCompactsDedupesAndSkipsBlankTitle(t *testing.T) {
	withDefaultHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Host, "crossref.org") {
			t.Fatalf("unexpected host: %s", r.URL.Host)
		}
		return newJSONResp(200, `{"message":{"total-results":3,"items":[{"DOI":"10.1/a.v1","title":["Same Paper"],"author":[{"given":"Ada","family":"Lovelace"}],"published-online":{"date-parts":[[2026]]},"container-title":["Robot Learning"],"type":"journal-article","is-referenced-by-count":1,"abstract":"too much detail"},{"DOI":"10.1/a.v2","title":["Same Paper"],"author":[{"given":"Ada","family":"Lovelace"}],"published-online":{"date-parts":[[2026]]},"container-title":["Robot Learning"],"type":"journal-article","is-referenced-by-count":1},{"DOI":"10.1/blank","title":[""],"published-online":{"date-parts":[[2026]]},"type":"journal-article"}]}}`, nil), nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","source":"crossref","query":"same paper","limit":3}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError {
		t.Fatalf("unexpected error response: %s", resp.Content)
	}
	if got := strings.Count(resp.Content, "CandidateID:"); got != 1 {
		t.Fatalf("expected one usable deduped candidate, got %d in: %s", got, resp.Content)
	}
	if strings.Contains(resp.Content, "Abstract:") || strings.Contains(resp.Content, "BibTeX:") {
		t.Fatalf("explicit CrossRef output should be compact: %s", resp.Content)
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if md["candidate_count"] != float64(1) || md["compact_output"] != true {
		t.Fatalf("unexpected compact metadata: %v", md)
	}
}

func TestScholarSearch_DownloadRejectsOversizedPDFBeforeWrite(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Multi-Agent Planning","year":2026,"venue":"NeurIPS","abstract":"multi-agent systems","externalIds":{"ArXiv":"2601.00001"},"openAccessPdf":{"url":"https://example.test/a.pdf"}}]}`, nil), nil
	}, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			ContentLength: scholarMaxPDFBytes + 1,
			Body:          ioNopCloser(strings.NewReader("")),
		}, nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-large")
	_, _ = tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"multi-agent","limit":1}`})
	cid := tool.candidates["sess-large"][0].CandidateID
	dir := t.TempDir()
	resp, err := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","candidate_id":%q,"destination":%q}`, cid, dir)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !resp.IsError || !strings.Contains(resp.Content, "file too large") {
		t.Fatalf("expected oversized rejection, got: %s", resp.Content)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no files written")
	}
}

func TestValidateDownloadCandidate_MatchesHyphenatedTopic(t *testing.T) {
	candidate := scholarCandidate{Paper: paperResult{
		Title:    "Multi-Agent Planning for Research Teams",
		Year:     2026,
		Venue:    "NeurIPS",
		Abstract: "A study of multi agent systems.",
	}}
	verdict := validateDownloadCandidate(candidate, scholarSearchParams{
		Topic:           "multiagent systems",
		RequiredTerms:   []string{"multi-agent"},
		YearMin:         FlexibleInt(2026),
		YearMax:         FlexibleInt(2026),
		RequireTopVenue: true,
	})
	if !verdict.Allowed {
		t.Fatalf("expected hyphenated topic to pass, got: %v", verdict.Reasons)
	}
}

func TestValidateDownloadCandidate_RejectsUnknownYearWhenBounded(t *testing.T) {
	candidate := scholarCandidate{Paper: paperResult{
		Title:    "Multi-Agent Planning",
		Venue:    "NeurIPS",
		Abstract: "multi-agent systems",
	}}
	verdict := validateDownloadCandidate(candidate, scholarSearchParams{YearMin: FlexibleInt(2026)})
	if verdict.Allowed || !strings.Contains(strings.Join(verdict.Reasons, "; "), "year is unknown") {
		t.Fatalf("expected unknown year rejection, got: %#v", verdict)
	}
}

func TestOpenAlexSearchEmitsCandidateID(t *testing.T) {
	withDefaultHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		body := `{"meta":{"count":1},"results":[{"id":"https://openalex.org/W1","title":"Multi-Agent Planning","publication_year":2026,"cited_by_count":7,"doi":"https://doi.org/10.1000/test","authorships":[{"author":{"display_name":"Ada Lovelace"}}],"primary_location":{"source":{"display_name":"NeurIPS","type":"conference"}},"open_access":{"is_oa":true,"oa_url":"https://example.test/paper.pdf"},"abstract_inverted_index":{"multi":[0],"agent":[1],"planning":[2]}}]}`
		return newJSONResp(200, body, nil), nil
	})
	var candidates []paperResult
	result, meta, err := openAlexSearchWithCandidates(context.Background(), "multi-agent", 1, 0, func(rank int, p paperResult) string {
		candidates = append(candidates, p)
		return "cand_openalex"
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !strings.Contains(result, "CandidateID: cand_openalex") {
		t.Fatalf("missing CandidateID in OpenAlex output: %s", result)
	}
	if len(candidates) != 1 || candidates[0].OpenAccessPdf == nil || candidates[0].OpenAccessPdf.URL == "" {
		t.Fatalf("expected registered OpenAlex PDF candidate, got %#v", candidates)
	}
	if meta["source"] != "openalex" || meta["action"] != "search" {
		t.Fatalf("unexpected metadata: %v", meta)
	}
}

func TestUnpaywallLookupEmitsCandidateID(t *testing.T) {
	withDefaultHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		body := `{"doi":"10.1000/test","title":"Multi-Agent Planning","year":2026,"is_oa":true,"oa_status":"gold","journal_name":"NeurIPS","best_oa_location":{"url_for_pdf":"https://example.test/paper.pdf","host_type":"publisher","version":"publishedVersion"},"z_authors":[{"given":"Ada","family":"Lovelace"}]}`
		return newJSONResp(200, body, nil), nil
	})
	var candidates []paperResult
	result, _, err := unpaywallLookupWithCandidate(context.Background(), "10.1000/test", func(rank int, p paperResult) string {
		candidates = append(candidates, p)
		return "cand_unpaywall"
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if !strings.Contains(result, "CandidateID: cand_unpaywall") {
		t.Fatalf("missing CandidateID in Unpaywall output: %s", result)
	}
	if len(candidates) != 1 || candidates[0].OpenAccessPdf == nil || candidates[0].OpenAccessPdf.URL == "" {
		t.Fatalf("expected registered Unpaywall PDF candidate, got %#v", candidates)
	}
}

func TestScholarAPIGet_RateLimitCooldown(t *testing.T) {
	var count atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		count.Add(1)
		return newJSONResp(429, `{"error":"rate limited"}`, map[string]string{"Retry-After": "60"}), nil
	}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, _ = scholarAPIGet(ctx, "/paper/search?query=x")
	before := count.Load()
	_, err := scholarAPIGet(context.Background(), "/paper/search?query=x")
	if err == nil || !strings.Contains(err.Error(), "cooldown active") {
		t.Fatalf("expected cooldown error, got: %v", err)
	}
	if count.Load() != before {
		t.Fatalf("expected no extra network call during cooldown")
	}
}

func TestScholarSearch_DefaultsActionToSearch(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":0,"offset":0,"data":[]}`, nil), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"query":"test query"}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError || !strings.Contains(resp.Content, "Scholar Search") {
		t.Fatalf("expected default search behavior, got: %s", resp.Content)
	}
}

func TestScholarSearch_SemanticScholarCacheHit(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return newJSONResp(200, `{"total":0,"offset":0,"data":[]}`, nil), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	input := `{"action":"search","source":"semantic_scholar","query":"cache me","limit":1}`
	resp1, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: input})
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	resp2, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: input})
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one Semantic Scholar network call, got %d", calls.Load())
	}
	var md1, md2 map[string]any
	if err := json.Unmarshal([]byte(resp1.Metadata), &md1); err != nil {
		t.Fatalf("first metadata is not valid json: %v", err)
	}
	if err := json.Unmarshal([]byte(resp2.Metadata), &md2); err != nil {
		t.Fatalf("second metadata is not valid json: %v", err)
	}
	if md1["cache_hit"] != false || md2["cache_hit"] != true {
		t.Fatalf("unexpected cache metadata: first=%v second=%v", md1, md2)
	}
}

func TestScholarSearch_SemanticScholarRetryMetadata(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return newJSONResp(502, "bad gateway", nil), nil
		}
		return newJSONResp(200, `{"total":0,"offset":0,"data":[]}`, nil), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","source":"semantic_scholar","query":"retry metadata","limit":1}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError {
		t.Fatalf("expected successful retry, got: %s", resp.Content)
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata is not valid json: %v", err)
	}
	if md["attempts"] != float64(2) || md["retry_count"] != float64(1) {
		t.Fatalf("unexpected retry metadata: %v", md)
	}
}

func TestArxivSearch_ExactIDUsesIDQuery(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("search_query")
		if !strings.HasPrefix(q, "id:") {
			t.Fatalf("expected id query, got %s", q)
		}
		if strings.HasPrefix(q, "all:") {
			t.Fatalf("unexpected broad all query: %s", q)
		}
		return newJSONResp(200, `<feed><totalResults>0</totalResults></feed>`, nil), nil
	}, nil)
	_, _, err := arxivSearchWithCandidates(context.Background(), "arXiv:2301.12345v1", 1, 0, nil)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
}

func TestArxivSearch_MalformedVersionUsesBroadQuery(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("search_query")
		if !strings.HasPrefix(q, "all:") {
			t.Fatalf("expected broad all query for malformed version, got %s", q)
		}
		return newJSONResp(200, `<feed><totalResults>0</totalResults></feed>`, nil), nil
	}, nil)
	_, _, err := arxivSearchWithCandidates(context.Background(), "2301.12345vtransformers", 1, 0, nil)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
}

func TestUnpaywallLookup_404IsNoResultsNotCooldown(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return newJSONResp(404, `{"error":"not found"}`, nil), nil
	}, nil)
	result, meta, err := unpaywallLookupWithCandidate(context.Background(), "10.1000/missing", nil)
	if err != nil {
		t.Fatalf("expected not found response without error, got: %v", err)
	}
	if !strings.Contains(result, "not found") {
		t.Fatalf("unexpected result: %s", result)
	}
	if meta["error_kind"] != "no_results" {
		t.Fatalf("expected no_results metadata, got: %v", meta)
	}
	if _, _, err := unpaywallLookupWithCandidate(context.Background(), "10.1000/missing", nil); err != nil {
		t.Fatalf("second not-found lookup should not hit cooldown: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected no-results not to enter cooldown/cache, got %d calls", calls.Load())
	}
}

func TestPubMedSearch_NoResultsCacheMetadata(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return newJSONResp(200, `{"esearchresult":{"idlist":[],"count":"0"}}`, nil), nil
	}, nil)
	_, meta1, err1 := pubmedSearchWithCandidates(context.Background(), "no hits", 2, 0, nil)
	if err1 != nil {
		t.Fatalf("first call failed: %v", err1)
	}
	_, meta2, err2 := pubmedSearchWithCandidates(context.Background(), "no hits", 2, 0, nil)
	if err2 != nil {
		t.Fatalf("second call failed: %v", err2)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one PubMed eSearch call, got %d", calls.Load())
	}
	if meta1["cache_hit"] != false || meta2["cache_hit"] != true {
		t.Fatalf("unexpected no-results cache metadata: first=%v second=%v", meta1, meta2)
	}
}

func TestOpenAlexSearch_CacheHitOnExactKey(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return newJSONResp(200, `{"results":[],"meta":{"count":0}}`, nil), nil
	}, nil)
	_, meta1, err1 := openAlexSearchWithCandidates(context.Background(), "vlm", 3, 2, nil)
	if err1 != nil {
		t.Fatalf("first call failed: %v", err1)
	}
	_, meta2, err2 := openAlexSearchWithCandidates(context.Background(), "vlm", 3, 2, nil)
	if err2 != nil {
		t.Fatalf("second call failed: %v", err2)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one network call, got %d", calls.Load())
	}
	if meta1["cache_hit"] != false || meta2["cache_hit"] != true {
		t.Fatalf("unexpected cache metadata: first=%v second=%v", meta1, meta2)
	}
}

func TestOpenAlexSearch_RetryOnReadError(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) < 3 {
			return &http.Response{StatusCode: 200, Body: failingReadCloser{}}, nil
		}
		return newJSONResp(200, `{"results":[],"meta":{"count":0}}`, nil), nil
	}, nil)
	_, meta, err := openAlexSearchWithCandidates(context.Background(), "read retry", 1, 0, nil)
	if err != nil {
		t.Fatalf("expected read retry success, got: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected three attempts after read failures, got %d", calls.Load())
	}
	if meta["attempts"] != 3 || meta["retry_count"] != 2 {
		t.Fatalf("unexpected read retry metadata: %v", meta)
	}
}

func TestOpenAlexSearch_SingleflightDeduplicatesInFlightQuery(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return newJSONResp(200, `{"results":[],"meta":{"count":0}}`, nil), nil
	}, nil)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	run := func() {
		defer wg.Done()
		_, _, err := openAlexSearchWithCandidates(context.Background(), "same query", 1, 0, nil)
		errCh <- err
	}

	wg.Add(1)
	go run()
	<-entered
	wg.Add(1)
	go run()
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one network call for in-flight duplicate query, got %d", calls.Load())
	}
}

func TestOpenAlexSearch_CacheHitBypassesSourceCooldown(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		switch r.URL.Query().Get("search") {
		case "cached":
			return newJSONResp(200, `{"results":[],"meta":{"count":0}}`, nil), nil
		case "limited":
			return newJSONResp(429, `{"error":"limited"}`, map[string]string{"Retry-After": "60"}), nil
		default:
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
			return nil, nil
		}
	}, nil)
	_, _, err := openAlexSearchWithCandidates(context.Background(), "cached", 1, 0, nil)
	if err != nil {
		t.Fatalf("cache seed failed: %v", err)
	}
	if _, _, err := openAlexSearchWithCandidates(context.Background(), "limited", 1, 0, nil); err == nil {
		t.Fatal("expected rate limit error")
	}
	_, meta, err := openAlexSearchWithCandidates(context.Background(), "cached", 1, 0, nil)
	if err != nil {
		t.Fatalf("cached query should bypass cooldown: %v", err)
	}
	if meta["cache_hit"] != true {
		t.Fatalf("expected cache hit while source cooldown is active, got: %v", meta)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected only seed and rate-limit network calls, got %d", calls.Load())
	}
}

func TestScholarCache_PrunesExpiredEntries(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"results":[],"meta":{"count":0}}`, nil), nil
	}, nil)
	scholarHTTPStateMu.Lock()
	scholarCacheByKey["expired"] = scholarCacheEntry{body: []byte("stale"), expiresAt: time.Now().Add(-time.Second)}
	scholarHTTPStateMu.Unlock()
	if _, _, err := openAlexSearchWithCandidates(context.Background(), "fresh", 1, 0, nil); err != nil {
		t.Fatalf("fresh search failed: %v", err)
	}
	scholarHTTPStateMu.Lock()
	_, stillPresent := scholarCacheByKey["expired"]
	scholarHTTPStateMu.Unlock()
	if stillPresent {
		t.Fatal("expected expired cache entry to be pruned")
	}
}

func TestScholarSearch_AdditionalSourcesUseIsolatedMetadata(t *testing.T) {
	var coreCalls, ericCalls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.core.ac.uk":
			coreCalls.Add(1)
			return newJSONResp(200, `{"totalHits":0,"results":[]}`, nil), nil
		case "api.ies.ed.gov":
			ericCalls.Add(1)
			return newJSONResp(200, `{"response":{"numFound":0,"docs":[]}}`, nil), nil
		default:
			t.Fatalf("unexpected host: %s", r.URL.Host)
			return nil, nil
		}
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	coreResp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","source":"core","query":"shared","limit":1}`})
	if err != nil {
		t.Fatalf("core run failed: %v", err)
	}
	ericResp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","source":"eric","query":"shared","limit":1}`})
	if err != nil {
		t.Fatalf("eric run failed: %v", err)
	}
	var coreMeta, ericMeta map[string]any
	if err := json.Unmarshal([]byte(coreResp.Metadata), &coreMeta); err != nil {
		t.Fatalf("core metadata is not valid json: %v", err)
	}
	if err := json.Unmarshal([]byte(ericResp.Metadata), &ericMeta); err != nil {
		t.Fatalf("eric metadata is not valid json: %v", err)
	}
	if coreMeta["source"] != "core" || ericMeta["source"] != "eric" {
		t.Fatalf("unexpected source metadata: core=%v eric=%v", coreMeta, ericMeta)
	}
	if coreCalls.Load() != 1 || ericCalls.Load() != 1 {
		t.Fatalf("expected isolated source calls, got core=%d eric=%d", coreCalls.Load(), ericCalls.Load())
	}
}

func TestScholarAPIGet_RetryOn5xx(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return newJSONResp(502, "bad gateway", nil), nil
	}, nil)
	_, err := scholarAPIGet(context.Background(), "/paper/search?query=retry")
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 3 {
		t.Fatalf("expected bounded Semantic Scholar retries (3 attempts), got %d", calls.Load())
	}
}

func TestCrossrefSearch_RetryOn5xxAndStop(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return newJSONResp(502, "bad gateway", nil), nil
	}, nil)
	_, meta, err := crossrefSearchWithCandidates(context.Background(), "abc", 1, 0, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 3 {
		t.Fatalf("expected bounded retries (3 attempts), got %d", calls.Load())
	}
	if meta["error_kind"] != "temporary_failure" {
		t.Fatalf("unexpected metadata: %v", meta)
	}
}

func TestCrossrefSearch_NoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return newJSONResp(400, "bad request", nil), nil
	}, nil)
	_, meta, err := crossrefSearchWithCandidates(context.Background(), "abc", 1, 0, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Fatalf("expected no retry for 4xx, got %d calls", calls.Load())
	}
	if meta["error_kind"] != "non_retryable" {
		t.Fatalf("unexpected metadata: %v", meta)
	}
}

func TestScholarWorkflow_FallbackFromSemantic429ToOpenAlex(t *testing.T) {
	var semanticCalls, openalexCalls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.semanticscholar.org":
			semanticCalls.Add(1)
			return newJSONResp(429, `{"error":"rate limit"}`, map[string]string{"Retry-After": "10"}), nil
		case "api.openalex.org":
			openalexCalls.Add(1)
			return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W1","title":"Fallback Paper","publication_year":2024,"cited_by_count":2,"doi":"https://doi.org/10.1/fallback","authorships":[{"author":{"display_name":"A"}}],"primary_location":{"source":{"display_name":"V","type":"journal"}}}],"meta":{"count":1}}`, nil), nil
		default:
			t.Fatalf("unexpected host: %s", r.URL.Host)
			return nil, nil
		}
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"fallback test","limit":1}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError || !strings.Contains(resp.Content, "Selected source: openalex") {
		t.Fatalf("expected openalex fallback success, got: %s", resp.Content)
	}
	if semanticCalls.Load() != 1 || openalexCalls.Load() != 1 {
		t.Fatalf("unexpected call counts: semantic=%d openalex=%d", semanticCalls.Load(), openalexCalls.Load())
	}
}

func TestScholarWorkflow_FallbackFromSemanticEmptyToOpenAlex(t *testing.T) {
	var semanticCalls, openalexCalls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.semanticscholar.org":
			semanticCalls.Add(1)
			return newJSONResp(200, `{"total":0,"offset":0,"data":[]}`, nil), nil
		case "api.openalex.org":
			openalexCalls.Add(1)
			return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W1","title":"Fallback Paper","publication_year":2024,"cited_by_count":2,"doi":"https://doi.org/10.1/fallback","authorships":[{"author":{"display_name":"A"}}],"primary_location":{"source":{"display_name":"V","type":"journal"}}}],"meta":{"count":1}}`, nil), nil
		default:
			t.Fatalf("unexpected host: %s", r.URL.Host)
			return nil, nil
		}
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"empty fallback","limit":1}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError || !strings.Contains(resp.Content, "Selected source: openalex") || !strings.Contains(resp.Content, "CandidateID:") {
		t.Fatalf("expected openalex fallback success, got: %s", resp.Content)
	}
	if semanticCalls.Load() != 1 || openalexCalls.Load() != 1 {
		t.Fatalf("unexpected call counts: semantic=%d openalex=%d", semanticCalls.Load(), openalexCalls.Load())
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if md["workflow"] != "scholar_search" || md["candidate_count"].(float64) != 1 {
		t.Fatalf("unexpected workflow metadata: %v", md)
	}
}

func TestScholarWorkflow_DefaultSearchHonorsYearMin(t *testing.T) {
	var semanticCalls, openalexCalls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.semanticscholar.org":
			semanticCalls.Add(1)
			return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"old-semantic","title":"Old Semantic Paper","year":2020,"venue":"S","citationCount":10}]}`, nil), nil
		case "api.openalex.org":
			openalexCalls.Add(1)
			return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W1","title":"Recent OpenAlex Paper","publication_year":2025,"cited_by_count":4,"doi":"https://doi.org/10.1/recent","authorships":[{"author":{"display_name":"A"}}],"primary_location":{"source":{"display_name":"V","type":"journal"}}}],"meta":{"count":1}}`, nil), nil
		default:
			t.Fatalf("unexpected host: %s", r.URL.Host)
			return nil, nil
		}
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"year filtered","limit":1,"year_min":2024}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError || !strings.Contains(resp.Content, "Recent OpenAlex Paper") {
		t.Fatalf("expected recent OpenAlex fallback, got: %s", resp.Content)
	}
	if strings.Contains(resp.Content, "Old Semantic Paper") {
		t.Fatalf("old semantic paper should be filtered out: %s", resp.Content)
	}
	if semanticCalls.Load() != 1 || openalexCalls.Load() != 1 {
		t.Fatalf("unexpected call counts: semantic=%d openalex=%d", semanticCalls.Load(), openalexCalls.Load())
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if md["year_min"].(float64) != 2024 {
		t.Fatalf("expected year_min metadata, got %v", md)
	}
}

func TestScholarWorkflow_FilteredParallelSizeSearchTriesLaterSources(t *testing.T) {
	var semanticCalls, openalexCalls, arxivCalls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.semanticscholar.org":
			semanticCalls.Add(1)
			return newJSONResp(200, `{"total":3,"offset":0,"data":[{"paperId":"old-semantic-1","title":"Old Semantic 1","year":2020},{"paperId":"old-semantic-2","title":"Old Semantic 2","year":2020},{"paperId":"old-semantic-3","title":"Old Semantic 3","year":2020}]}`, nil), nil
		case "api.openalex.org":
			openalexCalls.Add(1)
			return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W1","title":"Old OpenAlex 1","publication_year":2020},{"id":"https://openalex.org/W2","title":"Old OpenAlex 2","publication_year":2020},{"id":"https://openalex.org/W3","title":"Old OpenAlex 3","publication_year":2020}],"meta":{"count":3}}`, nil), nil
		case "export.arxiv.org":
			arxivCalls.Add(1)
			return newJSONResp(200, `<feed><totalResults>3</totalResults><entry><id>http://arxiv.org/abs/2501.00001v1</id><title>Recent arXiv 1</title><summary>A</summary><published>2025-01-01T00:00:00Z</published><author><name>Ada Lovelace</name></author><link href="http://arxiv.org/pdf/2501.00001v1" type="application/pdf"/></entry><entry><id>http://arxiv.org/abs/2501.00002v1</id><title>Recent arXiv 2</title><summary>B</summary><published>2025-01-02T00:00:00Z</published><author><name>Grace Hopper</name></author><link href="http://arxiv.org/pdf/2501.00002v1" type="application/pdf"/></entry><entry><id>http://arxiv.org/abs/2501.00003v1</id><title>Recent arXiv 3</title><summary>C</summary><published>2025-01-03T00:00:00Z</published><author><name>Katherine Johnson</name></author><link href="http://arxiv.org/pdf/2501.00003v1" type="application/pdf"/></entry></feed>`, nil), nil
		default:
			t.Fatalf("unexpected host: %s", r.URL.Host)
			return nil, nil
		}
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	wf := tool.runSearchWorkflow(context.Background(), "year filtered", 5, 0, []string{"semantic_scholar", "openalex", "arxiv"}, scholarSearchFilters{YearMin: 2024})
	if len(wf.Candidates) == 0 || !strings.Contains(wf.Candidates[0].Paper.Title, "Recent arXiv") {
		t.Fatalf("expected later arxiv candidates after filtering early sources, got %#v", wf.Candidates)
	}
	if semanticCalls.Load() != 1 || openalexCalls.Load() != 1 || arxivCalls.Load() != 1 {
		t.Fatalf("unexpected call counts: semantic=%d openalex=%d arxiv=%d", semanticCalls.Load(), openalexCalls.Load(), arxivCalls.Load())
	}
}

func TestScholarSearch_ExplicitOpenAlexHonorsYearMin(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.openalex.org" {
			t.Fatalf("unexpected host: %s", r.URL.Host)
		}
		return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W1","title":"Old OpenAlex","publication_year":2020,"cited_by_count":9,"doi":"https://doi.org/10.1/old","authorships":[{"author":{"display_name":"A"}}],"primary_location":{"source":{"display_name":"V","type":"journal"}}},{"id":"https://openalex.org/W2","title":"Recent OpenAlex","publication_year":2025,"cited_by_count":4,"doi":"https://doi.org/10.1/recent","authorships":[{"author":{"display_name":"B"}}],"primary_location":{"source":{"display_name":"V","type":"journal"}}}],"meta":{"count":2}}`, nil), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","source":"openalex","query":"year filtered explicit","limit":2,"year_min":2024}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError || !strings.Contains(resp.Content, "Recent OpenAlex") {
		t.Fatalf("expected recent result, got: %s", resp.Content)
	}
	if strings.Contains(resp.Content, "Old OpenAlex") {
		t.Fatalf("old result should be filtered out: %s", resp.Content)
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if md["filtered_by_year_count"].(float64) != 1 {
		t.Fatalf("expected one filtered result, got %v", md)
	}
}

func TestScholarSearch_UnpaywallHonorsYearMin(t *testing.T) {
	withDefaultHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		body := `{"doi":"10.1000/old","title":"Old Unpaywall Paper","year":2020,"is_oa":true,"oa_status":"gold","journal_name":"Old Journal","best_oa_location":{"url_for_pdf":"https://example.test/old.pdf","host_type":"publisher","version":"publishedVersion"},"z_authors":[{"given":"Ada","family":"Lovelace"}]}`
		return newJSONResp(200, body, nil), nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","source":"unpaywall","query":"10.1000/old","year_min":2024}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError {
		t.Fatalf("expected filtered no-results response, got error: %s", resp.Content)
	}
	if strings.Contains(resp.Content, "Old Unpaywall Paper") || strings.Contains(resp.Content, "CandidateID:") {
		t.Fatalf("old unpaywall result should be filtered out: %s", resp.Content)
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if md["filtered_by_year_count"].(float64) != 1 || md["candidate_count"].(float64) != 0 {
		t.Fatalf("unexpected filter metadata: %v", md)
	}
}

func TestScholarWorkflow_DefaultAggregatesMultipleSourcesWhenLimitNeedsFill(t *testing.T) {
	var semanticCalls, openalexCalls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.semanticscholar.org":
			semanticCalls.Add(1)
			return newJSONResp(200, `{"total":3,"offset":0,"data":[{"paperId":"s1","title":"Semantic One","year":2024,"venue":"S","citationCount":1},{"paperId":"s2","title":"Semantic Two","year":2024,"venue":"S","citationCount":2},{"paperId":"s3","title":"Semantic Three","year":2024,"venue":"S","citationCount":3}]}`, nil), nil
		case "api.openalex.org":
			openalexCalls.Add(1)
			return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W1","title":"OpenAlex One","publication_year":2025,"cited_by_count":4,"doi":"https://doi.org/10.1/oa1","authorships":[{"author":{"display_name":"A"}}],"primary_location":{"source":{"display_name":"V","type":"journal"}}},{"id":"https://openalex.org/W2","title":"OpenAlex Two","publication_year":2025,"cited_by_count":5,"doi":"https://doi.org/10.1/oa2","authorships":[{"author":{"display_name":"B"}}],"primary_location":{"source":{"display_name":"V","type":"journal"}}}],"meta":{"count":2}}`, nil), nil
		default:
			t.Fatalf("unexpected host: %s", r.URL.Host)
			return nil, nil
		}
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"aggregate test","limit":5}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError {
		t.Fatalf("unexpected error response: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "Sources used: semantic_scholar, openalex") {
		t.Fatalf("expected multiple sources in compact output, got: %s", resp.Content)
	}
	if got := strings.Count(resp.Content, "CandidateID:"); got != 5 {
		t.Fatalf("expected 5 candidates, got %d in: %s", got, resp.Content)
	}
	if semanticCalls.Load() != 1 || openalexCalls.Load() != 1 {
		t.Fatalf("unexpected call counts: semantic=%d openalex=%d", semanticCalls.Load(), openalexCalls.Load())
	}
}

func TestScholarWorkflow_DefaultSearchOutputIsCompact(t *testing.T) {
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.semanticscholar.org" {
			t.Fatalf("unexpected host: %s", r.URL.Host)
		}
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Compact Paper","authors":[{"name":"Ada Lovelace"}],"year":2026,"venue":"NeurIPS","abstract":"This long abstract should not be included in compact search output.","citationCount":9,"externalIds":{"DOI":"10.1/compact"},"openAccessPdf":{"url":"https://example.test/compact.pdf"}}]}`, nil), nil
	}, nil)
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	resp, err := tool.Run(context.Background(), ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"compact","limit":1}`})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !strings.Contains(resp.Content, "CandidateID:") {
		t.Fatalf("expected candidate id in compact output: %s", resp.Content)
	}
	if strings.Contains(resp.Content, "Abstract:") || strings.Contains(resp.Content, "BibTeX:") {
		t.Fatalf("default search output should omit abstracts and BibTeX: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "Use action=details") {
		t.Fatalf("expected details guidance in compact output: %s", resp.Content)
	}
}

func TestScholarWorkflow_DedupeByDOIAndTitle(t *testing.T) {
	papers := []paperResult{
		{Title: "Multi-Agent Planning", ExternalIDs: externalIDs{DOI: "10.1000/x"}},
		{Title: "Different Title", ExternalIDs: externalIDs{DOI: "10.1000/x"}},
		{Title: "A New Paper", ExternalIDs: externalIDs{ArXiv: "2601.00001"}},
		{Title: "A New Paper!!"},
	}
	got := dedupeScholarPapers(papers)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique papers, got %d", len(got))
	}
}

func TestScholarWorkflow_DedupePapersPrefersDownloadableDuplicate(t *testing.T) {
	papers := []paperResult{
		{Title: "Same Paper", ExternalIDs: externalIDs{DOI: "10.1000/same"}},
		{Title: "Same Paper", ExternalIDs: externalIDs{DOI: "10.1000/same"}, OpenAccessPdf: &openAccessPdf{URL: "https://example.test/same.pdf"}},
	}
	got := dedupeScholarPapers(papers)
	if len(got) != 1 || paperPDFURL(got[0]) == "" {
		t.Fatalf("expected downloadable paper duplicate to be kept, got %#v", got)
	}
}

func TestScholarWorkflow_DedupePrefersDownloadableCandidate(t *testing.T) {
	candidates := []scholarCandidate{
		{CandidateID: "no-pdf", Source: "semantic_scholar", Paper: paperResult{Title: "Same Paper", ExternalIDs: externalIDs{DOI: "10.1000/same"}}},
		{CandidateID: "with-pdf", Source: "openalex", PDFURL: "https://example.test/same.pdf", Paper: paperResult{Title: "Same Paper", ExternalIDs: externalIDs{DOI: "10.1000/same"}}},
	}
	got := dedupeScholarCandidates(candidates)
	if len(got) != 1 || got[0].CandidateID != "with-pdf" {
		t.Fatalf("expected downloadable duplicate to be kept, got %#v", got)
	}
}

func TestScholarWorkflow_DownloadStopsAfterFirstValidCandidate(t *testing.T) {
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	candidates := []scholarCandidate{
		{CandidateID: "c1", Source: "semantic_scholar", PDFURL: "https://example.test/old.pdf", Paper: paperResult{Title: "Old", Year: 2020, Venue: "Other"}},
		{CandidateID: "c2", Source: "openalex", PDFURL: "https://example.test/good.pdf", Paper: paperResult{Title: "Good Multi-Agent", Year: 2026, Venue: "NeurIPS"}},
		{CandidateID: "c3", Source: "arxiv", PDFURL: "https://example.test/also.pdf", Paper: paperResult{Title: "Also Good", Year: 2026, Venue: "ICLR"}},
	}
	wf := tool.runDownloadWorkflow(candidates, scholarSearchParams{
		YearMin:         FlexibleInt(2026),
		RequireTopVenue: true,
		Topic:           "multi-agent",
	})
	if wf.Selected.CandidateID != "c2" {
		t.Fatalf("expected first valid candidate selected, got: %s", wf.Selected.CandidateID)
	}
	if len(wf.Attempts) != 2 {
		t.Fatalf("expected stop after first success with 2 attempts, got %d", len(wf.Attempts))
	}
}

func TestScholarWorkflow_DownloadRejectReasonVenueYear(t *testing.T) {
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	candidates := []scholarCandidate{
		{CandidateID: "c1", Source: "semantic_scholar", PDFURL: "https://example.test/qip.pdf", Paper: paperResult{Title: "Quantum Walks", Year: 2025, Venue: "QIP"}},
	}
	wf := tool.runDownloadWorkflow(candidates, scholarSearchParams{
		YearMin:         FlexibleInt(2026),
		RequireTopVenue: true,
		Topic:           "multi-agent systems",
	})
	if wf.Selected.CandidateID != "" {
		t.Fatalf("expected no selected candidate")
	}
	if len(wf.Attempts) != 1 || !strings.Contains(wf.Attempts[0].Reason, "year 2025 < year_min 2026") {
		t.Fatalf("expected year rejection reason, got: %#v", wf.Attempts)
	}
}

func TestScholarWorkflow_QueryDownloadFallsBackAndStopsAfterSuccess(t *testing.T) {
	var semanticCalls, openalexCalls, downloadCalls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.semanticscholar.org":
			semanticCalls.Add(1)
			return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p-old","title":"Quantum Walks","year":2024,"venue":"QIP","abstract":"spin systems"}]}`, nil), nil
		case "api.openalex.org":
			openalexCalls.Add(1)
			return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W1","title":"Good Multi-Agent Planning","publication_year":2026,"cited_by_count":4,"doi":"https://doi.org/10.1/good","authorships":[{"author":{"display_name":"A"}}],"primary_location":{"source":{"display_name":"NeurIPS","type":"conference"}},"open_access":{"is_oa":true,"oa_url":"https://example.test/good.pdf"}}],"meta":{"count":1}}`, nil), nil
		default:
			t.Fatalf("unexpected host: %s", r.URL.Host)
			return nil, nil
		}
	}, func(r *http.Request) (*http.Response, error) {
		downloadCalls.Add(1)
		return &http.Response{StatusCode: 200, Body: ioNopCloser(strings.NewReader("%PDF-1.4 good"))}, nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	dir := t.TempDir()
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-query-download")
	resp, err := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","query":"multi-agent planning","limit":1,"year_min":2026,"topic":"multi-agent","destination":%q}`, dir)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError || !strings.Contains(resp.Content, "Selected source: openalex") {
		t.Fatalf("expected query download fallback success, got: %s", resp.Content)
	}
	if semanticCalls.Load() != 1 || openalexCalls.Load() != 1 || downloadCalls.Load() != 1 {
		t.Fatalf("unexpected call counts: semantic=%d openalex=%d downloads=%d", semanticCalls.Load(), openalexCalls.Load(), downloadCalls.Load())
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata is not valid json: %v", err)
	}
	if md["progress_kind"] != "downloaded_artifact" || md["source"] != "openalex" {
		t.Fatalf("unexpected workflow metadata: %v", md)
	}
}

func TestScholarWorkflow_QueryDownloadDuplicateFallbackPrefersPDF(t *testing.T) {
	var semanticCalls, openalexCalls, downloadCalls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.semanticscholar.org":
			semanticCalls.Add(1)
			return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p1","title":"Same Multi-Agent Paper","year":2026,"venue":"NeurIPS","abstract":"multi-agent planning","externalIds":{"DOI":"10.1000/same"}}]}`, nil), nil
		case "api.openalex.org":
			openalexCalls.Add(1)
			return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W1","title":"Same Multi-Agent Paper","publication_year":2026,"cited_by_count":4,"doi":"https://doi.org/10.1000/same","authorships":[{"author":{"display_name":"A"}}],"primary_location":{"source":{"display_name":"NeurIPS","type":"conference"}},"open_access":{"is_oa":true,"oa_url":"https://example.test/same.pdf"}}],"meta":{"count":1}}`, nil), nil
		default:
			t.Fatalf("unexpected host: %s", r.URL.Host)
			return nil, nil
		}
	}, func(r *http.Request) (*http.Response, error) {
		downloadCalls.Add(1)
		return &http.Response{StatusCode: 200, Body: ioNopCloser(strings.NewReader("%PDF-1.4 same"))}, nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	dir := t.TempDir()
	resp, err := tool.Run(context.WithValue(context.Background(), SessionIDContextKey, "sess-dup-pdf"), ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","query":"same paper","limit":1,"year_min":2026,"topic":"multi-agent","destination":%q}`, dir)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError || !strings.Contains(resp.Content, "Selected source: openalex") {
		t.Fatalf("expected duplicate fallback to downloadable OpenAlex candidate, got: %s", resp.Content)
	}
	if semanticCalls.Load() != 1 || openalexCalls.Load() != 1 || downloadCalls.Load() != 1 {
		t.Fatalf("unexpected call counts: semantic=%d openalex=%d downloads=%d", semanticCalls.Load(), openalexCalls.Load(), downloadCalls.Load())
	}
}

func TestScholarWorkflow_QueryDownloadTriesNextCandidateAfterBrokenPDF(t *testing.T) {
	var semanticCalls, openalexCalls, downloadCalls atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.semanticscholar.org":
			semanticCalls.Add(1)
			return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p-bad","title":"Good Multi-Agent Planning","year":2026,"venue":"NeurIPS","abstract":"multi-agent planning","externalIds":{"DOI":"10.1000/bad"},"openAccessPdf":{"url":"https://example.test/bad.pdf"}}]}`, nil), nil
		case "api.openalex.org":
			openalexCalls.Add(1)
			return newJSONResp(200, `{"results":[{"id":"https://openalex.org/W2","title":"Better Multi-Agent Planning","publication_year":2026,"cited_by_count":6,"doi":"https://doi.org/10.1000/good","authorships":[{"author":{"display_name":"B"}}],"primary_location":{"source":{"display_name":"NeurIPS","type":"conference"}},"open_access":{"is_oa":true,"oa_url":"https://example.test/good.pdf"}}],"meta":{"count":1}}`, nil), nil
		default:
			t.Fatalf("unexpected host: %s", r.URL.Host)
			return nil, nil
		}
	}, func(r *http.Request) (*http.Response, error) {
		downloadCalls.Add(1)
		if strings.Contains(r.URL.Path, "bad.pdf") {
			return &http.Response{StatusCode: 404, Body: ioNopCloser(strings.NewReader("missing"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: ioNopCloser(strings.NewReader("%PDF-1.4 good"))}, nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	dir := t.TempDir()
	resp, err := tool.Run(context.WithValue(context.Background(), SessionIDContextKey, "sess-broken-pdf"), ToolCall{Name: "ScholarSearch", Input: fmt.Sprintf(`{"action":"download","query":"multi-agent planning","limit":1,"year_min":2026,"topic":"multi-agent","destination":%q}`, dir)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if resp.IsError || !strings.Contains(resp.Content, "Better Multi-Agent Planning") {
		t.Fatalf("expected fallback download success, got: %s", resp.Content)
	}
	if semanticCalls.Load() != 1 || openalexCalls.Load() != 1 || downloadCalls.Load() != 2 {
		t.Fatalf("unexpected call counts: semantic=%d openalex=%d downloads=%d", semanticCalls.Load(), openalexCalls.Load(), downloadCalls.Load())
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(resp.Metadata), &md); err != nil {
		t.Fatalf("metadata is not valid json: %v", err)
	}
	attempts, ok := md["attempt_summary"].([]any)
	if !ok || len(attempts) != 2 {
		t.Fatalf("expected failed and successful download attempts in metadata, got: %v", md["attempt_summary"])
	}
}

func TestScholarWorkflow_ExistingPDFReturnsWorkflowMetadata(t *testing.T) {
	var dlCount atomic.Int32
	withScholarClients(t, func(r *http.Request) (*http.Response, error) {
		return newJSONResp(200, `{"total":1,"offset":0,"data":[{"paperId":"p-existing","title":"Existing Multi-Agent Planning","year":2026,"venue":"NeurIPS","abstract":"multi-agent systems","externalIds":{"ArXiv":"2601.00002"},"openAccessPdf":{"url":"https://example.test/existing.pdf"}}]}`, nil), nil
	}, func(r *http.Request) (*http.Response, error) {
		dlCount.Add(1)
		return &http.Response{StatusCode: 200, Body: ioNopCloser(strings.NewReader("%PDF-1.4 existing"))}, nil
	})
	tool := NewScholarSearchTool(nil).(*scholarSearchTool)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-existing-metadata")
	_, _ = tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: `{"action":"search","query":"existing multi-agent","limit":1}`})
	cid := tool.candidates["sess-existing-metadata"][0].CandidateID
	dir := t.TempDir()
	input := fmt.Sprintf(`{"action":"download","candidate_id":%q,"destination":%q}`, cid, dir)
	first, err := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: input})
	if err != nil || first.IsError {
		t.Fatalf("first download failed: %v %s", err, first.Content)
	}
	second, err := tool.Run(ctx, ToolCall{Name: "ScholarSearch", Input: input})
	if err != nil {
		t.Fatalf("second download failed: %v", err)
	}
	if second.IsError || !strings.Contains(second.Content, "PDF already exists") {
		t.Fatalf("expected existing PDF success, got: %s", second.Content)
	}
	if dlCount.Load() != 1 {
		t.Fatalf("expected second call to avoid download, got %d downloads", dlCount.Load())
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(second.Metadata), &md); err != nil {
		t.Fatalf("metadata is not valid json: %v", err)
	}
	if md["workflow"] != "scholar_download" || md["progress_kind"] != "downloaded_artifact" || md["durable_progress"] != true {
		t.Fatalf("unexpected existing-file metadata: %v", md)
	}
}
