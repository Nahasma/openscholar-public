package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openscholar/openscholar/internal/kb"
)

type kbAddTestIndexer struct {
	parseResult *kb.ParseResult
	parseErr    error
	buildErr    error
}

func (m *kbAddTestIndexer) ParseDocument(_ context.Context, _ string, _ kb.ParseOptions) (*kb.ParseResult, error) {
	return m.parseResult, m.parseErr
}

func (m *kbAddTestIndexer) BuildTree(_ context.Context, _ string, _ kb.IndexOptions) (*kb.IndexResult, error) {
	if m.buildErr != nil {
		return nil, m.buildErr
	}
	return &kb.IndexResult{Tree: kb.PaperTree{DocName: "semantic"}}, nil
}

type kbAddTestService struct {
	lastSemanticStatus string
	addParsedCalled    bool
	enqueueTask        kb.KBTask
	enqueueErr         error
	syncTask           kb.KBTask
	syncErr            error
}

func (m *kbAddTestService) AddParsedPaper(_ context.Context, paper kb.Paper, _ kb.ParseResult, semanticTreeStatus string) (kb.PaperIndexState, error) {
	m.addParsedCalled = true
	m.lastSemanticStatus = semanticTreeStatus
	return kb.PaperIndexState{
		PaperID:                paper.PaperID,
		IndexLevel:             kb.IndexLevelSimpleFullText,
		RawParseStatus:         "ready",
		FTSStatus:              "ready",
		SemanticTreeStatus:     semanticTreeStatus,
		SemanticTreeAvailable:  false,
		ContentAvailable:       true,
		FlatPageIndexAvailable: true,
		Extractor:              "simple_extract",
	}, nil
}

func (m *kbAddTestService) AttachSemanticTree(_ context.Context, _ string, _ kb.IndexResult) error {
	return nil
}

func (m *kbAddTestService) EnqueueSemanticTreeTask(_ context.Context, paperID, _ string, _ bool) (kb.KBTask, error) {
	if m.enqueueTask.TaskID == "" {
		m.enqueueTask = kb.KBTask{TaskID: "task-bg-1", PaperID: paperID, TaskType: kb.KBTaskTypeSemanticTree, Status: kb.KBTaskStatusQueued}
	}
	return m.enqueueTask, m.enqueueErr
}

func (m *kbAddTestService) RunSemanticTreeSync(ctx context.Context, indexer kb.Indexer, paperID, filePath string, _ bool) (kb.KBTask, error) {
	if m.syncTask.TaskID == "" {
		m.syncTask = kb.KBTask{TaskID: "task-sync-1", PaperID: paperID, TaskType: kb.KBTaskTypeSemanticTree, Status: kb.KBTaskStatusSucceeded}
	}
	if _, err := indexer.BuildTree(ctx, filePath, kb.DefaultIndexOptions()); err != nil {
		m.syncTask.Status = kb.KBTaskStatusFailed
		return m.syncTask, err
	}
	return m.syncTask, m.syncErr
}

func (m *kbAddTestService) AddPaper(context.Context, kb.Paper, kb.PaperTree) error { return nil }
func (m *kbAddTestService) GetPaper(context.Context, string) (*kb.Paper, error)    { return nil, nil }
func (m *kbAddTestService) GetTree(context.Context, string) (*kb.PaperTree, error) { return nil, nil }
func (m *kbAddTestService) ListPapers(context.Context, int, int) ([]kb.Paper, error) {
	return nil, nil
}
func (m *kbAddTestService) RemovePaper(context.Context, string) error { return nil }
func (m *kbAddTestService) CountPapers(context.Context) (int64, error) {
	return 0, nil
}
func (m *kbAddTestService) SearchByTitle(context.Context, string, int) ([]kb.Paper, error) {
	return nil, nil
}
func (m *kbAddTestService) FindByDOI(context.Context, string) (*kb.Paper, error) { return nil, nil }
func (m *kbAddTestService) FindByArxivID(context.Context, string) (*kb.Paper, error) {
	return nil, nil
}
func (m *kbAddTestService) GetNodeSummaries(context.Context, string) ([]kb.NodeSummary, error) {
	return nil, nil
}
func (m *kbAddTestService) CrossPaperSearch(context.Context, kb.LLMCaller, string, int) (*kb.CrossSearchResult, error) {
	return nil, nil
}

func TestKBAdd_DefaultSemanticTreeSkip(t *testing.T) {
	svc := &kbAddTestService{}
	idx := &kbAddTestIndexer{
		parseResult: &kb.ParseResult{
			PaperTitle: "P",
			DocType:    "pdf",
			Chunks:     []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", Content: "x", Source: "simple_extract"}},
			Extractor:  "simple_extract",
		},
	}
	tool := NewKBAddTool(svc, idx)

	resp, err := tool.Run(context.Background(), ToolCall{
		Name:  "KBAdd",
		Input: `{"file_path":"/tmp/test.pdf","paper_id":"p1","title":"P"}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.True(t, svc.addParsedCalled)
	assert.Equal(t, "not_requested", svc.lastSemanticStatus)
}

func TestKBAdd_SemanticTreeSyncFailurePreservesSuccess(t *testing.T) {
	svc := &kbAddTestService{}
	idx := &kbAddTestIndexer{
		parseResult: &kb.ParseResult{
			PaperTitle: "P",
			DocType:    "pdf",
			Chunks:     []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", Content: "x", Source: "simple_extract"}},
			Extractor:  "simple_extract",
		},
		buildErr: errors.New("semantic failed"),
	}
	tool := NewKBAddTool(svc, idx)

	resp, err := tool.Run(context.Background(), ToolCall{
		Name:  "KBAdd",
		Input: `{"file_path":"/tmp/test.pdf","paper_id":"p2","title":"P","semantic_tree":"sync"}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "raw ingest preserved")

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	assert.Equal(t, "sync", meta["semantic_tree_mode"])
}

func TestKBAdd_BackgroundEnqueueReturnsTaskID(t *testing.T) {
	svc := &kbAddTestService{
		enqueueTask: kb.KBTask{
			TaskID:   "task-bg-42",
			TaskType: kb.KBTaskTypeSemanticTree,
			Status:   kb.KBTaskStatusQueued,
		},
	}
	idx := &kbAddTestIndexer{
		parseResult: &kb.ParseResult{
			PaperTitle: "P",
			DocType:    "pdf",
			Chunks:     []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", Content: "x", Source: "simple_extract"}},
			Extractor:  "simple_extract",
		},
	}
	tool := NewKBAddTool(svc, idx)

	resp, err := tool.Run(context.Background(), ToolCall{
		Name:  "KBAdd",
		Input: `{"file_path":"/tmp/test.pdf","paper_id":"p3","title":"P","semantic_tree":"background"}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "task-bg-42")

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	assert.Equal(t, "queued", meta["semantic_tree_status"])
	assert.Equal(t, "task-bg-42", meta["semantic_tree_task_id"])
}
