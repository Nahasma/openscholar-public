package kb

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/openscholar/openscholar/internal/db"
)

// CrossSearchResult contains the synthesized answer from searching across all papers.
type CrossSearchResult struct {
	Answer      string                 `json:"answer"`
	Sources     []CrossSourceRef       `json:"sources"`
	Diagnostics CrossSearchDiagnostics `json:"diagnostics,omitempty"`
}

// CrossSourceRef identifies a source from a cross-paper search.
type CrossSourceRef struct {
	PaperID    string `json:"paper_id"`
	PaperTitle string `json:"paper_title"`
	NodeID     string `json:"node_id,omitempty"`
	NodeTitle  string `json:"node_title,omitempty"`
	StartPage  int    `json:"start_page,omitempty"`
	EndPage    int    `json:"end_page,omitempty"`
}

type channelErr struct {
	name string
	err  error
}

type paperAnswer struct {
	paperID string
	title   string
	answer  string
	sources []SourceRef
	diag    SearchDiagnostics
	err     error
}

// CrossPaperSearch searches across all papers using FTS5 + RRF fusion.
func (s *service) CrossPaperSearch(ctx context.Context, callLLM LLMCaller, question string, limit int) (*CrossSearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	sanitized := SanitizeFTSQuery(question)
	if strings.TrimSpace(sanitized) == "" {
		return &CrossSearchResult{
			Answer: "No searchable terms found after sanitization. Please include specific keywords.",
			Diagnostics: CrossSearchDiagnostics{
				Query:          question,
				FTSQuery:       sanitized,
				SanitizedQuery: sanitized,
				NoHitReason:    "empty_query_after_sanitization",
				FinalRoute:     "no_hits",
			},
		}, nil
	}

	var searchErrs []channelErr
	diagnostics := CrossSearchDiagnostics{
		Query:          question,
		FTSQuery:       sanitized,
		SanitizedQuery: sanitized,
	}

	// 1. FTS5 search on papers
	var paperResults []FTSResult
	if s.fts != nil {
		var err error
		paperResults, err = s.fts.SearchPapers(ctx, sanitized, 20)
		if err != nil {
			searchErrs = append(searchErrs, channelErr{name: "papers_fts", err: err})
		}
	} else {
		searchErrs = append(searchErrs, channelErr{name: "papers_fts", err: errors.New("FTS5 searcher unavailable")})
	}
	diagnostics.Channels = append(diagnostics.Channels, channelHit("papers_fts", sanitized, len(paperResults), 0, channelErrors(searchErrs, "papers_fts")))

	// 2. FTS5 search on nodes
	var nodeResults []FTSResult
	if s.fts != nil {
		var err error
		nodeResults, err = s.fts.SearchNodes(ctx, sanitized, 50)
		if err != nil {
			searchErrs = append(searchErrs, channelErr{name: "node_summaries_fts", err: err})
		}
	}
	diagnostics.Channels = append(diagnostics.Channels, channelHit("node_summaries_fts", sanitized, len(nodeResults), 0, channelErrors(searchErrs, "node_summaries_fts")))

	// 3. FTS5 search on raw chunks
	var chunkResults []db.PaperChunk
	chunkRows, chunkErr := s.searchAllPaperChunksTitleContent(ctx, sanitized, 80)
	if chunkErr != nil {
		searchErrs = append(searchErrs, channelErr{name: "paper_chunks_fts", err: chunkErr})
	} else {
		chunkResults = chunkRows
	}
	diagnostics.Channels = append(diagnostics.Channels, channelHit("paper_chunks_fts", sanitized, len(chunkResults), len(chunkResults), channelErrors(searchErrs, "paper_chunks_fts")))
	diagnostics.CandidateChunkCount = len(chunkResults)
	if len(searchErrs) > 0 {
		diagnostics.Errors = append(diagnostics.Errors, splitErrorSummary(formatSearchErrors(searchErrs))...)
	}

	if len(paperResults) == 0 && len(nodeResults) == 0 && len(chunkResults) == 0 && len(searchErrs) > 0 {
		return nil, fmt.Errorf("cross-paper search failed: %s", formatSearchErrors(searchErrs))
	}

	// 4. RRF merge
	merged := RRFMerge(60, ftsToRanked(paperResults), ftsToRanked(nodeResults), chunksToRanked(chunkResults))

	// 5. Extract unique top papers
	seen := make(map[string]bool)
	var topPaperIDs []string
	hintsByPaper := make(map[string][]string)
	paperDiagByID := make(map[string]*CrossPaperDiagnostics)
	for _, item := range merged {
		if !seen[item.PaperID] {
			seen[item.PaperID] = true
			topPaperIDs = append(topPaperIDs, item.PaperID)
		}
		if _, ok := paperDiagByID[item.PaperID]; !ok {
			paperDiagByID[item.PaperID] = &CrossPaperDiagnostics{
				PaperID:    item.PaperID,
				FTSQuery:   sanitized,
				FinalRoute: "candidate",
			}
		}
		ch := paperDiagByID[item.PaperID]
		if item.NodeID != "" {
			ch.CandidateChunkCount++
		}
		if strings.HasPrefix(item.NodeID, "page_") || strings.HasPrefix(item.NodeID, "chunk_") {
			ch.HitChannels = appendIfMissing(ch.HitChannels, "paper_chunks_fts")
		} else {
			ch.HitChannels = appendIfMissing(ch.HitChannels, "node_summaries_fts")
		}
		if item.NodeID != "" {
			hintsByPaper[item.PaperID] = appendIfMissing(hintsByPaper[item.PaperID], item.NodeID)
		}
		if len(topPaperIDs) >= limit {
			break
		}
	}

	if len(topPaperIDs) == 0 {
		errSummary := formatSearchErrors(searchErrs)
		diagnostics.NoHitReason = "no_channel_hits"
		diagnostics.FinalRoute = "no_hits"
		return &CrossSearchResult{
			Answer: fmt.Sprintf("No relevant papers found.\nSanitized query: %s\nHits: papers_fts=%d, node_summaries_fts=%d, paper_chunks_fts=%d\nErrors: %s",
				sanitized, len(paperResults), len(nodeResults), len(chunkResults), errSummary),
			Diagnostics: diagnostics,
		}, nil
	}
	diagnostics.CandidatePaperCount = len(topPaperIDs)

	// 6. QueryPaper on each top paper
	var answers []paperAnswer

	querySvc, ok := any(s).(interface {
		QueryPaper(ctx context.Context, callLLM LLMCaller, paperID string, question string, opts QueryOptions) (*SearchResult, error)
	})
	if !ok {
		return nil, fmt.Errorf("KB service does not support QueryPaper")
	}

	for _, pid := range topPaperIDs {
		paper, err := s.GetPaper(ctx, pid)
		if err != nil {
			answers = append(answers, paperAnswer{paperID: pid, err: err})
			continue
		}
		result, err := querySvc.QueryPaper(ctx, callLLM, pid, question, QueryOptions{
			PreferredRoute: "auto",
			CandidateHints: hintsByPaper[pid],
		})
		if err != nil {
			if pd := paperDiagByID[pid]; pd != nil {
				pd.Errors = append(pd.Errors, boundedSnippet(err.Error()))
				pd.FinalRoute = "error"
			}
			answers = append(answers, paperAnswer{paperID: pid, title: paper.Title, err: err})
			continue
		}
		if pd := paperDiagByID[pid]; pd != nil {
			pd.PaperTitle = paper.Title
			pd.FinalRoute = valueOr(result.Diagnostics.Route, "answered")
			pd.LLMCalled = result.Diagnostics.LLMCalled || result.Diagnostics.LLMCallCount > 0
			pd.LLMCallCount = result.Diagnostics.LLMCallCount
			pd.SourceCount = len(result.Sources)
			pd.FallbackReason = result.Diagnostics.FallbackReason
			pd.NoHitReason = result.Diagnostics.NoHitReason
			if result.Diagnostics.Warning != "" {
				pd.Warnings = append(pd.Warnings, result.Diagnostics.Warning)
			}
			pd.Warnings = append(pd.Warnings, result.Diagnostics.Warnings...)
			pd.Errors = append(pd.Errors, result.Diagnostics.Errors...)
			if result.Diagnostics.CandidateChunkCount > 0 {
				pd.CandidateChunkCount = result.Diagnostics.CandidateChunkCount
			}
		}
		answers = append(answers, paperAnswer{
			paperID: pid,
			title:   paper.Title,
			answer:  result.Answer,
			sources: result.Sources,
			diag:    result.Diagnostics,
		})
	}
	diagnostics.Papers = flattenPaperDiags(topPaperIDs, paperDiagByID)

	if len(answers) == 0 {
		diagnostics.NoHitReason = "all_candidate_queries_failed"
		diagnostics.FinalRoute = "all_failed"
		return &CrossSearchResult{
			Answer: fmt.Sprintf("Found candidate papers but no per-paper answers.\nSanitized query: %s\nHits: papers_fts=%d, node_summaries_fts=%d, paper_chunks_fts=%d\nErrors: %s",
				sanitized, len(paperResults), len(nodeResults), len(chunkResults), collectAnswerErrors(answers)),
			Diagnostics: diagnostics,
		}, nil
	}
	filtered := answers[:0]
	for _, a := range answers {
		if a.err == nil {
			filtered = append(filtered, a)
		}
	}
	answers = filtered
	if len(answers) == 0 {
		diagnostics.NoHitReason = "all_candidate_queries_failed"
		diagnostics.FinalRoute = "all_failed"
		return &CrossSearchResult{
			Answer:      "All per-paper queries failed. Try /ask-paper with a specific paper ID.",
			Diagnostics: diagnostics,
		}, nil
	}

	// 7. Single paper: return directly
	if len(answers) == 1 {
		a := answers[0]
		sources := make([]CrossSourceRef, len(a.sources))
		for i, src := range a.sources {
			sources[i] = CrossSourceRef{
				PaperID: a.paperID, PaperTitle: a.title,
				NodeID: src.NodeID, NodeTitle: src.Title,
				StartPage: src.StartPage, EndPage: src.EndPage,
			}
		}
		diagnostics.FinalRoute = valueOr(a.diag.Route, "single_paper")
		diagnostics.LLMCallCount = a.diag.LLMCallCount
		diagnostics.LLMCalled = a.diag.LLMCalled || a.diag.LLMCallCount > 0
		diagnostics.SourceCount = len(sources)
		diagnostics.FallbackReason = a.diag.FallbackReason
		diagnostics.NoHitReason = a.diag.NoHitReason
		return &CrossSearchResult{Answer: a.answer, Sources: sources, Diagnostics: diagnostics}, nil
	}

	// 8. Multi-paper: LLM synthesis
	var contextParts []string
	var allSources []CrossSourceRef
	for _, a := range answers {
		contextParts = append(contextParts, fmt.Sprintf("## Paper: %s (ID: %s)\n%s", a.title, a.paperID, a.answer))
		for _, src := range a.sources {
			allSources = append(allSources, CrossSourceRef{
				PaperID: a.paperID, PaperTitle: a.title,
				NodeID: src.NodeID, NodeTitle: src.Title,
				StartPage: src.StartPage, EndPage: src.EndPage,
			})
		}
	}

	prompt := fmt.Sprintf(crossPaperSynthesisPrompt, question, strings.Join(contextParts, "\n\n"))
	response, err := callLLM(ctx, prompt)
	if err != nil {
		// Fallback: concatenate
		var sb strings.Builder
		for _, a := range answers {
			fmt.Fprintf(&sb, "**%s**: %s\n\n", a.title, a.answer)
		}
		diagnostics.FinalRoute = "multi_paper_fallback_concat"
		diagnostics.LLMCalled = true
		diagnostics.LLMCallCount = totalLLMCallCount(answers) + 1
		diagnostics.SourceCount = len(allSources)
		diagnostics.FallbackReason = "synthesis_llm_failed"
		diagnostics.Warnings = append(diagnostics.Warnings, boundedSnippet(err.Error()))
		return &CrossSearchResult{Answer: sb.String(), Sources: allSources, Diagnostics: diagnostics}, nil
	}

	diagnostics.FinalRoute = "multi_paper_synthesis"
	diagnostics.LLMCalled = true
	diagnostics.LLMCallCount = totalLLMCallCount(answers) + 1
	diagnostics.SourceCount = len(allSources)
	return &CrossSearchResult{Answer: response, Sources: allSources, Diagnostics: diagnostics}, nil
}

func chunksToRanked(chunks []db.PaperChunk) []RankedItem {
	items := make([]RankedItem, 0, len(chunks))
	for _, ch := range chunks {
		title := ch.Title.String
		if title == "" {
			title = ch.ChunkID
		}
		items = append(items, RankedItem{
			PaperID: ch.PaperID,
			NodeID:  ch.ChunkID,
			Title:   title,
			Snippet: ch.Content,
		})
	}
	return items
}

func appendIfMissing(values []string, v string) []string {
	for _, existing := range values {
		if existing == v {
			return values
		}
	}
	return append(values, v)
}

func formatSearchErrors(errs []channelErr) string {
	if len(errs) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, fmt.Sprintf("%s=%s", e.name, boundedSnippet(e.err.Error())))
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func collectAnswerErrors(answers []paperAnswer) string {
	parts := make([]string, 0, len(answers))
	for _, a := range answers {
		if a.err == nil {
			continue
		}
		label := a.paperID
		if a.title != "" {
			label = a.title + " (" + a.paperID + ")"
		}
		parts = append(parts, fmt.Sprintf("%s: %s", label, boundedSnippet(a.err.Error())))
	}
	if len(parts) == 0 {
		return "none"
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func channelErrors(errs []channelErr, name string) []string {
	var out []string
	for _, e := range errs {
		if e.name == name && e.err != nil {
			out = append(out, boundedSnippet(e.err.Error()))
		}
	}
	return out
}

func channelHit(name, ftsQuery string, hitCount, candidateCount int, errs []string) SearchChannelHit {
	return SearchChannelHit{
		Channel:        name,
		FTSQuery:       ftsQuery,
		HitCount:       hitCount,
		CandidateCount: candidateCount,
		Errors:         errs,
	}
}

func splitErrorSummary(summary string) []string {
	if strings.TrimSpace(summary) == "" || summary == "none" {
		return nil
	}
	parts := strings.Split(summary, "; ")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func flattenPaperDiags(order []string, byID map[string]*CrossPaperDiagnostics) []CrossPaperDiagnostics {
	out := make([]CrossPaperDiagnostics, 0, len(order))
	for _, pid := range order {
		if d := byID[pid]; d != nil {
			out = append(out, *d)
		}
	}
	return out
}

func totalLLMCallCount(answers []paperAnswer) int {
	total := 0
	for _, a := range answers {
		total += a.diag.LLMCallCount
	}
	return total
}

func valueOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
