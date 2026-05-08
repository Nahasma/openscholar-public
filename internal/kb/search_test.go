package kb_test

import (
	"context"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/kb"
	"github.com/openscholar/openscholar/internal/testutil"
)

type treeSearcher interface {
	TreeSearch(ctx context.Context, callLLM kb.LLMCaller, paperID string, question string) (*kb.SearchResult, error)
}

func newSearchTestService(t *testing.T) kb.Service {
	t.Helper()
	conn, q := testutil.SetupTestDB(t)
	return kb.NewService(q, conn)
}

func addSearchTestPaper(t *testing.T, svc kb.Service, content string) {
	t.Helper()
	tree := kb.PaperTree{
		DocName: "search-test",
		Structure: []kb.TreeNode{{
			NodeID:     "n1",
			Title:      "Method",
			StartIndex: 2,
			EndIndex:   2,
			Summary:    "front summary only",
			Content:    content,
		}},
	}
	paper := kb.Paper{PaperID: "paper-search", Title: "Search Test"}
	if err := svc.AddPaper(context.Background(), paper, tree); err != nil {
		t.Fatalf("AddPaper() error = %v", err)
	}
}

func TestTreeSearchRepairsTruncatedSelection(t *testing.T) {
	svc := newSearchTestService(t)
	addSearchTestPaper(t, svc, "full method text")

	calls := 0
	result, err := svc.(treeSearcher).TreeSearch(context.Background(), func(_ context.Context, prompt string) (string, error) {
		calls++
		switch calls {
		case 1:
			return `{"thinking":"truncated","node_list":["n1"]`, nil
		case 2:
			if !strings.Contains(prompt, "Previous response snippet") {
				t.Fatalf("repair prompt missing snippet: %s", prompt)
			}
			return `{"thinking":"repaired","node_list":["n1"]}`, nil
		default:
			return "answer from repaired selection", nil
		}
	}, "paper-search", "method")
	if err != nil {
		t.Fatalf("TreeSearch() error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("LLM calls = %d, want 3", calls)
	}
	if result.Diagnostics.ParseRetryCount != 1 {
		t.Fatalf("ParseRetryCount = %d, want 1", result.Diagnostics.ParseRetryCount)
	}
	if result.Diagnostics.SelectionParseError != "" {
		t.Fatalf("SelectionParseError = %q, want empty", result.Diagnostics.SelectionParseError)
	}
	if len(result.Diagnostics.SelectedNodeIDs) != 1 || result.Diagnostics.SelectedNodeIDs[0] != "n1" {
		t.Fatalf("SelectedNodeIDs = %#v, want n1", result.Diagnostics.SelectedNodeIDs)
	}
}

func TestTreeSearchRepairsMissingNodeList(t *testing.T) {
	svc := newSearchTestService(t)
	addSearchTestPaper(t, svc, "full method text")

	calls := 0
	result, err := svc.(treeSearcher).TreeSearch(context.Background(), func(_ context.Context, _ string) (string, error) {
		calls++
		switch calls {
		case 1:
			return `{"thinking":"missing list"}`, nil
		case 2:
			return `{"thinking":"repaired","node_list":["n1"]}`, nil
		default:
			return "answer from repaired shape", nil
		}
	}, "paper-search", "method")
	if err != nil {
		t.Fatalf("TreeSearch() error = %v", err)
	}
	if result.Diagnostics.ParseRetryCount != 1 {
		t.Fatalf("ParseRetryCount = %d, want 1", result.Diagnostics.ParseRetryCount)
	}
	if result.Diagnostics.SelectionParseError != "" {
		t.Fatalf("SelectionParseError = %q, want empty", result.Diagnostics.SelectionParseError)
	}
}

func TestTreeSearchInvalidNodeUsesLexicalFallback(t *testing.T) {
	svc := newSearchTestService(t)
	addSearchTestPaper(t, svc, "method content")

	result, err := svc.(treeSearcher).TreeSearch(context.Background(), func(_ context.Context, _ string) (string, error) {
		return `{"thinking":"bad id","node_list":["missing-node"]}`, nil
	}, "paper-search", "method")
	if err != nil {
		t.Fatalf("TreeSearch() error = %v", err)
	}
	if !result.Diagnostics.FallbackUsed {
		t.Fatal("FallbackUsed = false, want true")
	}
	if len(result.Diagnostics.InvalidNodeIDs) != 1 || result.Diagnostics.InvalidNodeIDs[0] != "missing-node" {
		t.Fatalf("InvalidNodeIDs = %#v, want missing-node", result.Diagnostics.InvalidNodeIDs)
	}
	if len(result.Sources) != 1 || result.Sources[0].NodeID != "n1" {
		t.Fatalf("Sources = %#v, want n1", result.Sources)
	}
}

func TestTreeSearchEmptySelectionUsesContentFTS(t *testing.T) {
	svc := newSearchTestService(t)
	addSearchTestPaper(t, svc, "tailtoken appears only in full page text")

	var answerPrompt string
	calls := 0
	result, err := svc.(treeSearcher).TreeSearch(context.Background(), func(_ context.Context, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return `{"thinking":"summary did not show it","node_list":[]}`, nil
		}
		answerPrompt = prompt
		return "tailtoken appears in full text (p. 2)", nil
	}, "paper-search", "tailtoken: where is it?")
	if err != nil {
		t.Fatalf("TreeSearch() error = %v", err)
	}
	if !strings.Contains(answerPrompt, "tailtoken appears only in full page text") {
		t.Fatalf("answer prompt did not include FTS content: %s", answerPrompt)
	}
	if !result.Diagnostics.UsedFullContent {
		t.Fatal("UsedFullContent = false, want true")
	}
	if !result.Diagnostics.FallbackUsed {
		t.Fatal("FallbackUsed = false, want true")
	}
}

func TestTreeSearchFullTreeSelectionIsNotOverriddenByFTS(t *testing.T) {
	svc := newSearchTestService(t)
	indexed := svc.(interface {
		AddIndexedPaper(context.Context, kb.Paper, kb.IndexResult) error
	})
	tree := kb.PaperTree{
		DocName: "full-tree",
		Structure: []kb.TreeNode{
			{NodeID: "n1", Title: "Selected Method", StartIndex: 1, EndIndex: 1, Summary: "selected", Content: "selected content with method"},
			{NodeID: "n2", Title: "Other Result", StartIndex: 2, EndIndex: 2, Summary: "other", Content: "other content with result"},
		},
	}
	err := indexed.AddIndexedPaper(context.Background(), kb.Paper{PaperID: "paper-full-tree", Title: "Full Tree"}, kb.IndexResult{
		Tree:             tree,
		ModelUsed:        "pageindex",
		IndexLevel:       kb.IndexLevelFullTree,
		Extractor:        "pageindex",
		ContentAvailable: true,
	})
	if err != nil {
		t.Fatalf("AddIndexedPaper() error = %v", err)
	}

	var answerPrompt string
	calls := 0
	_, err = svc.(treeSearcher).TreeSearch(context.Background(), func(_ context.Context, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return `{"thinking":"tree chose method","node_list":["n1"]}`, nil
		}
		answerPrompt = prompt
		return "answer from selected method", nil
	}, "paper-full-tree", "method result")
	if err != nil {
		t.Fatalf("TreeSearch() error = %v", err)
	}
	if !strings.Contains(answerPrompt, "selected content with method") {
		t.Fatalf("answer prompt missing selected content: %s", answerPrompt)
	}
	if strings.Contains(answerPrompt, "other content with result") {
		t.Fatalf("FTS content overrode selected tree node: %s", answerPrompt)
	}
}

func TestTreeSearchUsesFullContentBeyondSummary(t *testing.T) {
	svc := newSearchTestService(t)
	fullContent := strings.Repeat("front ", 1200) + "tailtoken appears only after the summary and prefix window"
	addSearchTestPaper(t, svc, fullContent)

	var answerPrompt string
	calls := 0
	result, err := svc.(treeSearcher).TreeSearch(context.Background(), func(_ context.Context, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return `{"thinking":"select method","node_list":["n1"]}`, nil
		}
		answerPrompt = prompt
		return "tailtoken appears in the method (p. 2)", nil
	}, "paper-search", "tailtoken")
	if err != nil {
		t.Fatalf("TreeSearch() error = %v", err)
	}
	if !strings.Contains(answerPrompt, "tailtoken appears only after the summary and prefix window") {
		t.Fatalf("answer prompt did not include full content tail: %s", answerPrompt)
	}
	if !result.Diagnostics.UsedFullContent {
		t.Fatal("UsedFullContent = false, want true")
	}
	if result.Diagnostics.RetrievalMode != kb.RetrievalModeFullContent {
		t.Fatalf("RetrievalMode = %q, want %q", result.Diagnostics.RetrievalMode, kb.RetrievalModeFullContent)
	}
}
