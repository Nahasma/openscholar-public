package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/llm/provider"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

type finalizationProvider struct {
	streamToolCalls []message.ToolCall
	finalToolsLen   int
	finalCalled     bool
}

func (p *finalizationProvider) SendMessages(_ context.Context, _ []message.Message, ts []tools.BaseTool) (*provider.ProviderResponse, error) {
	p.finalCalled = true
	p.finalToolsLen = len(ts)
	return &provider.ProviderResponse{
		Content:      "Final answer from existing evidence.",
		Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 5},
		FinishReason: message.FinishReasonEndTurn,
	}, nil
}

func (p *finalizationProvider) StreamResponse(_ context.Context, _ []message.Message, _ []tools.BaseTool) <-chan provider.ProviderEvent {
	ch := make(chan provider.ProviderEvent, 3)
	go func() {
		defer close(ch)
		ch <- provider.ProviderEvent{Type: provider.EventContentStart}
		ch <- provider.ProviderEvent{Type: provider.EventContentDelta, Content: "Searching."}
		ch <- provider.ProviderEvent{
			Type: provider.EventComplete,
			Response: &provider.ProviderResponse{
				Content:      "Searching.",
				ToolCalls:    p.streamToolCalls,
				Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 5},
				FinishReason: message.FinishReasonToolUse,
			},
		}
	}()
	return ch
}

func (p *finalizationProvider) CountTokens(context.Context, []message.Message, []tools.BaseTool) (provider.TokenCount, error) {
	return provider.TokenCount{InputTokens: 10}, nil
}

func (p *finalizationProvider) Model() models.Model {
	return models.Model{ID: "test-model"}
}

func TestProcessGeneration_MaxToolCallsFinalizesWithoutTools(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	sess := testutil.CreateTestSession(t, sessions)

	calls := make([]message.ToolCall, 0, defaultMaxToolCalls)
	for i := 0; i < defaultMaxToolCalls; i++ {
		calls = append(calls, message.ToolCall{
			ID:    fmt.Sprintf("tc-finalize-%d", i),
			Name:  "WebSearch",
			Input: `{"query":"same"}`,
		})
	}
	p := &finalizationProvider{streamToolCalls: calls}
	ag := NewAgentForTest(p, sessions, messages, []tools.BaseTool{&countingNamedTool{name: "WebSearch"}})

	eventCh, err := ag.Run(context.Background(), sess.ID, "search until budget")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	event := <-eventCh
	if !event.Done || event.TerminalReason != ReasonMaxTurnsReached {
		t.Fatalf("expected max-turn terminal event, got %#v", event)
	}
	if !p.finalCalled || p.finalToolsLen != 0 {
		t.Fatalf("expected tool-free finalization call, called=%v tools=%d", p.finalCalled, p.finalToolsLen)
	}
	if len(event.Message.ToolCalls()) != 0 || event.Message.Content().Text != "Final answer from existing evidence." {
		t.Fatalf("expected final text message without tool calls, got parts=%#v", event.Message.Parts)
	}
}

type convergenceProvider struct {
	streamCalls   int
	finalToolsLen int
	finalCalled   bool
}

func (p *convergenceProvider) SendMessages(_ context.Context, _ []message.Message, ts []tools.BaseTool) (*provider.ProviderResponse, error) {
	p.finalCalled = true
	p.finalToolsLen = len(ts)
	return &provider.ProviderResponse{
		Content:      "Final answer after convergence.",
		Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 5},
		FinishReason: message.FinishReasonEndTurn,
	}, nil
}

func (p *convergenceProvider) StreamResponse(_ context.Context, _ []message.Message, _ []tools.BaseTool) <-chan provider.ProviderEvent {
	p.streamCalls++
	callNo := p.streamCalls
	ch := make(chan provider.ProviderEvent, 3)
	go func() {
		defer close(ch)
		ch <- provider.ProviderEvent{Type: provider.EventContentStart}
		ch <- provider.ProviderEvent{Type: provider.EventContentDelta, Content: "Fetching."}
		ch <- provider.ProviderEvent{
			Type: provider.EventComplete,
			Response: &provider.ProviderResponse{
				Content: "Fetching.",
				ToolCalls: []message.ToolCall{{
					ID:    fmt.Sprintf("fetch-%d", callNo),
					Name:  "WebFetch",
					Input: fmt.Sprintf(`{"url":"https://papers.nips.cc/paper_files/paper/2024","prompt":"continue from %d"}`, callNo),
				}},
				Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 5},
				FinishReason: message.FinishReasonToolUse,
			},
		}
	}()
	return ch
}

func (p *convergenceProvider) CountTokens(context.Context, []message.Message, []tools.BaseTool) (provider.TokenCount, error) {
	return provider.TokenCount{InputTokens: 10}, nil
}

func (p *convergenceProvider) Model() models.Model {
	return models.Model{ID: "test-model"}
}

type convergenceFetchTool struct{}

func (t convergenceFetchTool) Info() tools.ToolInfo { return tools.ToolInfo{Name: "WebFetch"} }
func (t convergenceFetchTool) Run(_ context.Context, _ tools.ToolCall) (tools.ToolResponse, error) {
	md, _ := json.Marshal(map[string]any{
		"tool":              "WebFetch",
		"provider":          "http",
		"source":            "https://papers.nips.cc/paper_files/paper/2024",
		"progress_kind":     "fetched_page",
		"target_key":        "webfetch:https://papers.nips.cc/paper_files/paper/2024",
		"canonical_url":     "https://papers.nips.cc/paper_files/paper/2024",
		"content_class":     "list_page",
		"prompt_class":      "continue_same_target",
		"evidence_keys":     []string{"url:https://papers.nips.cc/paper_files/paper/2024"},
		"outcome_hash":      "sha256:same",
		"low_value_reason":  "repeated_target",
		"durable_progress":  false,
		"candidate_count":   0,
		"hit_count":         0,
		"public_summary":    "large list page",
		"result_byte_count": 1024,
	})
	return tools.ToolResponse{Content: "Fetched list page.", Metadata: string(md)}, nil
}

func TestProcessGeneration_SearchConvergenceFinalizesWithoutTools(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	sess := testutil.CreateTestSession(t, sessions)

	p := &convergenceProvider{}
	ag := NewAgentForTest(p, sessions, messages, []tools.BaseTool{convergenceFetchTool{}})

	eventCh, err := ag.Run(context.Background(), sess.ID, "read NeurIPS list repeatedly")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	event := <-eventCh
	if !event.Done || event.TerminalReason != ReasonLoopHookStopped {
		t.Fatalf("expected convergence terminal event, got %#v", event)
	}
	if p.streamCalls >= 16 {
		t.Fatalf("expected convergence before 16 tool loops, got %d", p.streamCalls)
	}
	if !p.finalCalled || p.finalToolsLen != 0 {
		t.Fatalf("expected tool-free finalization call, called=%v tools=%d", p.finalCalled, p.finalToolsLen)
	}
	if len(event.Message.ToolCalls()) != 0 || event.Message.Content().Text != "Final answer after convergence." {
		t.Fatalf("expected final text message without tool calls, got parts=%#v", event.Message.Parts)
	}
}
