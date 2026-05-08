package agent_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/llm/agent"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/llm/provider"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
)

// multiMockProvider supports returning a sequence of responses for multi-turn tests.
type multiMockProvider struct {
	responses []*provider.ProviderResponse
	errors    []error
	callIdx   int
}

func (m *multiMockProvider) SendMessages(ctx context.Context, msgs []message.Message, ts []tools.BaseTool) (*provider.ProviderResponse, error) {
	if m.callIdx >= len(m.responses) {
		return nil, fmt.Errorf("no more responses (call %d)", m.callIdx)
	}
	resp := m.responses[m.callIdx]
	var err error
	if m.callIdx < len(m.errors) {
		err = m.errors[m.callIdx]
	}
	m.callIdx++
	return resp, err
}

func (m *multiMockProvider) StreamResponse(ctx context.Context, msgs []message.Message, ts []tools.BaseTool) <-chan provider.ProviderEvent {
	return nil
}

func (m *multiMockProvider) CountTokens(ctx context.Context, msgs []message.Message, ts []tools.BaseTool) (provider.TokenCount, error) {
	_ = ctx
	_ = msgs
	_ = ts
	return provider.TokenCount{InputTokens: 100}, nil
}

func (m *multiMockProvider) Model() models.Model { return models.Model{ID: "test"} }

// makeForkedRunner creates a ForkedRunner with the given provider.
func makeForkedRunner(p provider.Provider) agent.ForkedRunner {
	return agent.NewForkedRunner(func() (provider.Provider, error) {
		return p, nil
	}, nil)
}

// makeSnapshot creates a minimal CacheSafeSnapshot for testing.
func makeSnapshot(sessionID string, activeTools []tools.BaseTool) *agent.CacheSafeSnapshot {
	return &agent.CacheSafeSnapshot{
		SessionID:     sessionID,
		Model:         models.Model{ID: "test"},
		ActiveTools:   activeTools,
		MessagePrefix: []message.Message{},
		CapturedAt:    time.Now(),
	}
}

// TestForkedRunner_SaveAndLoadSnapshot verifies snapshot storage and retrieval.
func TestForkedRunner_SaveAndLoadSnapshot(t *testing.T) {
	runner := agent.NewForkedRunner(func() (provider.Provider, error) {
		return nil, fmt.Errorf("no provider needed")
	}, nil)

	snap := agent.CacheSafeSnapshot{
		SessionID:  "session-1",
		Model:      models.Model{ID: "claude-3"},
		CapturedAt: time.Now(),
	}
	runner.SaveSnapshot("session-1", snap)

	got, ok := runner.LatestSnapshot("session-1")
	require.True(t, ok)
	assert.Equal(t, "session-1", got.SessionID)
	assert.Equal(t, models.ModelID("claude-3"), got.Model.ID)

	_, ok = runner.LatestSnapshot("nonexistent")
	assert.False(t, ok)
}

// TestForkedRunner_BasicRun verifies a simple prompt → text response flow.
func TestForkedRunner_BasicRun(t *testing.T) {
	mock := &multiMockProvider{
		responses: []*provider.ProviderResponse{
			{
				Content:      "Hello from forked!",
				FinishReason: message.FinishReasonEndTurn,
				Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 5},
			},
		},
	}

	runner := makeForkedRunner(mock)
	snapshot := makeSnapshot("sess-1", nil)

	result, err := runner.Run(context.Background(), agent.ForkedRunOptions{
		Label:           "basic-test",
		Prompt:          "Say hello",
		ParentSessionID: "sess-1",
		Snapshot:        snapshot,
	})

	require.NoError(t, err)
	assert.NoError(t, result.Err)
	assert.Equal(t, "Hello from forked!", result.FinalMessage.Content().Text)
	assert.Equal(t, int64(10), result.Usage.InputTokens)
	assert.Equal(t, int64(5), result.Usage.OutputTokens)
	assert.Equal(t, 0, result.Turns) // no tool turns
}

// TestForkedRunner_ToolCallFlow verifies tool use → re-send flow.
func TestForkedRunner_ToolCallFlow(t *testing.T) {
	toolCallID := "call-001"
	mock := &multiMockProvider{
		responses: []*provider.ProviderResponse{
			// First: LLM requests tool use
			{
				Content:      "",
				FinishReason: message.FinishReasonToolUse,
				ToolCalls: []message.ToolCall{
					{ID: toolCallID, Name: "mockTool", Input: `{"q":"test"}`},
				},
				Usage: provider.TokenUsage{InputTokens: 10, OutputTokens: 2},
			},
			// Second: LLM produces final answer
			{
				Content:      "Final answer after tool",
				FinishReason: message.FinishReasonEndTurn,
				Usage:        provider.TokenUsage{InputTokens: 20, OutputTokens: 8},
			},
		},
	}

	tool := &mockTool{name: "mockTool", result: tools.NewTextResponse("tool result data")}
	snapshot := makeSnapshot("sess-2", []tools.BaseTool{tool})

	runner := makeForkedRunner(mock)

	result, err := runner.Run(context.Background(), agent.ForkedRunOptions{
		Label:            "tool-call-test",
		Prompt:           "Use the tool",
		ParentSessionID:  "sess-2",
		Snapshot:         snapshot,
		AllowedToolNames: map[string]struct{}{"mockTool": {}},
		MaxTurns:         5,
	})

	require.NoError(t, err)
	assert.NoError(t, result.Err)
	assert.Equal(t, "Final answer after tool", result.FinalMessage.Content().Text)
	assert.Equal(t, 1, result.Turns) // one tool turn
	assert.Equal(t, 2, mock.callIdx) // two provider calls

	// Accumulated usage
	assert.Equal(t, int64(30), result.Usage.InputTokens)
	assert.Equal(t, int64(10), result.Usage.OutputTokens)
}

// TestForkedRunner_MaxTurnsLimit verifies tool loop stops at MaxTurns.
func TestForkedRunner_MaxTurnsLimit(t *testing.T) {
	toolCallID := "call-loop"
	toolResp := &provider.ProviderResponse{
		FinishReason: message.FinishReasonToolUse,
		ToolCalls: []message.ToolCall{
			{ID: toolCallID, Name: "mockTool", Input: `{}`},
		},
		Usage: provider.TokenUsage{InputTokens: 5, OutputTokens: 1},
	}

	// Provide many responses (all requesting tool use)
	responses := make([]*provider.ProviderResponse, 10)
	for i := range responses {
		responses[i] = toolResp
	}

	mock := &multiMockProvider{responses: responses}
	tool := &mockTool{name: "mockTool", result: tools.NewTextResponse("looping")}
	snapshot := makeSnapshot("sess-3", []tools.BaseTool{tool})

	runner := makeForkedRunner(mock)

	result, err := runner.Run(context.Background(), agent.ForkedRunOptions{
		Label:            "max-turns-test",
		Prompt:           "Loop forever",
		ParentSessionID:  "sess-3",
		Snapshot:         snapshot,
		AllowedToolNames: map[string]struct{}{"mockTool": {}},
		MaxTurns:         2, // limit to 2 tool turns
	})

	require.NoError(t, err)
	// Should stop at MaxTurns
	assert.LessOrEqual(t, result.Turns, 2)
}

// TestForkedRunner_Timeout verifies that context timeout is respected.
func TestForkedRunner_Timeout(t *testing.T) {
	// Provider that blocks until context is cancelled
	blockingProvider := &blockingMockProvider{}

	runner := agent.NewForkedRunner(func() (provider.Provider, error) {
		return blockingProvider, nil
	}, nil)

	snapshot := makeSnapshot("sess-timeout", nil)

	result, err := runner.Run(context.Background(), agent.ForkedRunOptions{
		Label:           "timeout-test",
		Prompt:          "This will time out",
		ParentSessionID: "sess-timeout",
		Snapshot:        snapshot,
		Timeout:         50 * time.Millisecond,
	})

	// Should return a context error
	assert.Error(t, err)
	_ = result // result may be partial
}

// blockingMockProvider blocks in SendMessages until context is cancelled.
type blockingMockProvider struct{}

func (b *blockingMockProvider) SendMessages(ctx context.Context, msgs []message.Message, ts []tools.BaseTool) (*provider.ProviderResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingMockProvider) StreamResponse(ctx context.Context, msgs []message.Message, ts []tools.BaseTool) <-chan provider.ProviderEvent {
	return nil
}

func (b *blockingMockProvider) CountTokens(ctx context.Context, msgs []message.Message, ts []tools.BaseTool) (provider.TokenCount, error) {
	_ = ctx
	_ = msgs
	_ = ts
	return provider.TokenCount{InputTokens: 100}, nil
}

func (b *blockingMockProvider) Model() models.Model { return models.Model{ID: "blocking"} }

// TestForkedRunner_NonWhitelistTool verifies non-whitelisted tools return error results.
func TestForkedRunner_NonWhitelistTool(t *testing.T) {
	toolCallID := "call-forbidden"
	mock := &multiMockProvider{
		responses: []*provider.ProviderResponse{
			// LLM tries to call a forbidden tool
			{
				FinishReason: message.FinishReasonToolUse,
				ToolCalls: []message.ToolCall{
					{ID: toolCallID, Name: "forbiddenTool", Input: `{}`},
				},
				Usage: provider.TokenUsage{InputTokens: 5, OutputTokens: 1},
			},
			// After getting error result, LLM responds normally
			{
				Content:      "I cannot use that tool",
				FinishReason: message.FinishReasonEndTurn,
				Usage:        provider.TokenUsage{InputTokens: 10, OutputTokens: 5},
			},
		},
	}

	// Snapshot has no tools
	snapshot := makeSnapshot("sess-whitelist", nil)

	runner := makeForkedRunner(mock)

	result, err := runner.Run(context.Background(), agent.ForkedRunOptions{
		Label:            "whitelist-test",
		Prompt:           "Use the forbidden tool",
		ParentSessionID:  "sess-whitelist",
		Snapshot:         snapshot,
		AllowedToolNames: map[string]struct{}{}, // empty whitelist
		MaxTurns:         3,
	})

	require.NoError(t, err)
	assert.NoError(t, result.Err)

	// The messages should include a tool result with an error
	var foundErrorToolResult bool
	for _, msg := range result.Messages {
		for _, part := range msg.Parts {
			if tr, ok := part.(message.ToolResult); ok {
				if tr.IsError && tr.ToolCallID == toolCallID {
					foundErrorToolResult = true
				}
			}
		}
	}
	assert.True(t, foundErrorToolResult, "expected an error tool result for forbidden tool")
}

// TestForkedRunner_OnComplete verifies the OnComplete callback is invoked.
func TestForkedRunner_OnComplete(t *testing.T) {
	mock := &multiMockProvider{
		responses: []*provider.ProviderResponse{
			{
				Content:      "Done",
				FinishReason: message.FinishReasonEndTurn,
				Usage:        provider.TokenUsage{InputTokens: 5, OutputTokens: 3},
			},
		},
	}

	runner := makeForkedRunner(mock)
	snapshot := makeSnapshot("sess-callback", nil)

	callbackCalled := false
	var callbackResult agent.ForkedRunResult

	result, err := runner.Run(context.Background(), agent.ForkedRunOptions{
		Label:           "callback-test",
		Prompt:          "Hello",
		ParentSessionID: "sess-callback",
		Snapshot:        snapshot,
		OnComplete: func(r agent.ForkedRunResult) {
			callbackCalled = true
			callbackResult = r
		},
	})

	require.NoError(t, err)
	assert.True(t, callbackCalled)
	assert.Equal(t, result.FinalMessage.Content().Text, callbackResult.FinalMessage.Content().Text)
}

// TestForkedRunner_NilSnapshot verifies run works with nil snapshot (empty prefix).
func TestForkedRunner_NilSnapshot(t *testing.T) {
	mock := &multiMockProvider{
		responses: []*provider.ProviderResponse{
			{
				Content:      "No snapshot response",
				FinishReason: message.FinishReasonEndTurn,
				Usage:        provider.TokenUsage{InputTokens: 3, OutputTokens: 2},
			},
		},
	}

	runner := makeForkedRunner(mock)

	result, err := runner.Run(context.Background(), agent.ForkedRunOptions{
		Label:           "nil-snapshot",
		Prompt:          "Prompt with no snapshot",
		ParentSessionID: "sess-nil",
		Snapshot:        nil, // no snapshot
	})

	require.NoError(t, err)
	assert.Equal(t, "No snapshot response", result.FinalMessage.Content().Text)
}

// TestForkedRunner_MessagePrefixCopied verifies the snapshot MessagePrefix is deep-copied.
func TestForkedRunner_MessagePrefixCopied(t *testing.T) {
	mock := &multiMockProvider{
		responses: []*provider.ProviderResponse{
			{
				Content:      "Prefix seen",
				FinishReason: message.FinishReasonEndTurn,
				Usage:        provider.TokenUsage{},
			},
		},
	}

	snapshot := &agent.CacheSafeSnapshot{
		SessionID: "sess-prefix",
		Model:     models.Model{ID: "test"},
		MessagePrefix: []message.Message{
			{
				Role:  message.User,
				Parts: []message.ContentPart{message.TextContent{Text: "prior context"}},
			},
		},
		CapturedAt: time.Now(),
	}

	runner := makeForkedRunner(mock)

	result, err := runner.Run(context.Background(), agent.ForkedRunOptions{
		Label:           "prefix-test",
		Prompt:          "Continue",
		ParentSessionID: "sess-prefix",
		Snapshot:        snapshot,
	})

	require.NoError(t, err)
	// Ensure the snapshot prefix wasn't modified (deep copy check)
	assert.Len(t, snapshot.MessagePrefix, 1)
	assert.Equal(t, "prior context", snapshot.MessagePrefix[0].Content().Text)
	assert.Equal(t, "Prefix seen", result.FinalMessage.Content().Text)
}
