package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

type scholarSearchResult struct {
	Source string
	Query  string
	Meta   map[string]any
	Err    error
	Papers []paperResult
}

type scholarSearchWorkflowResult struct {
	SelectedSource string
	Sources        []string
	Candidates     []scholarCandidate
	Attempts       []map[string]any
}

type scholarWorkflowPaper struct {
	Source string
	Paper  paperResult
}

type scholarDownloadAttempt struct {
	CandidateID string
	Source      string
	Title       string
	Result      string
	Reason      string
	FilePath    string
}

type scholarDownloadWorkflowResult struct {
	Selected scholarCandidate
	Attempts []scholarDownloadAttempt
}

type scholarSearchFilters struct {
	YearMin int
	YearMax int
}

func scholarSearchFiltersFromParams(params scholarSearchParams) scholarSearchFilters {
	return scholarSearchFilters{
		YearMin: int(params.YearMin),
		YearMax: int(params.YearMax),
	}
}

func (f scholarSearchFilters) empty() bool {
	return f.YearMin <= 0 && f.YearMax <= 0
}

func (f scholarSearchFilters) apply(papers []paperResult) ([]paperResult, int) {
	if f.empty() || len(papers) == 0 {
		return papers, 0
	}
	out := make([]paperResult, 0, len(papers))
	filtered := 0
	for _, p := range papers {
		if p.Year == 0 {
			filtered++
			continue
		}
		if f.YearMin > 0 && p.Year < f.YearMin {
			filtered++
			continue
		}
		if f.YearMax > 0 && p.Year > f.YearMax {
			filtered++
			continue
		}
		out = append(out, p)
	}
	return out, filtered
}

func (f scholarSearchFilters) addMetadata(md map[string]any, filteredOut int) {
	if md == nil {
		return
	}
	if f.YearMin > 0 {
		md["year_min"] = f.YearMin
	}
	if f.YearMax > 0 {
		md["year_max"] = f.YearMax
	}
	if filteredOut > 0 {
		md["filtered_by_year_count"] = filteredOut
	}
}

func normalizeScholarTitle(s string) string {
	return compactAlnum(s)
}

func dedupeScholarPapers(papers []paperResult) []paperResult {
	out := make([]paperResult, 0, len(papers))
	seen := map[string]int{}
	for _, p := range papers {
		keys := scholarPaperDedupeKeys(p)
		duplicateIndex := -1
		for _, k := range keys {
			if idx, ok := seen[k]; ok {
				duplicateIndex = idx
				break
			}
		}
		if duplicateIndex >= 0 {
			if strings.TrimSpace(paperPDFURL(out[duplicateIndex])) == "" && strings.TrimSpace(paperPDFURL(p)) != "" {
				out[duplicateIndex] = p
				for _, k := range keys {
					seen[k] = duplicateIndex
				}
			}
			continue
		}
		idx := len(out)
		out = append(out, p)
		for _, k := range keys {
			seen[k] = idx
		}
	}
	return out
}

func scholarPaperDedupeKeys(p paperResult) []string {
	keys := []string{}
	if doi := normalizePaperIdentifier("doi:" + p.ExternalIDs.DOI); doi != "" {
		keys = append(keys, "doi:"+doi)
	}
	if ax := normalizePaperIdentifier("arxiv:" + p.ExternalIDs.ArXiv); ax != "" {
		keys = append(keys, "arxiv:"+ax)
	}
	if pdf := strings.TrimSpace(strings.ToLower(paperPDFURL(p))); pdf != "" {
		keys = append(keys, "url:"+pdf)
	}
	if title := normalizeScholarTitle(p.Title); title != "" {
		keys = append(keys, "title:"+title)
	}
	return keys
}

func (t *scholarSearchTool) runSearchWorkflow(ctx context.Context, query string, limit, offset int, schedule []string, filters scholarSearchFilters) scholarSearchWorkflowResult {
	if len(schedule) == 0 {
		schedule = defaultScholarSearchWorkflowSchedule()
	}
	result := scholarSearchWorkflowResult{}
	if limit <= 0 {
		limit = 5
	}
	if limit <= 3 {
		return t.runSearchWorkflowSequential(ctx, query, limit, offset, schedule, filters)
	}
	if !filters.empty() {
		return t.runSearchWorkflowSequential(ctx, query, limit, offset, schedule, filters)
	}
	sourceLimit := scholarWorkflowSourceLimit(limit)
	activeSchedule := schedule
	neededSources := (limit + sourceLimit - 1) / sourceLimit
	if neededSources < 1 {
		neededSources = 1
	}
	if neededSources < len(activeSchedule) {
		activeSchedule = activeSchedule[:neededSources]
	}
	collected := []scholarWorkflowPaper{}
	results := make([]scholarSearchResult, len(activeSchedule))
	var wg sync.WaitGroup
	sem := make(chan struct{}, defaultScholarSearchWorkflowConcurrency())
	for i, source := range activeSchedule {
		wg.Add(1)
		go func(i int, source string) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				results[i] = scholarSearchResult{Source: source, Query: query, Err: ctx.Err()}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			results[i] = t.searchSource(ctx, source, query, sourceLimit, offset, filters)
		}(i, source)
	}
	wg.Wait()

	for i, source := range activeSchedule {
		if len(collected) >= limit {
			break
		}
		attempt := map[string]any{"source": source}
		res := results[i]
		if res.Meta != nil {
			attempt["meta"] = res.Meta
		}
		if res.Err != nil {
			attempt["result"] = "error"
			attempt["reason"] = res.Err.Error()
			result.Attempts = append(result.Attempts, attempt)
			continue
		}
		if len(res.Papers) == 0 {
			attempt["result"] = "empty"
			result.Attempts = append(result.Attempts, attempt)
			continue
		}
		deduped := dedupeScholarPapers(res.Papers)
		attempt["result"] = "ok"
		attempt["raw_count"] = len(res.Papers)
		attempt["deduped_count"] = len(deduped)
		result.Attempts = append(result.Attempts, attempt)
		if result.SelectedSource == "" {
			result.SelectedSource = source
		}
		collected = mergeScholarWorkflowPapers(collected, source, deduped, limit)
		result.Sources = scholarWorkflowSources(collected)
	}
	for i, item := range collected {
		result.Candidates = append(result.Candidates, t.registerCandidate(ctx, item.Source, query, offset+i+1, item.Paper))
	}
	return result
}

func (t *scholarSearchTool) runSearchWorkflowSequential(ctx context.Context, query string, limit, offset int, schedule []string, filters scholarSearchFilters) scholarSearchWorkflowResult {
	result := scholarSearchWorkflowResult{}
	collected := []scholarWorkflowPaper{}
	for _, source := range schedule {
		if len(collected) >= limit {
			break
		}
		attempt := map[string]any{"source": source}
		sourceLimit := scholarWorkflowSourceLimit(limit)
		if remaining := limit - len(collected); remaining < sourceLimit {
			sourceLimit = remaining
		}
		res := t.searchSource(ctx, source, query, sourceLimit, offset, filters)
		if res.Meta != nil {
			attempt["meta"] = res.Meta
		}
		if res.Err != nil {
			attempt["result"] = "error"
			attempt["reason"] = res.Err.Error()
			result.Attempts = append(result.Attempts, attempt)
			continue
		}
		if len(res.Papers) == 0 {
			attempt["result"] = "empty"
			result.Attempts = append(result.Attempts, attempt)
			continue
		}
		deduped := dedupeScholarPapers(res.Papers)
		attempt["result"] = "ok"
		attempt["raw_count"] = len(res.Papers)
		attempt["deduped_count"] = len(deduped)
		result.Attempts = append(result.Attempts, attempt)
		if result.SelectedSource == "" {
			result.SelectedSource = source
		}
		collected = mergeScholarWorkflowPapers(collected, source, deduped, limit)
		result.Sources = scholarWorkflowSources(collected)
	}
	for i, item := range collected {
		result.Candidates = append(result.Candidates, t.registerCandidate(ctx, item.Source, query, offset+i+1, item.Paper))
	}
	return result
}

func defaultScholarSearchWorkflowSchedule() []string {
	return []string{
		"semantic_scholar",
		"openalex",
		"arxiv",
		"crossref",
		"pubmed",
		"europepmc",
	}
}

func scholarWorkflowSourceLimit(limit int) int {
	if limit <= 3 {
		return limit
	}
	return 3
}

func defaultScholarSearchWorkflowConcurrency() int {
	return 4
}

func mergeScholarWorkflowPapers(existing []scholarWorkflowPaper, source string, papers []paperResult, limit int) []scholarWorkflowPaper {
	for _, p := range papers {
		duplicateIndex := scholarWorkflowDuplicateIndex(existing, p)
		if duplicateIndex >= 0 {
			if strings.TrimSpace(paperPDFURL(existing[duplicateIndex].Paper)) == "" && strings.TrimSpace(paperPDFURL(p)) != "" {
				existing[duplicateIndex] = scholarWorkflowPaper{Source: source, Paper: p}
			}
			continue
		}
		if len(existing) >= limit {
			continue
		}
		existing = append(existing, scholarWorkflowPaper{Source: source, Paper: p})
	}
	return existing
}

func scholarWorkflowDuplicateIndex(existing []scholarWorkflowPaper, p paperResult) int {
	keys := scholarPaperDedupeKeys(p)
	for idx, item := range existing {
		for _, existingKey := range scholarPaperDedupeKeys(item.Paper) {
			for _, key := range keys {
				if key == existingKey {
					return idx
				}
			}
		}
	}
	return -1
}

func scholarWorkflowSources(items []scholarWorkflowPaper) []string {
	out := []string{}
	for _, item := range items {
		out = appendUniqueString(out, item.Source)
	}
	return out
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (t *scholarSearchTool) runDownloadWorkflow(candidates []scholarCandidate, params scholarSearchParams) scholarDownloadWorkflowResult {
	res := scholarDownloadWorkflowResult{}
	for _, candidate := range dedupeScholarCandidates(candidates) {
		attempt := scholarDownloadAttempt{
			CandidateID: candidate.CandidateID,
			Source:      candidate.Source,
			Title:       candidate.Paper.Title,
		}
		if strings.TrimSpace(candidate.PDFURL) == "" {
			attempt.Result = "rejected"
			attempt.Reason = "candidate has no open-access PDF"
			res.Attempts = append(res.Attempts, attempt)
			continue
		}
		verdict := validateDownloadCandidate(candidate, params)
		if !verdict.Allowed {
			attempt.Result = "rejected"
			attempt.Reason = strings.Join(verdict.Reasons, "; ")
			res.Attempts = append(res.Attempts, attempt)
			continue
		}
		attempt.Result = "selected"
		res.Selected = candidate
		res.Attempts = append(res.Attempts, attempt)
		break
	}
	return res
}

func dedupeScholarCandidates(candidates []scholarCandidate) []scholarCandidate {
	keep := make([]scholarCandidate, 0, len(candidates))
	seen := map[string]int{}
	for _, c := range candidates {
		keys := scholarCandidateDedupeKeys(c)
		duplicateIndex := -1
		for _, k := range keys {
			if idx, ok := seen[k]; ok {
				duplicateIndex = idx
				break
			}
		}
		if duplicateIndex >= 0 {
			if strings.TrimSpace(keep[duplicateIndex].PDFURL) == "" && strings.TrimSpace(c.PDFURL) != "" {
				keep[duplicateIndex] = c
				for _, k := range keys {
					seen[k] = duplicateIndex
				}
			}
			continue
		}
		idx := len(keep)
		keep = append(keep, c)
		for _, k := range keys {
			seen[k] = idx
		}
	}
	return keep
}

func scholarCandidateDedupeKeys(c scholarCandidate) []string {
	keys := []string{}
	if doi := normalizePaperIdentifier("doi:" + c.Paper.ExternalIDs.DOI); doi != "" {
		keys = append(keys, "doi:"+doi)
	}
	if ax := normalizePaperIdentifier("arxiv:" + c.Paper.ExternalIDs.ArXiv); ax != "" {
		keys = append(keys, "arxiv:"+ax)
	}
	if pdf := strings.TrimSpace(strings.ToLower(c.PDFURL)); pdf != "" {
		keys = append(keys, "url:"+pdf)
	}
	if title := normalizeScholarTitle(c.Paper.Title); title != "" {
		keys = append(keys, "title:"+title)
	}
	return keys
}

func scholarCandidateSources(candidates []scholarCandidate, maxSources int) []map[string]string {
	if maxSources <= 0 {
		maxSources = len(candidates)
	}
	sources := make([]map[string]string, 0, min(len(candidates), maxSources))
	for i, c := range candidates {
		if i >= maxSources {
			break
		}
		u := strings.TrimSpace(c.PDFURL)
		if u == "" {
			u = strings.TrimSpace(firstNonEmpty(c.Paper.ExternalIDs.DOI, c.Paper.ExternalIDs.ArXiv, c.Paper.PaperID))
		}
		sources = append(sources, map[string]string{
			"title":   strings.TrimSpace(c.Paper.Title),
			"url":     u,
			"snippet": strings.TrimSpace(fmt.Sprintf("%s %d %s", c.Source, c.Paper.Year, c.Paper.Venue)),
		})
	}
	return sources
}

func (t *scholarSearchTool) searchSource(ctx context.Context, source, query string, limit, offset int, filters scholarSearchFilters) scholarSearchResult {
	collect := func(target *[]paperResult) scholarCandidateRegistrar {
		return func(rank int, p paperResult) string {
			*target = append(*target, p)
			return ""
		}
	}
	switch source {
	case "semantic_scholar":
		path := fmt.Sprintf("/paper/search?query=%s&limit=%d&offset=%d&fields=%s",
			url.QueryEscape(query), limit, offset, scholarDefaultFields)
		queryKey := scholarSearchQueryKey(query, offset, limit)
		body, meta, cacheHit := scholarCacheGet("semantic_scholar", "search", queryKey)
		if !cacheHit {
			var stats scholarAPIStats
			var err error
			body, stats, err = scholarAPIGetWithStats(ctx, path)
			if err != nil {
				return scholarSearchResult{Source: source, Query: query, Err: err}
			}
			scholarCacheSet(queryKey, body)
			meta = map[string]any{
				"source":      "semantic_scholar",
				"action":      "search",
				"query_key":   queryKey,
				"cache_hit":   false,
				"attempts":    stats.Attempts,
				"retry_count": stats.RetryCount,
			}
		}
		var resp searchResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return scholarSearchResult{Source: source, Query: query, Err: err}
		}
		papers, filteredOut := filters.apply(resp.Data)
		if !filters.empty() {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["raw_candidate_count_before_filters"] = len(resp.Data)
			filters.addMetadata(meta, filteredOut)
		}
		return scholarSearchResult{Source: source, Query: query, Meta: meta, Papers: papers}
	case "openalex":
		var papers []paperResult
		_, meta, err := openAlexSearchWithCandidates(ctx, query, limit, offset, collect(&papers))
		papers, filteredOut := filters.apply(papers)
		if !filters.empty() {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["raw_candidate_count_before_filters"] = len(papers) + filteredOut
			filters.addMetadata(meta, filteredOut)
		}
		return scholarSearchResult{Source: source, Query: query, Meta: meta, Err: err, Papers: papers}
	case "arxiv":
		var papers []paperResult
		_, meta, err := arxivSearchWithCandidates(ctx, query, limit, offset, collect(&papers))
		papers, filteredOut := filters.apply(papers)
		if !filters.empty() {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["raw_candidate_count_before_filters"] = len(papers) + filteredOut
			filters.addMetadata(meta, filteredOut)
		}
		return scholarSearchResult{Source: source, Query: query, Meta: meta, Err: err, Papers: papers}
	case "crossref":
		var papers []paperResult
		_, meta, err := crossrefSearchWithCandidates(ctx, query, limit, offset, collect(&papers))
		papers, filteredOut := filters.apply(papers)
		if !filters.empty() {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["raw_candidate_count_before_filters"] = len(papers) + filteredOut
			filters.addMetadata(meta, filteredOut)
		}
		return scholarSearchResult{Source: source, Query: query, Meta: meta, Err: err, Papers: papers}
	case "pubmed":
		var papers []paperResult
		_, meta, err := pubmedSearchWithCandidates(ctx, query, limit, offset, collect(&papers))
		papers, filteredOut := filters.apply(papers)
		if !filters.empty() {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["raw_candidate_count_before_filters"] = len(papers) + filteredOut
			filters.addMetadata(meta, filteredOut)
		}
		return scholarSearchResult{Source: source, Query: query, Meta: meta, Err: err, Papers: papers}
	case "core":
		var papers []paperResult
		_, meta, err := coreSearchWithCandidates(ctx, query, limit, offset, collect(&papers))
		papers, filteredOut := filters.apply(papers)
		if !filters.empty() {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["raw_candidate_count_before_filters"] = len(papers) + filteredOut
			filters.addMetadata(meta, filteredOut)
		}
		return scholarSearchResult{Source: source, Query: query, Meta: meta, Err: err, Papers: papers}
	case "eric":
		var papers []paperResult
		_, meta, err := ericSearchWithCandidates(ctx, query, limit, offset, collect(&papers))
		papers, filteredOut := filters.apply(papers)
		if !filters.empty() {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["raw_candidate_count_before_filters"] = len(papers) + filteredOut
			filters.addMetadata(meta, filteredOut)
		}
		return scholarSearchResult{Source: source, Query: query, Meta: meta, Err: err, Papers: papers}
	case "europepmc":
		var papers []paperResult
		_, meta, err := europePMCSearchWithCandidates(ctx, query, limit, offset, collect(&papers))
		papers, filteredOut := filters.apply(papers)
		if !filters.empty() {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["raw_candidate_count_before_filters"] = len(papers) + filteredOut
			filters.addMetadata(meta, filteredOut)
		}
		return scholarSearchResult{Source: source, Query: query, Meta: meta, Err: err, Papers: papers}
	case "patentsview":
		var papers []paperResult
		_, meta, err := patentsviewSearchWithCandidates(ctx, query, limit, offset, collect(&papers))
		papers, filteredOut := filters.apply(papers)
		if !filters.empty() {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["raw_candidate_count_before_filters"] = len(papers) + filteredOut
			filters.addMetadata(meta, filteredOut)
		}
		return scholarSearchResult{Source: source, Query: query, Meta: meta, Err: err, Papers: papers}
	}
	return scholarSearchResult{Source: source, Query: query, Err: fmt.Errorf("unsupported source: %s", source)}
}
