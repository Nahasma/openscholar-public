package components

import (
	"path/filepath"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/message"
)

// BuildBlocks converts a slice of messages into a flat slice of BlockVM view-models.
//
// Each message role is handled differently:
//   - User messages produce a single BlockUser.
//   - Assistant messages produce one or more blocks: optional BlockThinking,
//     optional BlockAssistantMarkdown or BlockStreamTail, and one or more
//     BlockToolHeader/BlockToolPreview/BlockToolDetail per tool call.
//   - Tool messages are skipped; their results are accessible via toolMessages.
func BuildBlocks(
	messages []message.Message,
	toolMessages map[string]message.Message,
	expandedToolCalls map[string]bool,
	focusedToolCallID string,
) []BlockVM {
	var blocks []BlockVM

	for _, msg := range messages {
		switch msg.Role {
		case message.User:
			blocks = append(blocks, buildUserBlocks(msg, expandedToolCalls, focusedToolCallID)...)
		case message.Assistant:
			blocks = append(blocks, buildAssistantBlocks(msg, toolMessages, expandedToolCalls, focusedToolCallID)...)
		case message.System:
			blocks = append(blocks, buildSystemBlocks(msg)...)
		case message.Tool:
			// Tool results are rendered inline with assistant messages via toolMessages map.
		}
	}

	markConsecutiveThinking(blocks)
	return blocks
}

// BuildBlocksGrouped is like BuildBlocks but applies tool grouping when enabled.
// Consecutive same-type tool calls within an assistant turn are merged into
// BlockToolGroupHeader + N × BlockToolGroupItem blocks.
// The optional resolveExpand callback allows the caller to inject transcript-wide
// expand logic (e.g. transcriptExpanded) so that ctrl+o correctly expands groups.
func BuildBlocksGrouped(
	messages []message.Message,
	toolMessages map[string]message.Message,
	expandedToolCalls map[string]bool,
	focusedToolCallID string,
	resolveExpand ...func(string) ExpandState,
) []BlockVM {
	blocks := BuildBlocks(messages, toolMessages, expandedToolCalls, focusedToolCallID)
	var resolver func(string) ExpandState
	if len(resolveExpand) > 0 {
		resolver = resolveExpand[0]
	}
	return groupConsecutiveTools(blocks, expandedToolCalls, focusedToolCallID, resolver)
}

func buildSystemBlocks(msg message.Message) []BlockVM {
	if flag, ok := msg.Meta["ui_command_activity"].(bool); ok && flag {
		invocation, _ := msg.Meta["command_invocation"].(string)
		summary, _ := msg.Meta["command_summary"].(string)
		if strings.TrimSpace(invocation) == "" || strings.TrimSpace(summary) == "" {
			return nil
		}
		return []BlockVM{{
			ID:      MakeBlockID(msg.ID, 0),
			MsgID:   msg.ID,
			Kind:    BlockCommandActivity,
			Content: summary,
			Dirty:   true,
			Meta: BlockMeta{
				CommandInvocation: invocation,
				CommandSummary:    summary,
			},
		}}
	}
	content := strings.TrimSpace(msg.Content().String())
	if content == "" {
		return nil
	}
	return []BlockVM{
		{
			ID:      MakeBlockID(msg.ID, 0),
			MsgID:   msg.ID,
			Kind:    BlockSystem,
			Content: content,
			Dirty:   true,
		},
	}
}

func toolResultForCall(toolCallID string, toolMessages map[string]message.Message) (message.ToolResult, bool) {
	if toolMessages == nil {
		return message.ToolResult{}, false
	}
	toolMsg, ok := toolMessages[toolCallID]
	if !ok {
		return message.ToolResult{}, false
	}
	for _, part := range toolMsg.Parts {
		if tr, ok := part.(message.ToolResult); ok && tr.ToolCallID == toolCallID {
			return tr, true
		}
	}
	return message.ToolResult{}, false
}

// groupConsecutiveTools scans blocks for consecutive same-type tool header blocks
// within the same message, and merges them into group header + group items.
func groupConsecutiveTools(blocks []BlockVM, expandedToolCalls map[string]bool, focusedToolCallID string, resolveExpand func(string) ExpandState) []BlockVM {
	if len(blocks) < 2 {
		return blocks
	}

	var result []BlockVM
	i := 0
	for i < len(blocks) {
		b := blocks[i]

		// Only group BlockToolHeader blocks
		if b.Kind != BlockToolHeader {
			result = append(result, b)
			i++
			continue
		}

		// Scan forward for consecutive same-tool-name headers in the same message
		groupStart := i
		toolName := b.Meta.ToolName
		msgID := b.MsgID
		j := i + 1
		for j < len(blocks) {
			next := blocks[j]
			if next.MsgID != msgID {
				break
			}
			// Skip tool preview/detail blocks that follow the header
			if next.Kind == BlockToolPreview || next.Kind == BlockToolDetail {
				j++
				continue
			}
			if next.Kind == BlockToolHeader && next.Meta.ToolName == toolName {
				j++
				continue
			}
			break
		}

		// Collect only the header blocks in this range
		var headers []BlockVM
		for k := groupStart; k < j; k++ {
			if blocks[k].Kind == BlockToolHeader && blocks[k].Meta.ToolName == toolName {
				headers = append(headers, blocks[k])
			}
		}

		// Need at least 2 to form a group
		if len(headers) < 2 {
			// No grouping — emit original blocks
			for k := groupStart; k < j; k++ {
				result = append(result, blocks[k])
			}
			i = j
			continue
		}

		// Build group header
		finishedCount := 0
		// NOTE: totalElapsed is 0 because the current message model doesn't carry
		// per-tool-call timing data. When timing is added to ToolCall metadata,
		// aggregate it here and pass via GroupElapsed.
		var totalElapsed float64
		for _, h := range headers {
			if message.IsTerminalToolState(h.Meta.ToolState) {
				finishedCount++
			}
		}

		queuedCount := 0
		runningCount := 0
		for _, h := range headers {
			switch h.Meta.ToolState {
			case message.ToolCallQueued:
				queuedCount++
			case message.ToolCallRunning:
				runningCount++
			}
		}

		groupKey := "group_" + headers[0].Meta.ToolCallID
		isGroupExpanded := expandedToolCalls[groupKey]
		// Use resolver for transcript-wide expand (ctrl+o) if available
		if !isGroupExpanded && resolveExpand != nil {
			isGroupExpanded = resolveExpand(groupKey) >= ExpandInline
		}

		// CC alignment: extract last tool's param summary for hint line
		lastParamHint := ""
		if len(headers) > 0 {
			lastH := headers[len(headers)-1]
			lastParamHint = extractToolParamsSummary(lastH.Meta.ToolName, lastH.Meta.ToolInput)
		}

		groupHeader := BlockVM{
			ID:    MakeBlockID(msgID, 9000+groupStart), // high index to avoid collision
			MsgID: msgID,
			Kind:  BlockToolGroupHeader,
			Dirty: true,
			Meta: BlockMeta{
				GroupToolName: toolName,
				GroupTotal:    len(headers),
				GroupFinished: finishedCount,
				GroupQueued:   queuedCount,
				GroupRunning:  runningCount,
				GroupElapsed:  totalElapsed,
				GroupExpanded: isGroupExpanded,
				GroupHint:     lastParamHint,
				SemanticKey:   groupKey,
				IsFocused:     focusedToolCallID == groupKey,
			},
		}
		result = append(result, groupHeader)

		// If expanded, emit each header as a group item
		if isGroupExpanded {
			for _, h := range headers {
				item := h // copy
				item.Kind = BlockToolGroupItem
				item.Dirty = true
				result = append(result, item)

				// Also include the preview/detail block if it exists
				for k := groupStart; k < j; k++ {
					if (blocks[k].Kind == BlockToolPreview || blocks[k].Kind == BlockToolDetail) &&
						blocks[k].Meta.ToolCallID == h.Meta.ToolCallID {
						result = append(result, blocks[k])
						break
					}
				}
			}
		}

		i = j
	}

	return result
}

// markConsecutiveThinking marks thinking blocks that follow another thinking
// block (possibly separated by tool blocks) as consecutive. Consecutive thinking
// blocks are suppressed visually when collapsed to reduce noise.
func markConsecutiveThinking(blocks []BlockVM) {
	sawThinking := false
	for i := range blocks {
		if blocks[i].Kind == BlockThinking {
			if sawThinking {
				blocks[i].Meta.IsConsecutiveThinking = true
			}
			sawThinking = true
		} else if blocks[i].Kind == BlockUser || blocks[i].Kind == BlockAssistantMarkdown {
			// A user message or substantive assistant text resets the streak.
			sawThinking = false
		}
		// BlockToolHeader, BlockToolPreview, BlockToolDetail etc. do not reset —
		// they appear between thinking blocks and are part of the same turn.
	}
}

func buildUserBlocks(msg message.Message, expandedToolCalls map[string]bool, focusedToolCallID string) []BlockVM {
	content := msg.Content().String()
	if content == "" {
		return nil
	}

	// Detect conversation summary messages
	isSummary := strings.HasPrefix(content, "[Conversation Summary]")
	summaryKey := ""
	if isSummary {
		summaryKey = "summary_" + msg.ID
	}

	// Collect BinaryContent attachments
	var attachments []BlockAttachment
	for _, part := range msg.Parts {
		if bc, ok := part.(message.BinaryContent); ok {
			attachments = append(attachments, BlockAttachment{
				FileName: filepath.Base(bc.Path),
			})
		}
	}

	return []BlockVM{
		{
			ID:      MakeBlockID(msg.ID, 0),
			MsgID:   msg.ID,
			Kind:    BlockUser,
			Content: content,
			Dirty:   true,
			Meta: BlockMeta{
				SemanticKey: summaryKey,
				Attachments: attachments,
				IsSummary:   isSummary,
				IsExpanded:  isSummary && expandedToolCalls[summaryKey],
				IsFocused:   isSummary && focusedToolCallID == summaryKey,
			},
		},
	}
}

func buildAssistantBlocks(
	msg message.Message,
	toolMessages map[string]message.Message,
	expandedToolCalls map[string]bool,
	focusedToolCallID string,
) []BlockVM {
	var blocks []BlockVM
	idx := 0
	isStreaming := !msg.IsFinished()

	// 1. Reasoning/thinking content
	for _, part := range msg.Parts {
		if rc, ok := part.(message.ReasoningContent); ok {
			thinking := rc.Thinking
			if thinking != "" {
				thinkingKey := "thinking_" + msg.ID
				blocks = append(blocks, BlockVM{
					ID:      MakeBlockID(msg.ID, idx),
					MsgID:   msg.ID,
					Kind:    BlockThinking,
					Content: thinking,
					Dirty:   true,
					Meta: BlockMeta{
						SemanticKey:       thinkingKey,
						IsExpanded:        expandedToolCalls[thinkingKey],
						IsFocused:         focusedToolCallID == thinkingKey,
						IsMessageFinished: !isStreaming,
					},
				})
				idx++
			}
		}
	}

	// 2. Text content — finished markdown or streaming tail
	content := normalizeAssistantContentForBlock(msg.Content().String())
	toolCalls := msg.ToolCalls()

	if content != "" {
		kind := BlockStreamTail
		if !isStreaming {
			kind = BlockAssistantMarkdown
		}
		blocks = append(blocks, BlockVM{
			ID:      MakeBlockID(msg.ID, idx),
			MsgID:   msg.ID,
			Kind:    kind,
			Content: content,
			Dirty:   true,
			Meta: BlockMeta{
				IsFirstContentBlock: true, // CC alignment: first text block gets ⏺ dot
			},
		})
		idx++
	} else if isStreaming && len(toolCalls) == 0 {
		// Empty content + streaming + no tool calls = initial streaming animation
		blocks = append(blocks, BlockVM{
			ID:    MakeBlockID(msg.ID, idx),
			MsgID: msg.ID,
			Kind:  BlockStreamTail,
			Dirty: true,
			// Content is empty — renderStreamTailBlock will show spinner animation
		})
		idx++
	}

	// 3. Tool calls (toolCalls already assigned above)
	for toolIdx, tc := range toolCalls {
		state := tc.EffectiveState()
		finished := tc.IsTerminal()
		toolResult, hasToolResult := toolResultForCall(tc.ID, toolMessages)

		// CC alignment: detect tool error from result message
		toolIsError := state == message.ToolCallErrored
		if hasToolResult {
			toolIsError = toolIsError || toolResult.IsError
		}

		// Tool header block — SemanticKey = tc.ID so V2 inline expand can find it
		blocks = append(blocks, BlockVM{
			ID:    MakeBlockID(msg.ID, idx),
			MsgID: msg.ID,
			Kind:  BlockToolHeader,
			Dirty: true,
			Meta: BlockMeta{
				ToolCallID:   tc.ID,
				ToolName:     tc.Name,
				ToolInput:    tc.Input,
				ToolFinished: finished,
				ToolState:    state,
				ToolIdx:      toolIdx,
				IsFocused:    focusedToolCallID == tc.ID,
				IsError:      toolIsError,
				SemanticKey:  tc.ID,
			},
		})
		idx++

		// Tool result block (preview or detail), only if the tool call is finished
		if finished && hasToolResult {
			isExpanded := expandedToolCalls[tc.ID]
			resultKind := BlockToolPreview
			if isExpanded {
				resultKind = BlockToolDetail
			}

			blocks = append(blocks, BlockVM{
				ID:      MakeBlockID(msg.ID, idx),
				MsgID:   msg.ID,
				Kind:    resultKind,
				Content: toolResult.Content,
				Dirty:   true,
				Meta: BlockMeta{
					ToolCallID:   tc.ID,
					ToolName:     tc.Name,
					ToolInput:    tc.Input,
					ToolFinished: finished,
					ToolState:    state,
					ToolIdx:      toolIdx,
					IsExpanded:   isExpanded,
					IsFocused:    focusedToolCallID == tc.ID,
					IsError:      toolIsError,
					SemanticKey:  tc.ID,
				},
			})
			idx++
		}
	}

	return blocks
}

func normalizeAssistantContentForBlock(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	for {
		switch {
		case strings.HasPrefix(content, "\n"):
			content = content[1:]
		case strings.HasPrefix(content, " \n"):
			content = content[2:]
		case strings.HasPrefix(content, "\t\n"):
			content = content[2:]
		default:
			if idx := strings.IndexByte(content, '\n'); idx >= 0 && strings.TrimSpace(content[:idx]) == "" {
				content = content[idx+1:]
				continue
			}
			return content
		}
	}
}
