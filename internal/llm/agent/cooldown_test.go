package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/hooks"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/testutil"
)

type countingScholarTool struct {
	calls int
}

func (t *countingScholarTool) Info() tools.ToolInfo { return tools.ToolInfo{Name: "ScholarSearch"} }
func (t *countingScholarTool) Run(_ context.Context, call tools.ToolCall) (tools.ToolResponse, error) {
	t.calls++
	if t.calls == 1 {
		md := map[string]any{
			"error_kind": "rate_limited", "tool": "ScholarSearch", "provider": "semantic_scholar", "source": "semantic_scholar",
			"recoverable": true, "action": "search", "query_key": "search:semantic_scholar:q1:0:5", "suggested_sources": []string{"openalex", "arxiv", "crossref"},
		}
		return tools.WithResponseMetadata(tools.NewTextErrorResponse("limited"), md), nil
	}
	return tools.NewTextResponse("ok"), nil
}

type sourceCooldownScholarTool struct {
	calls int
}

func (t *sourceCooldownScholarTool) Info() tools.ToolInfo {
	return tools.ToolInfo{Name: "ScholarSearch"}
}
func (t *sourceCooldownScholarTool) Run(_ context.Context, call tools.ToolCall) (tools.ToolResponse, error) {
	t.calls++
	source, action := parseScholarInput(call.Input)
	if source == "" {
		source = "semantic_scholar"
	}
	if action == "" {
		action = "search"
	}
	md := map[string]any{
		"error_kind":  "rate_limited",
		"tool":        "ScholarSearch",
		"provider":    source,
		"source":      source,
		"recoverable": true,
		"action":      action,
		"query_key":   "search:" + source + ":q:0:5",
	}
	return tools.WithResponseMetadata(tools.NewTextErrorResponse("limited"), md), nil
}

type countingNamedTool struct {
	name  string
	calls int
}

func (t *countingNamedTool) Info() tools.ToolInfo { return tools.ToolInfo{Name: t.name} }
func (t *countingNamedTool) Run(_ context.Context, _ tools.ToolCall) (tools.ToolResponse, error) {
	t.calls++
	return tools.NewTextResponse("ok"), nil
}

func newAgentForExecuteToolCallsTest(t *testing.T, toolList []tools.BaseTool) (*agent, session.Service, message.Service, string) {
	t.Helper()
	_, q := testutil.SetupTestDB(t)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	sess := testutil.CreateTestSession(t, sessions)
	ag := &agent{
		Broker:   pubsub.NewBroker[AgentEvent](),
		messages: messages,
		tools:    toolList,
	}
	return ag, sessions, messages, sess.ID
}

func TestExecuteToolCalls_SkipsExactDuplicateInSameTurn(t *testing.T) {
	tool := &countingNamedTool{name: "WebSearch"}
	ag, _, messages, sessID := newAgentForExecuteToolCallsTest(t, []tools.BaseTool{tool})
	assistantMsg, _ := messages.Create(context.Background(), sessID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "WebSearch", Input: `{"query":"same","allowed_domains":["a.com"]}`},
			message.ToolCall{ID: "tc2", Name: "WebSearch", Input: `{"allowed_domains":["a.com"],"query":"same"}`},
		},
	})
	_, toolMsg, err := ag.executeToolCalls(context.Background(), assistantMsg)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if tool.calls != 1 {
		t.Fatalf("expected duplicate to be skipped, got calls=%d", tool.calls)
	}
	res := toolMsg.ToolResults()
	if len(res) != 2 || !res[1].IsError || !strings.Contains(res[1].Metadata, "duplicate_tool_call") {
		t.Fatalf("expected duplicate synthetic result, got %#v", res)
	}
}

func TestExecuteToolCalls_SkipsWhenExecutionBudgetExhausted(t *testing.T) {
	tool := &countingNamedTool{name: "WebSearch"}
	ag, _, messages, sessID := newAgentForExecuteToolCallsTest(t, []tools.BaseTool{tool})
	assistantMsg, _ := messages.Create(context.Background(), sessID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "WebSearch", Input: `{"query":"one"}`},
			message.ToolCall{ID: "tc2", Name: "WebSearch", Input: `{"query":"two"}`},
		},
	})
	ctx := context.WithValue(context.Background(), toolExecutionBudgetKey, toolExecutionBudget{RemainingToolCalls: 1, RemainingSearchCalls: 1})
	_, toolMsg, err := ag.executeToolCalls(ctx, assistantMsg)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if tool.calls != 1 {
		t.Fatalf("expected only one call within budget, got calls=%d", tool.calls)
	}
	res := toolMsg.ToolResults()
	if len(res) != 2 || !res[1].IsError || !strings.Contains(res[1].Metadata, "tool_budget_exceeded") {
		t.Fatalf("expected budget synthetic result, got %#v", res)
	}
}

func TestExecuteToolCalls_KBSearchNoLongerHardBlockedByKeywordGuard(t *testing.T) {
	tool := &countingNamedTool{name: "KBSearch"}
	ag, _, messages, sessID := newAgentForExecuteToolCallsTest(t, []tools.BaseTool{tool})
	_, _ = messages.Create(context.Background(), sessID, message.CreateMessageParams{
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "llama的核心公式介绍一下"},
		},
	})
	assistantMsg, _ := messages.Create(context.Background(), sessID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "KBSearch", Input: `{"question":"LLaMA model architecture formulas SwiGLU RoPE RMSNorm"}`},
		},
	})
	_, _, err := ag.executeToolCalls(context.Background(), assistantMsg)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if tool.calls != 1 {
		t.Fatalf("expected KBSearch to execute without hard keyword skip, got calls=%d", tool.calls)
	}
}

func TestExecuteToolCalls_ScholarCooldownSkipsSameBatchSemantic(t *testing.T) {
	tool := &countingScholarTool{}
	ag, _, messages, sessID := newAgentForExecuteToolCallsTest(t, []tools.BaseTool{tool})
	assistantMsg, _ := messages.Create(context.Background(), sessID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"search","query":"q1"}`},
			message.ToolCall{ID: "tc2", Name: "ScholarSearch", Input: `{"action":"search","query":"q2"}`},
		},
	})
	_, toolMsg, err := ag.executeToolCalls(context.Background(), assistantMsg)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if tool.calls != 1 {
		t.Fatalf("expected only first call to execute, got %d", tool.calls)
	}
	res := toolMsg.ToolResults()
	if len(res) != 2 || !res[1].IsError || !strings.Contains(res[1].Content, "Skipped: Semantic Scholar provider cooldown") {
		t.Fatalf("unexpected second result: %#v", res)
	}
	if !strings.Contains(res[1].Metadata, "provider_cooldown") {
		t.Fatalf("expected provider_cooldown metadata on skipped call")
	}
}

func TestExecuteToolCalls_ScholarCooldownSkipFiresPolicyHooks(t *testing.T) {
	tool := &countingScholarTool{}
	ag, _, messages, sessID := newAgentForExecuteToolCallsTest(t, []tools.BaseTool{tool})
	recorder := &recordingHookService{}
	ag.hookService = recorder
	assistantMsg, _ := messages.Create(context.Background(), sessID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"search","query":"q1"}`},
			message.ToolCall{ID: "tc2", Name: "ScholarSearch", Input: `{"action":"search","query":"q2"}`},
		},
	})

	_, _, err := ag.executeToolCalls(context.Background(), assistantMsg)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}

	var sawFailure, sawCooldown bool
	for _, evt := range recorder.snapshot() {
		if evt.input.ErrorKind != "provider_cooldown" {
			continue
		}
		switch evt.event {
		case hooks.PostToolUseFailure:
			sawFailure = true
		case hooks.ProviderCooldown:
			sawCooldown = true
		}
	}
	if !sawFailure || !sawCooldown {
		t.Fatalf("expected skipped cooldown result to fire failure and provider hooks, got %#v", recorder.snapshot())
	}
}

func TestExecuteToolCalls_ScholarCooldownDoesNotSkipExplicitOpenAlex(t *testing.T) {
	tool := &countingScholarTool{}
	ag, _, messages, sessID := newAgentForExecuteToolCallsTest(t, []tools.BaseTool{tool})
	assistantMsg, _ := messages.Create(context.Background(), sessID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"search","query":"q1"}`},
			message.ToolCall{ID: "tc2", Name: "ScholarSearch", Input: `{"action":"search","source":"openalex","query":"q2"}`},
		},
	})
	_, _, err := ag.executeToolCalls(context.Background(), assistantMsg)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if tool.calls != 2 {
		t.Fatalf("expected openalex call to execute, got calls=%d", tool.calls)
	}
}

func TestExecuteToolCalls_ScholarCooldownSkipsSameBatchExplicitArxiv(t *testing.T) {
	tool := &sourceCooldownScholarTool{}
	ag, _, messages, sessID := newAgentForExecuteToolCallsTest(t, []tools.BaseTool{tool})
	assistantMsg, _ := messages.Create(context.Background(), sessID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"search","source":"arxiv","query":"q1"}`},
			message.ToolCall{ID: "tc2", Name: "ScholarSearch", Input: `{"action":"search","source":"arxiv","query":"q2"}`},
		},
	})
	_, toolMsg, err := ag.executeToolCalls(context.Background(), assistantMsg)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if tool.calls != 1 {
		t.Fatalf("expected only first arxiv call to execute, got %d", tool.calls)
	}
	res := toolMsg.ToolResults()
	if len(res) != 2 || !res[1].IsError || !strings.Contains(res[1].Content, "arxiv provider cooldown") {
		t.Fatalf("unexpected second result: %#v", res)
	}
	if !strings.Contains(res[1].Metadata, `"source":"arxiv"`) || !strings.Contains(res[1].Metadata, "provider_cooldown") {
		t.Fatalf("expected arxiv provider_cooldown metadata, got %s", res[1].Metadata)
	}
	var md map[string]any
	if err := json.Unmarshal([]byte(res[1].Metadata), &md); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	for _, source := range md["suggested_sources"].([]any) {
		if source == "arxiv" {
			t.Fatalf("suggested sources should exclude cooled-down arxiv provider, got %s", res[1].Metadata)
		}
	}
}

func TestExecuteToolCalls_ScholarCooldownSkipsSemanticBoundNonSearchAction(t *testing.T) {
	tool := &countingScholarTool{}
	ag, _, messages, sessID := newAgentForExecuteToolCallsTest(t, []tools.BaseTool{tool})
	assistantMsg, _ := messages.Create(context.Background(), sessID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"search","query":"q1"}`},
			message.ToolCall{ID: "tc2", Name: "ScholarSearch", Input: `{"action":"details","source":"openalex","id":"p1"}`},
		},
	})
	_, toolMsg, err := ag.executeToolCalls(context.Background(), assistantMsg)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if tool.calls != 1 {
		t.Fatalf("expected details call to be skipped during semantic cooldown, got calls=%d", tool.calls)
	}
	res := toolMsg.ToolResults()
	if len(res) != 2 || !strings.Contains(res[1].Metadata, "provider_cooldown") {
		t.Fatalf("expected skipped details result with cooldown metadata, got %#v", res)
	}
}

func TestScholarToolCallParallelSafeOnlyExplicitNonSemanticSearch(t *testing.T) {
	tests := []struct {
		name string
		call message.ToolCall
		want bool
	}{
		{
			name: "default source preserves semantic cooldown ordering",
			call: message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"search","query":"q"}`},
			want: false,
		},
		{
			name: "semantic source preserves cooldown ordering",
			call: message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"search","source":"semantic_scholar","query":"q"}`},
			want: false,
		},
		{
			name: "details action is not parallelized",
			call: message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"details","source":"openalex","id":"p"}`},
			want: false,
		},
		{
			name: "explicit non semantic search can run in parallel",
			call: message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"search","source":"openalex","query":"q"}`},
			want: true,
		},
		{
			name: "arxiv search preserves provider cooldown ordering",
			call: message.ToolCall{ID: "tc1", Name: "ScholarSearch", Input: `{"action":"search","source":"arxiv","query":"q"}`},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scholarToolCallParallelSafe(tt.call); got != tt.want {
				t.Fatalf("scholarToolCallParallelSafe() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviderCooldownLoopHook_StopsAcrossDifferentQueries(t *testing.T) {
	h := NewProviderCooldownLoopHook(2)
	state := &LoopState{
		LastRoundToolOutcomes: []ToolOutcome{{ToolName: "ScholarSearch", IsError: true, ErrorKind: "rate_limited", Provider: "semantic_scholar", Source: "semantic_scholar", QueryKey: "search:semantic_scholar:q1:0:5"}},
	}
	d1 := h.Check(context.Background(), state)
	if d1.Action != ActionContinue {
		t.Fatalf("first cooldown should continue")
	}
	state.LastRoundToolOutcomes = []ToolOutcome{{ToolName: "ScholarSearch", IsError: true, ErrorKind: "provider_cooldown", Provider: "semantic_scholar", Source: "semantic_scholar", QueryKey: "search:semantic_scholar:q2:0:5"}}
	d2 := h.Check(context.Background(), state)
	if d2.Action != ActionTerminate || d2.Code != StopCodeProviderCooldown {
		t.Fatalf("expected provider cooldown terminate, got %#v", d2)
	}
}

func TestProviderCooldownLoopHook_DoesNotLeakAcrossLoopStates(t *testing.T) {
	h := NewProviderCooldownLoopHook(2)
	first := &LoopState{
		LastRoundToolOutcomes: []ToolOutcome{{ToolName: "ScholarSearch", IsError: true, ErrorKind: "rate_limited", Provider: "semantic_scholar", Source: "semantic_scholar"}},
	}
	if d := h.Check(context.Background(), first); d.Action != ActionContinue {
		t.Fatalf("first generation first cooldown should continue, got %#v", d)
	}
	nextGeneration := &LoopState{
		LastRoundToolOutcomes: []ToolOutcome{{ToolName: "ScholarSearch", IsError: true, ErrorKind: "provider_cooldown", Provider: "semantic_scholar", Source: "semantic_scholar"}},
	}
	if d := h.Check(context.Background(), nextGeneration); d.Action != ActionContinue {
		t.Fatalf("new generation first cooldown should not inherit prior count, got %#v", d)
	}
}

func TestSemanticCooldownSignature_StableAcrossQueryKeyVariants(t *testing.T) {
	tc := message.ToolCall{ID: "tc1", Name: "ScholarSearch"}
	out1 := []ToolOutcome{{ToolCallID: "tc1", ToolName: "ScholarSearch", ErrorKind: "provider_cooldown", Provider: "semantic_scholar", Source: "semantic_scholar", Action: "search", QueryKey: "search:semantic_scholar:q_1:0:5"}}
	out2 := []ToolOutcome{{ToolCallID: "tc1", ToolName: "ScholarSearch", ErrorKind: "provider_cooldown", Provider: "semantic_scholar", Source: "semantic_scholar", Action: "search", QueryKey: "search:semantic_scholar:q_2:0:5"}}
	s1 := semanticToolSignature(tc, out1)
	s2 := semanticToolSignature(tc, out2)
	if s1 == "" || s1 != s2 {
		t.Fatalf("expected stable semantic cooldown signature, got %q vs %q", s1, s2)
	}
}

func TestSemanticToolSignature_WebSearchIncludesAllowedAndBlockedDomains(t *testing.T) {
	tc1 := message.ToolCall{ID: "tc1", Name: "WebSearch", Input: `{"query":" Multi   Agent ","allowed_domains":["B.com","a.com"]}`}
	tc2 := message.ToolCall{ID: "tc2", Name: "WebSearch", Input: `{"query":"multi agent","allowed_domains":["a.com","b.com"]}`}
	tc3 := message.ToolCall{ID: "tc3", Name: "WebSearch", Input: `{"query":"multi agent","blocked_domains":["a.com"]}`}
	s1 := semanticToolSignature(tc1, []ToolOutcome{{ToolCallID: "tc1", ToolName: "WebSearch"}})
	s2 := semanticToolSignature(tc2, []ToolOutcome{{ToolCallID: "tc2", ToolName: "WebSearch"}})
	s3 := semanticToolSignature(tc3, []ToolOutcome{{ToolCallID: "tc3", ToolName: "WebSearch"}})
	if s1 == "" || s1 != s2 {
		t.Fatalf("expected equivalent allowed-domain signatures, got %q vs %q", s1, s2)
	}
	if s1 == s3 {
		t.Fatalf("blocked domains should affect signature: %q", s1)
	}
}

func TestScholarCooldownSkipMetadataJSON(t *testing.T) {
	raw := scholarCooldownSkipMetadata(`{"action":"citations","query":"q","source":"semantic_scholar"}`)
	var md map[string]any
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		t.Fatalf("invalid json metadata: %v", err)
	}
	if md["error_kind"] != "provider_cooldown" || md["provider"] != "semantic_scholar" {
		t.Fatalf("unexpected metadata: %v", md)
	}
}
