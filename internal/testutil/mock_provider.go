package testutil

import (
	"context"

	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/provider"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/message"
)

// MockProvider implements provider.Provider with canned responses.
type MockProvider struct {
	ModelVal        models.Model
	Response        string
	ToolCallsFn     func() []message.ToolCall
	ErrorFn         func() error         // Return error from SendMessages
	FinishReasonVal message.FinishReason // Override finish reason (default: end_turn)
	CallCount       int                  // Number of SendMessages calls
	LastMessages    []message.Message    // Messages from the last SendMessages call
}

func (m *MockProvider) SendMessages(ctx context.Context, msgs []message.Message, t []tools.BaseTool) (*provider.ProviderResponse, error) {
	m.CallCount++
	m.LastMessages = msgs

	if m.ErrorFn != nil {
		if err := m.ErrorFn(); err != nil {
			return nil, err
		}
	}

	finishReason := m.FinishReasonVal
	if finishReason == "" {
		finishReason = message.FinishReasonEndTurn
	}

	resp := &provider.ProviderResponse{
		Content:      m.Response,
		Usage:        provider.TokenUsage{InputTokens: 100, OutputTokens: 50},
		FinishReason: finishReason,
	}
	if m.ToolCallsFn != nil {
		resp.ToolCalls = m.ToolCallsFn()
	}
	return resp, nil
}

func (m *MockProvider) StreamResponse(ctx context.Context, msgs []message.Message, t []tools.BaseTool) <-chan provider.ProviderEvent {
	m.CallCount++
	m.LastMessages = msgs

	ch := make(chan provider.ProviderEvent, 3)
	go func() {
		defer close(ch)

		if m.ErrorFn != nil {
			if err := m.ErrorFn(); err != nil {
				ch <- provider.ProviderEvent{Type: provider.EventError, Error: err}
				return
			}
		}

		finishReason := m.FinishReasonVal
		if finishReason == "" {
			finishReason = message.FinishReasonEndTurn
		}
		var toolCalls []message.ToolCall
		if m.ToolCallsFn != nil {
			toolCalls = m.ToolCallsFn()
		}

		ch <- provider.ProviderEvent{Type: provider.EventContentStart}
		ch <- provider.ProviderEvent{Type: provider.EventContentDelta, Content: m.Response}
		ch <- provider.ProviderEvent{
			Type: provider.EventComplete,
			Response: &provider.ProviderResponse{
				Content:      m.Response,
				ToolCalls:    toolCalls,
				Usage:        provider.TokenUsage{InputTokens: 100, OutputTokens: 50},
				FinishReason: finishReason,
			},
		}
	}()
	return ch
}

func (m *MockProvider) CountTokens(ctx context.Context, msgs []message.Message, t []tools.BaseTool) (provider.TokenCount, error) {
	_ = ctx
	m.LastMessages = msgs
	return provider.TokenCount{InputTokens: 100}, nil
}

func (m *MockProvider) Model() models.Model { return m.ModelVal }
