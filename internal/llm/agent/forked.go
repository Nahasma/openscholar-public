package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Nahasma/openscholar-public/internal/llm/provider"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
)

// ForkedRunOptions configures fork behavior.
type ForkedRunOptions struct {
	Label            string              // used for logging and debugging (e.g. "session-memory")
	Prompt           string              // user prompt sent to LLM
	ParentSessionID  string
	Snapshot         *CacheSafeSnapshot
	MaxTurns         int                 // max tool call rounds (default 3)
	MaxTokens        int                 // output token cap (default 1024)
	Timeout          time.Duration       // timeout (default 30s)
	AllowedToolNames map[string]struct{} // tool whitelist (nil = no tools)
	PersistMessages  bool                // whether to write to message DB (default false)
	OnComplete       func(ForkedRunResult)
}

// ForkedRunResult holds the result of a forked run.
type ForkedRunResult struct {
	FinalMessage message.Message
	Messages     []message.Message  // full message sequence (not persisted)
	Usage        provider.TokenUsage
	Turns        int
	Err          error
}

// ForkedRunner manages snapshot-based background LLM query runners.
type ForkedRunner interface {
	SaveSnapshot(sessionID string, snapshot CacheSafeSnapshot)
	LatestSnapshot(sessionID string) (CacheSafeSnapshot, bool)
	Run(ctx context.Context, opts ForkedRunOptions) (ForkedRunResult, error)
}

type forkedRunner struct {
	snapshots       sync.Map // map[string]CacheSafeSnapshot
	providerFactory func() (provider.Provider, error)
	logger          *slog.Logger
}

// NewForkedRunner creates a ForkedRunner instance.
func NewForkedRunner(
	providerFactory func() (provider.Provider, error),
	logger *slog.Logger,
) ForkedRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &forkedRunner{
		providerFactory: providerFactory,
		logger:          logger,
	}
}

// SaveSnapshot stores the latest snapshot for a session.
func (r *forkedRunner) SaveSnapshot(sessionID string, snapshot CacheSafeSnapshot) {
	r.snapshots.Store(sessionID, snapshot)
}

// LatestSnapshot retrieves the most recent snapshot for a session.
func (r *forkedRunner) LatestSnapshot(sessionID string) (CacheSafeSnapshot, bool) {
	val, ok := r.snapshots.Load(sessionID)
	if !ok {
		return CacheSafeSnapshot{}, false
	}
	return val.(CacheSafeSnapshot), true
}

// Run executes a forked LLM query using the provided options.
func (r *forkedRunner) Run(ctx context.Context, opts ForkedRunOptions) (ForkedRunResult, error) {
	// Apply defaults
	if opts.MaxTurns <= 0 {
		opts.MaxTurns = 3
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 1024
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}

	// Create context with timeout
	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	label := opts.Label
	if label == "" {
		label = "forked"
	}

	r.logger.Debug("forked run starting",
		"label", label,
		"parent_session", opts.ParentSessionID,
		"max_turns", opts.MaxTurns,
	)

	// Create new provider instance
	prov, err := r.providerFactory()
	if err != nil {
		return ForkedRunResult{Err: fmt.Errorf("forked[%s]: provider creation failed: %w", label, err)},
			fmt.Errorf("forked[%s]: provider creation failed: %w", label, err)
	}

	// Build initial message list
	var msgs []message.Message
	if opts.Snapshot != nil {
		msgs = deepCopyMessages(opts.Snapshot.MessagePrefix)
	}

	// Append user prompt message
	userMsg := message.Message{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: opts.Prompt}},
	}
	msgs = append(msgs, userMsg)

	// Filter tools to whitelist
	var activeTools []tools.BaseTool
	if opts.Snapshot != nil && opts.AllowedToolNames != nil {
		for _, t := range opts.Snapshot.ActiveTools {
			if _, ok := opts.AllowedToolNames[t.Info().Name]; ok {
				activeTools = append(activeTools, t)
			}
		}
	}

	// Build tool lookup map for execution
	toolMap := make(map[string]tools.BaseTool, len(activeTools))
	for _, t := range activeTools {
		toolMap[t.Info().Name] = t
	}

	var totalUsage provider.TokenUsage
	var finalMsg message.Message
	turns := 0

	for turns < opts.MaxTurns {
		select {
		case <-runCtx.Done():
			err := runCtx.Err()
			r.logger.Debug("forked run context done", "label", label, "err", err)
			result := ForkedRunResult{
				FinalMessage: finalMsg,
				Messages:     msgs,
				Usage:        totalUsage,
				Turns:        turns,
				Err:          err,
			}
			if opts.OnComplete != nil {
				opts.OnComplete(result)
			}
			return result, err
		default:
		}

		resp, err := prov.SendMessages(runCtx, msgs, activeTools)
		if err != nil {
			r.logger.Debug("forked run SendMessages error", "label", label, "err", err)
			result := ForkedRunResult{
				FinalMessage: finalMsg,
				Messages:     msgs,
				Usage:        totalUsage,
				Turns:        turns,
				Err:          fmt.Errorf("forked[%s]: SendMessages failed: %w", label, err),
			}
			if opts.OnComplete != nil {
				opts.OnComplete(result)
			}
			return result, result.Err
		}

		// Accumulate usage
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens
		totalUsage.CacheCreationTokens += resp.Usage.CacheCreationTokens
		totalUsage.CacheReadTokens += resp.Usage.CacheReadTokens

		// Build assistant message from response
		assistantParts := []message.ContentPart{}
		if resp.Content != "" {
			assistantParts = append(assistantParts, message.TextContent{Text: resp.Content})
		}
		for _, tc := range resp.ToolCalls {
			assistantParts = append(assistantParts, tc)
		}
		assistantParts = append(assistantParts, message.Finish{Reason: resp.FinishReason})

		assistantMsg := message.Message{
			Role:  message.Assistant,
			Parts: assistantParts,
		}
		msgs = append(msgs, assistantMsg)
		finalMsg = assistantMsg

		// If LLM wants to use tools, execute them
		if resp.FinishReason != message.FinishReasonToolUse || len(resp.ToolCalls) == 0 {
			break
		}

		turns++

		// Execute tool calls
		toolResults := make([]message.ContentPart, 0, len(resp.ToolCalls))
		for _, tc := range resp.ToolCalls {
			var tr message.ToolResult
			t, ok := toolMap[tc.Name]
			if !ok {
				tr = message.ToolResult{
					ToolCallID: tc.ID,
					Name:       tc.Name,
					Content:    fmt.Sprintf("tool '%s' is not in the allowed tool whitelist", tc.Name),
					IsError:    true,
				}
			} else {
				toolResp, toolErr := t.Run(runCtx, tools.ToolCall{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Input,
				})
				if toolErr != nil {
					tr = message.ToolResult{
						ToolCallID: tc.ID,
						Name:       tc.Name,
						Content:    toolErr.Error(),
						IsError:    true,
					}
				} else {
					tr = message.ToolResult{
						ToolCallID: tc.ID,
						Name:       tc.Name,
						Content:    toolResp.Content,
						Metadata:   toolResp.Metadata,
						IsError:    toolResp.IsError,
					}
				}
			}
			toolResults = append(toolResults, tr)
		}

		// Append tool results as a user (tool) message
		toolMsg := message.Message{
			Role:  message.Tool,
			Parts: toolResults,
		}
		msgs = append(msgs, toolMsg)
	}

	result := ForkedRunResult{
		FinalMessage: finalMsg,
		Messages:     msgs,
		Usage:        totalUsage,
		Turns:        turns,
	}

	r.logger.Debug("forked run complete",
		"label", label,
		"turns", turns,
		"output_tokens", totalUsage.OutputTokens,
	)

	if opts.OnComplete != nil {
		opts.OnComplete(result)
	}

	return result, nil
}

// deepCopyMessages is defined in compact.go — reused here.
