package tui

import (
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/openscholar/openscholar/internal/message"
)

// CodeBlock represents a fenced code block extracted from markdown.
type CodeBlock struct {
	Language string
	Content  string
}

// clearCopyFeedbackMsg clears the transient footer notice after a delay.
type clearCopyFeedbackMsg struct{}

// clearStatusAfterDelay returns a command that clears the transient footer notice after 2 seconds.
func clearStatusAfterDelay() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return clearCopyFeedbackMsg{}
	})
}

// copyToClipboard strips ANSI escape codes and writes text to the system clipboard.
func copyToClipboard(text string) string {
	cleaned := xansi.Strip(text)
	if err := clipboard.WriteAll(cleaned); err != nil {
		return "Failed to copy: " + err.Error()
	}
	return ""
}

// extractLastAssistantText returns the plain text of the last assistant message.
func (m *Model) extractLastAssistantText() string {
	for i := len(m.chat.messages) - 1; i >= 0; i-- {
		if m.chat.messages[i].Role == message.Assistant {
			return m.chat.messages[i].Content().Text
		}
	}
	return ""
}

// extractFocusedContent extracts the text content of the currently focused item.
func (m *Model) extractFocusedContent() string {
	if m.chat.focusedToolCallID == "" {
		return ""
	}

	for _, msg := range m.chat.messages {
		if msg.Role == message.Assistant {
			// Check thinking blocks
			if targetMsgID, ok := strings.CutPrefix(m.chat.focusedToolCallID, "thinking_"); ok {
				if msg.ID == targetMsgID {
					for _, part := range msg.Parts {
						if rc, ok := part.(message.ReasoningContent); ok {
							return rc.Thinking
						}
					}
				}
				continue
			}

			// Check tool calls
			for _, tc := range msg.ToolCalls() {
				if tc.ID == m.chat.focusedToolCallID {
					// Return tool result if available, otherwise tool input
					if toolMsg, ok := m.chat.toolMessages[tc.ID]; ok {
						for _, tr := range toolMsg.ToolResults() {
							return tr.Content
						}
					}
					return tc.Input
				}
			}
		}

		// Check summary blocks
		if msg.Role == message.User {
			if targetMsgID, ok := strings.CutPrefix(m.chat.focusedToolCallID, "summary_"); ok && msg.ID == targetMsgID {
				return msg.Content().Text
			}
		}
	}
	return ""
}

// extractCodeBlocks parses markdown text and returns all fenced code blocks.
func extractCodeBlocks(text string) []CodeBlock {
	lines := strings.Split(text, "\n")
	var blocks []CodeBlock
	var current *CodeBlock

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if current == nil {
			if lang, ok := strings.CutPrefix(trimmed, "```"); ok {
				current = &CodeBlock{Language: strings.TrimSpace(lang)}
			}
		} else {
			if strings.HasPrefix(trimmed, "```") {
				// Close current block
				blocks = append(blocks, *current)
				current = nil
			} else {
				if current.Content != "" {
					current.Content += "\n"
				}
				current.Content += line
			}
		}
	}

	// Handle unclosed fence (streaming)
	if current != nil && current.Content != "" {
		blocks = append(blocks, *current)
	}

	return blocks
}
