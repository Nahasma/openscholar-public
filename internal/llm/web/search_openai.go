package web

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openai/openai-go"
)

// OpenAISearchBackend executes web searches via OpenAI Chat Completions with
// the web_search_options parameter.
type OpenAISearchBackend struct {
	client openai.Client
	model  string // e.g. "gpt-4o-search-preview"
}

// NewOpenAISearch creates an OpenAI search backend.
func NewOpenAISearch(client openai.Client, model string) *OpenAISearchBackend {
	return &OpenAISearchBackend{client: client, model: model}
}

// SearchWeb implements SearchProvider by calling OpenAI Chat Completions with
// web_search_options enabled.  Domain filtering is not supported by OpenAI —
// if opts specifies any domain filters an error is returned.
func (b *OpenAISearchBackend) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	if len(opts.AllowedDomains) > 0 || len(opts.BlockedDomains) > 0 {
		return nil, &SearchError{
			Provider: "openai",
			Kind:     SearchErrTemporary,
			Cause:    errors.New("openai web search does not support domain filtering"),
		}
	}

	start := time.Now()

	params := openai.ChatCompletionNewParams{
		Model: b.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(query),
		},
		WebSearchOptions: openai.ChatCompletionNewParamsWebSearchOptions{
			SearchContextSize: "medium",
		},
	}

	resp, err := b.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, &SearchError{
			Provider: "openai",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("openai web search: %w", err),
		}
	}

	var summary string
	var hits []SearchHit

	if len(resp.Choices) > 0 {
		msg := resp.Choices[0].Message
		summary = msg.Content

		for _, ann := range msg.Annotations {
			uc := ann.URLCitation
			if uc.URL != "" {
				hits = append(hits, SearchHit{
					Title: uc.Title,
					URL:   uc.URL,
				})
			}
		}
	}

	return &SearchResult{
		Hits:     hits,
		Summary:  summary,
		Duration: time.Since(start).Seconds(),
		Backend:  "openai",
	}, nil
}

// SupportsFilter reports that this backend does not support domain filtering.
func (b *OpenAISearchBackend) SupportsFilter() bool { return false }
