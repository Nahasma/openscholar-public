package agent

import (
	"fmt"

	"github.com/openscholar/openscholar/internal/message"
)

// MicroCompactConfig configures enhanced micro-compact behavior.
type MicroCompactConfig struct {
	KeepRecent    int // keep recent N tool results (default 5)
	MaxResultSize int // max chars per tool result before truncation (default 8000)
	SummaryMaxLen int // max length for replacement summary (default 200)
}

// DefaultMicroCompactConfig returns default configuration.
func DefaultMicroCompactConfig() MicroCompactConfig {
	return MicroCompactConfig{
		KeepRecent:    5,
		MaxResultSize: 8000,
		SummaryMaxLen: 200,
	}
}

// EnhancedMicroCompact is an improved version of MicroCompact with configurable limits.
// In addition to replacing old tool results with placeholders, it also truncates
// oversized tool results in the "keep recent" window.
//
// Returns the compacted slice and the number of tokens saved (estimated).
func EnhancedMicroCompact(msgs []message.Message, cfg MicroCompactConfig) ([]message.Message, int) {
	if cfg.KeepRecent <= 0 {
		cfg.KeepRecent = 5
	}
	if cfg.MaxResultSize <= 0 {
		cfg.MaxResultSize = 8000
	}
	if cfg.SummaryMaxLen <= 0 {
		cfg.SummaryMaxLen = 200
	}

	toolNames := buildToolNameMap(msgs)

	// Count total tool result parts
	totalToolResults := 0
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			if _, ok := part.(message.ToolResult); ok {
				totalToolResults++
			}
		}
	}

	compressCount := totalToolResults - cfg.KeepRecent
	if compressCount < 0 {
		compressCount = 0
	}

	result := make([]message.Message, 0, len(msgs))
	compressedSoFar := 0
	savedChars := 0

	for _, msg := range msgs {
		newMsg := message.Message{
			ID:        msg.ID,
			Role:      msg.Role,
			SessionID: msg.SessionID,
			Model:     msg.Model,
			CreatedAt: msg.CreatedAt,
			UpdatedAt: msg.UpdatedAt,
			Parts:     make([]message.ContentPart, 0, len(msg.Parts)),
		}

		for _, part := range msg.Parts {
			switch p := part.(type) {
			case message.ToolResult:
				if compressedSoFar < compressCount {
					// Old result — compress
					toolName := toolNames[p.ToolCallID]
					if toolName == "" {
						toolName = "unknown"
					}

					if p.IsError {
						// Error results: summarize but keep
						summary := fmt.Sprintf("[error result kept: tool=%s id=%s] %s",
							toolName, p.ToolCallID, truncateForCompact(p.Content, cfg.SummaryMaxLen))
						savedChars += len(p.Content) - len(summary)
						newMsg.Parts = append(newMsg.Parts, message.ToolResult{
							ToolCallID: p.ToolCallID,
							Name:       p.Name,
							Content:    summary,
							IsError:    true,
						})
					} else {
						placeholder := fmt.Sprintf("[compacted: tool=%s id=%s]", toolName, p.ToolCallID)
						savedChars += len(p.Content) - len(placeholder)
						newMsg.Parts = append(newMsg.Parts, message.ToolResult{
							ToolCallID: p.ToolCallID,
							Name:       p.Name,
							Content:    placeholder,
							IsError:    false,
						})
					}
					compressedSoFar++
				} else if len(p.Content) > cfg.MaxResultSize {
					// Recent but oversized — truncate
					truncated := p.Content[:cfg.MaxResultSize] + fmt.Sprintf("\n... [truncated %d chars]", len(p.Content)-cfg.MaxResultSize)
					savedChars += len(p.Content) - len(truncated)
					newMsg.Parts = append(newMsg.Parts, message.ToolResult{
						ToolCallID: p.ToolCallID,
						Name:       p.Name,
						Content:    truncated,
						Metadata:   p.Metadata,
						IsError:    p.IsError,
					})
				} else {
					newMsg.Parts = append(newMsg.Parts, p)
				}
			default:
				newMsg.Parts = append(newMsg.Parts, part)
			}
		}

		result = append(result, newMsg)
	}

	tokensSaved := savedChars / 4 // rough estimate
	return result, tokensSaved
}
