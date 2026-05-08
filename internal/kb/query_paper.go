package kb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/db"
)

const (
	queryPaperDefaultTopKChunks      = 8
	queryPaperDefaultMaxContextChars = 14000
	queryPaperMaxScanChunks          = 96
	queryPaperDeepReadContentBytes   = 300000
)

var metadataKeywords = []string{
	"title", "author", "authors", "year", "venue", "doi", "arxiv", "path", "file", "pages",
	"status", "indexed", "index", "tree", "content", "fts", "metadata",
}

var contentIntentKeywords = []string{
	"result", "method", "finding", "evidence", "claim", "contribution", "experiment", "analysis", "approach", "limitation", "explain",
}

var deepReadKeywords = []string{
	"entire paper", "whole paper", "full paper", "all sections", "in detail", "comprehensive", "thorough",
	"line by line", "everything", "full summary", "summarize the paper",
}

const localAnswerPrompt = `Answer the following question based ONLY on the provided paper chunks.
Include page references for factual claims as (p. X) or (pp. X-Y) when page numbers are available.

Question: %s

Paper: %s

Context:
%s

Requirements:
- Use only the provided context
- If context is insufficient, say so explicitly
- Keep the answer concise and factual`

func (s *service) QueryPaper(ctx context.Context, callLLM LLMCaller, paperID string, question string, opts QueryOptions) (*SearchResult, error) {
	paper, err := s.GetPaper(ctx, paperID)
	if err != nil {
		return nil, fmt.Errorf("failed to load paper %s: %w", paperID, err)
	}

	state, _ := s.q.GetPaperIndexState(ctx, paperID)
	diag := diagnosticsFromState(state)

	route := pickQueryRoute(question, opts, state)
	if route == "metadata" {
		return s.queryPaperMetadata(paper, state, route, question), nil
	}
	if route == "deep_read_task" {
		task, taskErr := s.EnsureDeepReadTask(ctx, paperID, question, opts.AllowTaskCreate)
		if taskErr == nil && task.Status == KBTaskStatusSucceeded && strings.TrimSpace(task.ResultJSON) != "" {
			var done DeepReadTaskResult
			if err := json.Unmarshal([]byte(task.ResultJSON), &done); err == nil && strings.TrimSpace(done.Answer) != "" {
				return &SearchResult{
					Answer:  done.Answer,
					Sources: done.Sources,
					Diagnostics: SearchDiagnostics{
						Route:               "deep_read_task",
						TaskID:              task.TaskID,
						TaskStatus:          task.Status,
						LLMCallCount:        0,
						DeepReadRecommended: false,
						ContentAvailable:    diag.ContentAvailable,
						FTSStatus:           diag.FTSStatus,
						SemanticTreeStatus:  diag.SemanticTreeStatus,
					},
				}, nil
			}
		}
		if taskErr == nil {
			msg := fmt.Sprintf("Deep-read task status: %s (task_id=%s).", task.Status, task.TaskID)
			if task.Status == "not_created" {
				msg = "This looks like a broad whole-document request. Deep-read task creation is disabled for this call."
			}
			return &SearchResult{
				Answer: msg,
				Diagnostics: SearchDiagnostics{
					Route:               "deep_read_task",
					TaskID:              task.TaskID,
					TaskStatus:          task.Status,
					LLMCallCount:        0,
					DeepReadRecommended: true,
					ContentAvailable:    diag.ContentAvailable,
					FTSStatus:           diag.FTSStatus,
					SemanticTreeStatus:  diag.SemanticTreeStatus,
				},
			}, nil
		}
		return &SearchResult{
			Answer: "This looks like a broad whole-document request. Use deep-read workflow to process the full paper safely.",
			Diagnostics: SearchDiagnostics{
				Route:               "deep_read_task",
				LLMCallCount:        0,
				DeepReadRecommended: true,
				ContentAvailable:    diag.ContentAvailable,
				FTSStatus:           diag.FTSStatus,
				SemanticTreeStatus:  diag.SemanticTreeStatus,
			},
		}, nil
	}

	if route == "semantic_tree" && strings.EqualFold(diag.SemanticTreeStatus, "ready") {
		result, semErr := s.TreeSearch(ctx, callLLM, paperID, question)
		if semErr == nil {
			result.Diagnostics.Route = "semantic_tree"
			if result.Diagnostics.LLMCallCount == 0 {
				result.Diagnostics.LLMCallCount = 2
			}
			return result, nil
		}
		diag.FallbackUsed = true
		diag.Warning = "Semantic tree query failed; fell back to local page chunks."
		diag.ContentSearchError = boundedSnippet(semErr.Error())
	}

	return s.queryPaperLocalPages(ctx, callLLM, paper, state, question, opts, diag)
}

func (s *service) queryPaperMetadata(paper *Paper, state db.PaperIndexState, route string, question string) *SearchResult {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Paper: %s\n", paper.Title))
	sb.WriteString(fmt.Sprintf("Paper ID: %s\n", paper.PaperID))
	if paper.Year > 0 {
		sb.WriteString(fmt.Sprintf("Year: %d\n", paper.Year))
	}
	if paper.Venue != "" {
		sb.WriteString(fmt.Sprintf("Venue: %s\n", paper.Venue))
	}
	if paper.DOI != "" {
		sb.WriteString(fmt.Sprintf("DOI: %s\n", paper.DOI))
	}
	if paper.ArxivID != "" {
		sb.WriteString(fmt.Sprintf("ArXiv: %s\n", paper.ArxivID))
	}
	if metadataQuestionAsksForPath(question) {
		if paper.FilePath != "" {
			sb.WriteString(fmt.Sprintf("File: %s\n", paper.FilePath))
		} else if paper.PDFPath != "" {
			sb.WriteString(fmt.Sprintf("File: %s\n", paper.PDFPath))
		}
	}
	if len(paper.Authors) > 0 {
		names := make([]string, 0, len(paper.Authors))
		for _, a := range paper.Authors {
			if strings.TrimSpace(a.Name) != "" {
				names = append(names, strings.TrimSpace(a.Name))
			}
		}
		if len(names) > 0 {
			sb.WriteString(fmt.Sprintf("Authors: %s\n", strings.Join(names, ", ")))
		}
	}
	if state.TotalPages.Valid {
		sb.WriteString(fmt.Sprintf("Pages: %d\n", state.TotalPages.Int64))
	}
	sb.WriteString(fmt.Sprintf("Index status: raw_parse=%s, fts=%s, semantic_tree=%s\n",
		nullStringValue(state.RawParseStatus),
		nullStringValue(state.FtsStatus),
		nullStringValue(state.SemanticTreeStatus),
	))
	diag := diagnosticsFromState(state)
	diag.Route = route
	diag.LLMCallCount = 0
	return &SearchResult{Answer: strings.TrimSpace(sb.String()), Diagnostics: diag}
}

func (s *service) queryPaperLocalPages(ctx context.Context, callLLM LLMCaller, paper *Paper, state db.PaperIndexState, question string, opts QueryOptions, diag SearchDiagnostics) (*SearchResult, error) {
	topK := opts.TopKChunks
	if topK <= 0 {
		topK = queryPaperDefaultTopKChunks
	}
	maxContextChars := opts.MaxContextChars
	if maxContextChars <= 0 {
		maxContextChars = queryPaperDefaultMaxContextChars
	}
	if maxContextChars > 16000 {
		maxContextChars = 16000
	}

	ftsQuery := SanitizeFTSQuery(question)
	diag.Route = "local_pages"
	diag.SanitizedQuery = ftsQuery

	var chunks []db.PaperChunk
	var ftsErr error
	if len(opts.CandidateHints) > 0 {
		chunks = s.loadHintedChunks(ctx, paper.PaperID, opts.CandidateHints, topK)
	}
	if ftsQuery != "" {
		ftsChunks, err := s.searchPaperChunksTitleContent(ctx, paper.PaperID, ftsQuery, int64(topK))
		ftsErr = err
		if ftsErr == nil {
			chunks = appendUniqueChunks(chunks, ftsChunks, topK)
		}
	}
	if ftsErr != nil {
		diag.ContentSearchError = boundedSnippet(ftsErr.Error())
		diag.FallbackUsed = true
	}
	if len(chunks) == 0 {
		allChunks, err := s.q.GetPaperChunks(ctx, paper.PaperID)
		if err != nil {
			return nil, fmt.Errorf("failed to load paper chunks: %w", err)
		}
		if len(allChunks) > queryPaperMaxScanChunks {
			allChunks = allChunks[:queryPaperMaxScanChunks]
		}
		chunks = fallbackChunkScan(allChunks, question, topK)
	}
	if len(chunks) == 0 {
		return &SearchResult{
			Answer: "No relevant local content found for this question.",
			Diagnostics: SearchDiagnostics{
				Route:            "local_pages",
				LLMCallCount:     0,
				SanitizedQuery:   ftsQuery,
				ContentAvailable: diag.ContentAvailable,
				FTSStatus:        diag.FTSStatus,
			},
		}, nil
	}

	contextText, sources, selected, hitCount := buildChunkContext(chunks, maxContextChars)
	if strings.TrimSpace(contextText) == "" || len(sources) == 0 {
		return &SearchResult{
			Answer: "No relevant local content found for this question.",
			Diagnostics: SearchDiagnostics{
				Route:            "local_pages",
				LLMCallCount:     0,
				SanitizedQuery:   ftsQuery,
				ChunkHitCount:    hitCount,
				SelectedChunkIDs: selected,
				ContentAvailable: diag.ContentAvailable,
				FTSStatus:        diag.FTSStatus,
			},
		}, nil
	}
	diag.ChunkHitCount = hitCount
	diag.SelectedChunkIDs = selected

	answer, err := callLLM(ctx, fmt.Sprintf(localAnswerPrompt, question, paper.Title, contextText))
	if err != nil {
		return nil, fmt.Errorf("local pages answer LLM call failed: %w", err)
	}
	diag.LLMCallCount = 1

	return &SearchResult{
		Answer:      answer,
		Sources:     sources,
		Diagnostics: diag,
	}, nil
}

func diagnosticsFromState(state db.PaperIndexState) SearchDiagnostics {
	return SearchDiagnostics{
		IndexLevel:             nullStringValue(state.IndexLevel),
		RawParseStatus:         nullStringValue(state.RawParseStatus),
		ContentAvailable:       state.ContentAvailable > 0,
		FTSStatus:              nullStringValue(state.FtsStatus),
		SemanticTreeStatus:     nullStringValue(state.SemanticTreeStatus),
		SemanticTreeAvailable:  state.SemanticTreeAvailable > 0,
		FlatPageIndexAvailable: state.FlatPageIndexAvailable > 0,
	}
}

func metadataQuestionAsksForPath(question string) bool {
	q := strings.ToLower(question)
	for _, k := range []string{"path", "file", "source"} {
		if strings.Contains(q, k) {
			return true
		}
	}
	return false
}

func pickQueryRoute(question string, opts QueryOptions, state db.PaperIndexState) string {
	preferred := strings.ToLower(strings.TrimSpace(opts.PreferredRoute))
	switch preferred {
	case "metadata", "local_pages", "semantic_tree", "deep_read_task":
		return preferred
	}
	if shouldUseDeepRead(question, state) {
		return "deep_read_task"
	}
	if isMetadataOnlyQuestion(question) {
		return "metadata"
	}
	if strings.EqualFold(nullStringValue(state.SemanticTreeStatus), "ready") && state.ContentAvailable == 0 {
		return "semantic_tree"
	}
	if strings.EqualFold(nullStringValue(state.SemanticTreeStatus), "ready") && strings.Contains(strings.ToLower(question), "section") {
		return "semantic_tree"
	}
	return "local_pages"
}

func isMetadataOnlyQuestion(question string) bool {
	q := strings.ToLower(question)
	hasMeta := false
	for _, k := range metadataKeywords {
		if strings.Contains(q, k) {
			hasMeta = true
			break
		}
	}
	if !hasMeta {
		return false
	}
	for _, k := range contentIntentKeywords {
		if strings.Contains(q, k) {
			return false
		}
	}
	return true
}

func shouldUseDeepRead(question string, state db.PaperIndexState) bool {
	largeDoc := state.ContentBytes >= queryPaperDeepReadContentBytes || (state.TotalPages.Valid && state.TotalPages.Int64 >= 80)
	if !largeDoc {
		return false
	}
	q := strings.ToLower(question)
	for _, k := range deepReadKeywords {
		if strings.Contains(q, k) {
			return true
		}
	}
	return false
}

func fallbackChunkScan(chunks []db.PaperChunk, question string, topK int) []db.PaperChunk {
	terms := QueryTerms(question)
	if len(terms) == 0 {
		if len(chunks) <= topK {
			return chunks
		}
		return chunks[:topK]
	}
	type scoredChunk struct {
		chunk db.PaperChunk
		score int
	}
	scored := make([]scoredChunk, 0, len(chunks))
	for _, c := range chunks {
		text := strings.ToLower(c.Title.String + " " + c.Content)
		score := 0
		for _, t := range terms {
			score += strings.Count(text, t)
		}
		if score > 0 {
			scored = append(scored, scoredChunk{chunk: c, score: score})
		}
	}
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	if len(scored) == 0 {
		if len(chunks) <= topK {
			return chunks
		}
		return chunks[:topK]
	}
	if len(scored) > topK {
		scored = scored[:topK]
	}
	out := make([]db.PaperChunk, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.chunk)
	}
	return out
}

func (s *service) loadHintedChunks(ctx context.Context, paperID string, hints []string, limit int) []db.PaperChunk {
	if limit <= 0 {
		limit = queryPaperDefaultTopKChunks
	}
	out := make([]db.PaperChunk, 0, minInt(len(hints), limit))
	seen := make(map[string]struct{}, len(hints))
	for _, hint := range hints {
		hint = strings.TrimSpace(hint)
		if hint == "" {
			continue
		}
		if _, ok := seen[hint]; ok {
			continue
		}
		seen[hint] = struct{}{}
		chunk, err := s.q.GetPaperChunk(ctx, db.GetPaperChunkParams{PaperID: paperID, ChunkID: hint})
		if err != nil || strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		out = append(out, chunk)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func appendUniqueChunks(dst []db.PaperChunk, src []db.PaperChunk, limit int) []db.PaperChunk {
	if limit <= 0 {
		limit = queryPaperDefaultTopKChunks
	}
	seen := make(map[string]struct{}, len(dst)+len(src))
	for _, ch := range dst {
		seen[ch.ChunkID] = struct{}{}
	}
	for _, ch := range src {
		if len(dst) >= limit {
			break
		}
		if _, ok := seen[ch.ChunkID]; ok {
			continue
		}
		seen[ch.ChunkID] = struct{}{}
		dst = append(dst, ch)
	}
	return dst
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func buildChunkContext(chunks []db.PaperChunk, maxChars int) (string, []SourceRef, []string, int) {
	var sb strings.Builder
	sources := make([]SourceRef, 0, len(chunks))
	selectedIDs := make([]string, 0, len(chunks))
	remaining := maxChars
	for _, chunk := range chunks {
		if remaining <= 0 {
			break
		}
		content := strings.TrimSpace(chunk.Content)
		if content == "" {
			continue
		}
		if len(content) > remaining {
			content = content[:remaining]
		}
		title := strings.TrimSpace(chunk.Title.String)
		if title == "" {
			title = chunk.ChunkID
		}
		sp := int(chunk.PageStart.Int64)
		ep := int(chunk.PageEnd.Int64)
		fmt.Fprintf(&sb, "## %s [%s] (pp. %d-%d)\n%s\n\n", title, chunk.ChunkID, sp, ep, content)
		sources = append(sources, SourceRef{NodeID: chunk.ChunkID, Title: title, StartPage: sp, EndPage: ep})
		selectedIDs = append(selectedIDs, chunk.ChunkID)
		remaining -= len(content)
	}
	return sb.String(), sources, selectedIDs, len(chunks)
}

func nullStringValue(ns sql.NullString) string {
	if !ns.Valid {
		return ""
	}
	return ns.String
}
