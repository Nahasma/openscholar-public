package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/kb"
)

type kbListTool struct {
	kbService kb.Service
}

type kbListParams struct {
	Limit   int  `json:"limit,omitempty"`
	Offset  int  `json:"offset,omitempty"`
	Verbose bool `json:"verbose,omitempty"`
}

// NewKBListTool creates a tool for listing papers in the knowledge base.
func NewKBListTool(kbService kb.Service) BaseTool {
	return &kbListTool{kbService: kbService}
}

func (t *kbListTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "KBList",
		Description: "List KB papers with metadata and indexing status (content/fts/semantic tree/task).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of papers to return (default: 20)",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Offset for pagination (default: 0)",
				},
				"verbose": map[string]any{
					"type":        "boolean",
					"description": "Return detailed metadata fields (default: false)",
				},
			},
		},
	}
}

func (t *kbListTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params kbListParams
	_ = json.Unmarshal([]byte(call.Input), &params)

	if params.Limit <= 0 {
		params.Limit = 5
	}

	if bulkReader, ok := t.kbService.(interface {
		ListPapersWithStatusView(ctx context.Context, limit, offset int) ([]kb.PaperListItem, error)
	}); ok {
		items, err := bulkReader.ListPapersWithStatusView(ctx, params.Limit, params.Offset)
		if err == nil {
			count, _ := t.kbService.CountPapers(ctx)
			if params.Verbose {
				return NewTextResponse(renderKBListItemsDetailed(items, count)), nil
			}
			return NewTextResponse(renderKBListItemsCompact(items, count, params.Limit, params.Offset)), nil
		}
	}

	papers, err := t.kbService.ListPapers(ctx, params.Limit, params.Offset)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to list papers: %v", err)), nil
	}

	count, _ := t.kbService.CountPapers(ctx)

	if len(papers) == 0 {
		return NewTextResponse("Knowledge base is empty. Use KBAdd to add papers."), nil
	}
	if !params.Verbose {
		return NewTextResponse(renderKBPapersCompact(papers, count, params.Limit, params.Offset)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Knowledge Base (%d papers total):\n\n", count))
	stateReader, _ := t.kbService.(interface {
		GetPaperIndexStateView(ctx context.Context, paperID string) (kb.PaperIndexState, error)
	})
	taskReader, _ := t.kbService.(interface {
		ListPaperTasksView(ctx context.Context, paperID string, limit int) ([]kb.KBTask, error)
	})
	for _, p := range papers {
		authorsStr := "unknown"
		if len(p.Authors) > 0 {
			names := make([]string, 0, len(p.Authors))
			for _, a := range p.Authors {
				if strings.TrimSpace(a.Name) != "" {
					names = append(names, strings.TrimSpace(a.Name))
				}
			}
			if len(names) > 0 {
				authorsStr = strings.Join(names, ", ")
			}
		}
		yearStr := "unknown"
		if p.Year > 0 {
			yearStr = fmt.Sprintf("%d", p.Year)
		}
		venueStr := strings.TrimSpace(p.Venue)
		if venueStr == "" {
			venueStr = "unknown"
		}
		docType := strings.TrimSpace(p.DocType)
		if docType == "" {
			docType = "unknown"
		}
		content := "unknown"
		fts := "unknown"
		semantic := "unknown"
		activeTask := "none"
		if stateReader != nil {
			if st, err := stateReader.GetPaperIndexStateView(ctx, p.PaperID); err == nil {
				if st.ContentAvailable {
					content = "ready"
				} else {
					content = emptyAs(st.RawParseStatus, "missing")
				}
				fts = emptyAs(st.FTSStatus, "unknown")
				semantic = emptyAs(st.SemanticTreeStatus, "unknown")
			}
		}
		if taskReader != nil {
			if tasks, err := taskReader.ListPaperTasksView(ctx, p.PaperID, 5); err == nil {
				for _, task := range tasks {
					if task.Status == "running" || task.Status == "pending" {
						activeTask = task.TaskType + ":" + task.Status
						break
					}
				}
			}
		}
		indexed := "unknown"
		if p.IndexedAt > 0 {
			indexed = fmt.Sprintf("%d", p.IndexedAt)
		}
		sb.WriteString(fmt.Sprintf("- ID: `%s`\n  title: %s\n  authors: %s\n  year: %s\n  venue: %s\n  doc_type: %s\n  content: %s\n  fts: %s\n  semantic_tree: %s\n  active_task: %s\n  indexed_at: %s\n\n",
			p.PaperID, p.Title, authorsStr, yearStr, venueStr, docType, content, fts, semantic, activeTask, indexed))
	}

	return NewTextResponse(strings.TrimSpace(sb.String())), nil
}

func renderKBListItemsDetailed(items []kb.PaperListItem, count int64) string {
	if len(items) == 0 {
		return "Knowledge base is empty. Use KBAdd to add papers."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Knowledge Base (%d papers total):\n\n", count))
	for _, item := range items {
		p := item.Paper
		authorsStr := "unknown"
		if len(p.Authors) > 0 {
			names := make([]string, 0, len(p.Authors))
			for _, a := range p.Authors {
				if strings.TrimSpace(a.Name) != "" {
					names = append(names, strings.TrimSpace(a.Name))
				}
			}
			if len(names) > 0 {
				authorsStr = strings.Join(names, ", ")
			}
		}
		yearStr := "unknown"
		if p.Year > 0 {
			yearStr = fmt.Sprintf("%d", p.Year)
		}
		venueStr := strings.TrimSpace(p.Venue)
		if venueStr == "" {
			venueStr = "unknown"
		}
		docType := strings.TrimSpace(p.DocType)
		if docType == "" {
			docType = "unknown"
		}
		content := "unknown"
		if item.State.ContentAvailable {
			content = "ready"
		} else {
			content = emptyAs(item.State.RawParseStatus, "missing")
		}
		fts := emptyAs(item.State.FTSStatus, "unknown")
		semantic := emptyAs(item.State.SemanticTreeStatus, "unknown")
		activeTask := "none"
		if item.Task != nil && (item.Task.Status == kb.KBTaskStatusRunning || item.Task.Status == kb.KBTaskStatusQueued) {
			activeTask = item.Task.TaskType + ":" + item.Task.Status
		}
		indexed := "unknown"
		if p.IndexedAt > 0 {
			indexed = fmt.Sprintf("%d", p.IndexedAt)
		}
		sb.WriteString(fmt.Sprintf("- ID: `%s`\n  title: %s\n  authors: %s\n  year: %s\n  venue: %s\n  doc_type: %s\n  content: %s\n  fts: %s\n  semantic_tree: %s\n  active_task: %s\n  indexed_at: %s\n\n",
			p.PaperID, p.Title, authorsStr, yearStr, venueStr, docType, content, fts, semantic, activeTask, indexed))
	}
	return strings.TrimSpace(sb.String())
}

func renderKBListItemsCompact(items []kb.PaperListItem, count int64, limit, offset int) string {
	if len(items) == 0 {
		return "Knowledge base is empty. Use KBAdd to add papers."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Knowledge Base (%d papers total)\n", count))
	sb.WriteString(fmt.Sprintf("Showing %d item(s) (offset=%d, limit=%d):\n", len(items), offset, limit))
	for _, item := range items {
		p := item.Paper
		yearStr := "unknown"
		if p.Year > 0 {
			yearStr = fmt.Sprintf("%d", p.Year)
		}
		authorsStr := "unknown"
		if len(p.Authors) > 0 {
			names := make([]string, 0, len(p.Authors))
			for _, a := range p.Authors {
				if n := strings.TrimSpace(a.Name); n != "" {
					names = append(names, n)
				}
			}
			if len(names) > 0 {
				authorsStr = strings.Join(names, ", ")
			}
		}
		status := fmt.Sprintf("content=%s fts=%s tree=%s",
			compactContentStatus(item.State),
			emptyAs(item.State.FTSStatus, "unknown"),
			emptyAs(item.State.SemanticTreeStatus, "unknown"))
		sb.WriteString(fmt.Sprintf("- `%s` | %s | %s | %s | %s\n", p.PaperID, p.Title, yearStr, authorsStr, status))
	}
	sb.WriteString("\nHints: set `verbose=true` for detailed metadata; use `offset`/`limit` for pagination.")
	return strings.TrimSpace(sb.String())
}

func renderKBPapersCompact(papers []kb.Paper, count int64, limit, offset int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Knowledge Base (%d papers total)\n", count))
	sb.WriteString(fmt.Sprintf("Showing %d item(s) (offset=%d, limit=%d):\n", len(papers), offset, limit))
	for _, p := range papers {
		yearStr := "unknown"
		if p.Year > 0 {
			yearStr = fmt.Sprintf("%d", p.Year)
		}
		authorsStr := "unknown"
		if len(p.Authors) > 0 {
			names := make([]string, 0, len(p.Authors))
			for _, a := range p.Authors {
				if n := strings.TrimSpace(a.Name); n != "" {
					names = append(names, n)
				}
			}
			if len(names) > 0 {
				authorsStr = strings.Join(names, ", ")
			}
		}
		sb.WriteString(fmt.Sprintf("- `%s` | %s | %s | %s | status=unknown\n", p.PaperID, p.Title, yearStr, authorsStr))
	}
	sb.WriteString("\nHints: set `verbose=true` for detailed metadata; use `offset`/`limit` for pagination.")
	return strings.TrimSpace(sb.String())
}

func compactContentStatus(state kb.PaperIndexState) string {
	if state.ContentAvailable {
		return "ready"
	}
	return emptyAs(state.RawParseStatus, "missing")
}
