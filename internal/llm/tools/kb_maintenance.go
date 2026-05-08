package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/kb"
)

type kbHealthTool struct{ kbService kb.Service }
type kbRepairTool struct{ kbService kb.Service }
type kbReindexTool struct{ kbService kb.Service }

func NewKBHealthTool(kbService kb.Service) BaseTool  { return &kbHealthTool{kbService: kbService} }
func NewKBRepairTool(kbService kb.Service) BaseTool  { return &kbRepairTool{kbService: kbService} }
func NewKBReindexTool(kbService kb.Service) BaseTool { return &kbReindexTool{kbService: kbService} }

func (t *kbHealthTool) Info() ToolInfo {
	return ToolInfo{Name: "KBHealth", Description: "Run KB health checks for chunks/FTS/semantic-tree/tasks."}
}
func (t *kbRepairTool) Info() ToolInfo {
	return ToolInfo{Name: "KBRepair", Description: "Repair safe KB issues. Dry-run by default; set apply=true to write."}
}
func (t *kbReindexTool) Info() ToolInfo {
	return ToolInfo{Name: "KBReindex", Description: "Rebuild KB search indexes and enqueue semantic-tree reindex intents. Dry-run by default."}
}

func (t *kbHealthTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	reader, ok := t.kbService.(interface {
		KBHealth(ctx context.Context, opts kb.KBHealthOptions) (*kb.KBHealthReport, error)
	})
	if !ok {
		return NewTextErrorResponse("KB service does not support health checks"), nil
	}
	var params struct {
		LeaseStaleSeconds int64 `json:"lease_stale_seconds,omitempty"`
	}
	_ = json.Unmarshal([]byte(call.Input), &params)
	report, err := reader.KBHealth(ctx, kb.KBHealthOptions{LeaseStaleSeconds: params.LeaseStaleSeconds})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("KBHealth failed: %v", err)), nil
	}
	var sb strings.Builder
	sb.WriteString("KB health report:\n")
	for _, c := range report.Checks {
		sb.WriteString(fmt.Sprintf("- %s: %s (%d)\n", c.Name, c.Status, c.Count))
	}
	return WithResponseMetadata(NewTextResponse(strings.TrimSpace(sb.String())), map[string]any{"health": report}), nil
}

func (t *kbRepairTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	runner, ok := t.kbService.(interface {
		KBRepair(ctx context.Context, opts kb.KBRepairOptions) (*kb.KBMaintenanceReport, error)
	})
	if !ok {
		return NewTextErrorResponse("KB service does not support repair"), nil
	}
	var params struct {
		PaperID string `json:"paper_id,omitempty"`
		Apply   bool   `json:"apply,omitempty"`
	}
	_ = json.Unmarshal([]byte(call.Input), &params)
	report, err := runner.KBRepair(ctx, kb.KBRepairOptions{PaperID: strings.TrimSpace(params.PaperID), Apply: params.Apply})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("KBRepair failed: %v", err)), nil
	}
	return WithResponseMetadata(NewTextResponse(formatMaintenanceReport(report)), map[string]any{"maintenance": report}), nil
}

func (t *kbReindexTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	runner, ok := t.kbService.(interface {
		KBReindex(ctx context.Context, opts kb.KBReindexOptions) (*kb.KBMaintenanceReport, error)
	})
	if !ok {
		return NewTextErrorResponse("KB service does not support reindex"), nil
	}
	var params struct {
		PaperID string `json:"paper_id,omitempty"`
		Apply   bool   `json:"apply,omitempty"`
	}
	_ = json.Unmarshal([]byte(call.Input), &params)
	report, err := runner.KBReindex(ctx, kb.KBReindexOptions{PaperID: strings.TrimSpace(params.PaperID), Apply: params.Apply})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("KBReindex failed: %v", err)), nil
	}
	return WithResponseMetadata(NewTextResponse(formatMaintenanceReport(report)), map[string]any{"maintenance": report}), nil
}

func formatMaintenanceReport(report *kb.KBMaintenanceReport) string {
	if report == nil {
		return "No maintenance report."
	}
	return fmt.Sprintf("KB %s (%s): changed=%d skipped=%d errors=%d", report.Operation, ternary(report.DryRun, "dry-run", "applied"), len(report.Changed), len(report.Skipped), len(report.Errors))
}

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
