package tools

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/fileop"
	"github.com/openscholar/openscholar/internal/permission"
)

const (
	semanticScholarBaseURL = "https://api.semanticscholar.org/graph/v1"
	scholarDefaultFields   = "title,authors,year,venue,externalIds,abstract,citationCount,referenceCount,openAccessPdf"
	scholarBriefFields     = "title,authors,year,venue,externalIds,citationCount"

	scholarCandidateTTL        = 2 * time.Hour
	scholarMaxCandidates       = 100
	scholarMaxDownloadsPerTurn = 5
	scholarMaxPDFBytes         = 50 * 1024 * 1024
	scholarMaxBytesPerTurn     = 200 * 1024 * 1024
)

// Rate limiter: ensure at least 1s between requests to stay within Semantic Scholar free API limits.
var (
	scholarLastRequest        time.Time
	scholarRateMu             sync.Mutex
	scholarCooldownUntil      time.Time
	scholarHTTPClient         = &http.Client{Timeout: 30 * time.Second}
	scholarDownloadHTTPClient = &http.Client{Timeout: 120 * time.Second}
)

// --- API response types ---

type paperResult struct {
	PaperID        string         `json:"paperId"`
	Title          string         `json:"title"`
	Authors        []authorResult `json:"authors"`
	Year           int            `json:"year"`
	Venue          string         `json:"venue"`
	Abstract       string         `json:"abstract"`
	CitationCount  int            `json:"citationCount"`
	ReferenceCount int            `json:"referenceCount"`
	ExternalIDs    externalIDs    `json:"externalIds"`
	OpenAccessPdf  *openAccessPdf `json:"openAccessPdf"`
}

type authorResult struct {
	Name string `json:"name"`
}

type externalIDs struct {
	DOI   string `json:"DOI"`
	ArXiv string `json:"ArXiv"`
}

type openAccessPdf struct {
	URL string `json:"url"`
}

type searchResponse struct {
	Total  int           `json:"total"`
	Offset int           `json:"offset"`
	Data   []paperResult `json:"data"`
}

type citationWrapper struct {
	CitingPaper paperResult `json:"citingPaper"`
}

type referenceWrapper struct {
	CitedPaper paperResult `json:"citedPaper"`
}

type scholarRateLimitError struct {
	ErrorKind           string
	RetryAfterMs        int64
	CooldownUntilUnixMs int64
}

func (e *scholarRateLimitError) Error() string {
	if e == nil {
		return "semantic scholar rate limited"
	}
	switch e.ErrorKind {
	case "provider_cooldown":
		return "Semantic Scholar cooldown active"
	default:
		return "rate limited by Semantic Scholar API"
	}
}

// --- Tool implementation ---

type scholarSearchTool struct {
	permissions     permission.Service
	mu              sync.Mutex
	candidates      map[string][]scholarCandidate
	seq             uint64
	downloadBudgets map[string]downloadBudget
}

type scholarSearchParams struct {
	Action          string      `json:"action"`
	Query           string      `json:"query,omitempty"`
	ID              string      `json:"id,omitempty"`
	CandidateID     string      `json:"candidate_id,omitempty"`
	Limit           FlexibleInt `json:"limit,omitempty"`
	Offset          FlexibleInt `json:"offset,omitempty"`
	Source          string      `json:"source,omitempty"`      // "semantic_scholar" (default), "arxiv", "openalex", "crossref", "pubmed", "core", "eric", "patentsview", "europepmc", "unpaywall"
	Destination     string      `json:"destination,omitempty"` // custom download directory (optional)
	DryRun          bool        `json:"dry_run,omitempty"`
	AllowUnverified bool        `json:"allow_unverified,omitempty"`
	Topic           string      `json:"topic,omitempty"`
	RequiredTerms   []string    `json:"required_terms,omitempty"`
	YearMin         FlexibleInt `json:"year_min,omitempty"`
	YearMax         FlexibleInt `json:"year_max,omitempty"`
	VenueAllowlist  []string    `json:"venue_allowlist,omitempty"`
	RequireTopVenue bool        `json:"require_top_venue,omitempty"`
	AllowPreprint   bool        `json:"allow_preprint,omitempty"`
}

type scholarCandidate struct {
	CandidateID string
	SessionID   string
	Source      string
	Query       string
	Rank        int
	Paper       paperResult
	PDFURL      string
	CreatedAt   time.Time
}

type downloadValidation struct {
	Allowed bool
	Reasons []string
}

type downloadBudget struct {
	Count int
	Bytes int64
}

type scholarCandidateRegistrar func(rank int, p paperResult) string

var topVenueSubstrings = []string{
	"neurips", "icml", "iclr", "aaai", "ijcai", "aamas", "acl", "emnlp", "naacl", "cvpr", "eccv", "iccv", "kdd",
}

func NewScholarSearchTool(perms permission.Service) BaseTool {
	return &scholarSearchTool{
		permissions:     perms,
		candidates:      map[string][]scholarCandidate{},
		downloadBudgets: map[string]downloadBudget{},
	}
}

func (t *scholarSearchTool) Info() ToolInfo {
	return ToolInfo{
		Name:           "ScholarSearch",
		MaxResultBytes: 8 * 1024, // 8 KB
		Description:    "Search academic papers from multiple sources. For broad/long literature scans, prefer Task agent_type=research so raw hits stay isolated and only compressed evidence returns to the parent. Supports 10 search providers covering CS, biomedicine, education, patents, and cross-disciplinary OA literature. Also supports paper details, citation/reference traversal, PDF download, and DOI-based OA lookup via Unpaywall.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"search", "details", "citations", "references", "download"},
					"description": "Action: 'search' (keyword search), 'details' (single paper by ID/DOI/ArXiv), 'citations' (papers citing this paper), 'references' (this paper's references), 'download' (download PDF by candidate_id/id, or search+download one paper when query is provided; use 'destination' to specify directory or defaults to .openscholar/papers/). Note: details/citations/references/download id lookup use Semantic Scholar for metadata.",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Search keywords (required for action=search; optional for action=download to run a search+download workflow). For source=unpaywall, provide a DOI instead.",
				},
				"id": map[string]any{
					"type":        "string",
					"description": "Paper identifier: Semantic Scholar paperId, 'DOI:10.xxx/yyy', or 'ArXiv:2301.xxxxx' (required for action=details/citations/references)",
				},
				"candidate_id": map[string]any{
					"type":        "string",
					"description": "Candidate identifier returned by search/details. Preferred for action=download; prevents downloading unseen papers.",
				},
				"source": map[string]any{
					"type":        "string",
					"enum":        []string{"semantic_scholar", "arxiv", "openalex", "crossref", "pubmed", "core", "eric", "patentsview", "europepmc", "unpaywall"},
					"description": "Optional single-source override. Usually omit this: default search runs a bounded multi-source workflow and returns compact deduped candidates. Options: semantic_scholar (CS, general), arxiv (preprints), openalex (250M+ broad coverage), crossref (DOI/published papers), pubmed (biomedical), core (OA full-text, theses, repositories), eric (education research), patentsview (US patents, requires API key), europepmc (biomedical full-text), unpaywall (DOI→OA lookup).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max results (default 5, max 20)",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Pagination offset for search (default 0)",
				},
				"destination": map[string]any{
					"type":        "string",
					"description": "Optional destination directory for downloaded PDFs. Absolute or relative path (resolved against workspace). Defaults to .openscholar/papers/ or research workspace papers/.",
				},
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Validate candidate and show planned target path without writing files.",
				},
				"allow_unverified": map[string]any{
					"type":        "boolean",
					"description": "Compatibility escape hatch. Allows download by id without candidate binding after warning.",
				},
				"topic": map[string]any{
					"type":        "string",
					"description": "Optional topic constraint for action=download validation; checked against title, abstract, and venue.",
				},
				"required_terms": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional terms that must appear in the candidate title, abstract, or venue before download.",
				},
				"year_min": map[string]any{
					"type":        "integer",
					"description": "Optional minimum publication year for action=search filtering and action=download validation.",
				},
				"year_max": map[string]any{
					"type":        "integer",
					"description": "Optional maximum publication year for action=search filtering and action=download validation.",
				},
				"venue_allowlist": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional venue substrings accepted when require_top_venue=true.",
				},
				"require_top_venue": map[string]any{
					"type":        "boolean",
					"description": "When true, download only if venue matches the built-in or supplied top-venue allowlist unless allow_preprint=true.",
				},
				"allow_preprint": map[string]any{
					"type":        "boolean",
					"description": "Allow arXiv/preprint candidates to pass require_top_venue validation.",
				},
			},
			"required": []string{},
		},
		Required: []string{},
	}
}

func (t *scholarSearchTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params scholarSearchParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}
	if strings.TrimSpace(params.Action) == "" && strings.TrimSpace(params.Query) != "" {
		params.Action = "search"
	}

	// Validate action
	switch params.Action {
	case "search", "details", "citations", "references", "download":
	default:
		return NewTextErrorResponse(fmt.Sprintf("Invalid action %q. Must be one of: search, details, citations, references, download", params.Action)), nil
	}

	// Validate required params per action
	if params.Action == "search" && params.Query == "" {
		return NewTextErrorResponse("query is required for action=search"), nil
	}
	if (params.Action == "details" || params.Action == "citations" || params.Action == "references") && params.ID == "" {
		return NewTextErrorResponse(fmt.Sprintf("id is required for action=%s", params.Action)), nil
	}
	if params.Action == "download" && params.ID == "" && params.CandidateID == "" && strings.TrimSpace(params.Query) == "" {
		return NewTextErrorResponse("id, candidate_id, or query is required for action=download"), nil
	}

	// Default and cap limit
	limit := int(params.Limit)
	offset := int(params.Offset)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	params.Limit = FlexibleInt(limit)
	params.Offset = FlexibleInt(offset)

	// Dispatch by source for search action
	if params.Action == "search" {
		filters := scholarSearchFiltersFromParams(params)
		source := params.Source
		if source == "" {
			return t.doSearchWithWorkflow(ctx, params.Query, limit, offset, filters)
		}
		switch source {
		case "semantic_scholar":
			return t.doSearch(ctx, params.Query, limit, offset, filters)
		case "arxiv", "openalex", "crossref", "pubmed", "core", "eric", "patentsview", "europepmc":
			return t.doCompactSourceSearch(ctx, source, params.Query, limit, offset, filters)
		case "unpaywall":
			var rawPapers []paperResult
			result, meta, err := unpaywallLookupWithCandidate(ctx, params.Query, func(rank int, p paperResult) string {
				rawPapers = append(rawPapers, p)
				filtered, _ := filters.apply([]paperResult{p})
				if len(filtered) == 0 {
					return ""
				}
				return t.registerCandidate(ctx, "unpaywall", params.Query, rank, p).CandidateID
			})
			if err != nil {
				return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("Unpaywall lookup failed: %v", err)), withSourceSuggestions(enrichScholarMetadata(meta, "search", source, params.Query, limit, offset))), nil
			}
			filtered, filteredOut := filters.apply(rawPapers)
			if !filters.empty() {
				if meta == nil {
					meta = map[string]any{}
				}
				meta["raw_candidate_count_before_filters"] = len(rawPapers)
				filters.addMetadata(meta, filteredOut)
			}
			if filteredOut > 0 && len(filtered) == 0 {
				result = fmt.Sprintf("=== Unpaywall: DOI %s ===\nNo results found after applying requested year bounds.\n", params.Query)
			}
			return scholarSearchSuccessResponse(result, meta, "search", source, params.Query, limit, offset), nil
		default:
			return NewTextErrorResponse(fmt.Sprintf("Unknown source %q. Use: semantic_scholar, arxiv, openalex, crossref, pubmed, core, eric, patentsview, europepmc, unpaywall", source)), nil
		}
	}

	// details/citations/references/download only work with Semantic Scholar
	switch params.Action {
	case "details":
		return t.doDetails(ctx, params.ID)
	case "citations":
		return t.doCitations(ctx, params.ID, limit)
	case "references":
		return t.doReferences(ctx, params.ID, limit)
	case "download":
		return t.doDownload(ctx, params)
	}
	return NewTextErrorResponse("unreachable"), nil
}

// --- Action implementations ---

func (t *scholarSearchTool) doSearch(ctx context.Context, query string, limit, offset int, filters scholarSearchFilters) (ToolResponse, error) {
	path := fmt.Sprintf("/paper/search?query=%s&limit=%d&offset=%d&fields=%s",
		url.QueryEscape(query), limit, offset, scholarDefaultFields)
	queryKey := scholarSearchQueryKey(query, offset, limit)

	body, meta, cacheHit := scholarCacheGet("semantic_scholar", "search", queryKey)
	if !cacheHit {
		var stats scholarAPIStats
		var err error
		body, stats, err = scholarAPIGetWithStats(ctx, path)
		if err != nil {
			if rl := scholarRateLimitErr(err); rl != nil {
				msg := "Semantic Scholar is rate limited for now. Use available results or retry later. You can also try source=openalex, source=arxiv, or source=crossref."
				return withScholarCooldownMetadata(NewTextErrorResponse(msg), "search", queryKey, rl), nil
			}
			md := withSourceSuggestions(map[string]any{
				"error_kind":  "temporary_failure",
				"tool":        "ScholarSearch",
				"provider":    "semantic_scholar",
				"source":      "semantic_scholar",
				"recoverable": true,
				"action":      "search",
				"query_key":   queryKey,
				"cache_hit":   false,
				"attempts":    stats.Attempts,
				"retry_count": stats.RetryCount,
			})
			return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("Search failed: %v", err)), md), nil
		}
		scholarCacheSet(queryKey, body)
		meta = map[string]any{
			"provider":      "semantic_scholar",
			"source":        "semantic_scholar",
			"action":        "search",
			"query_key":     queryKey,
			"progress_kind": "search_page",
			"cache_hit":     false,
			"attempts":      stats.Attempts,
			"retry_count":   stats.RetryCount,
		}
	}

	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("Failed to parse response: %v", err)), meta), nil
	}

	filtered, filteredOut := filters.apply(resp.Data)

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Semantic Scholar Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Found %d total results (showing %d, offset %d)\n", resp.Total, len(filtered), offset)
	if filteredOut > 0 {
		fmt.Fprintf(&sb, "Filtered out %d result(s) outside requested year bounds.\n", filteredOut)
	}
	sb.WriteString("\n")

	if len(filtered) == 0 {
		sb.WriteString("No results found. Try different keywords or broader terms.\n")
	}
	for i, p := range filtered {
		candidate := t.registerCandidate(ctx, "semantic_scholar", query, i+1+offset, p)
		fmt.Fprintf(&sb, "CandidateID: %s\n", candidate.CandidateID)
		formatPaperSearchEntry(&sb, i+1+offset, p)
	}

	if resp.Total > offset+len(resp.Data) {
		fmt.Fprintf(&sb, "\n--- More results available: use offset=%d to see next page ---\n", offset+len(resp.Data))
	}

	meta = enrichScholarMetadata(meta, "search", "semantic_scholar", query, limit, offset)
	filters.addMetadata(meta, filteredOut)
	meta["candidate_count"] = len(filtered)
	meta["raw_candidate_count"] = len(resp.Data)
	meta["evidence_keys"] = scholarEvidenceKeys(filtered, 5)
	return WithResponseMetadata(NewTextResponse(sb.String()), meta), nil
}

func (t *scholarSearchTool) doSearchWithWorkflow(ctx context.Context, query string, limit, offset int, filters scholarSearchFilters) (ToolResponse, error) {
	wf := t.runSearchWorkflow(ctx, query, limit, offset, defaultScholarSearchWorkflowSchedule(), filters)
	if len(wf.Candidates) == 0 {
		md := map[string]any{
			"source":          "workflow",
			"provider":        "workflow",
			"tool":            "ScholarSearch",
			"action":          "search",
			"query_key":       scholarSearchQueryKey(query, offset, limit),
			"progress_kind":   "none",
			"public_summary":  "Scholar workflow found no candidates.",
			"workflow":        "scholar_search",
			"fallback_from":   "semantic_scholar",
			"attempt_summary": wf.Attempts,
			"candidate_count": 0,
			"evidence_keys":   []string{},
		}
		if scholarWorkflowAllAttemptsErrored(wf.Attempts) {
			md["error_kind"] = "temporary_failure"
			md["recoverable"] = true
			return WithResponseMetadata(NewTextErrorResponse("ScholarSearch failed: all configured search sources failed."), md), nil
		}
		return WithResponseMetadata(NewTextResponse(fmt.Sprintf("=== Scholar Search: %q ===\nNo results found across configured sources. Try broader keywords or specify source=openalex, source=arxiv, or source=crossref.", query)), md), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Scholar Search: %q ===\n", query)
	fmt.Fprintf(&sb, "Selected source: %s\n", wf.SelectedSource)
	if len(wf.Sources) > 0 {
		fmt.Fprintf(&sb, "Sources used: %s\n", strings.Join(wf.Sources, ", "))
	}
	fmt.Fprintf(&sb, "Showing %d compact candidates\n", len(wf.Candidates))
	sb.WriteString("Use action=details with a DOI, ArXiv ID, or paperId for full metadata and BibTeX.\n\n")
	for i, c := range wf.Candidates {
		fmt.Fprintf(&sb, "CandidateID: %s\n", c.CandidateID)
		fmt.Fprintf(&sb, "Source: %s\n", c.Source)
		formatPaperSearchEntry(&sb, i+1+offset, c.Paper)
	}
	md := map[string]any{
		"source":          firstNonEmpty(wf.SelectedSource, "workflow"),
		"provider":        firstNonEmpty(wf.SelectedSource, "workflow"),
		"tool":            "ScholarSearch",
		"action":          "search",
		"query_key":       scholarSearchQueryKey(query, offset, limit),
		"progress_kind":   "search_page",
		"public_summary":  fmt.Sprintf("Scholar workflow found %d deduped compact candidates via %s.", len(wf.Candidates), strings.Join(wf.Sources, ", ")),
		"workflow":        "scholar_search",
		"fallback_from":   "semantic_scholar",
		"attempt_summary": wf.Attempts,
		"sources":         scholarCandidateSources(wf.Candidates, 5),
		"sources_used":    wf.Sources,
	}
	filters.addMetadata(md, 0)
	md["candidate_count"] = len(wf.Candidates)
	md["evidence_keys"] = scholarCandidateEvidenceKeys(wf.Candidates, 5)
	return WithResponseMetadata(NewTextResponse(sb.String()), md), nil
}

func (t *scholarSearchTool) doCompactSourceSearch(ctx context.Context, source, query string, limit, offset int, filters scholarSearchFilters) (ToolResponse, error) {
	res := t.searchSource(ctx, source, query, limit, offset, filters)
	md := enrichScholarMetadata(res.Meta, "search", source, query, limit, offset)
	if res.Err != nil {
		return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("%s search failed: %v", scholarSourceDisplayName(source), res.Err)), withSourceSuggestions(md)), nil
	}

	papers := filterUsableScholarPapers(dedupeScholarPapers(res.Papers))
	candidates := make([]scholarCandidate, 0, len(papers))
	for i, p := range papers {
		candidates = append(candidates, t.registerCandidate(ctx, source, query, i+1+offset, p))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== %s Search: %q ===\n", scholarSourceDisplayName(source), query)
	fmt.Fprintf(&sb, "Showing %d compact candidates", len(candidates))
	if skipped := len(res.Papers) - len(candidates); skipped > 0 {
		fmt.Fprintf(&sb, " (%d duplicate or unusable results omitted)", skipped)
	}
	sb.WriteString("\n")
	sb.WriteString("Use action=details with a DOI, ArXiv ID, or paperId for full metadata and BibTeX; omit source for bounded multi-source search.\n\n")

	if len(candidates) == 0 {
		sb.WriteString("No usable results found. Try broader keywords or omit source to use the multi-source workflow.\n")
	} else {
		for i, c := range candidates {
			fmt.Fprintf(&sb, "CandidateID: %s\n", c.CandidateID)
			fmt.Fprintf(&sb, "Source: %s\n", c.Source)
			formatPaperSearchEntry(&sb, i+1+offset, c.Paper)
		}
	}

	md["candidate_count"] = len(candidates)
	md["raw_candidate_count"] = len(res.Papers)
	if raw, ok := md["raw_candidate_count_before_filters"]; ok {
		md["raw_candidate_count"] = raw
	}
	md["evidence_keys"] = scholarCandidateEvidenceKeys(candidates, 5)
	md["sources"] = scholarCandidateSources(candidates, 5)
	md["compact_output"] = true
	addScholarContentEvidence(md, sb.String())
	return WithResponseMetadata(NewTextResponse(sb.String()), md), nil
}

func (t *scholarSearchTool) doDetails(ctx context.Context, id string) (ToolResponse, error) {
	path := fmt.Sprintf("/paper/%s?fields=%s", url.PathEscape(id), scholarDefaultFields)

	body, err := scholarAPIGet(ctx, path)
	if err != nil {
		if rl := scholarRateLimitErr(err); rl != nil {
			msg := "Semantic Scholar is rate limited for now. Details lookup is temporarily unavailable; retry later."
			return withScholarCooldownMetadata(NewTextErrorResponse(msg), "details", scholarActionQueryKey("details", id, 0, 0), rl), nil
		}
		md := withSourceSuggestions(enrichScholarMetadata(map[string]any{"error_kind": "temporary_failure", "recoverable": true}, "details", "semantic_scholar", id, 0, 0))
		return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("Lookup failed: %v", err)), md), nil
	}

	var p paperResult
	if err := json.Unmarshal(body, &p); err != nil {
		md := enrichScholarMetadata(map[string]any{"error_kind": "parse_error", "recoverable": false}, "details", "semantic_scholar", id, 0, 0)
		return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("Failed to parse response: %v", err)), md), nil
	}

	var sb strings.Builder
	sb.WriteString("=== Paper Details ===\n\n")
	candidate := t.registerCandidate(ctx, "semantic_scholar", "details:"+id, 1, p)
	fmt.Fprintf(&sb, "CandidateID: %s\n", candidate.CandidateID)
	formatPaperEntry(&sb, 1, p)
	md := enrichScholarMetadata(map[string]any{
		"candidate_count": 1,
		"evidence_keys":   scholarEvidenceKeys([]paperResult{p}, 5),
	}, "details", "semantic_scholar", id, 0, 0)
	return WithResponseMetadata(NewTextResponse(sb.String()), md), nil
}

func (t *scholarSearchTool) doCitations(ctx context.Context, id string, limit int) (ToolResponse, error) {
	path := fmt.Sprintf("/paper/%s/citations?fields=%s&limit=%d",
		url.PathEscape(id), scholarBriefFields, limit)

	body, err := scholarAPIGet(ctx, path)
	if err != nil {
		if rl := scholarRateLimitErr(err); rl != nil {
			msg := "Semantic Scholar is rate limited for now. Citations lookup is temporarily unavailable; retry later."
			return withScholarCooldownMetadata(NewTextErrorResponse(msg), "citations", scholarActionQueryKey("citations", id, 0, limit), rl), nil
		}
		md := withSourceSuggestions(enrichScholarMetadata(map[string]any{"error_kind": "temporary_failure", "recoverable": true}, "citations", "semantic_scholar", id, limit, 0))
		return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("Citations lookup failed: %v", err)), md), nil
	}

	var resp struct {
		Data []citationWrapper `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		md := enrichScholarMetadata(map[string]any{"error_kind": "parse_error", "recoverable": false}, "citations", "semantic_scholar", id, limit, 0)
		return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("Failed to parse response: %v", err)), md), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Papers Citing %s ===\n", id)
	fmt.Fprintf(&sb, "Showing %d citations\n\n", len(resp.Data))

	for i, w := range resp.Data {
		formatPaperBrief(&sb, i+1, w.CitingPaper)
	}
	papers := make([]paperResult, 0, len(resp.Data))
	for _, w := range resp.Data {
		papers = append(papers, w.CitingPaper)
	}
	md := enrichScholarMetadata(map[string]any{
		"candidate_count": len(resp.Data),
		"evidence_keys":   scholarEvidenceKeys(papers, 5),
	}, "citations", "semantic_scholar", id, limit, 0)
	return WithResponseMetadata(NewTextResponse(sb.String()), md), nil
}

func (t *scholarSearchTool) doReferences(ctx context.Context, id string, limit int) (ToolResponse, error) {
	path := fmt.Sprintf("/paper/%s/references?fields=%s&limit=%d",
		url.PathEscape(id), scholarBriefFields, limit)

	body, err := scholarAPIGet(ctx, path)
	if err != nil {
		if rl := scholarRateLimitErr(err); rl != nil {
			msg := "Semantic Scholar is rate limited for now. References lookup is temporarily unavailable; retry later."
			return withScholarCooldownMetadata(NewTextErrorResponse(msg), "references", scholarActionQueryKey("references", id, 0, limit), rl), nil
		}
		md := withSourceSuggestions(enrichScholarMetadata(map[string]any{"error_kind": "temporary_failure", "recoverable": true}, "references", "semantic_scholar", id, limit, 0))
		return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("References lookup failed: %v", err)), md), nil
	}

	var resp struct {
		Data []referenceWrapper `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		md := enrichScholarMetadata(map[string]any{"error_kind": "parse_error", "recoverable": false}, "references", "semantic_scholar", id, limit, 0)
		return WithResponseMetadata(NewTextErrorResponse(fmt.Sprintf("Failed to parse response: %v", err)), md), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== References of %s ===\n", id)
	fmt.Fprintf(&sb, "Showing %d references\n\n", len(resp.Data))

	for i, w := range resp.Data {
		formatPaperBrief(&sb, i+1, w.CitedPaper)
	}
	papers := make([]paperResult, 0, len(resp.Data))
	for _, w := range resp.Data {
		papers = append(papers, w.CitedPaper)
	}
	md := enrichScholarMetadata(map[string]any{
		"candidate_count": len(resp.Data),
		"evidence_keys":   scholarEvidenceKeys(papers, 5),
	}, "references", "semantic_scholar", id, limit, 0)
	return WithResponseMetadata(NewTextResponse(sb.String()), md), nil
}

// --- Download ---

// resolveDownloadDir 解析论文下载目标目录并执行边界校验。
// 优先级: destination 参数 > research workspace > 数据目录默认。
// 返回绝对路径和可能的校验错误。
func resolveDownloadDir(ctx context.Context, destination string) (string, error) {
	var dir string

	if destination != "" {
		if filepath.IsAbs(destination) {
			dir = filepath.Clean(destination)
		} else if workDir := ResearchWorkDir(ctx); workDir != "" {
			// research 模式：相对路径基于 research workdir
			dir = filepath.Join(workDir, destination)
		} else {
			// 非 research：相对路径基于 workspace
			dir = filepath.Join(WorkspaceDir(ctx), destination)
		}
	} else if workDir := ResearchWorkDir(ctx); workDir != "" {
		dir = filepath.Join(workDir, "papers")
	} else {
		// 默认：数据目录基于 workspace context
		dataDir := config.DataDirectory()
		dir = filepath.Join(dataDir, "papers")
	}

	// 边界校验：确保目标在允许范围内
	if err := ValidateWorkspacePath(ctx, dir); err != nil {
		return "", fmt.Errorf("destination outside workspace boundary: %w", err)
	}

	return dir, nil
}

func (t *scholarSearchTool) doDownload(ctx context.Context, params scholarSearchParams) (ToolResponse, error) {
	sessionID, _ := GetContextValues(ctx)
	papersDir, err := resolveDownloadDir(ctx, params.Destination)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Download destination rejected: %v", err)), nil
	}

	if params.ID == "" && params.CandidateID == "" && strings.TrimSpace(params.Query) != "" && !params.AllowUnverified {
		return t.doQueryDownloadWorkflow(ctx, sessionID, params, papersDir, int(params.Limit), int(params.Offset))
	}

	candidates, err := t.resolveDownloadCandidates(ctx, params)
	if err != nil {
		if rl := scholarRateLimitErr(err); rl != nil {
			msg := "Semantic Scholar is rate limited for now. Download lookup is temporarily unavailable; retry later."
			return withScholarCooldownMetadata(NewTextErrorResponse(msg), "download", scholarActionQueryKey("download", params.ID, 0, 0), rl), nil
		}
		return NewTextErrorResponse(err.Error()), nil
	}
	resp, err, ok, attempts := t.attemptDownloadCandidates(ctx, sessionID, candidates, params, papersDir, nil, nil)
	if err != nil || ok {
		return resp, err
	}
	return scholarDownloadFailureResponse(attempts, nil), nil
}

func (t *scholarSearchTool) doQueryDownloadWorkflow(ctx context.Context, sessionID string, params scholarSearchParams, papersDir string, limit, offset int) (ToolResponse, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	schedule := []string{"semantic_scholar", "openalex", "arxiv", "crossref"}
	var searchAttempts []map[string]any
	var downloadAttempts []scholarDownloadAttempt
	globalRank := offset

	for _, source := range schedule {
		attempt := map[string]any{"source": source}
		search := t.searchSource(ctx, source, params.Query, limit, offset, scholarSearchFiltersFromParams(params))
		if search.Meta != nil {
			attempt["meta"] = search.Meta
		}
		if search.Err != nil {
			attempt["result"] = "error"
			attempt["reason"] = search.Err.Error()
			searchAttempts = append(searchAttempts, attempt)
			continue
		}
		if len(search.Papers) == 0 {
			attempt["result"] = "empty"
			searchAttempts = append(searchAttempts, attempt)
			continue
		}

		sourceCandidates := make([]scholarCandidate, 0, len(search.Papers))
		for _, p := range dedupeScholarPapers(search.Papers) {
			globalRank++
			sourceCandidates = append(sourceCandidates, t.registerCandidate(ctx, source, params.Query, globalRank, p))
		}
		attempt["result"] = "ok"
		attempt["candidate_count"] = len(sourceCandidates)
		searchAttempts = append(searchAttempts, attempt)

		resp, err, ok, attempts := t.attemptDownloadCandidates(ctx, sessionID, sourceCandidates, params, papersDir, searchAttempts, downloadAttempts)
		downloadAttempts = attempts
		if err != nil || ok {
			return resp, err
		}
	}
	return scholarDownloadFailureResponse(downloadAttempts, searchAttempts), nil
}

func (t *scholarSearchTool) attemptDownloadCandidates(ctx context.Context, sessionID string, candidates []scholarCandidate, params scholarSearchParams, papersDir string, searchAttempts []map[string]any, attempts []scholarDownloadAttempt) (ToolResponse, error, bool, []scholarDownloadAttempt) {
	for _, candidate := range dedupeScholarCandidates(candidates) {
		attempt := scholarDownloadAttempt{
			CandidateID: candidate.CandidateID,
			Source:      candidate.Source,
			Title:       candidate.Paper.Title,
		}
		if strings.TrimSpace(candidate.PDFURL) == "" {
			attempt.Result = "rejected"
			attempt.Reason = "candidate has no open-access PDF"
			attempts = append(attempts, attempt)
			continue
		}
		verdict := validateDownloadCandidate(candidate, params)
		if !verdict.Allowed {
			attempt.Result = "rejected"
			attempt.Reason = strings.Join(verdict.Reasons, "; ")
			attempts = append(attempts, attempt)
			continue
		}

		target := filepath.Join(papersDir, downloadFileName(candidate))
		if params.DryRun {
			attempt.Result = "selected"
			attempt.FilePath = target
			attempts = append(attempts, attempt)
			resp := NewTextResponse(fmt.Sprintf("Dry run OK.\nCandidateID: %s\nTitle: %s\nYear: %d\nVenue: %s\nPDF: %s\nTarget: %s", candidate.CandidateID, candidate.Paper.Title, candidate.Paper.Year, candidate.Paper.Venue, candidate.PDFURL, target))
			return WithResponseMetadata(resp, scholarDownloadMetadata(candidate, target, 0, attempts, searchAttempts, false, false)), nil, true, attempts
		}

		if t.permissions != nil {
			description := fmt.Sprintf("Download %q (%d, %s) via candidate %s", candidate.Paper.Title, candidate.Paper.Year, candidate.Paper.Venue, candidate.CandidateID)
			if params.AllowUnverified && params.CandidateID == "" {
				description = "Unverified override: " + description
			}
			result := t.permissions.RequestDecision(permission.CreatePermissionRequest{
				SessionID:   sessionID,
				ToolName:    "ScholarSearch",
				Description: description,
				Action:      "download",
				Path:        papersDir,
			})
			if !result.Allowed {
				return ToolResponse{}, permission.NewPermissionError(result), false, attempts
			}
		}

		if err := os.MkdirAll(papersDir, 0755); err != nil {
			attempt.Result = "download_failed"
			attempt.Reason = fmt.Sprintf("failed to create destination directory: %v", err)
			attempts = append(attempts, attempt)
			continue
		}

		success, reason := t.downloadCandidateOnce(ctx, sessionID, candidate, params, papersDir, verdict)
		if reason != "" {
			attempt.Result = "download_failed"
			attempt.Reason = reason
			attempts = append(attempts, attempt)
			continue
		}
		attempt.Result = "downloaded"
		if success.Existing {
			attempt.Result = "exists"
		}
		attempt.FilePath = success.Path
		attempts = append(attempts, attempt)
		return scholarDownloadSuccessResponse(candidate, success.Path, success.Bytes, attempts, searchAttempts, success.Existing), nil, true, attempts
	}
	return ToolResponse{}, nil, false, attempts
}

type scholarDownloadSuccess struct {
	Path     string
	Bytes    int64
	Existing bool
}

func (t *scholarSearchTool) downloadCandidateOnce(ctx context.Context, sessionID string, candidate scholarCandidate, params scholarSearchParams, papersDir string, verdict downloadValidation) (scholarDownloadSuccess, string) {
	if params.DryRun {
		return scholarDownloadSuccess{}, ""
	}

	filePath := filepath.Join(papersDir, downloadFileName(candidate))

	// Avoid overwriting
	if _, err := os.Stat(filePath); err == nil {
		verified, verifyErr := manifestVerifiesDownload(papersDir, filePath, candidate)
		if verifyErr != nil {
			return scholarDownloadSuccess{}, fmt.Sprintf("target file already exists but manifest could not be verified: %v", verifyErr)
		}
		if !verified {
			return scholarDownloadSuccess{}, "target file already exists but manifest does not verify it belongs to this candidate"
		}
		absExisting, _ := filepath.Abs(filePath)
		info, _ := os.Stat(filePath)
		var size int64
		if info != nil {
			size = info.Size()
		}
		return scholarDownloadSuccess{Path: absExisting, Bytes: size, Existing: true}, ""
	}

	req, err := http.NewRequestWithContext(ctx, "GET", candidate.PDFURL, nil)
	if err != nil {
		return scholarDownloadSuccess{}, fmt.Sprintf("failed to create download request: %v", err)
	}
	req.Header.Set("User-Agent", "OpenScholar/1.0")

	resp, err := scholarDownloadHTTPClient.Do(req)
	if err != nil {
		return scholarDownloadSuccess{}, fmt.Sprintf("download failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return scholarDownloadSuccess{}, fmt.Sprintf("HTTP %d from %s", resp.StatusCode, candidate.PDFURL)
	}
	if resp.ContentLength > scholarMaxPDFBytes {
		return scholarDownloadSuccess{}, "file too large (>50MB)"
	}
	budgetKey := downloadBudgetKey(ctx, sessionID)
	remainingBudget, err := t.reserveDownloadSlot(budgetKey, resp.ContentLength)
	if err != nil {
		return scholarDownloadSuccess{}, err.Error()
	}

	w, err := fileop.CreateFileAtomic(filePath, 0o644)
	if err != nil {
		return scholarDownloadSuccess{}, fmt.Sprintf("failed to create file: %v", err)
	}
	writeLimit := minInt64(scholarMaxPDFBytes, remainingBudget)
	written, err := io.Copy(w, &io.LimitedReader{R: resp.Body, N: writeLimit + 1})
	if err != nil {
		w.Abort()
		return scholarDownloadSuccess{}, fmt.Sprintf("failed to write PDF: %v", err)
	}
	if written > writeLimit {
		w.Abort()
		return scholarDownloadSuccess{}, "file or turn download budget exceeded"
	}
	if err := w.Close(); err != nil {
		return scholarDownloadSuccess{}, fmt.Sprintf("failed to finalize PDF: %v", err)
	}
	t.recordDownloadBytes(budgetKey, written)

	absPath, _ := filepath.Abs(filePath)
	if err := appendDownloadManifest(papersDir, map[string]any{
		"candidate_id":  candidate.CandidateID,
		"paper_id":      candidate.Paper.PaperID,
		"title":         candidate.Paper.Title,
		"year":          candidate.Paper.Year,
		"venue":         candidate.Paper.Venue,
		"doi":           candidate.Paper.ExternalIDs.DOI,
		"arxiv":         candidate.Paper.ExternalIDs.ArXiv,
		"pdf_url":       candidate.PDFURL,
		"source":        candidate.Source,
		"query":         candidate.Query,
		"rank":          candidate.Rank,
		"constraints":   downloadConstraintsForManifest(params),
		"verdict":       verdict,
		"path":          absPath,
		"bytes":         written,
		"downloaded_at": time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return scholarDownloadSuccess{}, fmt.Sprintf("downloaded file but failed to write manifest: %v", err)
	}

	return scholarDownloadSuccess{Path: absPath, Bytes: written}, ""
}

func scholarDownloadSuccessResponse(candidate scholarCandidate, absPath string, written int64, attempts []scholarDownloadAttempt, searchAttempts []map[string]any, existing bool) ToolResponse {
	var sb strings.Builder
	if existing {
		fmt.Fprintf(&sb, "PDF already exists: %s\n", candidate.Paper.Title)
	} else {
		fmt.Fprintf(&sb, "Downloaded: %s\n", candidate.Paper.Title)
	}
	fmt.Fprintf(&sb, "CandidateID: %s\n", candidate.CandidateID)
	fmt.Fprintf(&sb, "Authors: %s\n", formatAuthorsShort(candidate.Paper.Authors))
	fmt.Fprintf(&sb, "Year: %d | Citations: %d\n", candidate.Paper.Year, candidate.Paper.CitationCount)
	fmt.Fprintf(&sb, "File: %s (%d KB)\n", absPath, written/1024)
	fmt.Fprintf(&sb, "Source: %s\n\n", candidate.PDFURL)
	sb.WriteString(fmt.Sprintf("Selected source: %s\n", candidate.Source))
	sb.WriteString("Use KBAdd to index this paper into the knowledge base for searchable Q&A.")

	md := scholarDownloadMetadata(candidate, absPath, written, attempts, searchAttempts, existing, true)
	return WithResponseMetadata(NewTextResponse(sb.String()), md)
}

func scholarDownloadMetadata(candidate scholarCandidate, absPath string, written int64, attempts []scholarDownloadAttempt, searchAttempts []map[string]any, existing bool, durable bool) map[string]any {
	progressKind := "downloaded_artifact"
	if !durable {
		progressKind = "validated_candidate"
	}
	summaryVerb := "Downloaded"
	if existing {
		summaryVerb = "Resolved existing"
	} else if !durable {
		summaryVerb = "Validated"
	}
	return map[string]any{
		"workflow":         "scholar_download",
		"tool":             "ScholarSearch",
		"action":           "download",
		"provider":         candidate.Source,
		"source":           candidate.Source,
		"progress_kind":    progressKind,
		"durable_progress": durable,
		"public_summary":   fmt.Sprintf("%s %q at %s.", summaryVerb, candidate.Paper.Title, absPath),
		"selected":         map[string]any{"candidate_id": candidate.CandidateID, "source": candidate.Source, "title": candidate.Paper.Title},
		"attempt_summary":  attempts,
		"search_attempts":  searchAttempts,
		"artifact":         map[string]any{"path": absPath, "bytes": written, "existing": existing},
		"artifacts":        []map[string]any{{"kind": "pdf", "path": absPath, "bytes": written, "existing": existing}},
		"sources":          scholarCandidateSources([]scholarCandidate{candidate}, 1),
	}
}

func scholarDownloadFailureResponse(attempts []scholarDownloadAttempt, searchAttempts []map[string]any) ToolResponse {
	reasons := make([]string, 0, len(attempts))
	errorKind := "validation_failed"
	for _, a := range attempts {
		if a.Reason == "" {
			continue
		}
		reasons = append(reasons, a.Reason)
		if a.Result == "download_failed" {
			errorKind = "download_failed"
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "no downloadable candidate satisfied constraints")
	}
	md := map[string]any{
		"tool":            "ScholarSearch",
		"action":          "download",
		"workflow":        "scholar_download",
		"error_kind":      errorKind,
		"recoverable":     true,
		"progress_kind":   "none",
		"public_summary":  "Scholar download workflow failed: " + strings.Join(reasons, "; "),
		"attempt_summary": attempts,
		"search_attempts": searchAttempts,
	}
	return WithResponseMetadata(NewTextErrorResponse("Download failed: "+strings.Join(reasons, "; ")), md)
}

func (t *scholarSearchTool) registerCandidate(ctx context.Context, source, query string, rank int, p paperResult) scholarCandidate {
	sessionID, _ := GetContextValues(ctx)
	if sessionID == "" {
		sessionID = "global"
	}
	now := time.Now()
	seq := atomic.AddUint64(&t.seq, 1)
	c := scholarCandidate{
		CandidateID: scholarCandidateID(sessionID, source, query, rank, seq, p),
		SessionID:   sessionID,
		Source:      source,
		Query:       query,
		Rank:        rank,
		Paper:       p,
		PDFURL:      paperPDFURL(p),
		CreatedAt:   now,
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	list := append(t.recentCandidatesLocked(sessionID, now), c)
	if len(list) > scholarMaxCandidates {
		list = list[len(list)-scholarMaxCandidates:]
	}
	t.candidates[sessionID] = list
	return c
}

func (t *scholarSearchTool) resolveDownloadCandidate(ctx context.Context, params scholarSearchParams) (scholarCandidate, error) {
	candidates, err := t.resolveDownloadCandidates(ctx, params)
	if err != nil {
		return scholarCandidate{}, err
	}
	if len(candidates) == 0 {
		return scholarCandidate{}, fmt.Errorf("no candidate matched")
	}
	return candidates[0], nil
}

func (t *scholarSearchTool) resolveDownloadCandidates(ctx context.Context, params scholarSearchParams) ([]scholarCandidate, error) {
	sessionID, _ := GetContextValues(ctx)
	if sessionID == "" {
		sessionID = "global"
	}
	t.mu.Lock()
	list := append([]scholarCandidate(nil), t.recentCandidatesLocked(sessionID, time.Now())...)
	t.mu.Unlock()

	if params.CandidateID != "" {
		for _, c := range list {
			if c.CandidateID == params.CandidateID {
				if c.PDFURL == "" {
					return nil, fmt.Errorf("candidate %s has no open-access PDF", c.CandidateID)
				}
				return []scholarCandidate{c}, nil
			}
		}
		return nil, fmt.Errorf("candidate_id %q was not found in current session results", params.CandidateID)
	}

	if params.AllowUnverified {
		if params.ID == "" {
			return nil, errors.New("id is required when allow_unverified=true")
		}
		path := fmt.Sprintf("/paper/%s?fields=%s", url.PathEscape(params.ID), scholarDefaultFields)
		body, err := scholarAPIGet(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("Paper lookup failed: %w", err)
		}
		var p paperResult
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, fmt.Errorf("Failed to parse paper: %v", err)
		}
		c := t.registerCandidate(ctx, "semantic_scholar", "unverified:"+params.ID, 0, p)
		if c.PDFURL == "" {
			return nil, fmt.Errorf("No open access PDF available for %q. Try downloading manually via DOI: %s", p.Title, p.ExternalIDs.DOI)
		}
		return []scholarCandidate{c}, nil
	}

	matches := []scholarCandidate{}
	for _, c := range list {
		if candidateMatchesID(c, params.ID) {
			matches = append(matches, c)
		}
	}
	if len(matches) == 1 {
		return matches, nil
	}
	if len(matches) > 1 {
		return matches, nil
	}
	return nil, fmt.Errorf("download blocked: id %q was not seen in this session. Run search/details and use CandidateID", params.ID)
}

func candidateMatchesID(c scholarCandidate, id string) bool {
	id = normalizePaperIdentifier(id)
	return id != "" && (id == normalizePaperIdentifier(c.Paper.PaperID) ||
		(c.Paper.ExternalIDs.DOI != "" && (id == normalizePaperIdentifier(c.Paper.ExternalIDs.DOI) || id == normalizePaperIdentifier("DOI:"+c.Paper.ExternalIDs.DOI))) ||
		(c.Paper.ExternalIDs.ArXiv != "" && (id == normalizePaperIdentifier(c.Paper.ExternalIDs.ArXiv) || id == normalizePaperIdentifier("ArXiv:"+c.Paper.ExternalIDs.ArXiv))))
}

func validateDownloadCandidate(c scholarCandidate, params scholarSearchParams) downloadValidation {
	reasons := []string{}
	yearMin := int(params.YearMin)
	yearMax := int(params.YearMax)
	if yearMin > 0 || yearMax > 0 {
		if c.Paper.Year == 0 {
			reasons = append(reasons, "year is unknown")
		} else {
			if yearMin > 0 && c.Paper.Year < yearMin {
				reasons = append(reasons, fmt.Sprintf("year %d < year_min %d", c.Paper.Year, yearMin))
			}
			if yearMax > 0 && c.Paper.Year > yearMax {
				reasons = append(reasons, fmt.Sprintf("year %d > year_max %d", c.Paper.Year, yearMax))
			}
		}
	}
	haystack := c.Paper.Title + " " + c.Paper.Abstract + " " + c.Paper.Venue
	if strings.TrimSpace(params.Topic) != "" && !matchesTopicOrTerm(haystack, params.Topic) {
		reasons = append(reasons, "topic does not match title/abstract/venue")
	}
	for _, term := range params.RequiredTerms {
		if strings.TrimSpace(term) == "" {
			continue
		}
		if !matchesTopicOrTerm(haystack, term) {
			reasons = append(reasons, fmt.Sprintf("required term %q not found", strings.TrimSpace(term)))
		}
	}
	if len(params.VenueAllowlist) > 0 && !matchesVenueAllowlist(c.Paper.Venue, params.VenueAllowlist) {
		reasons = append(reasons, "venue is not in venue allowlist")
	}
	if params.RequireTopVenue {
		isPreprint := isPreprintCandidate(c)
		switch {
		case isPreprint && params.AllowPreprint:
		case c.Paper.Venue == "":
			reasons = append(reasons, "missing venue for top-venue requirement")
		case !matchesVenueAllowlist(c.Paper.Venue, params.VenueAllowlist) && !matchesVenueAllowlist(c.Paper.Venue, topVenueSubstrings):
			reasons = append(reasons, "venue is not in top venue allowlist")
		}
	}
	return downloadValidation{Allowed: len(reasons) == 0, Reasons: reasons}
}

func matchesVenueAllowlist(venue string, allowlist []string) bool {
	v := compactAlnum(venue)
	for _, item := range allowlist {
		item = compactAlnum(item)
		if item != "" && strings.Contains(v, item) {
			return true
		}
	}
	return false
}

func paperPDFURL(p paperResult) string {
	if p.OpenAccessPdf != nil && p.OpenAccessPdf.URL != "" {
		return p.OpenAccessPdf.URL
	}
	if p.ExternalIDs.ArXiv != "" {
		return fmt.Sprintf("https://arxiv.org/pdf/%s.pdf", p.ExternalIDs.ArXiv)
	}
	return ""
}

func truncateFileStem(stem string) string {
	if stem == "" {
		return "paper"
	}
	if len(stem) > 80 {
		return stem[:80]
	}
	return stem
}

func downloadFileName(c scholarCandidate) string {
	stem := truncateFileStem(sanitizeFileName(c.Paper.Title))
	return fmt.Sprintf("%s_%s.pdf", stem, stablePaperSuffix(c))
}

func stablePaperSuffix(c scholarCandidate) string {
	stableID := firstNonEmpty(c.Paper.PaperID, c.Paper.ExternalIDs.DOI, c.Paper.ExternalIDs.ArXiv, c.PDFURL, c.Paper.Title)
	sum := sha256.Sum256([]byte(stableID))
	return hex.EncodeToString(sum[:5])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return "paper"
}

func paperAuthorsFromNames(names []string) []authorResult {
	authors := make([]authorResult, 0, len(names))
	for _, name := range names {
		authors = append(authors, authorResult{Name: name})
	}
	return authors
}

func appendDownloadManifest(dir string, record map[string]any) error {
	path := filepath.Join(dir, ".openscholar-downloads.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func manifestVerifiesDownload(dir, filePath string, c scholarCandidate) (bool, error) {
	manifestPath := filepath.Join(dir, ".openscholar-downloads.jsonl")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	absPath, _ := filepath.Abs(filePath)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return false, err
		}
		recordPath, _ := record["path"].(string)
		if recordPath != absPath {
			continue
		}
		if manifestRecordMatchesCandidate(record, c) {
			return true, nil
		}
	}
	return false, nil
}

func manifestRecordMatchesCandidate(record map[string]any, c scholarCandidate) bool {
	checks := []struct {
		key   string
		value string
	}{
		{"candidate_id", c.CandidateID},
		{"paper_id", c.Paper.PaperID},
		{"doi", c.Paper.ExternalIDs.DOI},
		{"arxiv", c.Paper.ExternalIDs.ArXiv},
		{"pdf_url", c.PDFURL},
	}
	for _, check := range checks {
		if check.value == "" {
			continue
		}
		if got, _ := record[check.key].(string); got != "" && got == check.value {
			return true
		}
	}
	return false
}

func (t *scholarSearchTool) recentCandidatesLocked(sessionID string, now time.Time) []scholarCandidate {
	list := t.candidates[sessionID]
	if len(list) == 0 {
		return nil
	}
	filtered := list[:0]
	for _, c := range list {
		if now.Sub(c.CreatedAt) <= scholarCandidateTTL {
			filtered = append(filtered, c)
		}
	}
	t.candidates[sessionID] = filtered
	return filtered
}

func scholarCandidateID(sessionID, source, query string, rank int, seq uint64, p paperResult) string {
	seed := fmt.Sprintf("%s|%s|%s|%d|%d|%s|%s|%s|%s", sessionID, source, query, rank, seq, p.PaperID, p.ExternalIDs.DOI, p.ExternalIDs.ArXiv, p.Title)
	sum := sha256.Sum256([]byte(seed))
	var randomBytes [6]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		fallback := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", seed, time.Now().UnixNano())))
		copy(randomBytes[:], fallback[:6])
	}
	return fmt.Sprintf("cand_%s_%s", hex.EncodeToString(sum[:4]), hex.EncodeToString(randomBytes[:]))
}

func normalizePaperIdentifier(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	id = strings.TrimPrefix(id, "doi:")
	id = strings.TrimPrefix(id, "arxiv:")
	id = strings.TrimPrefix(id, "arxiv/")
	return strings.TrimSpace(id)
}

func matchesTopicOrTerm(haystack, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return true
	}
	haystackLower := strings.ToLower(haystack)
	needleLower := strings.ToLower(needle)
	if strings.Contains(haystackLower, needleLower) {
		return true
	}
	if compact := compactAlnum(needle); compact != "" && strings.Contains(compactAlnum(haystack), compact) {
		return true
	}
	needleTokens := meaningfulTokens(needle)
	if len(needleTokens) == 0 {
		return false
	}
	haystackTokens := map[string]struct{}{}
	for _, token := range meaningfulTokens(haystack) {
		haystackTokens[token] = struct{}{}
	}
	compactHaystack := compactAlnum(haystack)
	matches := 0
	for _, token := range needleTokens {
		if _, ok := haystackTokens[token]; ok {
			matches++
			continue
		}
		if tokenCompact := compactAlnum(token); tokenCompact != "" && strings.Contains(compactHaystack, tokenCompact) {
			matches++
		}
	}
	if len(needleTokens) == 1 {
		return matches == 1
	}
	return matches >= 2
}

func meaningfulTokens(s string) []string {
	parts := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) < 3 || titleStopwords[part] {
			continue
		}
		tokens = append(tokens, part)
	}
	return tokens
}

func compactAlnum(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func isPreprintCandidate(c scholarCandidate) bool {
	if c.Paper.ExternalIDs.ArXiv != "" {
		return true
	}
	source := strings.ToLower(c.Source)
	venue := strings.ToLower(c.Paper.Venue)
	return strings.Contains(source, "arxiv") || strings.Contains(venue, "arxiv") || strings.Contains(venue, "preprint")
}

func downloadConstraintsForManifest(params scholarSearchParams) map[string]any {
	return map[string]any{
		"topic":             params.Topic,
		"required_terms":    params.RequiredTerms,
		"year_min":          int(params.YearMin),
		"year_max":          int(params.YearMax),
		"venue_allowlist":   params.VenueAllowlist,
		"require_top_venue": params.RequireTopVenue,
		"allow_preprint":    params.AllowPreprint,
		"allow_unverified":  params.AllowUnverified,
	}
}

func downloadBudgetKey(ctx context.Context, sessionID string) string {
	if sessionID == "" {
		sessionID = "global"
	}
	turnID, _ := ctx.Value(TurnIDContextKey).(string)
	if turnID == "" {
		turnID = "session"
	}
	return sessionID + ":" + turnID
}

func (t *scholarSearchTool) reserveDownloadSlot(key string, contentLength int64) (int64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	budget := t.downloadBudgets[key]
	if budget.Count >= scholarMaxDownloadsPerTurn {
		return 0, fmt.Errorf("per-turn download count limit reached (%d)", scholarMaxDownloadsPerTurn)
	}
	remaining := scholarMaxBytesPerTurn - budget.Bytes
	if remaining <= 0 {
		return 0, fmt.Errorf("per-turn byte limit reached (%d MB)", scholarMaxBytesPerTurn/(1024*1024))
	}
	if contentLength > remaining {
		return 0, fmt.Errorf("download would exceed per-turn byte limit (%d MB)", scholarMaxBytesPerTurn/(1024*1024))
	}
	budget.Count++
	t.downloadBudgets[key] = budget
	return remaining, nil
}

func (t *scholarSearchTool) recordDownloadBytes(key string, bytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	budget := t.downloadBudgets[key]
	budget.Bytes += bytes
	t.downloadBudgets[key] = budget
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// sanitizeFileName removes characters unsafe for filenames.
func sanitizeFileName(title string) string {
	var sb strings.Builder
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

// --- HTTP client with rate limiting ---

type scholarAPIStats struct {
	Attempts   int
	RetryCount int
}

func scholarAPIGet(ctx context.Context, path string) ([]byte, error) {
	body, _, err := scholarAPIGetWithStats(ctx, path)
	return body, err
}

func scholarAPIGetWithStats(ctx context.Context, path string) ([]byte, scholarAPIStats, error) {
	const maxRetries = scholarRetryMax
	stats := scholarAPIStats{}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		stats.Attempts = attempt + 1
		scholarRateMu.Lock()
		if time.Now().Before(scholarCooldownUntil) {
			remaining := time.Until(scholarCooldownUntil)
			retryAfterMs := remaining.Milliseconds()
			if retryAfterMs < 0 {
				retryAfterMs = 0
			}
			cooldownUntil := scholarCooldownUntil.UnixMilli()
			scholarRateMu.Unlock()
			return nil, stats, &scholarRateLimitError{
				ErrorKind:           "provider_cooldown",
				RetryAfterMs:        retryAfterMs,
				CooldownUntilUnixMs: cooldownUntil,
			}
		}
		scholarRateMu.Unlock()

		// Rate limit: at least 1s between requests
		scholarRateMu.Lock()
		elapsed := time.Since(scholarLastRequest)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
		scholarLastRequest = time.Now()
		scholarRateMu.Unlock()

		reqURL := semanticScholarBaseURL + path
		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, stats, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := scholarHTTPClient.Do(req)
		if err != nil {
			if scholarRetryableNetErr(err) && attempt < maxRetries {
				stats.RetryCount++
				time.Sleep(time.Duration(150*(1<<attempt)) * time.Millisecond)
				continue
			}
			return nil, stats, fmt.Errorf("request failed: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if attempt < maxRetries {
				stats.RetryCount++
				time.Sleep(time.Duration(150*(1<<attempt)) * time.Millisecond)
				continue
			}
			return nil, stats, fmt.Errorf("failed to read response: %w", err)
		}

		switch resp.StatusCode {
		case 200:
			return body, stats, nil
		case 404:
			return nil, stats, fmt.Errorf("paper not found (404)")
		case 429:
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			if retryAfter <= 0 {
				retryAfter = 10 * time.Second
			}
			scholarRateMu.Lock()
			scholarCooldownUntil = time.Now().Add(retryAfter)
			cooldownUntil := scholarCooldownUntil.UnixMilli()
			scholarRateMu.Unlock()
			return nil, stats, &scholarRateLimitError{
				ErrorKind:           "rate_limited",
				RetryAfterMs:        retryAfter.Milliseconds(),
				CooldownUntilUnixMs: cooldownUntil,
			}
		default:
			if resp.StatusCode >= 500 && attempt < maxRetries {
				stats.RetryCount++
				time.Sleep(time.Duration(150*(1<<attempt)) * time.Millisecond)
				continue
			}
			return nil, stats, fmt.Errorf("API returned status %d: %s", resp.StatusCode, truncateStr(string(body), 200))
		}
	}
	return nil, stats, fmt.Errorf("unexpected: exhausted retries")
}

func scholarRateLimitErr(err error) *scholarRateLimitError {
	var rl *scholarRateLimitError
	if errors.As(err, &rl) {
		return rl
	}
	return nil
}

func scholarNormalizeQueryPart(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "-"
	}
	var b strings.Builder
	prevUnderscore := false
	for _, r := range v {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "-"
	}
	return out
}

func scholarSearchQueryKey(query string, offset, limit int) string {
	return fmt.Sprintf("search:semantic_scholar:%s:%d:%d", scholarNormalizeQueryPart(query), offset, limit)
}

func scholarActionQueryKey(action, rawID string, offset, limit int) string {
	return fmt.Sprintf("%s:semantic_scholar:%s:%d:%d", action, scholarNormalizeQueryPart(rawID), offset, limit)
}

func withScholarCooldownMetadata(resp ToolResponse, action, queryKey string, rl *scholarRateLimitError) ToolResponse {
	md := map[string]any{
		"error_kind":        rl.ErrorKind,
		"tool":              "ScholarSearch",
		"provider":          "semantic_scholar",
		"source":            "semantic_scholar",
		"recoverable":       true,
		"action":            action,
		"query_key":         queryKey,
		"cache_hit":         false,
		"attempts":          0,
		"retry_count":       0,
		"suggested_sources": []string{"openalex", "arxiv", "crossref"},
		"progress_kind":     "none",
	}
	if rl.ErrorKind == "rate_limited" {
		md["attempts"] = 1
	}
	if rl.RetryAfterMs > 0 {
		md["retry_after_ms"] = rl.RetryAfterMs
	}
	if rl.CooldownUntilUnixMs > 0 {
		md["cooldown_until_unix_ms"] = rl.CooldownUntilUnixMs
	}
	return WithResponseMetadata(resp, md)
}

func withSourceSuggestions(md map[string]any) map[string]any {
	if md == nil {
		md = map[string]any{}
	}
	if md["suggested_sources"] == nil {
		md["suggested_sources"] = []string{"openalex", "arxiv", "crossref", "pubmed"}
	}
	if provider, _ := md["provider"].(string); strings.TrimSpace(provider) == "" {
		if source, _ := md["source"].(string); strings.TrimSpace(source) != "" {
			md["provider"] = source
		}
	}
	if progressKind, _ := md["progress_kind"].(string); strings.TrimSpace(progressKind) == "" {
		if errorKind, _ := md["error_kind"].(string); strings.TrimSpace(errorKind) != "" {
			md["progress_kind"] = "none"
		}
	}
	return md
}

func enrichScholarMetadata(md map[string]any, action, source, query string, limit, offset int) map[string]any {
	if md == nil {
		md = map[string]any{}
	}
	md["tool"] = "ScholarSearch"
	if strings.TrimSpace(action) != "" && md["action"] == nil {
		md["action"] = action
	}
	if strings.TrimSpace(source) != "" {
		if md["source"] == nil {
			md["source"] = source
		}
		if md["provider"] == nil {
			md["provider"] = source
		}
	}
	if md["query_key"] == nil && strings.TrimSpace(query) != "" {
		if strings.EqualFold(action, "search") {
			md["query_key"] = fmt.Sprintf("search:%s:%s:%d:%d", source, scholarNormalizeQueryPart(query), offset, limit)
		} else {
			md["query_key"] = scholarActionQueryKey(action, query, offset, limit)
		}
	}
	if md["progress_kind"] == nil {
		if errorKind, _ := md["error_kind"].(string); strings.TrimSpace(errorKind) != "" {
			md["progress_kind"] = "none"
		} else {
			md["progress_kind"] = "search_page"
		}
	}
	return md
}

func scholarSearchSuccessResponse(content string, md map[string]any, action, source, query string, limit, offset int) ToolResponse {
	md = enrichScholarMetadata(md, action, source, query, limit, offset)
	addScholarContentEvidence(md, content)
	return WithResponseMetadata(NewTextResponse(content), md)
}

func addScholarContentEvidence(md map[string]any, content string) {
	if md == nil {
		return
	}
	ids := candidateIDsFromScholarContent(content)
	if _, ok := md["candidate_count"]; !ok {
		md["candidate_count"] = len(ids)
	}
	if _, ok := md["evidence_keys"]; !ok {
		md["evidence_keys"] = stableScholarEvidenceKeysFromContent(content, 5)
	}
}

func candidateIDsFromScholarContent(content string) []string {
	out := make([]string, 0, 8)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "CandidateID:") {
			continue
		}
		id := strings.TrimSpace(strings.TrimPrefix(line, "CandidateID:"))
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func stableScholarEvidenceKeysFromContent(content string, max int) []string {
	out := make([]string, 0, max)
	seen := map[string]struct{}{}
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" || len(out) >= max {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "doi:"):
			add("doi:" + strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "DOI:"))))
		case strings.HasPrefix(lower, "arxiv:"):
			add("arxiv:" + strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "ArXiv:"))))
		case strings.HasPrefix(lower, "pdf:"):
			add("url:" + strings.TrimSpace(strings.TrimPrefix(line, "PDF:")))
		case strings.HasPrefix(lower, "full text:"):
			add("url:" + strings.TrimSpace(strings.TrimPrefix(line, "Full text:")))
		}
	}
	if len(out) >= max {
		return out
	}
	if len(out) > 0 {
		return out
	}
	for _, title := range scholarTitlesFromContent(content) {
		add("title_hash:" + stableTitleHash(title))
		if len(out) >= max {
			break
		}
	}
	return out
}

func scholarTitlesFromContent(content string) []string {
	out := make([]string, 0, 8)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "title:") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
			if title != "" {
				out = append(out, title)
			}
		}
	}
	return out
}

func stableTitleHash(title string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(title), " "))
	sum := sha256.Sum256([]byte(normalized))
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}

func scholarEvidenceKeys(papers []paperResult, max int) []string {
	out := make([]string, 0, min(len(papers), max))
	for i, p := range papers {
		if i >= max {
			break
		}
		if strings.TrimSpace(p.PaperID) != "" {
			out = append(out, "paper_id:"+strings.TrimSpace(p.PaperID))
			continue
		}
		if strings.TrimSpace(p.ExternalIDs.DOI) != "" {
			out = append(out, "doi:"+strings.ToLower(strings.TrimSpace(p.ExternalIDs.DOI)))
			continue
		}
		if strings.TrimSpace(p.ExternalIDs.ArXiv) != "" {
			out = append(out, "arxiv:"+strings.ToLower(strings.TrimSpace(p.ExternalIDs.ArXiv)))
			continue
		}
		if strings.TrimSpace(paperPDFURL(p)) != "" {
			out = append(out, "url:"+strings.TrimSpace(paperPDFURL(p)))
			continue
		}
		if strings.TrimSpace(p.Title) != "" {
			out = append(out, "title_hash:"+stableTitleHash(p.Title))
		}
	}
	return out
}

func scholarCandidateEvidenceKeys(candidates []scholarCandidate, max int) []string {
	papers := make([]paperResult, 0, min(len(candidates), max))
	for i, c := range candidates {
		if i >= max {
			break
		}
		papers = append(papers, c.Paper)
	}
	return scholarEvidenceKeys(papers, max)
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := time.ParseDuration(v + "s"); err == nil && secs >= 0 {
		return secs
	}
	if when, err := http.ParseTime(v); err == nil {
		return time.Until(when)
	}
	return 0
}

func scholarWorkflowAllAttemptsErrored(attempts []map[string]any) bool {
	if len(attempts) == 0 {
		return false
	}
	for _, attempt := range attempts {
		if result, _ := attempt["result"].(string); result != "error" {
			return false
		}
	}
	return true
}

func filterUsableScholarPapers(papers []paperResult) []paperResult {
	out := make([]paperResult, 0, len(papers))
	for _, p := range papers {
		if strings.TrimSpace(p.Title) == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func scholarSourceDisplayName(source string) string {
	switch source {
	case "semantic_scholar":
		return "Semantic Scholar"
	case "arxiv":
		return "arXiv"
	case "openalex":
		return "OpenAlex"
	case "crossref":
		return "CrossRef"
	case "pubmed":
		return "PubMed"
	case "core":
		return "CORE"
	case "eric":
		return "ERIC"
	case "patentsview":
		return "PatentsView"
	case "europepmc":
		return "Europe PMC"
	case "unpaywall":
		return "Unpaywall"
	default:
		return source
	}
}

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// --- Output formatting ---

func formatPaperSearchEntry(sb *strings.Builder, idx int, p paperResult) {
	citeKey := generateCiteKey(p)
	fmt.Fprintf(sb, "--- [%d] %s ---\n", idx, citeKey)
	fmt.Fprintf(sb, "Title: %s\n", p.Title)
	fmt.Fprintf(sb, "Authors: %s\n", formatAuthorsShort(p.Authors))
	fmt.Fprintf(sb, "Year: %d", p.Year)
	if p.Venue != "" {
		fmt.Fprintf(sb, " | Venue: %s", p.Venue)
	}
	if p.CitationCount > 0 {
		fmt.Fprintf(sb, " | Citations: %d", p.CitationCount)
	}
	sb.WriteString("\n")

	ids := []string{}
	if p.ExternalIDs.DOI != "" {
		ids = append(ids, "DOI: "+p.ExternalIDs.DOI)
	}
	if p.ExternalIDs.ArXiv != "" {
		ids = append(ids, "ArXiv: "+p.ExternalIDs.ArXiv)
	}
	if len(ids) > 0 {
		fmt.Fprintf(sb, "%s\n", strings.Join(ids, " | "))
	}
	if pdfURL := paperPDFURL(p); pdfURL != "" {
		fmt.Fprintf(sb, "PDF: %s\n", pdfURL)
	}
	sb.WriteString("\n")
}

func formatPaperEntry(sb *strings.Builder, idx int, p paperResult) {
	citeKey := generateCiteKey(p)
	fmt.Fprintf(sb, "--- [%d] %s ---\n", idx, citeKey)
	fmt.Fprintf(sb, "Title: %s\n", p.Title)
	fmt.Fprintf(sb, "Authors: %s\n", formatAuthorsShort(p.Authors))
	fmt.Fprintf(sb, "Year: %d", p.Year)
	if p.Venue != "" {
		fmt.Fprintf(sb, " | Venue: %s", p.Venue)
	}
	fmt.Fprintf(sb, " | Citations: %d\n", p.CitationCount)

	if p.ExternalIDs.DOI != "" {
		fmt.Fprintf(sb, "DOI: %s", p.ExternalIDs.DOI)
	}
	if p.ExternalIDs.ArXiv != "" {
		if p.ExternalIDs.DOI != "" {
			sb.WriteString(" | ")
		}
		fmt.Fprintf(sb, "ArXiv: %s", p.ExternalIDs.ArXiv)
	}
	if p.ExternalIDs.DOI != "" || p.ExternalIDs.ArXiv != "" {
		sb.WriteString("\n")
	}

	if p.OpenAccessPdf != nil && p.OpenAccessPdf.URL != "" {
		fmt.Fprintf(sb, "PDF: %s\n", p.OpenAccessPdf.URL)
	}

	if p.Abstract != "" {
		abstract := p.Abstract
		if len(abstract) > 300 {
			abstract = abstract[:300] + "..."
		}
		fmt.Fprintf(sb, "Abstract: %s\n", abstract)
	}

	fmt.Fprintf(sb, "\nBibTeX:\n%s\n\n", generateBibTeX(p))
}

func formatPaperBrief(sb *strings.Builder, idx int, p paperResult) {
	citeKey := generateCiteKey(p)
	fmt.Fprintf(sb, "[%d] %s — %s (%d)", idx, citeKey, p.Title, p.Year)
	if p.Venue != "" {
		fmt.Fprintf(sb, " [%s]", p.Venue)
	}
	fmt.Fprintf(sb, " (cited: %d)\n", p.CitationCount)

	if p.ExternalIDs.DOI != "" {
		fmt.Fprintf(sb, "    DOI: %s", p.ExternalIDs.DOI)
	}
	if p.ExternalIDs.ArXiv != "" {
		if p.ExternalIDs.DOI != "" {
			sb.WriteString(" | ")
		} else {
			sb.WriteString("    ")
		}
		fmt.Fprintf(sb, "ArXiv: %s", p.ExternalIDs.ArXiv)
	}
	if p.ExternalIDs.DOI != "" || p.ExternalIDs.ArXiv != "" {
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

// --- BibTeX generation ---

func generateBibTeX(p paperResult) string {
	citeKey := generateCiteKey(p)
	entryType := determineBibEntryType(p)
	authors := formatAuthorsBibTeX(p.Authors)

	var sb strings.Builder
	fmt.Fprintf(&sb, "@%s{%s,\n", entryType, citeKey)
	fmt.Fprintf(&sb, "  title     = {%s},\n", protectTitle(p.Title))
	fmt.Fprintf(&sb, "  author    = {%s},\n", authors)
	fmt.Fprintf(&sb, "  year      = {%d}", p.Year)

	switch entryType {
	case "inproceedings":
		if p.Venue != "" {
			fmt.Fprintf(&sb, ",\n  booktitle = {%s}", p.Venue)
		}
	case "article":
		if p.Venue != "" {
			fmt.Fprintf(&sb, ",\n  journal   = {%s}", p.Venue)
		}
	case "misc":
		if p.ExternalIDs.ArXiv != "" {
			fmt.Fprintf(&sb, ",\n  eprint    = {%s},\n  archiveprefix = {arXiv}", p.ExternalIDs.ArXiv)
		}
	}

	if p.ExternalIDs.DOI != "" {
		fmt.Fprintf(&sb, ",\n  doi       = {%s}", p.ExternalIDs.DOI)
	}

	sb.WriteString("\n}")
	return sb.String()
}

func generateCiteKey(p paperResult) string {
	// First author last name
	lastName := "unknown"
	if len(p.Authors) > 0 {
		lastName = extractLastName(p.Authors[0].Name)
	}

	// First content word from title
	contentWord := extractFirstContentWord(p.Title)

	year := p.Year
	if year == 0 {
		year = 9999
	}

	return fmt.Sprintf("%s%d%s", lastName, year, contentWord)
}

func extractLastName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "unknown"
	}
	last := parts[len(parts)-1]
	// Keep only letters
	var sb strings.Builder
	for _, r := range last {
		if unicode.IsLetter(r) {
			sb.WriteRune(unicode.ToLower(r))
		}
	}
	if sb.Len() == 0 {
		return "unknown"
	}
	return sb.String()
}

var titleStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "for": true,
	"in": true, "on": true, "and": true, "to": true, "with": true,
	"is": true, "are": true, "by": true, "from": true, "at": true,
	"as": true, "or": true, "its": true, "via": true, "how": true,
	"do": true, "does": true, "can": true, "not": true, "what": true,
}

func extractFirstContentWord(title string) string {
	words := strings.Fields(strings.ToLower(title))
	for _, w := range words {
		// Strip non-alpha characters
		var sb strings.Builder
		for _, r := range w {
			if unicode.IsLetter(r) {
				sb.WriteRune(r)
			}
		}
		clean := sb.String()
		if clean == "" {
			continue
		}
		if titleStopwords[clean] {
			continue
		}
		return clean
	}
	// Fallback: first word
	if len(words) > 0 {
		return words[0]
	}
	return "paper"
}

func determineBibEntryType(p paperResult) string {
	// ArXiv only → misc
	if p.ExternalIDs.ArXiv != "" && p.Venue == "" {
		return "misc"
	}

	// Check if venue looks like a journal
	venueLower := strings.ToLower(p.Venue)
	journalKeywords := []string{"journal", "transactions", "letters", "review", "magazine"}
	for _, kw := range journalKeywords {
		if strings.Contains(venueLower, kw) {
			return "article"
		}
	}

	return "inproceedings"
}

func formatAuthorsBibTeX(authors []authorResult) string {
	if len(authors) == 0 {
		return "Unknown"
	}

	var parts []string
	maxAuthors := 5
	for i, a := range authors {
		if i >= maxAuthors {
			parts = append(parts, "others")
			break
		}
		parts = append(parts, flipAuthorName(a.Name))
	}
	return strings.Join(parts, " and ")
}

func flipAuthorName(name string) string {
	// "First Middle Last" -> "Last, First Middle"
	fields := strings.Fields(name)
	if len(fields) <= 1 {
		return name
	}
	last := fields[len(fields)-1]
	first := strings.Join(fields[:len(fields)-1], " ")
	return last + ", " + first
}

func formatAuthorsShort(authors []authorResult) string {
	if len(authors) == 0 {
		return "Unknown"
	}
	var names []string
	for i, a := range authors {
		if i >= 3 {
			names = append(names, fmt.Sprintf("... (%d authors total)", len(authors)))
			break
		}
		names = append(names, a.Name)
	}
	return strings.Join(names, "; ")
}

// protectTitle wraps known acronyms/proper nouns in {} for BibTeX.
var protectedWords = map[string]bool{
	"Transformer": true, "Transformers": true, "BERT": true, "GPT": true,
	"ResNet": true, "ImageNet": true, "LSTM": true, "GAN": true, "GANs": true,
	"CNN": true, "RNN": true, "ViT": true, "CLIP": true, "DALL-E": true,
	"AlexNet": true, "VGG": true, "GoogLeNet": true, "DenseNet": true,
	"YOLO": true, "SSD": true, "Adam": true, "SGD": true, "BatchNorm": true,
	"LayerNorm": true, "Attention": true, "Bitcoin": true, "Ethereum": true,
	"Blockchain": true, "DeFi": true, "NFT": true, "IoT": true, "GPU": true,
	"TPU": true, "CUDA": true, "PyTorch": true, "TensorFlow": true,
	"Mamba": true, "LLaMA": true, "ChatGPT": true, "GPT-4": true,
	"DETR": true, "DINO": true, "SAM": true, "Swin": true, "ConvNeXt": true,
}

func protectTitle(title string) string {
	words := strings.Fields(title)
	for i, w := range words {
		// Strip trailing punctuation for lookup
		clean := strings.TrimRight(w, ".,;:!?")
		if protectedWords[clean] {
			words[i] = "{" + w + "}"
		}
	}
	return strings.Join(words, " ")
}
