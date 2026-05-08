package web

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// AnthropicSearchBackend executes web searches via Anthropic's
// web_search_20250305 server tool by making an independent Messages API call.
type AnthropicSearchBackend struct {
	client anthropic.Client
	model  string
}

// NewAnthropicSearch creates an Anthropic search backend.
func NewAnthropicSearch(client anthropic.Client, model string) *AnthropicSearchBackend {
	return &AnthropicSearchBackend{client: client, model: model}
}

// SearchWeb implements SearchProvider by calling the Anthropic Messages API
// with the web_search_20250305 server tool and parsing the response blocks.
func (b *AnthropicSearchBackend) SearchWeb(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	start := time.Now()

	maxUses := int64(8)
	if opts.MaxUses > 0 {
		maxUses = int64(opts.MaxUses)
	}

	toolParam := anthropic.WebSearchTool20250305Param{
		MaxUses:        param.NewOpt(maxUses),
		AllowedDomains: opts.AllowedDomains,
		BlockedDomains: opts.BlockedDomains,
	}

	resp, err := b.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(b.model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: "You are an assistant for performing a web search tool use"},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock("Perform a web search for the query: " + query),
			),
		},
		Tools: []anthropic.ToolUnionParam{
			{OfWebSearchTool20250305: &toolParam},
		},
	})
	if err != nil {
		return nil, &SearchError{
			Provider: "anthropic",
			Kind:     SearchErrTemporary,
			Cause:    fmt.Errorf("anthropic web search: %w", err),
		}
	}

	var hits []SearchHit
	var summaryAcc string

	for _, block := range resp.Content {
		switch block.Type {
		case "server_tool_use":
			// Skip — internal tool invocation marker.

		case "web_search_tool_result":
			wb := block.AsWebSearchToolResult()
			content := wb.Content
			// Check if it's an error result (no array of blocks).
			if content.JSON.OfWebSearchResultBlockArray.Valid() {
				for _, r := range content.AsWebSearchResultBlockArray() {
					hits = append(hits, SearchHit{Title: r.Title, URL: r.URL})
				}
			} else {
				// Error variant.
				errResult := content.AsResponseWebSearchToolResultError()
				summaryAcc += fmt.Sprintf("Web search error: %s\n", errResult.ErrorCode)
			}

		case "text":
			if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
				summaryAcc += tb.Text
			}
		}
	}

	return &SearchResult{
		Hits:     hits,
		Summary:  summaryAcc,
		Duration: time.Since(start).Seconds(),
		Backend:  "anthropic",
	}, nil
}

// SupportsFilter reports that this backend supports domain allow/block lists.
func (b *AnthropicSearchBackend) SupportsFilter() bool { return true }
