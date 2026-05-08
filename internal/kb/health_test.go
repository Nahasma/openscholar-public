package kb_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/db"
	"github.com/openscholar/openscholar/internal/kb"
	"github.com/openscholar/openscholar/internal/testutil"
)

func TestKBHealth_FindsStaleTaskAndMissingChunks(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	now := time.Now().Unix()
	_ = q.InsertPaper(context.Background(), db.InsertPaperParams{PaperID: "p-h", Title: "Health"})
	_ = q.UpsertPaperIndexState(context.Background(), db.UpsertPaperIndexStateParams{PaperID: "p-h", IndexLevel: ns("summary_only"), RawParseStatus: ns("failed"), ContentAvailable: 0, ContentBytes: 0, FlatPageIndexAvailable: 0, FtsStatus: ns("ready"), SemanticTreeStatus: ns("ready"), SemanticTreeAvailable: 1, UpdatedAt: now})
	_ = q.InsertKBTask(context.Background(), db.InsertKBTaskParams{TaskID: "t-stale", TaskType: kb.KBTaskTypeRepair, Status: "running", UpdatedAt: now - 4000, CreatedAt: now - 5000, LeaseUntil: ni(now - 10)})

	reader := svc.(interface {
		KBHealth(ctx context.Context, opts kb.KBHealthOptions) (*kb.KBHealthReport, error)
	})
	report, err := reader.KBHealth(context.Background(), kb.KBHealthOptions{})
	if err != nil {
		t.Fatalf("KBHealth: %v", err)
	}
	if len(report.Checks) == 0 {
		t.Fatal("expected checks")
	}
}

func ns(v string) sql.NullString { return sql.NullString{String: v, Valid: v != ""} }
func ni(v int64) sql.NullInt64   { return sql.NullInt64{Int64: v, Valid: true} }
