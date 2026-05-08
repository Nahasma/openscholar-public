package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/kb"
	"github.com/openscholar/openscholar/internal/memory"
)

type fakeKBMemoryService struct {
	lastCapture memory.KBCapture
	captureCnt  int
}

func (f *fakeKBMemoryService) Insert(ctx context.Context, item memory.MemoryItem) error {
	return nil
}

func (f *fakeKBMemoryService) Update(ctx context.Context, id string, content string, metadata map[string]any) error {
	return nil
}

func (f *fakeKBMemoryService) Delete(ctx context.Context, id string) error {
	return nil
}

func (f *fakeKBMemoryService) RetrieveForSession(ctx context.Context, sessionID string, limit int) ([]memory.MemoryItem, error) {
	return nil, nil
}

func (f *fakeKBMemoryService) RetrieveRecent(ctx context.Context, limit int) ([]memory.MemoryItem, error) {
	return nil, nil
}

func (f *fakeKBMemoryService) RetrieveForPrompt(ctx context.Context, sessionID string, opts memory.PromptMemoryOptions) ([]memory.MemoryItem, []memory.MemoryItem, error) {
	return nil, nil, nil
}

func (f *fakeKBMemoryService) ExecuteSkills(ctx context.Context, sessionID string, sessionText string) error {
	return nil
}

func (f *fakeKBMemoryService) CaptureKB(ctx context.Context, capture memory.KBCapture) error {
	f.lastCapture = capture
	f.captureCnt++
	return nil
}

func (f *fakeKBMemoryService) SetLLMCaller(caller memory.LLMCaller) {}

type fakeKBService struct {
	queryResult  *kb.SearchResult
	searchResult *kb.CrossSearchResult
}

func (f *fakeKBService) AddPaper(ctx context.Context, paper kb.Paper, tree kb.PaperTree) error {
	return nil
}

func (f *fakeKBService) GetPaper(ctx context.Context, paperID string) (*kb.Paper, error) {
	return &kb.Paper{PaperID: paperID, Title: "paper"}, nil
}

func (f *fakeKBService) ListPapers(ctx context.Context, limit, offset int) ([]kb.Paper, error) {
	return nil, nil
}

func (f *fakeKBService) RemovePaper(ctx context.Context, paperID string) error {
	return nil
}

func (f *fakeKBService) CountPapers(ctx context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeKBService) SearchByTitle(ctx context.Context, query string, limit int) ([]kb.Paper, error) {
	return nil, nil
}

func (f *fakeKBService) FindByDOI(ctx context.Context, doi string) (*kb.Paper, error) {
	return nil, nil
}

func (f *fakeKBService) FindByArxivID(ctx context.Context, arxivID string) (*kb.Paper, error) {
	return nil, nil
}

func (f *fakeKBService) GetNodeSummaries(ctx context.Context, paperID string) ([]kb.NodeSummary, error) {
	return nil, nil
}

func (f *fakeKBService) GetTree(ctx context.Context, paperID string) (*kb.PaperTree, error) {
	return nil, nil
}

func (f *fakeKBService) CrossPaperSearch(ctx context.Context, callLLM kb.LLMCaller, question string, limit int) (*kb.CrossSearchResult, error) {
	return f.searchResult, nil
}

func (f *fakeKBService) TreeSearch(ctx context.Context, callLLM kb.LLMCaller, paperID string, question string) (*kb.SearchResult, error) {
	return f.queryResult, nil
}

func (f *fakeKBService) QueryPaper(ctx context.Context, callLLM kb.LLMCaller, paperID string, question string, opts kb.QueryOptions) (*kb.SearchResult, error) {
	return f.queryResult, nil
}

func TestKBQueryTool_CapturesKBMemoryWithSession(t *testing.T) {
	mem := &fakeKBMemoryService{}
	tool := NewKBQueryTool(&fakeKBService{
		queryResult: &kb.SearchResult{
			Answer: "method x",
			Sources: []kb.SourceRef{{
				NodeID:    "node-1",
				Title:     "Method",
				StartPage: 2,
				EndPage:   3,
			}},
		},
	}, func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}, mem)

	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-1")
	_, err := tool.Run(ctx, ToolCall{Input: `{"paper_id":"paper-1","question":"what is the method?"}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if mem.captureCnt != 1 {
		t.Fatalf("CaptureKB count = %d, want 1", mem.captureCnt)
	}
	if mem.lastCapture.SessionID != "sess-1" {
		t.Fatalf("sessionID = %q, want sess-1", mem.lastCapture.SessionID)
	}
	if mem.lastCapture.PaperID != "paper-1" {
		t.Fatalf("paperID = %q, want paper-1", mem.lastCapture.PaperID)
	}
	if len(mem.lastCapture.Sources) != 1 {
		t.Fatalf("sources length = %d, want 1", len(mem.lastCapture.Sources))
	}
}

func TestKBQueryTool_SkipsCaptureWithoutSession(t *testing.T) {
	mem := &fakeKBMemoryService{}
	tool := NewKBQueryTool(&fakeKBService{
		queryResult: &kb.SearchResult{
			Answer: "method x",
		},
	}, func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}, mem)

	_, err := tool.Run(context.Background(), ToolCall{Input: `{"paper_id":"paper-1","question":"what is the method?"}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if mem.captureCnt != 0 {
		t.Fatalf("CaptureKB count = %d, want 0", mem.captureCnt)
	}
}

func TestKBQueryTool_CapturedAnswerIncludesLimitations(t *testing.T) {
	mem := &fakeKBMemoryService{}
	tool := NewKBQueryTool(&fakeKBService{
		queryResult: &kb.SearchResult{
			Answer: "method x",
			Sources: []kb.SourceRef{{
				NodeID:    "node-1",
				Title:     "Method",
				StartPage: 2,
				EndPage:   3,
			}},
			Diagnostics: kb.SearchDiagnostics{
				Route:            "local_pages",
				ContentAvailable: false,
			},
		},
	}, func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}, mem)

	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-limited")
	_, err := tool.Run(ctx, ToolCall{Input: `{"paper_id":"paper-1","question":"what is the method?"}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(mem.lastCapture.Answer, "content_available=false") {
		t.Fatalf("captured answer missing limitation: %q", mem.lastCapture.Answer)
	}
}

func TestKBSearchTool_CapturesKBMemoryWithSession(t *testing.T) {
	mem := &fakeKBMemoryService{}
	tool := NewKBSearchTool(&fakeKBService{
		searchResult: &kb.CrossSearchResult{
			Answer: "cross paper answer",
			Sources: []kb.CrossSourceRef{{
				PaperID:    "paper-1",
				PaperTitle: "paper",
				NodeID:     "node-1",
				NodeTitle:  "Method",
				StartPage:  4,
				EndPage:    5,
			}},
		},
	}, func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}, mem)

	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-2")
	_, err := tool.Run(ctx, ToolCall{Input: `{"question":"what methods exist?"}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if mem.captureCnt != 1 {
		t.Fatalf("CaptureKB count = %d, want 1", mem.captureCnt)
	}
	if mem.lastCapture.SourceType != "kb_search" {
		t.Fatalf("source_type = %q, want kb_search", mem.lastCapture.SourceType)
	}
	if mem.lastCapture.Question != "what methods exist?" {
		t.Fatalf("question = %q", mem.lastCapture.Question)
	}
	if len(mem.lastCapture.Sources) != 1 {
		t.Fatalf("sources length = %d, want 1", len(mem.lastCapture.Sources))
	}
}

func TestKBSearchTool_SkipsCaptureWithoutSources(t *testing.T) {
	mem := &fakeKBMemoryService{}
	tool := NewKBSearchTool(&fakeKBService{
		searchResult: &kb.CrossSearchResult{
			Answer: "diagnostic answer",
		},
	}, func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}, mem)

	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-3")
	_, err := tool.Run(ctx, ToolCall{Input: `{"question":"what methods exist?"}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if mem.captureCnt != 0 {
		t.Fatalf("CaptureKB count = %d, want 0", mem.captureCnt)
	}
}

func TestKBQueryTool_SkipsCaptureForMetadataOnlyResult(t *testing.T) {
	mem := &fakeKBMemoryService{}
	tool := NewKBQueryTool(&fakeKBService{
		queryResult: &kb.SearchResult{
			Answer: "Paper: demo",
			Diagnostics: kb.SearchDiagnostics{
				Route:        "metadata",
				LLMCallCount: 0,
			},
		},
	}, func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}, mem)

	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-1")
	_, err := tool.Run(ctx, ToolCall{Input: `{"paper_id":"paper-1","question":"what is the title and year?"}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if mem.captureCnt != 0 {
		t.Fatalf("CaptureKB count = %d, want 0", mem.captureCnt)
	}
}

func TestKBQueryTool_SkipsCaptureForDeepReadRecommendation(t *testing.T) {
	mem := &fakeKBMemoryService{}
	tool := NewKBQueryTool(&fakeKBService{
		queryResult: &kb.SearchResult{
			Answer: "Use deep-read workflow.",
			Sources: []kb.SourceRef{{
				NodeID:    "chunk-1",
				Title:     "Chunk",
				StartPage: 1,
				EndPage:   1,
			}},
			Diagnostics: kb.SearchDiagnostics{
				Route:               "deep_read_task",
				DeepReadRecommended: true,
			},
		},
	}, func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}, mem)

	ctx := context.WithValue(context.Background(), SessionIDContextKey, "sess-1")
	_, err := tool.Run(ctx, ToolCall{Input: `{"paper_id":"paper-1","question":"summarize the whole paper in detail"}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if mem.captureCnt != 0 {
		t.Fatalf("CaptureKB count = %d, want 0", mem.captureCnt)
	}
}
