package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openscholar/openscholar/internal/llm/models"
	toolsPkg "github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/message"
)

type anthropicClient struct {
	providerOptions providerClientOptions
	client          anthropic.Client
	cacheMonitor    *CacheMonitor
}

func newAnthropicClient(opts providerClientOptions) *anthropicClient {
	clientOptions := []option.RequestOption{}
	if opts.apiKey != "" {
		if opts.providerName == models.ProviderMiniMax && opts.effectiveAuth == "bearer" {
			clientOptions = append(clientOptions, option.WithAuthToken(opts.apiKey))
		} else {
			clientOptions = append(clientOptions, option.WithAPIKey(opts.apiKey))
		}
	}

	return &anthropicClient{
		providerOptions: opts,
		client:          anthropic.NewClient(clientOptions...),
		cacheMonitor:    NewCacheMonitorWithProvider(string(opts.model.Provider), string(opts.model.ID), opts.model.Provider == models.ProviderAnthropic),
	}
}

func newAnthropicClientWithBaseURL(opts providerClientOptions, baseURL string) *anthropicClient {
	clientOptions := []option.RequestOption{
		option.WithBaseURL(baseURL),
	}
	if opts.apiKey != "" {
		if opts.providerName == models.ProviderMiniMax && opts.effectiveAuth == "bearer" {
			clientOptions = append(clientOptions, option.WithAuthToken(opts.apiKey))
		} else {
			clientOptions = append(clientOptions, option.WithAPIKey(opts.apiKey))
		}
	}

	return &anthropicClient{
		providerOptions: opts,
		client:          anthropic.NewClient(clientOptions...),
		cacheMonitor:    NewCacheMonitorWithProvider(string(opts.model.Provider), string(opts.model.ID), opts.model.Provider == models.ProviderAnthropic),
	}
}

func (a *anthropicClient) convertMessages(messages []message.Message) (anthropicMessages []anthropic.MessageParam) {
	emittedToolUseIDs := make(map[string]bool)

	for _, msg := range messages {
		switch msg.Role {
		case message.User:
			var blocks []anthropic.ContentBlockParamUnion
			for _, part := range msg.Parts {
				switch p := part.(type) {
				case message.TextContent:
					if p.Text != "" {
						blocks = append(blocks, anthropic.NewTextBlock(p.Text))
					}
				case message.BinaryContent:
					if strings.HasPrefix(p.MIMEType, "application/pdf") {
						blocks = append(blocks, anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{
							Data:      p.String(models.ProviderAnthropic),
							MediaType: "application/pdf",
						}))
					} else {
						blocks = append(blocks, anthropic.NewImageBlockBase64(
							p.MIMEType, p.String(models.ProviderAnthropic),
						))
					}
				}
			}
			if len(blocks) == 0 {
				content := anthropic.NewTextBlock(msg.Content().String())
				blocks = append(blocks, content)
			}
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(blocks...))

		case message.Assistant:
			blocks := []anthropic.ContentBlockParamUnion{}
			if msg.Content().String() != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content().String()))
			}
			for _, toolCall := range msg.ToolCalls() {
				var inputMap map[string]any
				dec := json.NewDecoder(strings.NewReader(toolCall.Input))
				dec.UseNumber() // preserve number precision for large integers
				if err := dec.Decode(&inputMap); err != nil {
					continue
				}
				emittedToolUseIDs[toolCall.ID] = true
				blocks = append(blocks, anthropic.NewToolUseBlock(toolCall.ID, inputMap, toolCall.Name))
			}
			if len(blocks) == 0 {
				continue
			}
			anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(blocks...))

		case message.Tool:
			var results []anthropic.ContentBlockParamUnion
			for _, toolResult := range msg.ToolResults() {
				if !emittedToolUseIDs[toolResult.ToolCallID] {
					continue
				}
				results = append(results, anthropic.NewToolResultBlock(toolResult.ToolCallID, toolResult.Content, toolResult.IsError))
			}
			if len(results) == 0 {
				continue
			}
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(results...))
		}
	}

	// Add cache breakpoints to last 2 user messages for incremental caching
	addCacheBreakpoints(anthropicMessages, 2)

	return
}

// addCacheBreakpoints sets cache_control on the last n user-role messages.
// This enables Anthropic to cache the conversation history up to these breakpoints,
// so subsequent requests only pay full price for new messages.
func addCacheBreakpoints(messages []anthropic.MessageParam, n int) {
	if len(messages) == 0 || n <= 0 {
		return
	}

	count := 0
	for i := len(messages) - 1; i >= 0 && count < n; i-- {
		if messages[i].Role != "user" {
			continue
		}

		blocks := messages[i].Content
		if len(blocks) == 0 {
			continue
		}

		// Set cache_control on the last content block of this user message
		last := &blocks[len(blocks)-1]
		switch {
		case last.OfText != nil:
			last.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
		case last.OfToolResult != nil:
			last.OfToolResult.CacheControl = anthropic.NewCacheControlEphemeralParam()
		case last.OfDocument != nil:
			last.OfDocument.CacheControl = anthropic.NewCacheControlEphemeralParam()
		case last.OfImage != nil:
			last.OfImage.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		count++
	}
}

func (a *anthropicClient) convertTools(tools []toolsPkg.BaseTool) []anthropic.ToolUnionParam {
	anthropicTools := make([]anthropic.ToolUnionParam, len(tools))
	for i, tool := range tools {
		info := tool.Info()

		// info.Parameters is a full JSON Schema (with "type", "properties", "required").
		// ToolInputSchemaParam.Properties expects only the properties object,
		// and Required is passed separately.
		properties, _ := info.Parameters["properties"]
		var required []string
		if req, ok := info.Parameters["required"]; ok {
			if reqSlice, ok := req.([]string); ok {
				required = reqSlice
			} else if reqIface, ok := req.([]interface{}); ok {
				for _, v := range reqIface {
					if s, ok := v.(string); ok {
						required = append(required, s)
					}
				}
			}
		}

		toolParam := anthropic.ToolParam{
			Name:        info.Name,
			Description: anthropic.String(info.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
				Required:   required,
			},
		}

		// Apply cache_control to the last tool to cache the entire tool schema block.
		// The tool list is stable (sorted by name in DeferredRegistry.ActiveTools),
		// so this maximises cache hit rate.
		if i == len(tools)-1 {
			toolParam.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}

		anthropicTools[i] = anthropic.ToolUnionParam{OfTool: &toolParam}
	}
	return anthropicTools
}

func (a *anthropicClient) convertCountTokensTools(tools []toolsPkg.BaseTool) []anthropic.MessageCountTokensToolUnionParam {
	tokenTools := make([]anthropic.MessageCountTokensToolUnionParam, len(tools))
	for i, tool := range tools {
		info := tool.Info()

		properties, _ := info.Parameters["properties"]
		var required []string
		if req, ok := info.Parameters["required"]; ok {
			if reqSlice, ok := req.([]string); ok {
				required = reqSlice
			} else if reqIface, ok := req.([]interface{}); ok {
				for _, v := range reqIface {
					if s, ok := v.(string); ok {
						required = append(required, s)
					}
				}
			}
		}

		toolParam := anthropic.ToolParam{
			Name:        info.Name,
			Description: anthropic.String(info.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
				Required:   required,
			},
		}
		if i == len(tools)-1 {
			toolParam.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}

		tokenTools[i] = anthropic.MessageCountTokensToolUnionParam{OfTool: &toolParam}
	}
	return tokenTools
}

func (a *anthropicClient) finishReason(reason string) message.FinishReason {
	switch reason {
	case "end_turn":
		return message.FinishReasonEndTurn
	case "max_tokens":
		return message.FinishReasonMaxTokens
	case "tool_use":
		return message.FinishReasonToolUse
	default:
		return message.FinishReasonUnknown
	}
}

// buildSystemBlocks constructs the Anthropic system prompt block list.
// If SystemBlocks are configured, each block is converted to a TextBlockParam:
//   - static blocks (IsDynamic=false) get cache_control=ephemeral
//   - dynamic blocks (IsDynamic=true) have no cache_control
//
// Falls back to a single cached block from systemMessage when no blocks are configured.
func (a *anthropicClient) buildSystemBlocks(ctx context.Context) []anthropic.TextBlockParam {
	systemPrompt := resolveSystemPrompt(ctx, a.providerOptions)
	if len(systemPrompt.Blocks) > 0 {
		result := make([]anthropic.TextBlockParam, 0, len(systemPrompt.Blocks))
		for _, block := range systemPrompt.Blocks {
			if block.Text == "" {
				continue
			}
			tb := anthropic.TextBlockParam{Text: block.Text}
			if !block.IsDynamic {
				tb.CacheControl = anthropic.NewCacheControlEphemeralParam()
			}
			result = append(result, tb)
		}
		return result
	}

	if systemPrompt.Message == "" {
		return nil
	}

	// Fallback: single block with cache_control (legacy / non-Anthropic-split path)
	return []anthropic.TextBlockParam{
		{
			Text:         systemPrompt.Message,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		},
	}
}

func (a *anthropicClient) preparedMessages(ctx context.Context, messages []anthropic.MessageParam, tools []anthropic.ToolUnionParam) anthropic.MessageNewParams {
	temperature := 0.0
	if a.providerOptions.providerName == models.ProviderMiniMax {
		// MiniMax documents Anthropic-compatible temperature as (0.0, 1.0].
		temperature = 1.0
	}
	return anthropic.MessageNewParams{
		Model:       anthropic.Model(a.providerOptions.model.APIModel),
		MaxTokens:   a.providerOptions.maxTokens,
		Temperature: anthropic.Float(temperature),
		Messages:    messages,
		Tools:       tools,
		System:      a.buildSystemBlocks(ctx),
	}
}

func (a *anthropicClient) send(ctx context.Context, messages []message.Message, tools []toolsPkg.BaseTool) (*ProviderResponse, error) {
	convertedTools := a.convertTools(tools)
	preparedMessages := a.preparedMessages(ctx, a.convertMessages(messages), convertedTools)

	// Record pre-call state for cache monitoring
	systemTexts := make([]string, 0, len(preparedMessages.System))
	for _, b := range preparedMessages.System {
		systemTexts = append(systemTexts, b.Text)
	}
	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		toolNames = append(toolNames, t.Info().Name)
	}
	a.cacheMonitor.PreCall(systemTexts, toolNames, string(preparedMessages.Model))

	attempts := 0
	for {
		attempts++
		resp, err := a.client.Messages.New(ctx, preparedMessages)
		if err != nil {
			retry, after, retryErr := a.shouldRetry(attempts, err)
			if retryErr != nil {
				return nil, a.wrapMiniMax401Error(retryErr)
			}
			if retry {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(after) * time.Millisecond):
					continue
				}
			}
			return nil, a.wrapMiniMax401Error(err)
		}

		usage := a.usage(*resp)
		a.cacheMonitor.PostCall(int(usage.CacheReadTokens))

		content := ""
		for _, block := range resp.Content {
			if text, ok := block.AsAny().(anthropic.TextBlock); ok {
				content += text.Text
			}
		}

		toolCalls := a.toolCalls(*resp)
		// Normalize tool call inputs to ensure valid JSON objects
		var validToolCalls []message.ToolCall
		for _, tc := range toolCalls {
			if normalized, ok := message.NormalizeToolCallInput(tc.Input); ok {
				tc.Input = normalized
				validToolCalls = append(validToolCalls, tc)
			}
		}

		return &ProviderResponse{
			Content:      content,
			ToolCalls:    validToolCalls,
			Usage:        usage,
			FinishReason: a.finishReason(string(resp.StopReason)),
		}, nil
	}
}

func (a *anthropicClient) countTokens(ctx context.Context, messages []message.Message, tools []toolsPkg.BaseTool) (TokenCount, error) {
	params := anthropic.MessageCountTokensParams{
		Model:    anthropic.Model(a.providerOptions.model.APIModel),
		Messages: a.convertMessages(messages),
		Tools:    a.convertCountTokensTools(tools),
	}
	systemBlocks := a.buildSystemBlocks(ctx)
	if len(systemBlocks) > 0 {
		params.System = anthropic.MessageCountTokensParamsSystemUnion{
			OfTextBlockArray: systemBlocks,
		}
	}

	resp, err := a.client.Messages.CountTokens(ctx, params)
	if err != nil {
		return TokenCount{}, err
	}
	return TokenCount{InputTokens: resp.InputTokens}, nil
}

func (a *anthropicClient) stream(ctx context.Context, messages []message.Message, tools []toolsPkg.BaseTool) <-chan ProviderEvent {
	convertedTools := a.convertTools(tools)
	preparedMessages := a.preparedMessages(ctx, a.convertMessages(messages), convertedTools)

	// Record pre-call state for cache monitoring
	systemTexts := make([]string, 0, len(preparedMessages.System))
	for _, b := range preparedMessages.System {
		systemTexts = append(systemTexts, b.Text)
	}
	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		toolNames = append(toolNames, t.Info().Name)
	}
	a.cacheMonitor.PreCall(systemTexts, toolNames, string(preparedMessages.Model))

	attempts := 0
	eventChan := make(chan ProviderEvent)
	go func() {
		for {
			attempts++
			anthropicStream := a.client.Messages.NewStreaming(ctx, preparedMessages)
			accumulatedMessage := anthropic.Message{}
			currentToolCallID := ""
			eventsEmitted := false

			for anthropicStream.Next() {
				event := anthropicStream.Current()
				if err := accumulatedMessage.Accumulate(event); err != nil {
					log.Printf("Error accumulating message: %v", err)
					continue
				}

				switch event := event.AsAny().(type) {
				case anthropic.ContentBlockStartEvent:
					if event.ContentBlock.Type == "text" {
						eventsEmitted = true
						eventChan <- ProviderEvent{Type: EventContentStart}
					} else if event.ContentBlock.Type == "tool_use" {
						currentToolCallID = event.ContentBlock.ID
						eventsEmitted = true
						eventChan <- ProviderEvent{
							Type: EventToolUseStart,
							ToolCall: &message.ToolCall{
								ID:   event.ContentBlock.ID,
								Name: event.ContentBlock.Name,
							},
						}
					}

				case anthropic.ContentBlockDeltaEvent:
					if event.Delta.Type == "thinking_delta" && event.Delta.Thinking != "" {
						eventsEmitted = true
						eventChan <- ProviderEvent{
							Type:     EventThinkingDelta,
							Thinking: event.Delta.Thinking,
						}
					} else if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
						eventsEmitted = true
						eventChan <- ProviderEvent{
							Type:    EventContentDelta,
							Content: SanitizeContentDelta(event.Delta.Text),
						}
					} else if event.Delta.Type == "input_json_delta" && currentToolCallID != "" {
						eventsEmitted = true
						eventChan <- ProviderEvent{
							Type: EventToolUseDelta,
							ToolCall: &message.ToolCall{
								ID:    currentToolCallID,
								Input: event.Delta.PartialJSON,
							},
						}
					}

				case anthropic.ContentBlockStopEvent:
					eventsEmitted = true
					if currentToolCallID != "" {
						eventChan <- ProviderEvent{
							Type:     EventToolUseStop,
							ToolCall: &message.ToolCall{ID: currentToolCallID},
						}
						currentToolCallID = ""
					} else {
						eventChan <- ProviderEvent{Type: EventContentStop}
					}

				case anthropic.MessageStopEvent:
					content := ""
					for _, block := range accumulatedMessage.Content {
						if text, ok := block.AsAny().(anthropic.TextBlock); ok {
							content += text.Text
						}
					}
					usage := a.usage(accumulatedMessage)
					a.cacheMonitor.PostCall(int(usage.CacheReadTokens))
					eventsEmitted = true
					eventChan <- ProviderEvent{
						Type: EventComplete,
						Response: &ProviderResponse{
							Content:      content,
							ToolCalls:    a.toolCalls(accumulatedMessage),
							Usage:        usage,
							FinishReason: a.finishReason(string(accumulatedMessage.StopReason)),
						},
					}
				}
			}

			err := anthropicStream.Err()
			if err == nil || errors.Is(err, io.EOF) {
				close(eventChan)
				return
			}

			// For non-API errors (JSON parse errors, network errors),
			// safe to retry if no events have been emitted to the consumer yet
			if !eventsEmitted && !errors.Is(err, context.Canceled) && attempts <= maxRetries {
				var apierr *anthropic.Error
				if !errors.As(err, &apierr) {
					log.Printf("Stream error (attempt %d/%d), retrying: %v", attempts, maxRetries, err)
					continue
				}
			}

			retry, after, retryErr := a.shouldRetryStreamError(attempts, eventsEmitted, err)
			if retryErr != nil {
				eventChan <- ProviderEvent{Type: EventError, Error: a.wrapMiniMax401Error(retryErr)}
				close(eventChan)
				return
			}
			if retry {
				select {
				case <-ctx.Done():
					if ctx.Err() != nil {
						eventChan <- ProviderEvent{Type: EventError, Error: ctx.Err()}
					}
					close(eventChan)
					return
				case <-time.After(time.Duration(after) * time.Millisecond):
					continue
				}
			}

			if ctx.Err() != nil {
				eventChan <- ProviderEvent{Type: EventError, Error: ctx.Err()}
			}
			if ctx.Err() == nil {
				eventChan <- ProviderEvent{Type: EventError, Error: a.wrapMiniMax401Error(err)}
			}
			close(eventChan)
			return
		}
	}()
	return eventChan
}

func (a *anthropicClient) shouldRetry(attempts int, err error) (bool, int64, error) {
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		return false, 0, err
	}
	if apierr.StatusCode != 429 && apierr.StatusCode != 529 {
		return false, 0, err
	}
	if attempts > maxRetries {
		return false, 0, fmt.Errorf("maximum retry attempts reached: %d", maxRetries)
	}
	backoffMs := 2000 * (1 << (attempts - 1))
	jitterMs := int(float64(backoffMs) * 0.2)
	retryMs := backoffMs + jitterMs
	return true, int64(retryMs), nil
}

func (a *anthropicClient) shouldRetryStreamError(attempts int, eventsEmitted bool, err error) (bool, int64, error) {
	if eventsEmitted {
		return false, 0, err
	}
	return a.shouldRetry(attempts, err)
}

func (a *anthropicClient) toolCalls(msg anthropic.Message) []message.ToolCall {
	var toolCalls []message.ToolCall
	for _, block := range msg.Content {
		if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			toolCalls = append(toolCalls, message.ToolCall{
				ID:       variant.ID,
				Name:     variant.Name,
				Input:    string(variant.Input),
				Type:     string(variant.Type),
				Finished: true,
			})
		}
	}
	return toolCalls
}

func (a *anthropicClient) usage(msg anthropic.Message) TokenUsage {
	return TokenUsage{
		InputTokens:         msg.Usage.InputTokens,
		OutputTokens:        msg.Usage.OutputTokens,
		CacheCreationTokens: msg.Usage.CacheCreationInputTokens,
		CacheReadTokens:     msg.Usage.CacheReadInputTokens,
	}
}

// getCacheMonitor returns the cache monitor for this client.
func (a *anthropicClient) getCacheMonitor() *CacheMonitor {
	return a.cacheMonitor
}

func (a *anthropicClient) wrapMiniMax401Error(err error) error {
	if err == nil || a.providerOptions.providerName != models.ProviderMiniMax {
		return err
	}
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) || apierr.StatusCode != 401 {
		return err
	}

	requestID := ""
	if apierr.Response != nil {
		requestID = extractRequestID(apierr.Response.Header, []byte(apierr.RawJSON()))
	}
	authMode := a.providerOptions.effectiveAuth
	if authMode == "" {
		authMode = "anthropic_x_api_key"
	}
	recommendation := minimax401Recommendation(a.providerOptions.baseURL, a.providerOptions.profile, authMode, a.providerOptions.authSource)
	return &miniMaxAuthError{cause: err, message: fmt.Sprintf(
		"minimax auth failed (401): provider=%s model=%s baseURL=%s auth_source=%s auth_mode=%s request_id=%s recommendation=%s cause=%s",
		a.providerOptions.providerName,
		a.providerOptions.model.ID,
		a.providerOptions.baseURL,
		defaultString(a.providerOptions.authSource, "unknown"),
		authMode,
		defaultString(requestID, "n/a"),
		recommendation,
		redactSecrets(err.Error(), a.providerOptions.apiKey),
	)}
}

type miniMaxAuthError struct {
	message string
	cause   error
}

func (e *miniMaxAuthError) Error() string {
	return e.message
}

func (e *miniMaxAuthError) Unwrap() error {
	return e.cause
}

func (e *miniMaxAuthError) UserMessage() string {
	return "MiniMax authentication failed (401). Press Ctrl+O for details."
}

func (e *miniMaxAuthError) Detail() string {
	return e.message
}

func minimax401Recommendation(baseURL, profile, authMode, authSource string) string {
	lowerBase := strings.ToLower(strings.TrimSpace(baseURL))
	lowerProfile := strings.ToLower(strings.TrimSpace(profile))
	auth := strings.ToLower(strings.TrimSpace(authMode))
	switch {
	case (strings.Contains(lowerBase, "api.minimaxi.com") || lowerProfile == "token-plan") && auth == "anthropic_x_api_key":
		rec := "MiniMax Token Plan endpoint requires bearer auth; set authMode=bearer or remove the MiniMax authMode override, then compare with `openscholar doctor --providers` or `/doctor providers`"
		if strings.EqualFold(authSource, "env") {
			rec += "; note: env key overrides config key"
		}
		return rec
	case strings.Contains(lowerBase, "api.minimax.io"):
		rec := "deprecated MiniMax endpoint returned 401; use https://api.minimaxi.com/anthropic with bearer auth unless you intentionally configured a legacy profile"
		if strings.EqualFold(authSource, "env") {
			rec += "; note: env key overrides config key"
		}
		return rec
	default:
		rec := "verify key/source/auth mode and endpoint alignment for MiniMax"
		if strings.EqualFold(authSource, "env") {
			rec += "; note: env key overrides config key"
		}
		return rec
	}
}

func extractRequestID(headers map[string][]string, body []byte) string {
	get := func(key string) string {
		for k, vals := range headers {
			if strings.EqualFold(k, key) && len(vals) > 0 {
				if trimmed := strings.TrimSpace(vals[0]); trimmed != "" {
					return trimmed
				}
			}
		}
		return ""
	}
	for _, key := range []string{"x-request-id", "request-id", "x-amzn-requestid", "x-amz-request-id"} {
		if v := get(key); v != "" {
			return v
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"request_id", "requestId", "req_id"} {
		if val, ok := payload[key].(string); ok {
			if trimmed := strings.TrimSpace(val); trimmed != "" {
				return trimmed
			}
		}
	}
	if errObj, ok := payload["error"].(map[string]any); ok {
		for _, key := range []string{"request_id", "requestId", "req_id"} {
			if val, ok := errObj[key].(string); ok {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func redactSecrets(input, apiKey string) string {
	out := input
	if apiKey != "" {
		out = strings.ReplaceAll(out, apiKey, "[REDACTED]")
	}
	return out
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
