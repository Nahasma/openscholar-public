package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nahasma/openscholar-public/internal/memory"
	"github.com/Nahasma/openscholar-public/internal/message"
)

type fakeMemoryService struct {
	sessionItems []memory.MemoryItem
	globalItems  []memory.MemoryItem

	executeCh        chan struct{}
	lastExecuteSID   string
	lastExecuteText  string
	lastCapturedKB   memory.KBCapture
	capturedKBCount  int
	retrievePromptOK bool
}

func (f *fakeMemoryService) Insert(ctx context.Context, item memory.MemoryItem) error {
	return nil
}

func (f *fakeMemoryService) Update(ctx context.Context, id string, content string, metadata map[string]any) error {
	return nil
}

func (f *fakeMemoryService) Delete(ctx context.Context, id string) error {
	return nil
}

func (f *fakeMemoryService) RetrieveForSession(ctx context.Context, sessionID string, limit int) ([]memory.MemoryItem, error) {
	return nil, nil
}

func (f *fakeMemoryService) RetrieveRecent(ctx context.Context, limit int) ([]memory.MemoryItem, error) {
	return nil, nil
}

func (f *fakeMemoryService) RetrieveForPrompt(ctx context.Context, sessionID string, opts memory.PromptMemoryOptions) ([]memory.MemoryItem, []memory.MemoryItem, error) {
	return f.sessionItems, f.globalItems, nil
}

func (f *fakeMemoryService) ExecuteSkills(ctx context.Context, sessionID string, sessionText string) error {
	f.lastExecuteSID = sessionID
	f.lastExecuteText = sessionText
	if f.executeCh != nil {
		f.executeCh <- struct{}{}
	}
	return nil
}

func (f *fakeMemoryService) CaptureKB(ctx context.Context, capture memory.KBCapture) error {
	f.lastCapturedKB = capture
	f.capturedKBCount++
	return nil
}

func (f *fakeMemoryService) SetLLMCaller(caller memory.LLMCaller) {}

func TestTriggerMemoryCapture_InvokesExecuteSkills(t *testing.T) {
	mem := &fakeMemoryService{executeCh: make(chan struct{}, 1)}
	ag := &agent{memoryService: mem}

	ag.triggerMemoryCapture("sess-1", "remember this turn")

	select {
	case <-mem.executeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ExecuteSkills to be called")
	}

	if mem.lastExecuteSID != "sess-1" {
		t.Fatalf("sessionID = %q, want %q", mem.lastExecuteSID, "sess-1")
	}
	if mem.lastExecuteText != "remember this turn" {
		t.Fatalf("sessionText = %q, want %q", mem.lastExecuteText, "remember this turn")
	}
}

func TestBuildRequestView_IncludesFencedMemoryContext(t *testing.T) {
	now := time.Now()
	mem := &fakeMemoryService{
		sessionItems: []memory.MemoryItem{{
			Content:   "session scoped memory",
			CreatedAt: now,
		}},
		globalItems: []memory.MemoryItem{{
			Content:   "global memory",
			CreatedAt: now,
		}},
	}
	ag := &agent{memoryService: mem}

	view := ag.buildRequestView(context.Background(), "sess-1", []message.Message{
		{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: "hello"}},
		},
	})

	if len(view.Messages) == 0 {
		t.Fatal("expected request view messages")
	}

	firstText, ok := view.Messages[0].Parts[0].(message.TextContent)
	if !ok {
		t.Fatalf("expected first injected part to be text, got %T", view.Messages[0].Parts[0])
	}
	if firstText.Text == "" {
		t.Fatal("expected non-empty memory context")
	}
	if got := firstText.Text; !strings.HasPrefix(got, "<memory-context>") {
		t.Fatalf("expected fenced memory context, got %q", got)
	}
}
