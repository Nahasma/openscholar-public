package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/memory"
)

type kbSearchTool struct {
	kbService     kb.Service
	callLLM       LLMCaller
	memoryService memory.Service
}

type kbSearchParams struct {
	Question string `json:"question"`
}

// NewKBSearchTool creates a tool for searching across all papers in the knowledge base.
func NewKBSearchTool(kbService kb.Service, callLLM LLMCaller, memoryService memory.Service) BaseTool {
	return &kbSearchTool{kbService: kbService, callLLM: callLLM, memoryService: memoryService}
}

func (t *kbSearchTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "KBSearch",
		Description: "Search across the user's local KB papers using papers/node-summaries/raw-chunks full-text retrieval with RRF fusion, then query top papers. This can be slow because it may run per-paper LLM synthesis; use it when the user asks about local/added papers or KB-backed evidence. For broad textbook introductions or well-known concepts, answer from model prior first and use ScholarSearch/WebSearch only for missing citations or recent facts.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "The research question to search across all KB papers",
				},
			},
			"required": []string{"question"},
		},
		Required: []string{"question"},
	}
}

func (t *kbSearchTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params kbSearchParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	if params.Question == "" {
		return NewTextErrorResponse("question is required"), nil
	}

	// CrossPaperSearch uses kb.LLMCaller type — convert via type assertion
	type crossSearcher interface {
		CrossPaperSearch(ctx context.Context, callLLM kb.LLMCaller, question string, limit int) (*kb.CrossSearchResult, error)
	}
	searcher, ok := t.kbService.(crossSearcher)
	if !ok {
		return NewTextErrorResponse("KB service does not support cross-paper search"), nil
	}

	result, err := searcher.CrossPaperSearch(ctx, kb.LLMCaller(t.callLLM), params.Question, 5)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Cross-paper search failed: %v", err)), nil
	}

	// Format response
	var sb strings.Builder
	sb.WriteString("**Cross-Paper Answer:**\n\n")
	sb.WriteString(result.Answer)

	if len(result.Sources) > 0 {
		sb.WriteString("\n\n**Sources:**\n")
		seen := make(map[string]bool)
		for _, src := range result.Sources {
			key := src.PaperID
			if seen[key] {
				continue
			}
			seen[key] = true
			if src.StartPage > 0 {
				fmt.Fprintf(&sb, "- %s (p%d-%d) [%s]\n", src.PaperTitle, src.StartPage, src.EndPage, src.PaperID)
			} else {
				fmt.Fprintf(&sb, "- %s [%s]\n", src.PaperTitle, src.PaperID)
			}
		}
	}

	// Async memory extraction
	if t.memoryService != nil && len(result.Sources) > 0 {
		sessionID, _ := GetContextValues(ctx)
		if sessionID != "" && result.Answer != "" {
			sources := make([]map[string]any, 0, len(result.Sources))
			for _, src := range result.Sources {
				sources = append(sources, map[string]any{
					"paper_id":    src.PaperID,
					"paper_title": src.PaperTitle,
					"node_id":     src.NodeID,
					"node_title":  src.NodeTitle,
					"start_page":  src.StartPage,
					"end_page":    src.EndPage,
				})
			}
			_ = t.memoryService.CaptureKB(ctx, memory.KBCapture{
				SessionID:  sessionID,
				SourceType: "kb_search",
				Kind:       "kb.cross_answer",
				Question:   params.Question,
				Answer:     result.Answer,
				Sources:    sources,
			})
		}
	}

	uniqueSources := make([]map[string]string, 0, len(result.Sources))
	seenSrc := make(map[string]struct{}, len(result.Sources))
	for _, src := range result.Sources {
		key := src.PaperID + ":" + src.NodeID
		if _, ok := seenSrc[key]; ok {
			continue
		}
		seenSrc[key] = struct{}{}
		snippet := ""
		if src.StartPage > 0 {
			snippet = fmt.Sprintf("pp. %d-%d", src.StartPage, src.EndPage)
		}
		uniqueSources = append(uniqueSources, map[string]string{
			"title":   src.PaperTitle,
			"url":     fmt.Sprintf("kb://%s#%s", src.PaperID, src.NodeID),
			"snippet": snippet,
		})
	}

	return WithResponseMetadata(NewTextResponse(sb.String()), map[string]any{
		"tool":             "KBSearch",
		"provider":         "kb",
		"source":           "kb",
		"progress_kind":    "kb_answer",
		"durable_progress": true,
		"public_summary":   result.Answer,
		"sources":          uniqueSources,
		"diagnostics":      result.Diagnostics,
	}), nil
}
