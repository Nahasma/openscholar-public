package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/kb"
)

type fakeKBMaintenanceService struct {
	fakeKBService
	healthReport  *kb.KBHealthReport
	repairReport  *kb.KBMaintenanceReport
	reindexReport *kb.KBMaintenanceReport
}

func (f *fakeKBMaintenanceService) KBHealth(ctx context.Context, opts kb.KBHealthOptions) (*kb.KBHealthReport, error) {
	return f.healthReport, nil
}
func (f *fakeKBMaintenanceService) KBRepair(ctx context.Context, opts kb.KBRepairOptions) (*kb.KBMaintenanceReport, error) {
	return f.repairReport, nil
}
func (f *fakeKBMaintenanceService) KBReindex(ctx context.Context, opts kb.KBReindexOptions) (*kb.KBMaintenanceReport, error) {
	return f.reindexReport, nil
}

func TestKBMaintenanceTools_Run(t *testing.T) {
	svc := &fakeKBMaintenanceService{
		healthReport:  &kb.KBHealthReport{Checks: []kb.KBHealthCheck{{Name: "papers_without_chunks", Status: "ok", Count: 0}}},
		repairReport:  &kb.KBMaintenanceReport{Operation: "repair", DryRun: true, Skipped: []string{"dry-run"}},
		reindexReport: &kb.KBMaintenanceReport{Operation: "reindex", DryRun: false, Changed: []string{"rebuild"}},
	}

	resp, err := NewKBHealthTool(svc).Run(context.Background(), ToolCall{Input: `{}`})
	if err != nil || !strings.Contains(resp.Content, "KB health report") {
		t.Fatalf("KBHealth response err=%v content=%q", err, resp.Content)
	}
	resp, err = NewKBRepairTool(svc).Run(context.Background(), ToolCall{Input: `{}`})
	if err != nil || !strings.Contains(resp.Content, "KB repair") {
		t.Fatalf("KBRepair response err=%v content=%q", err, resp.Content)
	}
	resp, err = NewKBReindexTool(svc).Run(context.Background(), ToolCall{Input: `{"apply":true}`})
	if err != nil || !strings.Contains(resp.Content, "KB reindex") {
		t.Fatalf("KBReindex response err=%v content=%q", err, resp.Content)
	}
}
