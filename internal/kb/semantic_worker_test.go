package kb_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openscholar/openscholar/internal/kb"
)

type semanticWorkerTestIndexer struct {
	buildErr error
	onBuild  func()
}

func (m *semanticWorkerTestIndexer) BuildTree(_ context.Context, _ string, _ kb.IndexOptions) (*kb.IndexResult, error) {
	if m.onBuild != nil {
		m.onBuild()
	}
	if m.buildErr != nil {
		return nil, m.buildErr
	}
	return &kb.IndexResult{
		Tree: kb.PaperTree{
			DocName: "semantic",
			Structure: []kb.TreeNode{{
				NodeID: "n1", Title: "Sec", StartIndex: 1, EndIndex: 1, Summary: "sum", Content: "content",
			}},
		},
		IndexLevel:  kb.IndexLevelFullTree,
		TotalPages:  1,
		TotalTokens: 10,
		Extractor:   "pageindex",
	}, nil
}

func TestSemanticWorker_RunOnce_FailureMarksStateFailed(t *testing.T) {
	svc, conn := newServiceWithRawDB(t)
	ctx := context.Background()
	api := svc.(interface {
		AddParsedPaper(context.Context, kb.Paper, kb.ParseResult, string) (kb.PaperIndexState, error)
		EnqueueSemanticTreeTask(context.Context, string, string, bool) (kb.KBTask, error)
		GetPaperIndexStateView(context.Context, string) (kb.PaperIndexState, error)
	})

	_, err := api.AddParsedPaper(ctx, kb.Paper{PaperID: "wk-fail", Title: "fail", FilePath: "/tmp/fail.pdf", DocType: "pdf"}, kb.ParseResult{
		Chunks: []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", Content: "raw still here"}},
	}, "not_requested")
	require.NoError(t, err)
	_, err = api.EnqueueSemanticTreeTask(ctx, "wk-fail", "/tmp/fail.pdf", true)
	require.NoError(t, err)

	worker, err := kb.NewSemanticWorker(svc, &semanticWorkerTestIndexer{buildErr: errors.New("boom")}, "wk1", time.Minute)
	require.NoError(t, err)
	claimed, runErr := worker.RunOnce(ctx)
	require.NotNil(t, claimed)
	require.Error(t, runErr)

	state, err := api.GetPaperIndexStateView(ctx, "wk-fail")
	require.NoError(t, err)
	require.Equal(t, kb.KBTaskStatusFailed, state.SemanticTreeStatus)
	require.Contains(t, state.SemanticTreeError, "boom")
	require.Equal(t, 1, countRows(t, conn, "paper_chunks", "paper_id", "wk-fail"))
}

func TestRunSemanticTreeSync_Success(t *testing.T) {
	svc, _ := newServiceWithRawDB(t)
	ctx := context.Background()
	api := svc.(interface {
		AddParsedPaper(context.Context, kb.Paper, kb.ParseResult, string) (kb.PaperIndexState, error)
		RunSemanticTreeSync(context.Context, kb.Indexer, string, string, bool) (kb.KBTask, error)
		GetPaperIndexStateView(context.Context, string) (kb.PaperIndexState, error)
	})

	_, err := api.AddParsedPaper(ctx, kb.Paper{PaperID: "wk-sync", Title: "sync", FilePath: "/tmp/sync.pdf", DocType: "pdf"}, kb.ParseResult{
		Chunks: []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", Content: "raw"}},
	}, "not_requested")
	require.NoError(t, err)

	task, err := api.RunSemanticTreeSync(ctx, &semanticWorkerTestIndexer{}, "wk-sync", "/tmp/sync.pdf", true)
	require.NoError(t, err)
	require.Equal(t, kb.KBTaskStatusSucceeded, task.Status)

	state, err := api.GetPaperIndexStateView(ctx, "wk-sync")
	require.NoError(t, err)
	require.Equal(t, "ready", state.SemanticTreeStatus)
	require.True(t, state.SemanticTreeAvailable)
}

func TestRunSemanticTreeSync_SetsRecoverableLeaseWhileRunning(t *testing.T) {
	svc, conn := newServiceWithRawDB(t)
	ctx := context.Background()
	api := svc.(interface {
		AddParsedPaper(context.Context, kb.Paper, kb.ParseResult, string) (kb.PaperIndexState, error)
		RunSemanticTreeSync(context.Context, kb.Indexer, string, string, bool) (kb.KBTask, error)
	})

	_, err := api.AddParsedPaper(ctx, kb.Paper{PaperID: "wk-sync-lease", Title: "sync", FilePath: "/tmp/sync-lease.pdf", DocType: "pdf"}, kb.ParseResult{
		Chunks: []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", Content: "raw"}},
	}, "not_requested")
	require.NoError(t, err)

	checked := false
	indexer := &semanticWorkerTestIndexer{onBuild: func() {
		var leaseUntil sql.NullInt64
		err := conn.QueryRowContext(ctx, `SELECT lease_until FROM kb_tasks WHERE paper_id = ? AND status = ?`, "wk-sync-lease", kb.KBTaskStatusRunning).Scan(&leaseUntil)
		require.NoError(t, err)
		require.True(t, leaseUntil.Valid)
		require.Greater(t, leaseUntil.Int64, time.Now().Unix())
		checked = true
	}}
	_, err = api.RunSemanticTreeSync(ctx, indexer, "wk-sync-lease", "/tmp/sync-lease.pdf", true)
	require.NoError(t, err)
	require.True(t, checked)
}
