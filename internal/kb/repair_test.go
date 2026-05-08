package kb_test

import (
	"context"
	"testing"
	"time"

	"database/sql"
	"github.com/openscholar/openscholar/internal/db"
	"github.com/openscholar/openscholar/internal/kb"
	"github.com/openscholar/openscholar/internal/testutil"
)

func TestKBRepair_DryRunAndApply(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)
	now := time.Now().Unix()

	_ = q.InsertPaper(context.Background(), db.InsertPaperParams{PaperID: "p-r", Title: "Repair"})
	_ = q.InsertPaperChunk(context.Background(), db.InsertPaperChunkParams{PaperID: "p-r", ChunkID: "c1", Kind: "page", Content: "abc", CreatedAt: now})
	_ = q.InsertKBTask(context.Background(), db.InsertKBTaskParams{TaskID: "t-run", TaskType: kb.KBTaskTypeRepair, Status: "running", UpdatedAt: now - 100, CreatedAt: now - 200, LeaseUntil: sql.NullInt64{Int64: now - 1, Valid: true}})

	runner := svc.(interface {
		KBRepair(ctx context.Context, opts kb.KBRepairOptions) (*kb.KBMaintenanceReport, error)
		KBReindex(ctx context.Context, opts kb.KBReindexOptions) (*kb.KBMaintenanceReport, error)
		ClaimPendingKBTaskByType(ctx context.Context, taskType, leaseOwner string, leaseDuration time.Duration) (*kb.KBTask, error)
	})
	r1, err := runner.KBRepair(context.Background(), kb.KBRepairOptions{})
	if err != nil || !r1.DryRun {
		t.Fatalf("KBRepair dry-run err=%v report=%+v", err, r1)
	}
	r2, err := runner.KBRepair(context.Background(), kb.KBRepairOptions{Apply: true})
	if err != nil || r2.DryRun {
		t.Fatalf("KBRepair apply err=%v report=%+v", err, r2)
	}
	_ = q.InsertKBTask(context.Background(), db.InsertKBTaskParams{
		TaskID:     "semantic-tree:p-r",
		PaperID:    sql.NullString{String: "p-r", Valid: true},
		TaskType:   kb.KBTaskTypeSemanticTree,
		Status:     kb.KBTaskStatusSucceeded,
		ResultJson: sql.NullString{String: `{"status":"succeeded"}`, Valid: true},
		ResultRef:  sql.NullString{String: "kb_task://semantic-tree:p-r", Valid: true},
		CreatedAt:  now,
		UpdatedAt:  now,
		FinishedAt: sql.NullInt64{Int64: now, Valid: true},
	})
	r3, err := runner.KBReindex(context.Background(), kb.KBReindexOptions{PaperID: "p-r", Apply: true})
	if err != nil || r3.DryRun {
		t.Fatalf("KBReindex apply err=%v report=%+v", err, r3)
	}
	task, err := q.GetKBTask(context.Background(), "semantic-tree:p-r")
	if err != nil {
		t.Fatalf("GetKBTask: %v", err)
	}
	if task.Status != kb.KBTaskStatusQueued {
		t.Fatalf("reindex task status=%s, want queued", task.Status)
	}
	state, err := q.GetPaperIndexState(context.Background(), "p-r")
	if err != nil {
		t.Fatalf("GetPaperIndexState: %v", err)
	}
	if state.SemanticTreeStatus.String != kb.KBTaskStatusQueued {
		t.Fatalf("semantic state=%s, want queued", state.SemanticTreeStatus.String)
	}
	claimed, err := runner.ClaimPendingKBTaskByType(context.Background(), kb.KBTaskTypeSemanticTree, "repair-test", time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingKBTaskByType: %v", err)
	}
	if claimed == nil || claimed.TaskID != "semantic-tree:p-r" {
		t.Fatalf("claimed=%+v, want semantic-tree:p-r", claimed)
	}
}
