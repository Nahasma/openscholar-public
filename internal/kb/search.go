package kb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/db"
)

// LLMCaller is a function type that makes a simple LLM call.
// This avoids importing the provider package (which would cause a cycle).
type LLMCaller func(ctx context.Context, prompt string) (string, error)

// treeSearchResponse is the LLM output from the tree search step.
type treeSearchResponse struct {
	Thinking string   `json:"thinking"`
	NodeList []string `json:"node_list"`
}

// TreeSearch performs LLM reasoning on a paper's tree to answer a question.
func (s *service) TreeSearch(ctx context.Context, callLLM LLMCaller, paperID string, question string) (*SearchResult, error) {
	tree, err := s.GetTree(ctx, paperID)
	if err != nil {
		return nil, fmt.Errorf("failed to load tree for paper %s: %w", paperID, err)
	}

	paper, err := s.GetPaper(ctx, paperID)
	if err != nil {
		return nil, fmt.Errorf("failed to load paper %s: %w", paperID, err)
	}

	diagnostics := SearchDiagnostics{
		IndexLevel:    IndexLevelUnknown,
		RetrievalMode: RetrievalModeTreeSummary,
	}
	if meta, err := s.GetIndexMetadata(ctx, paperID); err == nil {
		diagnostics.IndexLevel = meta.IndexLevel
		diagnostics.ContentAvailable = meta.ContentAvailable
		if meta.SummaryOnly {
			diagnostics.RetrievalMode = RetrievalModeSummaryOnly
		}
		if !meta.ContentAvailable {
			diagnostics.Warning = "Full node content is unavailable; answer context is limited to tree summaries."
		}
	}

	// Format tree for LLM (compact: node_id + title + summary)
	treeText := FormatTreeForLLM(tree)

	// LLM reasoning: select relevant nodes
	searchPrompt := fmt.Sprintf(treeSearchPrompt, treeText, question)
	selectStart := time.Now()
	searchOutput, err := callLLM(ctx, searchPrompt)
	diagnostics.TreeSelectMS = time.Since(selectStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("tree search LLM call failed: %w", err)
	}

	searchResp, snippet, parseErr := parseTreeSelectionResponse(searchOutput)
	if parseErr != nil {
		diagnostics.SelectionParseError = parseErr.Error()
		diagnostics.SelectionSnippet = snippet

		repairPrompt := fmt.Sprintf(treeSearchRepairPrompt, question, snippet)
		repairOutput, repairCallErr := callLLM(ctx, repairPrompt)
		if repairCallErr == nil {
			diagnostics.ParseRetryCount = 1
			repaired, repairSnippet, repairParseErr := parseTreeSelectionResponse(repairOutput)
			if repairParseErr == nil {
				searchResp = repaired
				diagnostics.SelectionParseError = ""
				diagnostics.SelectionSnippet = repairSnippet
			} else {
				diagnostics.SelectionParseError = repairParseErr.Error()
				diagnostics.SelectionSnippet = repairSnippet
			}
		}
	}

	selectedIDs, invalidIDs := validateSelectedNodeIDs(tree, searchResp.NodeList)
	diagnostics.InvalidNodeIDs = invalidIDs
	if diagnostics.SelectionParseError != "" || len(selectedIDs) == 0 && len(searchResp.NodeList) > 0 {
		fallbackIDs := lexicalFallbackNodeIDs(tree, question, 5)
		if len(fallbackIDs) > 0 {
			selectedIDs = fallbackIDs
			diagnostics.FallbackUsed = true
			diagnostics.RetrievalMode = RetrievalModeLexicalFallback
			if diagnostics.SelectionParseError != "" {
				diagnostics.Warning = "Node selection JSON was invalid; used deterministic lexical fallback."
			} else {
				diagnostics.Warning = "Selected node IDs were invalid; used deterministic lexical fallback."
			}
		}
	}
	summaryByID := s.loadNodeSummaryMap(ctx, paperID)
	contentNodes, contentErr := s.searchContentContext(ctx, paperID, question, summaryByID)
	if contentErr != nil {
		diagnostics.ContentSearchError = boundedSnippet(contentErr.Error())
	}
	useFTSContent := diagnostics.IndexLevel == IndexLevelSimpleFullText && len(contentNodes) > 0
	if len(selectedIDs) == 0 && len(contentNodes) > 0 {
		selectedIDs = answerContextNodeIDs(contentNodes)
		diagnostics.FallbackUsed = true
		diagnostics.Warning = "Tree selection found no relevant node; used full-content search hits."
		useFTSContent = true
	}
	diagnostics.SelectedNodeIDs = selectedIDs

	if len(selectedIDs) == 0 {
		if diagnostics.SelectionParseError != "" && diagnostics.Warning == "" {
			diagnostics.Warning = "Node selection JSON was invalid and no deterministic fallback node matched the question."
		}
		return &SearchResult{
			Answer:      "No relevant sections found in the paper for this question.",
			Thinking:    searchResp.Thinking,
			Diagnostics: diagnostics,
		}, nil
	}

	// Find selected nodes and build source references
	selectedNodes := FindNodes(tree, selectedIDs)
	sources := make([]SourceRef, len(selectedNodes))
	for i, node := range selectedNodes {
		sources[i] = SourceRef{
			NodeID:    node.NodeID,
			Title:     node.Title,
			StartPage: node.StartIndex,
			EndPage:   node.EndIndex,
		}
	}

	// Build context from selected nodes
	if !useFTSContent {
		contentNodes = nil
	}
	contextText, contentSources, contentDiag := s.buildAnswerContext(ctx, paperID, question, selectedNodes, contentNodes, summaryByID)
	if len(contentSources) > 0 {
		sources = contentSources
	}
	if contentDiag.UsedFullContent {
		diagnostics.UsedFullContent = true
		diagnostics.RetrievalMode = RetrievalModeFullContent
		diagnostics.ContentChars = contentDiag.ContentChars
		diagnostics.ContentTruncated = contentDiag.ContentTruncated
	} else if diagnostics.RetrievalMode == RetrievalModeTreeSummary && diagnostics.IndexLevel == IndexLevelSummaryOnly {
		diagnostics.RetrievalMode = RetrievalModeSummaryOnly
	}

	// LLM generates answer with citations
	answerPrompt := fmt.Sprintf(answerGenerationPrompt, question, paper.Title, contextText)
	answerStart := time.Now()
	answer, err := callLLM(ctx, answerPrompt)
	diagnostics.AnswerMS = time.Since(answerStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("answer generation LLM call failed: %w", err)
	}

	return &SearchResult{
		Answer:      answer,
		Sources:     sources,
		Thinking:    searchResp.Thinking,
		Diagnostics: diagnostics,
	}, nil
}

type contextDiagnostics struct {
	UsedFullContent  bool
	ContentChars     int
	ContentTruncated bool
}

type answerContextNode struct {
	NodeID    string
	Title     string
	StartPage int
	EndPage   int
	Text      string
}

const (
	answerContextMaxChars     = 12000
	answerContextMaxNodeChars = 4000
	answerContextFTSLimit     = 5
)

func (s *service) buildAnswerContext(ctx context.Context, paperID, question string, selectedNodes []TreeNode, contentNodes []answerContextNode, summaryByID map[string]NodeSummary) (string, []SourceRef, contextDiagnostics) {
	if len(contentNodes) == 0 {
		contentNodes = s.selectedContentContext(ctx, paperID, selectedNodes, summaryByID)
	}
	if len(contentNodes) > 0 {
		text, sources, diag := buildContextFromAnswerNodes(contentNodes, queryTerms(question))
		return text, sources, diag
	}

	return buildContextFromNodes(selectedNodes), nil, contextDiagnostics{}
}

func (s *service) loadNodeSummaryMap(ctx context.Context, paperID string) map[string]NodeSummary {
	summaryByID := make(map[string]NodeSummary)
	if summaries, err := s.GetNodeSummaries(ctx, paperID); err == nil {
		for _, summary := range summaries {
			summaryByID[summary.NodeID] = summary
		}
	}
	return summaryByID
}

func (s *service) searchContentContext(ctx context.Context, paperID, question string, summaryByID map[string]NodeSummary) ([]answerContextNode, error) {
	ftsQuery := buildFTSQuery(question)
	if ftsQuery == "" {
		return nil, nil
	}
	rows, err := s.q.SearchNodeContents(ctx, db.SearchNodeContentsParams{
		PaperID: paperID,
		Query:   ftsQuery,
		Limit:   answerContextFTSLimit,
	})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	out := make([]answerContextNode, 0, len(rows))
	for _, row := range rows {
		title := row.Title.String
		startPage, endPage := 0, 0
		if summary, ok := summaryByID[row.NodeID]; ok {
			if title == "" {
				title = summary.Title
			}
			startPage = summary.StartPage
			endPage = summary.EndPage
		}
		out = append(out, answerContextNode{
			NodeID:    row.NodeID,
			Title:     title,
			StartPage: startPage,
			EndPage:   endPage,
			Text:      row.Content,
		})
	}
	return out, nil
}

func (s *service) selectedContentContext(ctx context.Context, paperID string, selectedNodes []TreeNode, summaryByID map[string]NodeSummary) []answerContextNode {
	rows, err := s.q.GetNodeContents(ctx, paperID)
	if err != nil || len(rows) == 0 {
		return nil
	}
	contentByID := make(map[string]db.NodeContent, len(rows))
	for _, row := range rows {
		contentByID[row.NodeID] = row
	}

	out := make([]answerContextNode, 0, len(selectedNodes))
	for _, node := range selectedNodes {
		content, ok := contentByID[node.NodeID]
		if !ok || strings.TrimSpace(content.Content) == "" {
			continue
		}
		title := node.Title
		startPage := node.StartIndex
		endPage := node.EndIndex
		if summary, ok := summaryByID[node.NodeID]; ok {
			if title == "" {
				title = summary.Title
			}
			startPage = summary.StartPage
			endPage = summary.EndPage
		}
		out = append(out, answerContextNode{
			NodeID:    node.NodeID,
			Title:     title,
			StartPage: startPage,
			EndPage:   endPage,
			Text:      content.Content,
		})
	}
	return out
}

func buildContextFromAnswerNodes(nodes []answerContextNode, terms []string) (string, []SourceRef, contextDiagnostics) {
	var sb strings.Builder
	sources := make([]SourceRef, 0, len(nodes))
	diag := contextDiagnostics{UsedFullContent: true}
	remaining := answerContextMaxChars

	for _, node := range nodes {
		text := strings.TrimSpace(node.Text)
		if text == "" || remaining <= 0 {
			continue
		}
		text, truncated := contentWindow(text, terms, answerContextMaxNodeChars)
		diag.ContentTruncated = diag.ContentTruncated || truncated
		if len(text) > remaining {
			text = text[:remaining]
			diag.ContentTruncated = true
		}
		fmt.Fprintf(&sb, "## %s [%s] (pp. %d-%d)\n%s\n\n",
			node.Title, node.NodeID, node.StartPage, node.EndPage, text)
		diag.ContentChars += len(text)
		remaining -= len(text)
		sources = append(sources, SourceRef{
			NodeID:    node.NodeID,
			Title:     node.Title,
			StartPage: node.StartPage,
			EndPage:   node.EndPage,
		})
	}

	return sb.String(), sources, diag
}

func buildContextFromNodes(nodes []TreeNode) string {
	var sb strings.Builder
	for _, node := range nodes {
		fmt.Fprintf(&sb, "## %s [%s] (pp. %d-%d)\n%s\n\n",
			node.Title, node.NodeID, node.StartIndex, node.EndIndex, node.Summary)
	}
	return sb.String()
}

func answerContextNodeIDs(nodes []answerContextNode) []string {
	ids := make([]string, 0, len(nodes))
	seen := make(map[string]struct{})
	for _, node := range nodes {
		if node.NodeID == "" {
			continue
		}
		if _, ok := seen[node.NodeID]; ok {
			continue
		}
		seen[node.NodeID] = struct{}{}
		ids = append(ids, node.NodeID)
	}
	return ids
}

func validateSelectedNodeIDs(tree *PaperTree, nodeIDs []string) ([]string, []string) {
	valid := make(map[string]struct{})
	walkTreeNodes(tree.Structure, func(node TreeNode) {
		valid[node.NodeID] = struct{}{}
	})

	seen := make(map[string]struct{})
	var selected []string
	var invalid []string
	for _, id := range nodeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := valid[id]; !ok {
			invalid = append(invalid, id)
			continue
		}
		selected = append(selected, id)
	}
	return selected, invalid
}

func lexicalFallbackNodeIDs(tree *PaperTree, question string, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	terms := queryTerms(question)
	if len(terms) == 0 {
		return nil
	}
	type scoredNode struct {
		id    string
		score int
	}
	var scored []scoredNode
	walkTreeNodes(tree.Structure, func(node TreeNode) {
		text := strings.ToLower(node.Title + " " + node.Summary)
		score := 0
		for _, term := range terms {
			score += strings.Count(text, term)
		}
		if score > 0 {
			scored = append(scored, scoredNode{id: node.NodeID, score: score})
		}
	})
	if len(scored) == 0 {
		return nil
	}
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	if len(scored) > limit {
		scored = scored[:limit]
	}
	ids := make([]string, len(scored))
	for i, item := range scored {
		ids[i] = item.id
	}
	return ids
}

func queryTerms(question string) []string {
	fields := strings.FieldsFunc(strings.ToLower(question), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	seen := make(map[string]struct{})
	var terms []string
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		terms = append(terms, field)
	}
	return terms
}

func buildFTSQuery(question string) string {
	terms := queryTerms(question)
	if len(terms) == 0 {
		return ""
	}
	if len(terms) > 12 {
		terms = terms[:12]
	}
	quoted := make([]string, len(terms))
	for i, term := range terms {
		quoted[i] = `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " OR ")
}

func contentWindow(text string, terms []string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return "", text != ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text, false
	}

	lower := strings.ToLower(text)
	matchByte := -1
	for _, term := range terms {
		if term == "" {
			continue
		}
		idx := strings.Index(lower, term)
		if idx >= 0 && (matchByte < 0 || idx < matchByte) {
			matchByte = idx
		}
	}
	if matchByte < 0 {
		return string(runes[:maxRunes]) + "\n[...]", true
	}

	center := byteOffsetToRuneIndex(text, matchByte)
	start := center - maxRunes/3
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
		start = end - maxRunes
		if start < 0 {
			start = 0
		}
	}

	window := string(runes[start:end])
	if start > 0 {
		window = "[...]\n" + window
	}
	if end < len(runes) {
		window += "\n[...]"
	}
	return window, true
}

func byteOffsetToRuneIndex(s string, offset int) int {
	if offset <= 0 {
		return 0
	}
	runeIndex := 0
	for byteIndex := range s {
		if byteIndex >= offset {
			return runeIndex
		}
		runeIndex++
	}
	return runeIndex
}

func walkTreeNodes(nodes []TreeNode, fn func(TreeNode)) {
	for _, node := range nodes {
		fn(node)
		walkTreeNodes(node.Nodes, fn)
	}
}
