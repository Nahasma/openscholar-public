package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/kb"
)

type kbTreeListFakeService struct {
	papers     []kb.Paper
	state      kb.PaperIndexState
	chunks     []kb.PaperChunk
	bulkItems  []kb.PaperListItem
	stateCalls int
	taskCalls  int
}

func (f *kbTreeListFakeService) AddPaper(context.Context, kb.Paper, kb.PaperTree) error { return nil }
func (f *kbTreeListFakeService) GetPaper(context.Context, string) (*kb.Paper, error) {
	return &kb.Paper{PaperID: "p1", Title: "T"}, nil
}
func (f *kbTreeListFakeService) ListPapers(context.Context, int, int) ([]kb.Paper, error) {
	return f.papers, nil
}
func (f *kbTreeListFakeService) RemovePaper(context.Context, string) error { return nil }
func (f *kbTreeListFakeService) CountPapers(context.Context) (int64, error) {
	return int64(len(f.papers)), nil
}
func (f *kbTreeListFakeService) SearchByTitle(context.Context, string, int) ([]kb.Paper, error) {
	return nil, nil
}
func (f *kbTreeListFakeService) FindByDOI(context.Context, string) (*kb.Paper, error) {
	return nil, nil
}
func (f *kbTreeListFakeService) FindByArxivID(context.Context, string) (*kb.Paper, error) {
	return nil, nil
}
func (f *kbTreeListFakeService) GetNodeSummaries(context.Context, string) ([]kb.NodeSummary, error) {
	return nil, nil
}
func (f *kbTreeListFakeService) GetTree(context.Context, string) (*kb.PaperTree, error) {
	return nil, context.Canceled
}
func (f *kbTreeListFakeService) CrossPaperSearch(context.Context, kb.LLMCaller, string, int) (*kb.CrossSearchResult, error) {
	return nil, nil
}
func (f *kbTreeListFakeService) GetPaperIndexStateView(context.Context, string) (kb.PaperIndexState, error) {
	f.stateCalls++
	return f.state, nil
}
func (f *kbTreeListFakeService) GetPaperChunksView(context.Context, string, int) ([]kb.PaperChunk, error) {
	return f.chunks, nil
}
func (f *kbTreeListFakeService) ListPaperTasksView(context.Context, string, int) ([]kb.KBTask, error) {
	f.taskCalls++
	return []kb.KBTask{{TaskType: "semantic_tree_build", Status: "running"}}, nil
}
func (f *kbTreeListFakeService) ListPapersWithStatusView(context.Context, int, int) ([]kb.PaperListItem, error) {
	return f.bulkItems, nil
}

func TestKBTreeTool_PagesViewShowsRequiredSentence(t *testing.T) {
	svc := &kbTreeListFakeService{
		state: kb.PaperIndexState{ContentAvailable: true, SemanticTreeAvailable: false, SemanticTreeStatus: "not_requested"},
		chunks: []kb.PaperChunk{
			{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Title: "Intro"},
		},
	}
	tool := NewKBTreeTool(svc)
	resp, err := tool.Run(context.Background(), ToolCall{Input: `{"paper_id":"p1","view":"pages"}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(resp.Content, "Semantic tree unavailable; showing flat page index from parsed text.") {
		t.Fatalf("missing required sentence: %s", resp.Content)
	}
}

func TestKBTreeTool_SemanticMissingDoesNotFallbackToPages(t *testing.T) {
	svc := &kbTreeListFakeService{
		state: kb.PaperIndexState{ContentAvailable: true, SemanticTreeAvailable: false, SemanticTreeStatus: "failed"},
		chunks: []kb.PaperChunk{
			{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Title: "Intro"},
		},
	}
	tool := NewKBTreeTool(svc)
	resp, err := tool.Run(context.Background(), ToolCall{Input: `{"paper_id":"p1","view":"semantic"}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(resp.Content, "Semantic tree unavailable.") {
		t.Fatalf("expected semantic unavailable: %s", resp.Content)
	}
	if strings.Contains(resp.Content, "flat page index") {
		t.Fatalf("semantic view should not show pages: %s", resp.Content)
	}
}

func TestKBListTool_DefaultCompactOutput(t *testing.T) {
	svc := &kbTreeListFakeService{
		papers: []kb.Paper{{PaperID: "p1", Title: "Demo"}},
		bulkItems: []kb.PaperListItem{{
			Paper: kb.Paper{PaperID: "p1", Title: "Demo"},
			State: kb.PaperIndexState{
				ContentAvailable:   false,
				RawParseStatus:     "failed",
				FTSStatus:          "missing",
				SemanticTreeStatus: "not_requested",
			},
		}},
	}
	tool := NewKBListTool(svc)
	resp, err := tool.Run(context.Background(), ToolCall{Input: `{"limit":1}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{"Knowledge Base (1 papers total)", "Showing 1 item(s)", "verbose=true"} {
		if !strings.Contains(resp.Content, want) {
			t.Fatalf("missing %q in output:\n%s", want, resp.Content)
		}
	}
	if strings.Contains(resp.Content, "authors: unknown") {
		t.Fatalf("compact output should not include detailed fields: %s", resp.Content)
	}
	if svc.stateCalls != 0 || svc.taskCalls != 0 {
		t.Fatalf("bulk KBList should not call per-paper readers, state=%d tasks=%d", svc.stateCalls, svc.taskCalls)
	}
}

func TestKBListTool_VerboseTrueKeepsDetailedFields(t *testing.T) {
	svc := &kbTreeListFakeService{
		papers: []kb.Paper{{PaperID: "p1", Title: "Demo"}},
		bulkItems: []kb.PaperListItem{{
			Paper: kb.Paper{PaperID: "p1", Title: "Demo"},
			State: kb.PaperIndexState{
				ContentAvailable:   false,
				RawParseStatus:     "failed",
				FTSStatus:          "missing",
				SemanticTreeStatus: "not_requested",
			},
		}},
	}
	tool := NewKBListTool(svc)
	resp, err := tool.Run(context.Background(), ToolCall{Input: `{"limit":1,"verbose":true}`})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{"authors: unknown", "year: unknown", "venue: unknown"} {
		if !strings.Contains(resp.Content, want) {
			t.Fatalf("missing %q in output:\n%s", want, resp.Content)
		}
	}
}
