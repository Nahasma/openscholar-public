package kb_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openscholar/openscholar/internal/kb"
)

func TestEnqueueSemanticTreeTask_UpdatesTaskAndState(t *testing.T) {
	svc, _ := newServiceWithRawDB(t)
	ctx := context.Background()

	writer := svc.(interface {
		AddParsedPaper(context.Context, kb.Paper, kb.ParseResult, string) (kb.PaperIndexState, error)
		EnqueueSemanticTreeTask(context.Context, string, string, bool) (kb.KBTask, error)
		GetPaperIndexStateView(context.Context, string) (kb.PaperIndexState, error)
	})

	_, err := writer.AddParsedPaper(ctx, kb.Paper{PaperID: "task-p1", Title: "P1", FilePath: "/tmp/p1.pdf", DocType: "pdf"}, kb.ParseResult{
		Chunks: []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", Content: "raw content"}},
	}, "not_requested")
	require.NoError(t, err)

	task, err := writer.EnqueueSemanticTreeTask(ctx, "task-p1", "/tmp/p1.pdf", true)
	require.NoError(t, err)
	require.Equal(t, kb.KBTaskStatusQueued, task.Status)
	require.Equal(t, kb.KBTaskTypeSemanticTree, task.TaskType)
	require.NotEmpty(t, task.TaskID)

	again, err := writer.EnqueueSemanticTreeTask(ctx, "task-p1", "/tmp/p1.pdf", true)
	require.NoError(t, err)
	require.Equal(t, task.TaskID, again.TaskID)

	state, err := writer.GetPaperIndexStateView(ctx, "task-p1")
	require.NoError(t, err)
	require.Equal(t, kb.KBTaskStatusQueued, state.SemanticTreeStatus)
	require.Equal(t, task.TaskID, state.SemanticTreeTaskID)
}

func TestClaimPendingKBTask_ClaimsStaleRunningLease(t *testing.T) {
	svc, _ := newServiceWithRawDB(t)
	ctx := context.Background()

	api := svc.(interface {
		AddParsedPaper(context.Context, kb.Paper, kb.ParseResult, string) (kb.PaperIndexState, error)
		EnqueueSemanticTreeTask(context.Context, string, string, bool) (kb.KBTask, error)
		ClaimPendingKBTask(context.Context, string, time.Duration) (*kb.KBTask, error)
		HeartbeatKBTask(context.Context, string, string, float64, time.Duration) error
		RetryKBTask(context.Context, string, string) error
	})

	_, err := api.AddParsedPaper(ctx, kb.Paper{PaperID: "paper-any", Title: "Any", FilePath: "/tmp/any.pdf", DocType: "pdf"}, kb.ParseResult{
		Chunks: []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", Content: "raw content"}},
	}, "not_requested")
	require.NoError(t, err)

	task, err := api.EnqueueSemanticTreeTask(ctx, "paper-any", "/tmp/any.pdf", true)
	require.NoError(t, err)

	claimed, err := api.ClaimPendingKBTask(ctx, "w1", 10*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, task.TaskID, claimed.TaskID)
	require.Equal(t, kb.KBTaskStatusRunning, claimed.Status)

	require.NoError(t, api.HeartbeatKBTask(ctx, claimed.TaskID, "w1", 0.5, -time.Second))

	reclaimed, err := api.ClaimPendingKBTask(ctx, "w2", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, reclaimed)
	require.Equal(t, task.TaskID, reclaimed.TaskID)
	require.Equal(t, "w2", reclaimed.LeaseOwner)

	require.NoError(t, api.RetryKBTask(ctx, reclaimed.TaskID, "retry"))
}
