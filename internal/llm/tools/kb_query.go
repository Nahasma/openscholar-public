package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/memory"
)

// LLMCaller is a function type for making simple LLM calls.
// Used to avoid importing the provider package (which would cause an import cycle).
type LLMCaller func(ctx context.Context, prompt string) (string, error)

type kbQueryTool struct {
	kbService     kb.Service
	callLLM       LLMCaller
	memoryService memory.Service
}

type kbQueryParams struct {
	PaperID  string `json:"paper_id"`
	Question string `json:"question"`
}

// NewKBQueryTool creates a tool for querying papers in the knowledge base.
func NewKBQueryTool(kbService kb.Service, callLLM LLMCaller, memoryService memory.Service) BaseTool {
	return &kbQueryTool{kbService: kbService, callLLM: callLLM, memoryService: memoryService}
}

func (t *kbQueryTool) Info() ToolInfo {
	return ToolInfo{
		Name:           "KBQuery",
		MaxResultBytes: 16 * 1024, // 16 KB
		Description:    "Ask a question about a specific paper in the knowledge base. Routes metadata/content queries and returns answers with source references.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"paper_id": map[string]any{
					"type":        "string",
					"description": "Paper identifier (as shown in /kb-list)",
				},
				"question": map[string]any{
					"type":        "string",
					"description": "The question to ask about the paper",
				},
			},
			"required": []string{"paper_id", "question"},
		},
		Required: []string{"paper_id", "question"},
	}
}

func (t *kbQueryTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params kbQueryParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	type paperQuerier interface {
		QueryPaper(ctx context.Context, callLLM kb.LLMCaller, paperID string, question string, opts kb.QueryOptions) (*kb.SearchResult, error)
	}

	searcher, ok := t.kbService.(paperQuerier)
	if !ok {
		return NewTextErrorResponse("KB service does not support query routing"), nil
	}

	result, err := searcher.QueryPaper(ctx, kb.LLMCaller(t.callLLM), params.PaperID, params.Question, kb.QueryOptions{AllowTaskCreate: true})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Query failed: %v", err)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Answer** (paper: %s)\n\n", params.PaperID))
	if result.Diagnostics.Route != "" {
		sb.WriteString(fmt.Sprintf("**Route:** `%s`\n\n", result.Diagnostics.Route))
	}
	if result.Diagnostics.Warning != "" {
		sb.WriteString(fmt.Sprintf("**Warning:** %s\n\n", result.Diagnostics.Warning))
	} else if !result.Diagnostics.ContentAvailable {
		sb.WriteString("**Warning:** Full node content is unavailable; this answer is based on tree summaries only.\n\n")
	} else if result.Diagnostics.IndexLevel == kb.IndexLevelSummaryOnly {
		sb.WriteString("**Warning:** This paper has a summary-only KB index; the answer is based on summaries, not full-document text.\n\n")
	} else if result.Diagnostics.IndexLevel == kb.IndexLevelSimpleFullText {
		sb.WriteString("**Index note:** PageIndex tree is unavailable; the answer uses simple page-level text when available.\n\n")
	}
	sb.WriteString(result.Answer)
	sb.WriteString("\n\n**Sources:**\n")
	for _, src := range result.Sources {
		sb.WriteString(fmt.Sprintf("- [%s] %s (pp. %d-%d)\n", src.NodeID, src.Title, src.StartPage, src.EndPage))
	}

	// Trigger memory extraction for KB findings
	if t.memoryService != nil && shouldCaptureKBQueryMemory(result) {
		sessionID, _ := GetContextValues(ctx)
		if sessionID != "" {
			sources := make([]map[string]any, 0, len(result.Sources))
			for _, src := range result.Sources {
				sources = append(sources, map[string]any{
					"node_id":     src.NodeID,
					"title":       src.Title,
					"start_page":  src.StartPage,
					"end_page":    src.EndPage,
					"route":       result.Diagnostics.Route,
					"fts_status":  result.Diagnostics.FTSStatus,
					"tree_status": result.Diagnostics.SemanticTreeStatus,
				})
			}
			_ = t.memoryService.CaptureKB(ctx, memory.KBCapture{
				SessionID:  sessionID,
				SourceType: "kb_query",
				Kind:       "kb.answer",
				Question:   params.Question,
				PaperID:    params.PaperID,
				Answer:     captureAnswerWithDiagnostics(result),
				Sources:    sources,
			})
		}
	}

	publicSummary := result.Answer
	if strings.TrimSpace(publicSummary) == "" {
		publicSummary = fmt.Sprintf("KBQuery returned no direct answer for paper %s.", params.PaperID)
	}
	sources := make([]map[string]string, 0, len(result.Sources))
	for _, src := range result.Sources {
		sources = append(sources, map[string]string{
			"title":   src.Title,
			"url":     fmt.Sprintf("kb://%s#%s", params.PaperID, src.NodeID),
			"snippet": fmt.Sprintf("pp. %d-%d", src.StartPage, src.EndPage),
		})
	}
	return WithResponseMetadata(NewTextResponse(sb.String()), map[string]any{
		"paper_id":         params.PaperID,
		"diagnostics":      result.Diagnostics,
		"tool":             "KBQuery",
		"provider":         "kb",
		"source":           "kb",
		"progress_kind":    "kb_answer",
		"durable_progress": true,
		"public_summary":   publicSummary,
		"sources":          sources,
	}), nil
}

func captureAnswerWithDiagnostics(result *kb.SearchResult) string {
	if result == nil {
		return ""
	}
	prefix := ""
	switch {
	case result.Diagnostics.Warning != "":
		prefix = "KB limitation: " + result.Diagnostics.Warning
	case !result.Diagnostics.ContentAvailable:
		prefix = "KB limitation: content_available=false; answer may be based on summaries or limited context."
	case result.Diagnostics.IndexLevel == kb.IndexLevelSummaryOnly:
		prefix = "KB limitation: index_level=summary_only; answer is not full-document reading."
	}
	if prefix == "" {
		return result.Answer
	}
	return prefix + "\n\n" + result.Answer
}

func shouldCaptureKBQueryMemory(result *kb.SearchResult) bool {
	if result == nil || strings.TrimSpace(result.Answer) == "" {
		return false
	}
	if result.Diagnostics.DeepReadRecommended {
		return false
	}
	if len(result.Sources) == 0 {
		return false
	}
	// Skip pure diagnostic/status responses.
	if result.Diagnostics.Route == "metadata" && result.Diagnostics.LLMCallCount == 0 {
		return false
	}
	return true
}
