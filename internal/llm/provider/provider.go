package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/llm/minimax"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
)

type EventType string

const maxRetries = 8

const (
	EventContentStart  EventType = "content_start"
	EventToolUseStart  EventType = "tool_use_start"
	EventToolUseDelta  EventType = "tool_use_delta"
	EventToolUseStop   EventType = "tool_use_stop"
	EventContentDelta  EventType = "content_delta"
	EventThinkingDelta EventType = "thinking_delta"
	EventContentStop   EventType = "content_stop"
	EventComplete      EventType = "complete"
	EventError         EventType = "error"
)

type TokenUsage struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

type TokenCount struct {
	InputTokens int64
}

var ErrCountTokensUnsupported = errors.New("provider does not support token counting")

func IsCountTokensUnsupported(err error) bool {
	return errors.Is(err, ErrCountTokensUnsupported)
}

func IsPromptTooLong(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	patterns := []string{
		"prompt is too long",
		"prompt too long",
		"context length exceeded",
		"maximum context length",
		"context window exceeded",
		"input is too long",
		"too many tokens",
		"prompt exceeds",
		"reduce the length of the messages",
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

type ProviderResponse struct {
	Content      string
	ToolCalls    []message.ToolCall
	Usage        TokenUsage
	FinishReason message.FinishReason
}

type ProviderEvent struct {
	Type     EventType
	Content  string
	Thinking string
	Response *ProviderResponse
	ToolCall *message.ToolCall
	Error    error
}

type Provider interface {
	SendMessages(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (*ProviderResponse, error)
	StreamResponse(ctx context.Context, messages []message.Message, tools []tools.BaseTool) <-chan ProviderEvent
	CountTokens(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (TokenCount, error)
	Model() models.Model
}

// SystemBlock is a segment of the system prompt with cache metadata.
// Use WithSystemBlocks to pass a slice of these to the provider.
// Static blocks (IsDynamic=false) will receive Anthropic cache_control=ephemeral;
// dynamic blocks will not, allowing frequent content changes without busting the static cache.
type SystemBlock struct {
	Text      string
	IsDynamic bool
}

type SystemPrompt struct {
	Message string
	Blocks  []SystemBlock
}

type providerClientOptions struct {
	providerName  models.ModelProvider
	apiKey        string
	baseURL       string
	profile       string
	authMode      string
	authSource    string
	effectiveAuth string
	model         models.Model
	maxTokens     int64
	systemMessage string
	// SystemBlocks, if non-empty, overrides systemMessage for Anthropic providers.
	// Each block carries cache metadata; static blocks get cache_control, dynamic ones do not.
	SystemBlocks []SystemBlock
	// SystemPromptSource resolves a per-request prompt when request context does not override it.
	SystemPromptSource func() SystemPrompt
}

type ProviderClientOption func(*providerClientOptions)

type ProviderClient interface {
	send(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (*ProviderResponse, error)
	stream(ctx context.Context, messages []message.Message, tools []tools.BaseTool) <-chan ProviderEvent
	countTokens(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (TokenCount, error)
}

type baseProvider[C ProviderClient] struct {
	options providerClientOptions
	client  C
}

type requestSystemPromptCtxKeyType struct{}

var requestSystemPromptCtxKey = requestSystemPromptCtxKeyType{}

func NewProvider(providerName models.ModelProvider, opts ...ProviderClientOption) (Provider, error) {
	clientOptions := providerClientOptions{}
	for _, o := range opts {
		o(&clientOptions)
	}
	spec, ok := models.ProviderSpecByID(providerName)
	if !ok {
		return nil, fmt.Errorf("provider not supported: %s", providerName)
	}
	clientOptions.providerName = providerName
	if clientOptions.model.Provider == "" {
		clientOptions.model.Provider = providerName
	}

	baseURL := clientOptions.baseURL
	if providerName == models.ProviderMiniMax {
		rt, err := minimax.ResolveRuntime(baseURL, clientOptions.profile, clientOptions.authMode)
		if err != nil {
			return nil, err
		}
		baseURL = rt.BaseURL
		clientOptions.effectiveAuth = string(rt.Auth)
	}
	if baseURL == "" && spec.Kind != models.ProviderKindNativeAnthropic && spec.Kind != models.ProviderKindNativeOpenAI {
		baseURL = spec.Endpoints.DefaultBaseURL
	}
	clientOptions.baseURL = baseURL

	switch spec.Kind {
	case models.ProviderKindNativeAnthropic:
		if baseURL != "" {
			return &baseProvider[*anthropicClient]{
				options: clientOptions,
				client:  newAnthropicClientWithBaseURL(clientOptions, baseURL),
			}, nil
		}
		return &baseProvider[*anthropicClient]{
			options: clientOptions,
			client:  newAnthropicClient(clientOptions),
		}, nil
	case models.ProviderKindNativeOpenAI:
		if baseURL != "" {
			return &baseProvider[*openaiClient]{
				options: clientOptions,
				client:  newOpenAIClientWithBaseURL(clientOptions, baseURL),
			}, nil
		}
		return &baseProvider[*openaiClient]{
			options: clientOptions,
			client:  newOpenAIClient(clientOptions),
		}, nil
	case models.ProviderKindAnthropicCompat:
		if baseURL == "" {
			return nil, fmt.Errorf("provider %s requires a base URL", providerName)
		}
		return &baseProvider[*anthropicClient]{
			options: clientOptions,
			client:  newAnthropicClientWithBaseURL(clientOptions, baseURL),
		}, nil
	case models.ProviderKindOpenAICompat, models.ProviderKindLocal, models.ProviderKindRouter:
		if baseURL == "" {
			return nil, fmt.Errorf("provider %s requires a base URL", providerName)
		}
		return &baseProvider[*openaiClient]{
			options: clientOptions,
			client:  newOpenAIClientWithBaseURL(clientOptions, baseURL),
		}, nil
	default:
		return nil, fmt.Errorf("provider %s has unsupported kind %q", providerName, spec.Kind)
	}
}

func (p *baseProvider[C]) cleanMessages(messages []message.Message) (cleaned []message.Message) {
	for _, msg := range messages {
		if len(msg.Parts) == 0 {
			continue
		}
		cleaned = append(cleaned, msg)
	}
	return
}

func (p *baseProvider[C]) SendMessages(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (*ProviderResponse, error) {
	messages = p.cleanMessages(messages)
	resp, err := p.client.send(ctx, messages, tools)
	if err == nil && resp != nil {
		resp.Content = SanitizeContentDelta(resp.Content)
	}
	return resp, err
}

func (p *baseProvider[C]) Model() models.Model {
	return p.options.model
}

func (p *baseProvider[C]) CountTokens(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (TokenCount, error) {
	if p.options.model.Provider != models.ProviderAnthropic {
		return TokenCount{}, fmt.Errorf("%s: %w", p.options.model.Provider, ErrCountTokensUnsupported)
	}
	messages = p.cleanMessages(messages)
	return p.client.countTokens(ctx, messages, tools)
}

func (p *baseProvider[C]) StreamResponse(ctx context.Context, messages []message.Message, tools []tools.BaseTool) <-chan ProviderEvent {
	messages = p.cleanMessages(messages)
	return p.client.stream(ctx, messages, tools)
}

func resolveSystemPrompt(ctx context.Context, options providerClientOptions) SystemPrompt {
	if requestPrompt, ok := ctx.Value(requestSystemPromptCtxKey).(SystemPrompt); ok {
		return requestPrompt
	}
	if options.SystemPromptSource != nil {
		if dynamic := options.SystemPromptSource(); dynamic.Message != "" || len(dynamic.Blocks) > 0 {
			return dynamic
		}
	}
	return SystemPrompt{
		Message: options.systemMessage,
		Blocks:  options.SystemBlocks,
	}
}

func joinSystemBlocks(blocks []SystemBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func WithAPIKey(apiKey string) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.apiKey = apiKey
	}
}

func WithBaseURL(baseURL string) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.baseURL = baseURL
	}
}

func WithProviderProfile(profile string) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.profile = profile
	}
}

func WithProviderAuthMode(authMode string) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.authMode = authMode
	}
}

func WithAuthSource(authSource string) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.authSource = authSource
	}
}

func WithModel(model models.Model) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.model = model
	}
}

func WithMaxTokens(maxTokens int64) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.maxTokens = maxTokens
	}
}

func WithSystemMessage(systemMessage string) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.systemMessage = systemMessage
	}
}

// WithSystemBlocks sets cache-aware system prompt blocks for Anthropic providers.
// When set, these blocks replace systemMessage for the Anthropic API call:
// static blocks receive cache_control, dynamic blocks do not.
// Non-Anthropic providers fall back to concatenating all block texts.
func WithSystemBlocks(blocks []SystemBlock) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.SystemBlocks = blocks
	}
}

func WithSystemPromptSource(fn func() SystemPrompt) ProviderClientOption {
	return func(options *providerClientOptions) {
		options.SystemPromptSource = fn
	}
}

func WithRequestSystemPrompt(ctx context.Context, prompt SystemPrompt) context.Context {
	return context.WithValue(ctx, requestSystemPromptCtxKey, prompt)
}

// CacheAwareProvider is an optional interface implemented by providers that
// expose their CacheMonitor for governance event notifications.
// Use type assertion to check if a provider supports this:
//
//	if cap, ok := p.(CacheAwareProvider); ok {
//	    cap.CacheMonitor().NotifyCompaction()
//	}
type CacheAwareProvider interface {
	CacheMonitor() *CacheMonitor
}

// CacheMonitor returns the Anthropic client's cache monitor for governance notifications.
// This implements CacheAwareProvider for baseProvider[*anthropicClient].
func (p *baseProvider[C]) CacheMonitor() *CacheMonitor {
	// Type-assert the client to access cacheMonitor.
	// Only anthropicClient has a cacheMonitor field.
	type cacheMonitorHolder interface {
		getCacheMonitor() *CacheMonitor
	}
	if cmh, ok := any(p.client).(cacheMonitorHolder); ok {
		return cmh.getCacheMonitor()
	}
	return nil
}
