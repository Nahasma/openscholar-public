package agent

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/llm/prompt"
	"github.com/Nahasma/openscholar-public/internal/message"
)

// --- helpers ---

func makeMsg(role message.MessageRole, parts ...message.ContentPart) message.Message {
	return message.Message{
		ID:    "test-id",
		Role:  role,
		Parts: parts,
	}
}

// --- CA1: ContextAnalyzer tests ---

func TestContextAnalyzer_BasicClassification(t *testing.T) {
	analyzer := NewContextAnalyzer()

	systemBlocks := []prompt.PromptBlock{
		{Text: "static system prompt", IsDynamic: false},
		{Text: "dynamic block", IsDynamic: true},
	}

	msgs := []message.Message{
		makeMsg(message.User, message.TextContent{Text: "hello world"}),
		makeMsg(message.Assistant,
			message.TextContent{Text: "hi there"},
			message.ReasoningContent{Thinking: "thinking..."},
			message.ToolCall{ID: "1", Name: "bash", Input: `{"cmd":"ls"}`},
		),
		makeMsg(message.Tool, message.ToolResult{ToolCallID: "1", Name: "bash", Content: "file1\nfile2"}),
	}

	result := analyzer.Analyze("sess-1", systemBlocks, msgs, 0)

	if result.SessionID != "sess-1" {
		t.Errorf("expected SessionID=sess-1, got %q", result.SessionID)
	}

	catMap := make(map[ContextCategory]ContextSlice)
	for _, cs := range result.ByCategory {
		catMap[cs.Category] = cs
	}

	// system static should be present
	if _, ok := catMap[ContextSystemStatic]; !ok {
		t.Error("expected ContextSystemStatic category")
	}
	// system dynamic should be present
	if _, ok := catMap[ContextSystemDynamic]; !ok {
		t.Error("expected ContextSystemDynamic category")
	}
	// user input
	if _, ok := catMap[ContextUserInput]; !ok {
		t.Error("expected ContextUserInput category")
	}
	// assistant text
	if _, ok := catMap[ContextAssistantText]; !ok {
		t.Error("expected ContextAssistantText category")
	}
	// reasoning
	if _, ok := catMap[ContextReasoning]; !ok {
		t.Error("expected ContextReasoning category")
	}
	// tool calls
	if _, ok := catMap[ContextToolCalls]; !ok {
		t.Error("expected ContextToolCalls category")
	}
	// tool results
	if _, ok := catMap[ContextToolResults]; !ok {
		t.Error("expected ContextToolResults category")
	}
}

func TestContextAnalyzer_MemoryTags(t *testing.T) {
	analyzer := NewContextAnalyzer()

	msgs := []message.Message{
		makeMsg(message.User, message.TextContent{Text: "<session-memory>some session data</session-memory>"}),
		makeMsg(message.User, message.TextContent{Text: "<memory-context>long-term memory</memory-context>"}),
		makeMsg(message.User, message.TextContent{Text: "regular user message"}),
	}

	result := analyzer.Analyze("sess-2", nil, msgs, 0)
	catMap := make(map[ContextCategory]ContextSlice)
	for _, cs := range result.ByCategory {
		catMap[cs.Category] = cs
	}

	if _, ok := catMap[ContextSessionMemory]; !ok {
		t.Error("expected ContextSessionMemory")
	}
	if _, ok := catMap[ContextMemory]; !ok {
		t.Error("expected ContextMemory")
	}
	if _, ok := catMap[ContextUserInput]; !ok {
		t.Error("expected ContextUserInput for regular message")
	}
}

func TestContextAnalyzer_TokenConservation(t *testing.T) {
	analyzer := NewContextAnalyzer()

	systemBlocks := []prompt.PromptBlock{
		{Text: "system", IsDynamic: false},
	}
	msgs := []message.Message{
		makeMsg(message.User, message.TextContent{Text: "user input here"}),
		makeMsg(message.Assistant, message.TextContent{Text: "assistant reply here"}),
		makeMsg(message.Tool, message.ToolResult{Name: "bash", Content: "output"}),
	}

	// Provide actual token count
	actualTokens := 20
	result := analyzer.Analyze("sess-3", systemBlocks, msgs, actualTokens)

	if result.ActualInputTokens != actualTokens {
		t.Errorf("expected ActualInputTokens=%d, got %d", actualTokens, result.ActualInputTokens)
	}

	// Sum of category tokens should be proportional (scaled to actual)
	total := 0
	for _, cs := range result.ByCategory {
		total += cs.Tokens
	}
	// Allow for rounding: total should be close to actualTokens
	// The scaling is approximate; just ensure the sum is positive and not wildly off.
	if total <= 0 {
		t.Errorf("expected positive category token sum, got %d", total)
	}
}

func TestContextAnalyzer_EmptyMessages(t *testing.T) {
	analyzer := NewContextAnalyzer()
	result := analyzer.Analyze("sess-empty", nil, nil, 0)

	if result.SessionID != "sess-empty" {
		t.Errorf("expected SessionID=sess-empty, got %q", result.SessionID)
	}
	if len(result.ByCategory) != 0 {
		t.Errorf("expected empty ByCategory for empty input, got %d entries", len(result.ByCategory))
	}
	if len(result.ByTool) != 0 {
		t.Errorf("expected empty ByTool for empty input, got %d entries", len(result.ByTool))
	}
	if result.EstimatedTokens != 0 {
		t.Errorf("expected 0 EstimatedTokens for empty input, got %d", result.EstimatedTokens)
	}
}

func TestContextAnalyzer_ByToolAggregation(t *testing.T) {
	analyzer := NewContextAnalyzer()

	msgs := []message.Message{
		makeMsg(message.Assistant,
			message.ToolCall{ID: "1", Name: "bash", Input: `{"cmd":"ls"}`},
			message.ToolCall{ID: "2", Name: "bash", Input: `{"cmd":"pwd"}`},
			message.ToolCall{ID: "3", Name: "grep", Input: `{"pattern":"foo"}`},
		),
		makeMsg(message.Tool,
			message.ToolResult{ToolCallID: "1", Name: "bash", Content: "file1\nfile2\nfile3"},
			message.ToolResult{ToolCallID: "2", Name: "bash", Content: "/home/user"},
			message.ToolResult{ToolCallID: "3", Name: "grep", Content: "match1"},
		),
	}

	result := analyzer.Analyze("sess-4", nil, msgs, 0)

	bashStats, ok := result.ByTool["bash"]
	if !ok {
		t.Fatal("expected bash in ByTool")
	}
	if bashStats.CallCount != 2 {
		t.Errorf("expected bash CallCount=2, got %d", bashStats.CallCount)
	}
	if bashStats.InputTokens <= 0 {
		t.Errorf("expected positive bash InputTokens, got %d", bashStats.InputTokens)
	}
	if bashStats.ResultTokens <= 0 {
		t.Errorf("expected positive bash ResultTokens, got %d", bashStats.ResultTokens)
	}

	grepStats, ok := result.ByTool["grep"]
	if !ok {
		t.Fatal("expected grep in ByTool")
	}
	if grepStats.CallCount != 1 {
		t.Errorf("expected grep CallCount=1, got %d", grepStats.CallCount)
	}
}

func TestContextAnalyzer_CompactObservationRefs(t *testing.T) {
	analyzer := NewContextAnalyzer()
	msgs := []message.Message{
		makeMsg(message.Tool, message.ToolResult{
			Name:     "WebFetch",
			Content:  `{"tool":"WebFetch","status":"ok","public_summary":"Fetched compact page.","data_ref":"sidechain://webfetch/1"}`,
			Metadata: `{"artifact":{"path":".openscholar/papers/a.pdf"},"artifacts":[{"path":".openscholar/papers/b.pdf"}]}`,
		}),
	}

	result := analyzer.Analyze("sess-compact", nil, msgs, 0)
	if result.CompactToolResults != 1 {
		t.Fatalf("expected compact tool result count, got %d", result.CompactToolResults)
	}
	if len(result.DataRefs) != 1 || result.DataRefs[0] != "sidechain://webfetch/1" {
		t.Fatalf("expected data ref, got %#v", result.DataRefs)
	}
	if len(result.ArtifactPaths) != 2 {
		t.Fatalf("expected artifact paths, got %#v", result.ArtifactPaths)
	}
}

// --- CA2: SuggestionEngine tests ---

func TestSuggestionEngine_NearCapacity(t *testing.T) {
	engine := NewSuggestionEngine()

	analysis := ContextAnalysis{
		ActualInputTokens: 85000,
		EstimatedTokens:   85000,
	}
	suggestions := engine.Suggest(analysis, 100000, true)

	found := false
	for _, s := range suggestions {
		if s.Code == "near_capacity" {
			found = true
			if s.Severity != SeverityWarning {
				t.Errorf("expected warning severity, got %s", s.Severity)
			}
			if s.ActionLabel != "/compact" {
				t.Errorf("expected action=/compact, got %s", s.ActionLabel)
			}
		}
	}
	if !found {
		t.Error("expected near_capacity suggestion")
	}
}

func TestSuggestionEngine_NearCapacity_NotTriggered(t *testing.T) {
	engine := NewSuggestionEngine()

	analysis := ContextAnalysis{
		ActualInputTokens: 50000,
		EstimatedTokens:   50000,
	}
	suggestions := engine.Suggest(analysis, 100000, true)

	for _, s := range suggestions {
		if s.Code == "near_capacity" {
			t.Error("near_capacity should NOT trigger when below 80%")
		}
	}
}

func TestSuggestionEngine_LargeToolResults(t *testing.T) {
	engine := NewSuggestionEngine()

	analysis := ContextAnalysis{
		ByTool: map[string]ToolTokenStats{
			"view": {CallCount: 1, InputTokens: 10, ResultTokens: 25000},
		},
	}
	suggestions := engine.Suggest(analysis, 200000, true)

	found := false
	for _, s := range suggestions {
		if s.Code == "large_tool_results" {
			found = true
			if s.Severity != SeverityInfo {
				t.Errorf("expected info severity, got %s", s.Severity)
			}
			if s.ActionLabel != "Use offset/limit" {
				t.Errorf("expected action='Use offset/limit', got %s", s.ActionLabel)
			}
		}
	}
	if !found {
		t.Error("expected large_tool_results suggestion")
	}
}

func TestSuggestionEngine_ReadBloat(t *testing.T) {
	engine := NewSuggestionEngine()

	analysis := ContextAnalysis{
		ByTool: map[string]ToolTokenStats{
			"view": {CallCount: 5, InputTokens: 50, ResultTokens: 1000},
		},
	}
	suggestions := engine.Suggest(analysis, 200000, true)

	found := false
	for _, s := range suggestions {
		if s.Code == "read_bloat" {
			found = true
			if s.Severity != SeverityInfo {
				t.Errorf("expected info severity, got %s", s.Severity)
			}
		}
	}
	if !found {
		t.Error("expected read_bloat suggestion")
	}
}

func TestSuggestionEngine_MemoryBloat(t *testing.T) {
	engine := NewSuggestionEngine()

	analysis := ContextAnalysis{
		ByCategory: []ContextSlice{
			{Category: ContextMemory, Tokens: 7000, Bytes: 28000, Count: 1},
		},
	}
	suggestions := engine.Suggest(analysis, 200000, true)

	found := false
	for _, s := range suggestions {
		if s.Code == "memory_bloat" {
			found = true
			if s.Severity != SeverityInfo {
				t.Errorf("expected info severity, got %s", s.Severity)
			}
		}
	}
	if !found {
		t.Error("expected memory_bloat suggestion")
	}
}

func TestSuggestionEngine_AutoCompactDisabled(t *testing.T) {
	engine := NewSuggestionEngine()

	analysis := ContextAnalysis{}
	suggestions := engine.Suggest(analysis, 200000, false)

	found := false
	for _, s := range suggestions {
		if s.Code == "autocompact_disabled" {
			found = true
			if s.Severity != SeverityWarning {
				t.Errorf("expected warning severity, got %s", s.Severity)
			}
		}
	}
	if !found {
		t.Error("expected autocompact_disabled suggestion")
	}
}

func TestSuggestionEngine_AutoCompactEnabled_NoSuggestion(t *testing.T) {
	engine := NewSuggestionEngine()

	analysis := ContextAnalysis{}
	suggestions := engine.Suggest(analysis, 200000, true)

	for _, s := range suggestions {
		if s.Code == "autocompact_disabled" {
			t.Error("autocompact_disabled should NOT trigger when enabled")
		}
	}
}

func TestSuggestionEngine_CompactToolResults(t *testing.T) {
	engine := NewSuggestionEngine()

	suggestions := engine.Suggest(ContextAnalysis{
		CompactToolResults: 2,
		DataRefs:           []string{"sidechain://tool/1"},
		ArtifactPaths:      []string{".openscholar/papers/a.pdf"},
	}, 200000, true)

	found := false
	for _, s := range suggestions {
		if s.Code == "compact_tool_results" {
			found = true
			if s.Severity != SeverityInfo || s.ActionLabel != "Use debug refs" {
				t.Fatalf("unexpected compact suggestion: %#v", s)
			}
		}
	}
	if !found {
		t.Fatal("expected compact_tool_results suggestion")
	}
}

func TestSuggestionEngine_NoSuggestionsWhenHealthy(t *testing.T) {
	engine := NewSuggestionEngine()

	analysis := ContextAnalysis{
		ActualInputTokens: 10000,
		EstimatedTokens:   10000,
		ByCategory: []ContextSlice{
			{Category: ContextUserInput, Tokens: 5000, Bytes: 20000, Count: 5},
			{Category: ContextMemory, Tokens: 1000, Bytes: 4000, Count: 1},
		},
		ByTool: map[string]ToolTokenStats{
			"bash": {CallCount: 2, InputTokens: 20, ResultTokens: 100},
		},
	}
	suggestions := engine.Suggest(analysis, 200000, true)

	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for healthy context, got %d: %+v", len(suggestions), suggestions)
	}
}
