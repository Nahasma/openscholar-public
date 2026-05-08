package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	toolsPkg "github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
)

type openaiClient struct {
	providerOptions providerClientOptions
	client          openai.Client
}

func newOpenAIClient(opts providerClientOptions) *openaiClient {
	var clientOptions []option.RequestOption
	if opts.apiKey != "" {
		clientOptions = append(clientOptions, option.WithAPIKey(opts.apiKey))
	}

	return &openaiClient{
		providerOptions: opts,
		client:          openai.NewClient(clientOptions...),
	}
}

func newOpenAIClientWithBaseURL(opts providerClientOptions, baseURL string) *openaiClient {
	var clientOptions []option.RequestOption
	if opts.apiKey != "" {
		clientOptions = append(clientOptions, option.WithAPIKey(opts.apiKey))
	}
	clientOptions = append(clientOptions, option.WithBaseURL(baseURL))

	return &openaiClient{
		providerOptions: opts,
		client:          openai.NewClient(clientOptions...),
	}
}

func (o *openaiClient) convertMessages(messages []message.Message) []openai.ChatCompletionMessageParamUnion {
	// Sanitize: collect tool_call IDs that have corresponding tool results
	toolResultIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role == message.Tool {
			for _, tr := range msg.ToolResults() {
				toolResultIDs[tr.ToolCallID] = true
			}
		}
	}

	var oaiMessages []openai.ChatCompletionMessageParamUnion

	for _, msg := range messages {
		switch msg.Role {
		case message.User:
			// Check for BinaryContent parts
			hasBinary := false
			for _, part := range msg.Parts {
				if _, ok := part.(message.BinaryContent); ok {
					hasBinary = true
					break
				}
			}
			if hasBinary {
				var contentParts []openai.ChatCompletionContentPartUnionParam
				for _, part := range msg.Parts {
					switch p := part.(type) {
					case message.TextContent:
						if p.Text != "" {
							contentParts = append(contentParts, openai.TextContentPart(p.Text))
						}
					case message.BinaryContent:
						dataURI := p.String(models.ProviderOpenAI)
						contentParts = append(contentParts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
							URL: dataURI,
						}))
					}
				}
				oaiMessages = append(oaiMessages, openai.UserMessage(contentParts))
			} else {
				oaiMessages = append(oaiMessages, openai.UserMessage(msg.Content().String()))
			}

		case message.Assistant:
			toolCalls := msg.ToolCalls()

			// Filter out orphaned tool calls that have no corresponding tool result
			var validToolCalls []message.ToolCall
			for _, tc := range toolCalls {
				if toolResultIDs[tc.ID] {
					validToolCalls = append(validToolCalls, tc)
				}
			}

			assistantMsg := openai.AssistantMessage(msg.Content().String())
			if len(validToolCalls) > 0 {
				oaiToolCalls := make([]openai.ChatCompletionMessageToolCallParam, len(validToolCalls))
				for i, tc := range validToolCalls {
					oaiToolCalls[i] = openai.ChatCompletionMessageToolCallParam{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Input,
						},
					}
				}
				assistantMsg.OfAssistant.ToolCalls = oaiToolCalls
			}

			// DeepSeek R1 requires reasoning_content field in assistant messages
			if o.providerOptions.model.CanReason && o.providerOptions.model.Provider == models.ProviderDeepSeek {
				reasoningText := ""
				for _, part := range msg.Parts {
					if rc, ok := part.(message.ReasoningContent); ok {
						reasoningText = rc.Thinking
						break
					}
				}
				assistantMsg.OfAssistant.WithExtraFields(map[string]any{
					"reasoning_content": reasoningText,
				})
			}

			oaiMessages = append(oaiMessages, assistantMsg)

		case message.Tool:
			for _, tr := range msg.ToolResults() {
				oaiMessages = append(oaiMessages, openai.ToolMessage(tr.Content, tr.ToolCallID))
			}
		}
	}
	return oaiMessages
}

func (o *openaiClient) convertTools(tools []toolsPkg.BaseTool) []openai.ChatCompletionToolParam {
	oaiTools := make([]openai.ChatCompletionToolParam, len(tools))
	for i, tool := range tools {
		info := tool.Info()
		paramsJSON, _ := json.Marshal(info.Parameters)
		var paramsMap map[string]interface{}
		json.Unmarshal(paramsJSON, &paramsMap)

		oaiTools[i] = openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        info.Name,
				Description: openai.String(info.Description),
				Parameters:  openai.FunctionParameters(paramsMap),
			},
		}
	}
	return oaiTools
}

func (o *openaiClient) send(ctx context.Context, messages []message.Message, tools []toolsPkg.BaseTool) (*ProviderResponse, error) {
	systemPrompt := resolveSystemPrompt(ctx, o.providerOptions)
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(o.providerOptions.model.APIModel),
		Messages: o.convertMessages(messages),
	}
	if o.providerOptions.maxTokens > 0 {
		params.MaxTokens = openai.Int(o.providerOptions.maxTokens)
	}

	if len(tools) > 0 {
		params.Tools = o.convertTools(tools)
	}

	systemMessage := systemPrompt.Message
	if systemMessage == "" && len(systemPrompt.Blocks) > 0 {
		systemMessage = joinSystemBlocks(systemPrompt.Blocks)
	}
	if systemMessage != "" {
		params.Messages = append(
			[]openai.ChatCompletionMessageParamUnion{openai.SystemMessage(systemMessage)},
			params.Messages...,
		)
	}

	resp, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return &ProviderResponse{}, nil
	}

	choice := resp.Choices[0]
	var toolCalls []message.ToolCall
	for _, tc := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, message.ToolCall{
			ID:       tc.ID,
			Name:     tc.Function.Name,
			Input:    tc.Function.Arguments,
			Type:     "function",
			Finished: true,
		})
	}

	finishReason := mapOpenAIFinishReason(string(choice.FinishReason), len(toolCalls) > 0)
	usage, _ := openAIUsage(resp.Usage)

	return &ProviderResponse{
		Content:      choice.Message.Content,
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		Usage:        usage,
	}, nil
}

func (o *openaiClient) countTokens(ctx context.Context, messages []message.Message, tools []toolsPkg.BaseTool) (TokenCount, error) {
	_ = ctx
	_ = messages
	_ = tools
	return TokenCount{}, fmt.Errorf("openai: %w", ErrCountTokensUnsupported)
}

func (o *openaiClient) stream(ctx context.Context, messages []message.Message, tools []toolsPkg.BaseTool) <-chan ProviderEvent {
	systemPrompt := resolveSystemPrompt(ctx, o.providerOptions)
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(o.providerOptions.model.APIModel),
		Messages: o.convertMessages(messages),
	}
	if o.providerOptions.maxTokens > 0 {
		params.MaxTokens = openai.Int(o.providerOptions.maxTokens)
	}

	if len(tools) > 0 {
		params.Tools = o.convertTools(tools)
	}

	systemMessage := systemPrompt.Message
	if systemMessage == "" && len(systemPrompt.Blocks) > 0 {
		systemMessage = joinSystemBlocks(systemPrompt.Blocks)
	}
	if systemMessage != "" {
		params.Messages = append(
			[]openai.ChatCompletionMessageParamUnion{openai.SystemMessage(systemMessage)},
			params.Messages...,
		)
	}

	eventChan := make(chan ProviderEvent)
	go func() {
		isDeepSeekR1 := o.providerOptions.model.CanReason && o.providerOptions.model.Provider == models.ProviderDeepSeek
		requestUsage := true

		var lastErr error
		for attempts := 1; attempts <= maxRetries; attempts++ {
			if attempts > 1 {
				delay := openaiRetryDelay(attempts)
				log.Printf("OpenAI stream retry (attempt %d/%d) after %v: %v", attempts, maxRetries, delay, lastErr)
				select {
				case <-ctx.Done():
					eventChan <- ProviderEvent{Type: EventError, Error: ctx.Err()}
					close(eventChan)
					return
				case <-time.After(delay):
				}
			}

			streamParams := params
			if requestUsage {
				streamParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{
					IncludeUsage: openai.Bool(true),
				}
			}

			stream := o.client.Chat.Completions.NewStreaming(ctx, streamParams)

			fullContent := ""
			var allToolCalls []message.ToolCall
			toolCallInputs := map[int]string{}
			eventsEmitted := false
			rawFinishReason := ""
			var usage TokenUsage
			hasUsage := false

			for stream.Next() {
				chunk := stream.Current()
				if chunkUsage, ok := openAIUsage(chunk.Usage); ok {
					usage = chunkUsage
					hasUsage = true
				}

				for _, choice := range chunk.Choices {
					if choice.FinishReason != "" {
						rawFinishReason = string(choice.FinishReason)
					}
					if isDeepSeekR1 {
						if rcField, ok := choice.Delta.JSON.ExtraFields["reasoning_content"]; ok && rcField.IsPresent() {
							var rc string
							if err := json.Unmarshal([]byte(rcField.Raw()), &rc); err == nil && rc != "" {
								eventChan <- ProviderEvent{
									Type:     EventThinkingDelta,
									Thinking: rc,
								}
								eventsEmitted = true
							}
						}
					}

					if choice.Delta.Content != "" {
						fullContent += choice.Delta.Content
						eventChan <- ProviderEvent{
							Type:    EventContentDelta,
							Content: SanitizeContentDelta(choice.Delta.Content),
						}
						eventsEmitted = true
					}

					for _, tc := range choice.Delta.ToolCalls {
						idx := int(tc.Index)
						if tc.ID != "" {
							for len(allToolCalls) <= idx {
								allToolCalls = append(allToolCalls, message.ToolCall{})
							}
							allToolCalls[idx].ID = tc.ID
							allToolCalls[idx].Name = tc.Function.Name
							allToolCalls[idx].Type = "function"
							eventChan <- ProviderEvent{
								Type: EventToolUseStart,
								ToolCall: &message.ToolCall{
									ID:   tc.ID,
									Name: tc.Function.Name,
								},
							}
							eventsEmitted = true
						}
						if tc.Function.Arguments != "" {
							toolCallInputs[idx] += tc.Function.Arguments
						}
					}
				}
			}

			if err := stream.Err(); err != nil {
				lastErr = err
				if requestUsage && !eventsEmitted && openAIIncludeUsageUnsupported(err) {
					requestUsage = false
					attempts--
					continue
				}
				// Only retry if no events have been emitted and error is retryable
				if !eventsEmitted && openaiShouldRetry(err) && attempts < maxRetries {
					continue
				}
				log.Printf("OpenAI stream error: %v", err)
				eventChan <- ProviderEvent{Type: EventError, Error: err}
				close(eventChan)
				return
			}

			// Success — emit tool stop events and complete
			for idx := range allToolCalls {
				allToolCalls[idx].Input = toolCallInputs[idx]
				allToolCalls[idx].Finished = true
				tc := allToolCalls[idx]
				eventChan <- ProviderEvent{
					Type:     EventToolUseStop,
					ToolCall: &tc,
				}
			}

			finishReason := mapOpenAIFinishReason(rawFinishReason, len(allToolCalls) > 0)

			eventChan <- ProviderEvent{
				Type: EventComplete,
				Response: &ProviderResponse{
					Content:      fullContent,
					ToolCalls:    allToolCalls,
					FinishReason: finishReason,
					Usage:        usageIfPresent(usage, hasUsage),
				},
			}

			close(eventChan)
			return
		}

		// All retries exhausted
		eventChan <- ProviderEvent{Type: EventError, Error: fmt.Errorf("max retries exceeded: %w", lastErr)}
		close(eventChan)
	}()

	return eventChan
}

func openAIUsage(u openai.CompletionUsage) (TokenUsage, bool) {
	hasUsage := u.JSON.PromptTokens.IsPresent() || u.JSON.CompletionTokens.IsPresent()
	usage := TokenUsage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}

	if u.PromptTokensDetails.JSON.CachedTokens.IsPresent() {
		usage.CacheReadTokens = u.PromptTokensDetails.CachedTokens
		usage.InputTokens -= usage.CacheReadTokens
		if usage.InputTokens < 0 {
			usage.InputTokens = 0
		}
	}

	return usage, hasUsage
}

func usageIfPresent(usage TokenUsage, hasUsage bool) TokenUsage {
	if hasUsage {
		return usage
	}
	return TokenUsage{}
}

func mapOpenAIFinishReason(raw string, hasToolCalls bool) message.FinishReason {
	switch raw {
	case "tool_calls":
		return message.FinishReasonToolUse
	case "length":
		return message.FinishReasonMaxTokens
	case "content_filter":
		return message.FinishReasonError
	case "", "stop":
		if hasToolCalls {
			return message.FinishReasonToolUse
		}
		return message.FinishReasonEndTurn
	default:
		if hasToolCalls {
			return message.FinishReasonToolUse
		}
		return message.FinishReasonUnknown
	}
}

// openaiShouldRetry returns true for HTTP status codes that are safe to retry.
func openaiShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	// Check for rate limit (429) or server errors (5xx) in error message
	// The openai-go SDK wraps HTTP errors
	type httpError interface {
		StatusCode() int
	}
	var he httpError
	if ok := errorAs(err, &he); ok {
		code := he.StatusCode()
		return code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
	}
	// For network errors, always retry
	return true
}

func openAIIncludeUsageUnsupported(err error) bool {
	if err == nil {
		return false
	}

	type httpError interface {
		StatusCode() int
	}
	var he httpError
	if ok := errorAs(err, &he); ok {
		code := he.StatusCode()
		if code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
			return false
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "include_usage") || strings.Contains(msg, "stream_options")
}

// errorAs is a helper to extract typed errors.
func errorAs[T any](err error, target *T) bool {
	for err != nil {
		if t, ok := err.(T); ok {
			*target = t
			return true
		}
		if u, ok := err.(interface{ Unwrap() error }); ok {
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}

// openaiRetryDelay returns exponential backoff duration with jitter.
func openaiRetryDelay(attempt int) time.Duration {
	backoffMs := 2000 * (1 << (attempt - 1))
	if backoffMs > 30000 {
		backoffMs = 30000
	}
	jitterMs := int(float64(backoffMs) * 0.2)
	return time.Duration(backoffMs+jitterMs) * time.Millisecond
}
