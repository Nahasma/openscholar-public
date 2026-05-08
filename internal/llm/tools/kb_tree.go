package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/kb"
)

type kbTreeTool struct {
	kbService kb.Service
}

type kbTreeParams struct {
	PaperID string `json:"paper_id"`
	View    string `json:"view,omitempty"`
}

// NewKBTreeTool creates a tool for displaying the tree structure of a paper.
func NewKBTreeTool(kbService kb.Service) BaseTool {
	return &kbTreeTool{kbService: kbService}
}

func (t *kbTreeTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "KBTree",
		Description: "Display semantic tree, flat page index, or index status for a KB paper.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"paper_id": map[string]any{
					"type":        "string",
					"description": "Paper identifier",
				},
				"view": map[string]any{
					"type":        "string",
					"description": "One of: auto | semantic | pages | status (default: auto)",
				},
			},
			"required": []string{"paper_id"},
		},
		Required: []string{"paper_id"},
	}
}

func (t *kbTreeTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params kbTreeParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}
	view := strings.ToLower(strings.TrimSpace(params.View))
	if view == "" {
		view = "auto"
	}
	if view != "auto" && view != "semantic" && view != "pages" && view != "status" {
		return NewTextErrorResponse("view must be one of: auto, semantic, pages, status"), nil
	}

	statusReader, ok := t.kbService.(interface {
		GetPaperIndexStateView(ctx context.Context, paperID string) (kb.PaperIndexState, error)
	})
	if !ok {
		return NewTextErrorResponse("KB service does not support index state view"), nil
	}
	state, err := statusReader.GetPaperIndexStateView(ctx, params.PaperID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to get paper status: %v", err)), nil
	}

	metadata := map[string]any{
		"paper_id":                  params.PaperID,
		"index_level":               state.IndexLevel,
		"content_available":         state.ContentAvailable,
		"fts_status":                state.FTSStatus,
		"semantic_tree_status":      state.SemanticTreeStatus,
		"semantic_tree_available":   state.SemanticTreeAvailable,
		"flat_page_index_available": state.FlatPageIndexAvailable,
		"semantic_tree_task_id":     state.SemanticTreeTaskID,
		"fallback_reason":           state.FallbackReason,
	}

	renderStatus := func() string {
		var sb strings.Builder
		fmt.Fprintf(&sb, "Paper ID: %s\n", params.PaperID)
		fmt.Fprintf(&sb, "index_level: %s\n", emptyAs(state.IndexLevel, "unknown"))
		fmt.Fprintf(&sb, "content_available: %t\n", state.ContentAvailable)
		fmt.Fprintf(&sb, "fts_status: %s\n", emptyAs(state.FTSStatus, "unknown"))
		fmt.Fprintf(&sb, "semantic_tree_status: %s\n", emptyAs(state.SemanticTreeStatus, "unknown"))
		fmt.Fprintf(&sb, "semantic_tree_available: %t\n", state.SemanticTreeAvailable)
		fmt.Fprintf(&sb, "flat_page_index_available: %t\n", state.FlatPageIndexAvailable)
		if state.SemanticTreeTaskID != "" {
			fmt.Fprintf(&sb, "semantic_tree_task_id: %s\n", state.SemanticTreeTaskID)
		}
		if state.FallbackReason != "" {
			fmt.Fprintf(&sb, "fallback_reason: %s\n", state.FallbackReason)
		}
		return strings.TrimSpace(sb.String())
	}

	renderPages := func() (string, error) {
		chunkReader, ok := t.kbService.(interface {
			GetPaperChunksView(ctx context.Context, paperID string, limit int) ([]kb.PaperChunk, error)
		})
		if !ok {
			return "", fmt.Errorf("KB service does not support chunk view")
		}
		chunks, err := chunkReader.GetPaperChunksView(ctx, params.PaperID, 0)
		if err != nil {
			return "", err
		}
		if len(chunks) == 0 {
			return "Semantic tree unavailable; showing flat page index from parsed text.\n\nNo chunks found.", nil
		}
		var sb strings.Builder
		sb.WriteString("Semantic tree unavailable; showing flat page index from parsed text.\n\n")
		for _, ch := range chunks {
			label := ch.ChunkID
			if ch.PageStart > 0 {
				label = fmt.Sprintf("p.%d", ch.PageStart)
				if ch.PageEnd > ch.PageStart {
					label = fmt.Sprintf("pp.%d-%d", ch.PageStart, ch.PageEnd)
				}
			}
			title := strings.TrimSpace(ch.Title)
			if title == "" {
				title = ch.Kind
			}
			fmt.Fprintf(&sb, "- %s `%s` %s\n", label, ch.ChunkID, title)
		}
		return strings.TrimSpace(sb.String()), nil
	}

	if view == "status" {
		return WithResponseMetadata(NewTextResponse(renderStatus()), metadata), nil
	}
	if view == "semantic" || (view == "auto" && state.SemanticTreeAvailable) {
		tree, treeErr := t.kbService.GetTree(ctx, params.PaperID)
		if treeErr != nil {
			if view == "semantic" {
				return WithResponseMetadata(NewTextResponse("Semantic tree unavailable.\n\n"+renderStatus()), metadata), nil
			}
		} else {
			return WithResponseMetadata(NewTextResponse(kb.FormatTreeAsText(tree)), metadata), nil
		}
	}
	if view == "pages" || (view == "auto" && state.ContentAvailable) {
		out, pageErr := renderPages()
		if pageErr != nil {
			return NewTextErrorResponse(fmt.Sprintf("Failed to load page chunks: %v", pageErr)), nil
		}
		return WithResponseMetadata(NewTextResponse(out), metadata), nil
	}
	return WithResponseMetadata(NewTextResponse(renderStatus()), metadata), nil
}

func emptyAs(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
