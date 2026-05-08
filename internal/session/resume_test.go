package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

func TestResumeServiceResolveLatestAndQuery(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)
	ctx := context.Background()

	s1, _ := sessSvc.Create(ctx, "alpha title")
	s1.ProjectPath = "/tmp/proj-a"
	s1.FirstPrompt = "first prompt about transformers"
	s1.RootSessionID = s1.ID
	_, _ = sessSvc.Save(ctx, s1)
	_, _ = msgSvc.Create(ctx, s1.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "first prompt about transformers"}},
	})

	s2, _ := sessSvc.Create(ctx, "beta title")
	s2.ProjectPath = "/tmp/proj-a"
	s2.RootSessionID = s2.ID
	_, _ = sessSvc.Save(ctx, s2)
	_, _ = msgSvc.Create(ctx, s2.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "second prompt"}},
	})
	time.Sleep(1100 * time.Millisecond)
	s2.Title = "beta title v2"
	_, _ = sessSvc.Save(ctx, s2)

	latest, err := resume.Resolve(ctx, session.ResumeRequest{Mode: session.ResolveLatest, ProjectPath: "/tmp/proj-a"})
	if err != nil {
		t.Fatalf("resolve latest: %v", err)
	}
	if !latest.Found || latest.Session.ID != s2.ID {
		t.Fatalf("expected latest to resolve to s2")
	}

	results, err := resume.Search(ctx, "/tmp/proj-a", "transformers", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 || results[0].ID != s1.ID {
		t.Fatalf("expected metadata search hit s1, got %+v", results)
	}
}

func TestResumeServiceLegacyBlankProjectPathMatchesCurrentProject(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)
	ctx := context.Background()

	legacy, _ := sessSvc.Create(ctx, "legacy blank project")
	legacy.ProjectPath = ""
	legacy.Worktree = ""
	legacy.RootSessionID = ""
	legacy.FirstPrompt = "legacy metadata needle"
	_, _ = sessSvc.Save(ctx, legacy)
	_, err := msgSvc.Create(ctx, legacy.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "legacy transcript zephyr"}},
	})
	if err != nil {
		t.Fatalf("create legacy message: %v", err)
	}

	list, err := resume.List(ctx, "/tmp/current-project", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) == 0 || list[0].ID != legacy.ID {
		t.Fatalf("expected legacy blank project session in picker list, got %+v", list)
	}

	latest, err := resume.Resolve(ctx, session.ResumeRequest{Mode: session.ResolveLatest, ProjectPath: "/tmp/current-project"})
	if err != nil {
		t.Fatalf("resolve latest: %v", err)
	}
	if !latest.Found || latest.Session.ID != legacy.ID {
		t.Fatalf("expected legacy blank project latest, got %+v", latest)
	}

	exact, err := resume.Resolve(ctx, session.ResumeRequest{
		Mode:        session.ResolveExact,
		Selector:    "legacy blank project",
		ProjectPath: "/tmp/current-project",
	})
	if err != nil {
		t.Fatalf("resolve exact: %v", err)
	}
	if !exact.Found || exact.Session.ID != legacy.ID {
		t.Fatalf("expected legacy blank project exact match, got %+v", exact)
	}

	metadata, err := resume.Search(ctx, "/tmp/current-project", "metadata needle", 10)
	if err != nil {
		t.Fatalf("metadata search: %v", err)
	}
	if len(metadata) == 0 || metadata[0].ID != legacy.ID {
		t.Fatalf("expected legacy blank project metadata hit, got %+v", metadata)
	}

	fts, err := resume.Search(ctx, "/tmp/current-project", "zephyr", 10)
	if err != nil {
		t.Fatalf("fts search: %v", err)
	}
	if len(fts) == 0 || fts[0].ID != legacy.ID {
		t.Fatalf("expected legacy blank project FTS hit, got %+v", fts)
	}
}

func TestResumeServiceSearchSupportsUnicodeQuery(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)
	ctx := context.Background()

	target, _ := sessSvc.Create(ctx, "量子论文")
	target.ProjectPath = "/tmp/unicode-project"
	target.FirstPrompt = "量子纠缠"
	target.RootSessionID = target.ID
	_, _ = sessSvc.Save(ctx, target)
	_, _ = msgSvc.Create(ctx, target.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "量子纠缠 transcript"}},
	})

	other, _ := sessSvc.Create(ctx, "latest unrelated")
	other.ProjectPath = "/tmp/unicode-project"
	other.RootSessionID = other.ID
	_, _ = sessSvc.Save(ctx, other)
	_, _ = msgSvc.Create(ctx, other.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "unrelated"}},
	})
	time.Sleep(1100 * time.Millisecond)
	other.Title = "latest unrelated v2"
	_, _ = sessSvc.Save(ctx, other)

	results, err := resume.Search(ctx, "/tmp/unicode-project", "量子", 10)
	if err != nil {
		t.Fatalf("search unicode: %v", err)
	}
	if len(results) == 0 || results[0].ID != target.ID {
		t.Fatalf("expected unicode query to find target, got %+v", results)
	}
}

func TestResumeServiceSearchTreatsUnderscoreLiterally(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)
	ctx := context.Background()

	target, _ := sessSvc.Create(ctx, "target")
	target.ProjectPath = "/tmp/underscore-project"
	target.FirstPrompt = "literal foo_bar value"
	target.RootSessionID = target.ID
	_, _ = sessSvc.Save(ctx, target)
	_, _ = msgSvc.Create(ctx, target.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "literal foo_bar transcript"}},
	})

	noise, _ := sessSvc.Create(ctx, "noise")
	noise.ProjectPath = "/tmp/underscore-project"
	noise.FirstPrompt = "wildcard-like fooXbar value"
	noise.RootSessionID = noise.ID
	_, _ = sessSvc.Save(ctx, noise)
	_, _ = msgSvc.Create(ctx, noise.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "fooXbar transcript"}},
	})

	results, err := resume.Search(ctx, "/tmp/underscore-project", "foo_bar", 10)
	if err != nil {
		t.Fatalf("search underscore: %v", err)
	}
	if len(results) != 1 || results[0].ID != target.ID {
		t.Fatalf("expected only literal underscore match, got %+v", results)
	}
}

func TestResumeServiceSearchTreatsPercentLiterally(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)
	ctx := context.Background()

	target, _ := sessSvc.Create(ctx, "target percent")
	target.ProjectPath = "/tmp/percent-project"
	target.FirstPrompt = "literal foo%bar value"
	target.RootSessionID = target.ID
	_, _ = sessSvc.Save(ctx, target)
	_, _ = msgSvc.Create(ctx, target.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "literal foo%bar transcript"}},
	})

	noise, _ := sessSvc.Create(ctx, "noise percent")
	noise.ProjectPath = "/tmp/percent-project"
	noise.FirstPrompt = "wildcard-like fooXbar value"
	noise.RootSessionID = noise.ID
	_, _ = sessSvc.Save(ctx, noise)
	_, _ = msgSvc.Create(ctx, noise.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "fooXbar transcript"}},
	})

	results, err := resume.Search(ctx, "/tmp/percent-project", "foo%bar", 1)
	if err != nil {
		t.Fatalf("search percent: %v", err)
	}
	if len(results) != 1 || results[0].ID != target.ID {
		t.Fatalf("expected only literal percent match, got %+v", results)
	}
}

func TestResumeServiceNormalizeMessagesForResume(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)

	msgs := []message.Message{
		{
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.ToolCall{ID: "tc-1", Name: "Read", Input: `{"path":"x"}`, Finished: true},
				message.ToolCall{ID: "tc-2", Name: "Bad", Input: `not-json`, Finished: false},
			},
		},
		{
			Role:  message.Tool,
			Parts: []message.ContentPart{message.ToolResult{ToolCallID: "tc-1", Name: "Read", Content: "ok"}},
		},
	}
	n := resume.NormalizeMessagesForResume(msgs)
	if len(n) != 2 {
		t.Fatalf("expected assistant/tool pair")
	}
	if got := len(n[0].ToolCalls()); got != 1 {
		t.Fatalf("expected invalid interrupted tool call to be dropped, got %d tool calls", got)
	}
}

func TestResumeServiceNormalizeMessagesForResume_DropsUnresolvedToolPairs(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)

	msgs := []message.Message{
		{
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "checking"},
				message.ToolCall{ID: "missing", Name: "Read", Input: `{"path":"x"}`, Finished: true},
			},
		},
		{
			Role:  message.Tool,
			Parts: []message.ContentPart{message.ToolResult{ToolCallID: "orphan", Name: "Read", Content: "x"}},
		},
	}
	n := resume.NormalizeMessagesForResume(msgs)
	if len(n) != 1 {
		t.Fatalf("expected only assistant text message to remain, got %d", len(n))
	}
	if got := len(n[0].ToolCalls()); got != 0 {
		t.Fatalf("expected unresolved tool call to be dropped, got %d", got)
	}
}

func TestResumeServiceNormalizeMessagesForResume_DropsInterleavedToolResults(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)

	msgs := []message.Message{
		{
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "checking"},
				message.ToolCall{ID: "tc-1", Name: "Read", Input: `{"path":"x"}`, Finished: true},
			},
		},
		{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: "interrupt"}},
		},
		{
			Role:  message.Tool,
			Parts: []message.ContentPart{message.ToolResult{ToolCallID: "tc-1", Name: "Read", Content: "late"}},
		},
	}
	n := resume.NormalizeMessagesForResume(msgs)
	if len(n) != 2 {
		t.Fatalf("expected assistant text and user message, got %d", len(n))
	}
	if got := len(n[0].ToolCalls()); got != 0 {
		t.Fatalf("expected interleaved tool call to be dropped, got %d", got)
	}
	if n[1].Role != message.User {
		t.Fatalf("expected user message to remain second, got %s", n[1].Role)
	}
}

func TestResumeServiceResolveChildIDReturnsRoot(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)
	ctx := context.Background()

	parent, _ := sessSvc.Create(ctx, "root")
	parent.ProjectPath = "/tmp/proj-child"
	parent.RootSessionID = parent.ID
	_, _ = sessSvc.Save(ctx, parent)
	_, _ = msgSvc.Create(ctx, parent.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "root prompt"}},
	})

	child, err := sessSvc.CreateTaskSession(ctx, "child-session", parent.ID, "child")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	grandchild, err := sessSvc.CreateTaskSession(ctx, "grandchild-session", child.ID, "grandchild")
	if err != nil {
		t.Fatalf("create grandchild: %v", err)
	}
	if grandchild.RootSessionID != parent.ID {
		t.Fatalf("grandchild root_session_id = %q, want %q", grandchild.RootSessionID, parent.ID)
	}
	resolved, err := resume.Resolve(ctx, session.ResumeRequest{
		Mode:        session.ResolveID,
		Selector:    grandchild.ID,
		ProjectPath: "/tmp/proj-child",
	})
	if err != nil {
		t.Fatalf("resolve grandchild: %v", err)
	}
	if !resolved.Found || resolved.Session.ID != parent.ID {
		t.Fatalf("expected grandchild id to resolve to root, got %+v", resolved)
	}
}

func TestResumeServiceSearchFTSFindsNestedChildRoot(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessSvc := session.NewService(q)
	msgSvc := message.NewService(q)
	resume := session.NewResumeService(q, sessSvc, msgSvc)
	ctx := context.Background()

	parent, _ := sessSvc.Create(ctx, "root fts")
	parent.ProjectPath = "/tmp/proj-fts"
	parent.RootSessionID = parent.ID
	_, _ = sessSvc.Save(ctx, parent)
	child, err := sessSvc.CreateTaskSession(ctx, "fts-child", parent.ID, "child")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	grandchild, err := sessSvc.CreateTaskSession(ctx, "fts-grandchild", child.ID, "grandchild")
	if err != nil {
		t.Fatalf("create grandchild: %v", err)
	}
	_, err = msgSvc.Create(ctx, grandchild.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "nested zephyr transcript hit"}},
	})
	if err != nil {
		t.Fatalf("create grandchild message: %v", err)
	}

	results, err := resume.Search(ctx, "/tmp/proj-fts", "zephyr", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 || results[0].ID != parent.ID {
		t.Fatalf("expected nested child FTS hit to resolve root, got %+v", results)
	}
}
