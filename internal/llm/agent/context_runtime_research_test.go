package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/provider"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/testutil"
)

type stubTool struct {
	name string
}

func (s stubTool) Info() tools.ToolInfo {
	return tools.ToolInfo{Name: s.name}
}

func (s stubTool) Run(ctx context.Context, call tools.ToolCall) (tools.ToolResponse, error) {
	return tools.NewTextResponse("ok"), nil
}

type fixedCountProvider struct {
	model      models.Model
	tokenCount int64
	countErr   error
}

func (p fixedCountProvider) SendMessages(context.Context, []message.Message, []tools.BaseTool) (*provider.ProviderResponse, error) {
	return nil, nil
}

func (p fixedCountProvider) StreamResponse(context.Context, []message.Message, []tools.BaseTool) <-chan provider.ProviderEvent {
	ch := make(chan provider.ProviderEvent)
	close(ch)
	return ch
}

func (p fixedCountProvider) CountTokens(context.Context, []message.Message, []tools.BaseTool) (provider.TokenCount, error) {
	if p.countErr != nil {
		return provider.TokenCount{}, p.countErr
	}
	return provider.TokenCount{InputTokens: p.tokenCount}, nil
}

func (p fixedCountProvider) Model() models.Model {
	return p.model
}

func TestBuildRequestView_ResearchModeActivatesDefaultResearchTools(t *testing.T) {
	reg := tools.NewDeferredRegistry(
		[]tools.BaseTool{stubTool{name: "View"}},
		[]tools.BaseTool{
			stubTool{name: "Task"},
			stubTool{name: "ResearchPipeline"},
			stubTool{name: "ResearchTask"},
			stubTool{name: "ResearchMessage"},
		},
	)
	ag := &agent{registry: reg}

	ctx := context.WithValue(context.Background(), tools.ResearchModeContextKey, true)
	view := ag.buildRequestView(ctx, "sess-1", []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "run research"}}},
	})

	seen := map[string]bool{}
	for _, tool := range view.ActiveTools {
		seen[tool.Info().Name] = true
	}

	for _, name := range []string{"Task", "ResearchPipeline", "ResearchTask", "ResearchMessage"} {
		if !seen[name] {
			t.Fatalf("expected %s in active research-mode tools", name)
		}
	}
}

func TestBuildRequestView_ResearchModeActivationDoesNotLeakToNonResearch(t *testing.T) {
	reg := tools.NewDeferredRegistry(
		[]tools.BaseTool{stubTool{name: "View"}},
		[]tools.BaseTool{
			stubTool{name: "Task"},
			stubTool{name: "ResearchPipeline"},
			stubTool{name: "ResearchTask"},
			stubTool{name: "ResearchMessage"},
		},
	)
	ag := &agent{registry: reg}

	researchCtx := context.WithValue(context.Background(), tools.ResearchModeContextKey, true)
	ag.buildRequestView(researchCtx, "sess-research", nil)

	view := ag.buildRequestView(context.Background(), "sess-default", nil)
	seen := map[string]bool{}
	for _, tool := range view.ActiveTools {
		seen[tool.Info().Name] = true
	}

	for _, name := range []string{"Task", "ResearchPipeline", "ResearchTask", "ResearchMessage"} {
		if seen[name] {
			t.Fatalf("did not expect %s to leak into non-research active tools", name)
		}
	}
}

func TestBuildRequestView_ResearchLeaderModeExcludesWriteTools(t *testing.T) {
	reg := tools.NewDeferredRegistry(
		[]tools.BaseTool{
			stubTool{name: "View"},
			stubTool{name: "Edit"},
			stubTool{name: "Write"},
			stubTool{name: "CodeAgent"},
			stubTool{name: "DocxEdit"},
			stubTool{name: "DocxPatch"},
		},
		[]tools.BaseTool{stubTool{name: "Task"}, stubTool{name: "ResearchPipeline"}},
	)
	ag := &agent{registry: reg}

	ctx := context.WithValue(context.Background(), tools.ResearchModeContextKey, true)
	ctx = context.WithValue(ctx, tools.ResearchToolProfileContextKey, tools.ResearchToolProfileLeader)
	view := ag.buildRequestView(ctx, "sess-1", nil)

	seen := map[string]bool{}
	for _, tool := range view.ActiveTools {
		seen[tool.Info().Name] = true
	}
	if seen["Edit"] || seen["Write"] || seen["CodeAgent"] || seen["DocxEdit"] || seen["DocxPatch"] {
		t.Fatalf("research leader mode should not expose write tools: %#v", seen)
	}
	if !seen["Task"] || !seen["ResearchPipeline"] {
		t.Fatalf("research leader mode should keep orchestration tools: %#v", seen)
	}
}

func TestBuildRequestView_ResearchWorkerModeKeepsWriteTools(t *testing.T) {
	reg := tools.NewDeferredRegistry(
		[]tools.BaseTool{
			stubTool{name: "View"},
			stubTool{name: "Edit"},
			stubTool{name: "Write"},
			stubTool{name: "CodeAgent"},
		},
		[]tools.BaseTool{stubTool{name: "Task"}, stubTool{name: "ResearchPipeline"}},
	)
	ag := &agent{registry: reg}

	ctx := context.WithValue(context.Background(), tools.ResearchModeContextKey, true)
	ctx = context.WithValue(ctx, tools.ResearchToolProfileContextKey, tools.ResearchToolProfileWorker)
	view := ag.buildRequestView(ctx, "sess-1", nil)

	seen := map[string]bool{}
	for _, tool := range view.ActiveTools {
		seen[tool.Info().Name] = true
	}
	for _, name := range []string{"View", "Edit", "Write", "CodeAgent"} {
		if !seen[name] {
			t.Fatalf("research worker mode should keep %s: %#v", name, seen)
		}
	}
}

func TestBuildRequestView_RuntimeAllowedToolsFilter(t *testing.T) {
	reg := tools.NewDeferredRegistry(
		[]tools.BaseTool{stubTool{name: "View"}, stubTool{name: "Bash"}},
		[]tools.BaseTool{stubTool{name: "WebSearch"}},
	)
	ag := &agent{registry: reg}

	ctx := WithRequestRuntime(context.Background(), RequestRuntime{
		AllowedTools: []string{"View", "WebSearch"},
	})
	view := ag.buildRequestView(ctx, "sess-1", nil)

	seen := map[string]bool{}
	for _, tool := range view.ActiveTools {
		seen[tool.Info().Name] = true
	}
	if !seen["View"] || !seen["WebSearch"] {
		t.Fatalf("expected View and WebSearch in active tools: %#v", seen)
	}
	if seen["Bash"] || seen["ToolSearch"] {
		t.Fatalf("unexpected tool leaked through runtime filter: %#v", seen)
	}
}

func TestBuildRequestView_RuntimeAllowedToolsFiltersIntentGatedKBTools(t *testing.T) {
	kbTools := []string{"KBAdd", "KBList", "KBTree", "KBQuery", "KBSearch", "KBHealth", "KBRepair", "KBReindex"}
	deferred := []tools.BaseTool{stubTool{name: "WebSearch"}}
	for _, name := range kbTools {
		deferred = append(deferred, stubTool{name: name})
	}
	reg := tools.NewDeferredRegistry(
		[]tools.BaseTool{stubTool{name: "View"}},
		deferred,
	)
	ag := &agent{registry: reg}

	ctx := WithRequestRuntime(context.Background(), RequestRuntime{
		AllowedTools: append([]string{"View", "WebSearch"}, kbTools...),
	})
	view := ag.buildRequestView(ctx, "sess-1", nil)

	seen := map[string]bool{}
	for _, tool := range view.ActiveTools {
		seen[tool.Info().Name] = true
	}
	for _, name := range kbTools {
		if seen[name] {
			t.Fatalf("runtime allowed tools should not expose %s without local KB intent: %#v", name, seen)
		}
	}
	if !seen["View"] || !seen["WebSearch"] {
		t.Fatalf("expected non-gated tools to remain: %#v", seen)
	}

	localCtx := tools.WithLocalKBIntent(ctx, true)
	localView := ag.buildRequestView(localCtx, "sess-1", nil)
	localSeen := map[string]bool{}
	for _, tool := range localView.ActiveTools {
		localSeen[tool.Info().Name] = true
	}
	for _, name := range kbTools {
		if !localSeen[name] {
			t.Fatalf("runtime allowed tools should expose %s with local KB intent: %#v", name, localSeen)
		}
	}
}

func TestBuildRequestView_IncludesRuntimeSystemPrompt(t *testing.T) {
	ag := &agent{agentName: config.AgentCoder}
	view := ag.buildRequestView(context.Background(), "sess-1", []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hello"}}},
	})
	if strings.TrimSpace(view.SystemMessage) == "" {
		t.Fatal("expected system message to be populated")
	}
	if len(view.SystemBlocks) == 0 {
		t.Fatal("expected system blocks to be populated")
	}
	if !strings.Contains(view.SystemMessage, "# Runtime Context") {
		t.Fatalf("expected runtime context in request system message, got: %s", view.SystemMessage)
	}
}

func TestPreflightRequestView_PreservesInjectedPrefixDuringCompaction(t *testing.T) {
	ag := &agent{
		agentProvider: fixedCountProvider{
			model:      models.Model{ID: "tiny", ContextWindow: 120, DefaultMaxTokens: 20},
			tokenCount: 95,
		},
	}

	prefix := message.Message{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "<session-memory>\nkeep me\n</session-memory>"}},
	}
	history := []message.Message{prefix}
	for i := 0; i < 8; i++ {
		history = append(history,
			message.Message{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "old user"}}},
			message.Message{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "old assistant"}}},
		)
	}

	view := RequestView{Messages: history, PrefixMessageCount: 1}
	qcfg := QueryConfig{
		EffectiveContextWindow: 100,
		AutoCompactThreshold:   80,
		Compact: CompactLimits{
			ReserveOutputTokens:     20,
			WarningBufferTokens:     20,
			AutoCompactBufferTokens: 20,
		},
	}

	preflight := ag.preflightRequestView(context.Background(), view, qcfg, false)
	got := preflight.Messages
	if len(got) >= len(history) {
		t.Fatalf("expected history to be compacted, got %d messages from %d", len(got), len(history))
	}
	if got[0].Content().Text != prefix.Content().Text {
		t.Fatalf("expected injected prefix to remain pinned, got %q", got[0].Content().Text)
	}
}

func TestPreflightRequestView_ZeroAutoThresholdDoesNotCompactBelowHardStop(t *testing.T) {
	ag := &agent{
		agentProvider: fixedCountProvider{
			model:      models.Model{ID: "small", ContextWindow: 4096, DefaultMaxTokens: 1024},
			tokenCount: 100,
		},
	}

	var history []message.Message
	for i := 0; i < 8; i++ {
		history = append(history,
			message.Message{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "user"}}},
			message.Message{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "assistant"}}},
		)
	}

	view := RequestView{Messages: history}
	qcfg := QueryConfig{
		EffectiveContextWindow: 1000,
		AutoCompactThreshold:   0,
		Compact: CompactLimits{
			ReserveOutputTokens:     20,
			WarningBufferTokens:     20,
			AutoCompactBufferTokens: 20,
		},
	}

	preflight := ag.preflightRequestView(context.Background(), view, qcfg, false)
	got := preflight.Messages
	if len(got) != len(history) {
		t.Fatalf("expected no compaction below hard stop when AutoCompactAt is zero, got %d messages from %d", len(got), len(history))
	}
}

func TestPreflightRequestView_ReportsCompactExhaustedAboveBlocking(t *testing.T) {
	ag := &agent{
		agentProvider: fixedCountProvider{
			model:      models.Model{ID: "small", ContextWindow: 120, DefaultMaxTokens: 20},
			tokenCount: 120,
		},
	}

	history := []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: strings.Repeat("x", 2000)}}},
		{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: strings.Repeat("y", 2000)}}},
	}
	view := RequestView{Messages: history}
	qcfg := QueryConfig{
		SessionID:                "sess-compact",
		EffectiveContextWindow:   100,
		AutoCompactThreshold:     80,
		BlockingCompactThreshold: 90,
		Compact: CompactLimits{
			ReserveOutputTokens:     20,
			WarningBufferTokens:     20,
			AutoCompactBufferTokens: 20,
			BlockingBufferTokens:    10,
		},
	}

	preflight := ag.preflightRequestView(context.Background(), view, qcfg, false)
	if !preflight.Compacted {
		t.Fatalf("expected preflight compaction")
	}
	if !preflight.Exhausted {
		t.Fatalf("expected compact exhausted when provider count remains above blocking threshold")
	}
}

func TestPreflightPromptTokens_FallbackCountsInjectedPrefixBeforeLastUsage(t *testing.T) {
	ag := &agent{
		agentProvider: fixedCountProvider{
			model:    models.Model{ID: "openai-compatible", ContextWindow: 4096, DefaultMaxTokens: 512},
			countErr: provider.ErrCountTokensUnsupported,
		},
	}

	view := RequestView{
		PrefixMessageCount: 1,
		Messages: []message.Message{
			{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "<session-memory>\n" + strings.Repeat("x", 4000) + "\n</session-memory>"}}},
			{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "old user"}}},
			{
				Role:  message.Assistant,
				Parts: []message.ContentPart{message.TextContent{Text: "old assistant"}},
				Usage: message.Usage{InputTokens: 10},
			},
			{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "latest"}}},
		},
	}

	promptTokens, estimate := ag.preflightPromptTokens(context.Background(), view)
	if !estimate.HasRealUsage {
		t.Fatal("expected TokenCountWithEstimation to see the prior real usage snapshot")
	}
	if estimate.TokenCount >= promptTokens {
		t.Fatalf("expected fallback prompt count to include prefix omitted by usage-anchored estimate, got prompt=%d estimate=%d", promptTokens, estimate.TokenCount)
	}
	if promptTokens < 900 {
		t.Fatalf("expected large injected prefix to be counted, got %d tokens", promptTokens)
	}
}

func TestExecuteToolCalls_RejectsToolOutsideRuntimeAllowList(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	sess := testutil.CreateTestSession(t, sessions)

	ag := &agent{
		Broker:   pubsub.NewBroker[AgentEvent](),
		messages: messages,
		tools:    []tools.BaseTool{stubTool{name: "View"}, stubTool{name: "Bash"}},
	}

	assistantMsg, err := messages.Create(context.Background(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{
				ID:    "tool-1",
				Name:  "Bash",
				Input: `{"cmd":"pwd"}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("create assistant message: %v", err)
	}

	ctx := WithRequestRuntime(context.Background(), RequestRuntime{
		AllowedTools: []string{"View"},
	})
	_, toolMsg, err := ag.executeToolCalls(ctx, assistantMsg)
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if toolMsg == nil {
		t.Fatalf("expected tool result message")
	}
	results := toolMsg.ToolResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(results))
	}
	if !results[0].IsError || !strings.Contains(results[0].Content, "Tool not allowed by runtime override") {
		t.Fatalf("unexpected tool result: %#v", results[0])
	}
}
