package components

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/message"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// buildToolMap extracts tool results from tool-role messages into a map keyed
// by tool call ID, matching the contract of BuildBlocks' toolMessages param.
func buildToolMap(msgs []message.Message) map[string]message.Message {
	m := make(map[string]message.Message)
	for _, msg := range msgs {
		if msg.Role != message.Tool {
			continue
		}
		for _, part := range msg.Parts {
			if tr, ok := part.(message.ToolResult); ok {
				m[tr.ToolCallID] = msg
			}
		}
	}
	return m
}

// blockKinds returns the Kind of every block in the slice.
func blockKinds(blocks []BlockVM) []BlockKind {
	kinds := make([]BlockKind, len(blocks))
	for i, b := range blocks {
		kinds[i] = b.Kind
	}
	return kinds
}

// allIDsUnique asserts that every block ID appears exactly once.
func allIDsUnique(t *testing.T, blocks []BlockVM) {
	t.Helper()
	seen := make(map[string]bool, len(blocks))
	for _, b := range blocks {
		if seen[b.ID] {
			t.Errorf("duplicate block ID: %s", b.ID)
		}
		seen[b.ID] = true
	}
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestBuildBlocks_SimpleChat verifies that fixtureSimpleChat produces the
// expected number and kinds of blocks.
func TestBuildBlocks_SimpleChat(t *testing.T) {
	msgs := fixtureSimpleChat()
	toolMap := buildToolMap(msgs)

	blocks := BuildBlocks(msgs, toolMap, nil, "")

	// fixtureSimpleChat has: user-1, asst-1 (finished text), user-2
	// Expected blocks: BlockUser, BlockAssistantMarkdown, BlockUser → 3 blocks
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d: %v", len(blocks), blockKinds(blocks))
	}

	if blocks[0].Kind != BlockUser {
		t.Errorf("blocks[0]: want BlockUser, got %d", blocks[0].Kind)
	}
	if blocks[1].Kind != BlockAssistantMarkdown {
		t.Errorf("blocks[1]: want BlockAssistantMarkdown, got %d", blocks[1].Kind)
	}
	if blocks[2].Kind != BlockUser {
		t.Errorf("blocks[2]: want BlockUser, got %d", blocks[2].Kind)
	}

	allIDsUnique(t, blocks)
}

func TestBuildBlocks_SystemMessage(t *testing.T) {
	msgs := []message.Message{
		{
			ID:   "sys-1",
			Role: message.System,
			Parts: []message.ContentPart{
				message.TextContent{Text: "command output"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	blocks := BuildBlocks(msgs, nil, nil, "")
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(blocks))
	}
	if blocks[0].Kind != BlockSystem {
		t.Fatalf("block kind = %v, want BlockSystem", blocks[0].Kind)
	}
	if blocks[0].Content != "command output" {
		t.Fatalf("content = %q", blocks[0].Content)
	}
}

func TestBuildBlocks_CommandActivitySystemMessage(t *testing.T) {
	msgs := []message.Message{
		{
			ID:   "sys-activity",
			Role: message.System,
			Meta: map[string]any{
				"ui_command_activity": true,
				"command_invocation":  "/config",
				"command_summary":     "Settings dialog dismissed",
			},
			Parts: []message.ContentPart{
				message.TextContent{Text: "ignored"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	blocks := BuildBlocks(msgs, nil, nil, "")
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(blocks))
	}
	if blocks[0].Kind != BlockCommandActivity {
		t.Fatalf("block kind = %v, want BlockCommandActivity", blocks[0].Kind)
	}
	if blocks[0].Meta.CommandInvocation != "/config" {
		t.Fatalf("invocation = %q", blocks[0].Meta.CommandInvocation)
	}
}

// TestBuildBlocks_SimpleChatContent verifies that the user and assistant text
// content is propagated correctly.
func TestBuildBlocks_SimpleChatContent(t *testing.T) {
	msgs := fixtureSimpleChat()
	blocks := BuildBlocks(msgs, nil, nil, "")

	if blocks[0].Content != "Hello, can you help me with my paper?" {
		t.Errorf("user content mismatch: %q", blocks[0].Content)
	}
	if blocks[1].Content != "Of course! I'd be happy to help you with your paper. What aspect would you like to work on first?" {
		t.Errorf("assistant content mismatch: %q", blocks[1].Content)
	}
}

func TestBuildBlocks_AssistantLeadingBlankLinesAreDisplayNormalized(t *testing.T) {
	msgs := []message.Message{
		{
			ID:   "asst-leading-blanks",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "\r\n\n  \n正文"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	blocks := BuildBlocks(msgs, nil, nil, "")
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(blocks))
	}
	if blocks[0].Kind != BlockAssistantMarkdown {
		t.Fatalf("block kind = %v, want BlockAssistantMarkdown", blocks[0].Kind)
	}
	if blocks[0].Content != "正文" {
		t.Fatalf("assistant content = %q, want leading blank lines stripped", blocks[0].Content)
	}
}

func TestBuildBlocks_AssistantStreamingUsesSameLeadingBlankNormalization(t *testing.T) {
	msgs := []message.Message{
		{
			ID:   "asst-stream-leading-blanks",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "\n\nstreaming text"},
			},
		},
	}
	blocks := BuildBlocks(msgs, nil, nil, "")
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(blocks))
	}
	if blocks[0].Kind != BlockStreamTail {
		t.Fatalf("block kind = %v, want BlockStreamTail", blocks[0].Kind)
	}
	if blocks[0].Content != "streaming text" {
		t.Fatalf("streaming content = %q, want leading blank lines stripped", blocks[0].Content)
	}
}

func TestBuildBlocks_AssistantNormalizationPreservesInternalParagraphsAndCodeFence(t *testing.T) {
	content := "\n\n第一段\n\n```go\n\nfmt.Println(\"x\")\n```\n\n第二段"
	blocks := BuildBlocks([]message.Message{
		{
			ID:   "asst-internal-blanks",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: content},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}, nil, nil, "")
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(blocks))
	}
	got := blocks[0].Content
	if strings.HasPrefix(got, "\n") {
		t.Fatalf("assistant content still has leading blank line: %q", got)
	}
	if !strings.Contains(got, "第一段\n\n```go\n\nfmt.Println") {
		t.Fatalf("internal paragraph or code fence blank lines were not preserved: %q", got)
	}
}

func TestBuildBlocks_UserLeadingBlankLinesAreUnchanged(t *testing.T) {
	blocks := BuildBlocks([]message.Message{
		{
			ID:   "user-leading-blanks",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "\n\nuser text"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}, nil, nil, "")
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(blocks))
	}
	if blocks[0].Content != "\n\nuser text" {
		t.Fatalf("user content changed: %q", blocks[0].Content)
	}
}

func TestRenderBlock_AssistantLeadingBlankLinesDoNotRenderAfterDot(t *testing.T) {
	blocks := BuildBlocks([]message.Message{
		{
			ID:   "asst-render-leading-blanks",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "\n\n正文"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}, nil, nil, "")
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(blocks))
	}
	rendered := stripANSI(RenderBlock(&blocks[0], makeCtx(60)))
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "正文") {
		t.Fatalf("assistant first rendered line should contain content after dot, got %q", rendered)
	}
}

// TestBuildBlocks_ToolCalls verifies that an assistant message with two tool
// calls produces header + preview blocks when not expanded.
func TestBuildBlocks_ToolCalls(t *testing.T) {
	msgs := fixtureWithToolCalls()
	toolMap := buildToolMap(msgs)

	blocks := BuildBlocks(msgs, toolMap, make(map[string]bool), "")

	// fixtureWithToolCalls: asst-tool-1 has text + 2 finished tool calls
	// Expected: BlockAssistantMarkdown, BlockToolHeader, BlockToolPreview,
	//           BlockToolHeader, BlockToolPreview → 5 blocks
	if len(blocks) != 5 {
		t.Fatalf("expected 5 blocks, got %d: %v", len(blocks), blockKinds(blocks))
	}

	want := []BlockKind{
		BlockAssistantMarkdown,
		BlockToolHeader,
		BlockToolPreview,
		BlockToolHeader,
		BlockToolPreview,
	}
	for i, k := range want {
		if blocks[i].Kind != k {
			t.Errorf("blocks[%d]: want %d, got %d", i, k, blocks[i].Kind)
		}
	}

	allIDsUnique(t, blocks)
}

// TestBuildBlocks_ToolCallsExpanded verifies that an expanded tool call
// produces BlockToolDetail instead of BlockToolPreview.
func TestBuildBlocks_ToolCallsExpanded(t *testing.T) {
	msgs := fixtureWithToolCalls()
	toolMap := buildToolMap(msgs)

	expanded := map[string]bool{"tc-bash-1": true}
	blocks := BuildBlocks(msgs, toolMap, expanded, "")

	// tc-bash-1 is expanded → its result block should be BlockToolDetail
	// tc-edit-1 is not expanded → its result block should be BlockToolPreview
	if blocks[2].Kind != BlockToolDetail {
		t.Errorf("tc-bash-1 result: want BlockToolDetail, got %d", blocks[2].Kind)
	}
	if blocks[2].Meta.IsExpanded != true {
		t.Errorf("tc-bash-1 result: want IsExpanded=true")
	}
	if blocks[4].Kind != BlockToolPreview {
		t.Errorf("tc-edit-1 result: want BlockToolPreview, got %d", blocks[4].Kind)
	}
	if blocks[4].Meta.IsExpanded != false {
		t.Errorf("tc-edit-1 result: want IsExpanded=false")
	}
}

// TestBuildBlocks_Streaming verifies that a non-finished assistant message
// produces BlockStreamTail instead of BlockAssistantMarkdown.
func TestBuildBlocks_Streaming(t *testing.T) {
	msgs := fixtureStreaming()
	toolMap := buildToolMap(msgs)

	blocks := BuildBlocks(msgs, toolMap, nil, "")

	// user + streaming assistant → BlockUser + BlockStreamTail
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d: %v", len(blocks), blockKinds(blocks))
	}
	if blocks[1].Kind != BlockStreamTail {
		t.Errorf("streaming block: want BlockStreamTail, got %d", blocks[1].Kind)
	}
	allIDsUnique(t, blocks)
}

// TestBuildBlocks_Thinking verifies that a message with ReasoningContent
// produces a BlockThinking block before the text block.
func TestBuildBlocks_Thinking(t *testing.T) {
	msgs := fixtureWithThinking()
	toolMap := buildToolMap(msgs)

	blocks := BuildBlocks(msgs, toolMap, nil, "")

	// user-think-1 + asst-think-1 (thinking + text finished)
	// → BlockUser, BlockThinking, BlockAssistantMarkdown
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d: %v", len(blocks), blockKinds(blocks))
	}
	if blocks[0].Kind != BlockUser {
		t.Errorf("blocks[0]: want BlockUser, got %d", blocks[0].Kind)
	}
	if blocks[1].Kind != BlockThinking {
		t.Errorf("blocks[1]: want BlockThinking, got %d", blocks[1].Kind)
	}
	if blocks[2].Kind != BlockAssistantMarkdown {
		t.Errorf("blocks[2]: want BlockAssistantMarkdown, got %d", blocks[2].Kind)
	}

	// Verify thinking content is present
	if blocks[1].Content == "" {
		t.Error("BlockThinking should have non-empty content")
	}

	allIDsUnique(t, blocks)
}

// TestBuildBlocks_EmptyUserMessage verifies that a user message with no text
// is skipped (produces no block).
func TestBuildBlocks_EmptyUserMessage(t *testing.T) {
	msgs := []message.Message{
		{
			ID:   "user-empty",
			Role: message.User,
			Parts: []message.ContentPart{
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
		{
			ID:   "user-ok",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "hello"},
			},
		},
	}

	blocks := BuildBlocks(msgs, nil, nil, "")

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (empty msg skipped), got %d", len(blocks))
	}
	if blocks[0].Kind != BlockUser {
		t.Errorf("expected BlockUser, got %d", blocks[0].Kind)
	}
	if blocks[0].Content != "hello" {
		t.Errorf("content mismatch: %q", blocks[0].Content)
	}
}

// TestBuildBlocks_ToolMessagesSkipped verifies that tool-role messages do not
// produce any direct blocks.
func TestBuildBlocks_ToolMessagesSkipped(t *testing.T) {
	msgs := []message.Message{
		{
			ID:   "tool-only",
			Role: message.Tool,
			Parts: []message.ContentPart{
				message.ToolResult{ToolCallID: "tc-1", Name: "Bash", Content: "output"},
			},
		},
	}

	blocks := BuildBlocks(msgs, nil, nil, "")
	if len(blocks) != 0 {
		t.Errorf("tool messages should produce no blocks, got %d", len(blocks))
	}
}

// TestBuildBlocks_BlockIDUniqueness verifies that all generated block IDs are
// unique across a complex fixture.
func TestBuildBlocks_BlockIDUniqueness(t *testing.T) {
	msgs := fixtureWithToolCalls()
	toolMap := buildToolMap(msgs)

	expanded := map[string]bool{"tc-bash-1": true, "tc-edit-1": true}
	blocks := BuildBlocks(msgs, toolMap, expanded, "")

	allIDsUnique(t, blocks)
}

// TestBuildBlocks_FocusedToolCallID verifies that IsFocused is set only on
// blocks whose ToolCallID matches focusedToolCallID.
func TestBuildBlocks_FocusedToolCallID(t *testing.T) {
	msgs := fixtureWithToolCalls()
	toolMap := buildToolMap(msgs)

	blocks := BuildBlocks(msgs, toolMap, make(map[string]bool), "tc-edit-1")

	for _, b := range blocks {
		if b.Meta.ToolCallID == "tc-edit-1" {
			if !b.Meta.IsFocused {
				t.Errorf("block %s (tool tc-edit-1): want IsFocused=true", b.ID)
			}
		} else if b.Meta.ToolCallID != "" {
			if b.Meta.IsFocused {
				t.Errorf("block %s (tool %s): want IsFocused=false", b.ID, b.Meta.ToolCallID)
			}
		}
	}
}

// TestBuildBlocks_ToolMetaFields verifies that ToolName, ToolInput, ToolFinished,
// and ToolIdx are populated correctly on tool-related blocks.
func TestBuildBlocks_ToolMetaFields(t *testing.T) {
	msgs := fixtureWithToolCalls()
	toolMap := buildToolMap(msgs)
	blocks := BuildBlocks(msgs, toolMap, make(map[string]bool), "")

	// blocks[1] is the header for tc-bash-1 (first tool call, idx 0)
	hdr := blocks[1]
	if hdr.Kind != BlockToolHeader {
		t.Fatalf("expected BlockToolHeader at index 1, got %d", hdr.Kind)
	}
	if hdr.Meta.ToolCallID != "tc-bash-1" {
		t.Errorf("ToolCallID: got %q, want tc-bash-1", hdr.Meta.ToolCallID)
	}
	if hdr.Meta.ToolName != "Bash" {
		t.Errorf("ToolName: got %q, want Bash", hdr.Meta.ToolName)
	}
	if hdr.Meta.ToolInput == "" {
		t.Error("ToolInput should not be empty")
	}
	if !hdr.Meta.ToolFinished {
		t.Error("ToolFinished should be true")
	}
	if hdr.Meta.ToolIdx != 0 {
		t.Errorf("ToolIdx: got %d, want 0", hdr.Meta.ToolIdx)
	}

	// blocks[3] is the header for tc-edit-1 (second tool call, idx 1)
	hdr2 := blocks[3]
	if hdr2.Meta.ToolIdx != 1 {
		t.Errorf("ToolIdx for second tool: got %d, want 1", hdr2.Meta.ToolIdx)
	}
}

// TestBuildBlocks_ToolResultContent verifies that tool result content is
// propagated from toolMessages into the result block.
func TestBuildBlocks_ToolResultContent(t *testing.T) {
	msgs := fixtureWithToolCalls()
	toolMap := buildToolMap(msgs)
	expanded := map[string]bool{"tc-bash-1": true}
	blocks := BuildBlocks(msgs, toolMap, expanded, "")

	// blocks[2] should be the expanded detail for tc-bash-1
	detail := blocks[2]
	if detail.Kind != BlockToolDetail {
		t.Fatalf("expected BlockToolDetail, got %d", detail.Kind)
	}
	if detail.Content == "" {
		t.Error("tool detail content should not be empty")
	}
}

// TestBuildBlocks_EmptyMessages verifies that an empty message slice returns
// an empty block slice without panicking.
func TestBuildBlocks_EmptyMessages(t *testing.T) {
	blocks := BuildBlocks(nil, nil, nil, "")
	if blocks != nil && len(blocks) != 0 {
		t.Errorf("expected empty result, got %d blocks", len(blocks))
	}
}

// TestBuildBlocks_MsgIDPropagation verifies that each block's MsgID matches
// the originating message's ID.
func TestBuildBlocks_MsgIDPropagation(t *testing.T) {
	msgs := fixtureSimpleChat()
	blocks := BuildBlocks(msgs, nil, nil, "")

	if blocks[0].MsgID != "user-1" {
		t.Errorf("blocks[0].MsgID = %q, want user-1", blocks[0].MsgID)
	}
	if blocks[1].MsgID != "asst-1" {
		t.Errorf("blocks[1].MsgID = %q, want asst-1", blocks[1].MsgID)
	}
	if blocks[2].MsgID != "user-2" {
		t.Errorf("blocks[2].MsgID = %q, want user-2", blocks[2].MsgID)
	}
}

// TestBuildBlocks_BlockIDFormat verifies that block IDs follow the
// "<msgID>-<index>" format.
func TestBuildBlocks_BlockIDFormat(t *testing.T) {
	msgs := fixtureWithThinking()
	blocks := BuildBlocks(msgs, nil, nil, "")

	// asst-think-1 has thinking (idx 0) and text (idx 1)
	// Find the thinking block
	for _, b := range blocks {
		if b.Kind == BlockThinking {
			want := fmt.Sprintf("%s-%d", b.MsgID, 0)
			if b.ID != want {
				t.Errorf("thinking block ID = %q, want %q", b.ID, want)
			}
		}
		if b.Kind == BlockAssistantMarkdown {
			want := fmt.Sprintf("%s-%d", b.MsgID, 1)
			if b.ID != want {
				t.Errorf("markdown block ID = %q, want %q", b.ID, want)
			}
		}
	}
}

// TestBuildBlocks_AssistantNoText verifies that an assistant message with only
// tool calls (no text content) does not produce a spurious text block.
func TestBuildBlocks_AssistantNoText(t *testing.T) {
	msg := message.Message{
		ID:    "asst-notext",
		Role:  message.Assistant,
		Model: models.ModelID("claude-3-5-sonnet-20241022"),
		Parts: []message.ContentPart{
			message.ToolCall{
				ID:       "tc-only",
				Name:     "Bash",
				Input:    `{"command":"ls"}`,
				Type:     "tool_use",
				Finished: true,
			},
			message.Finish{Reason: message.FinishReasonToolUse},
		},
	}

	toolMsg := message.Message{
		ID:   "tool-notext",
		Role: message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{ToolCallID: "tc-only", Name: "Bash", Content: "file1\nfile2"},
		},
	}

	msgs := []message.Message{msg, toolMsg}
	toolMap := buildToolMap(msgs)
	blocks := BuildBlocks(msgs, toolMap, make(map[string]bool), "")

	// Only header + preview (no text block)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (header+preview), got %d: %v", len(blocks), blockKinds(blocks))
	}
	if blocks[0].Kind != BlockToolHeader {
		t.Errorf("blocks[0]: want BlockToolHeader, got %d", blocks[0].Kind)
	}
	if blocks[1].Kind != BlockToolPreview {
		t.Errorf("blocks[1]: want BlockToolPreview, got %d", blocks[1].Kind)
	}
}

// TestBuildBlocks_EmptyStreamingAssistant verifies that a streaming assistant
// with no content and no tool calls produces a StreamTail block for the
// initial spinner animation. Regression test for Codex review issue #3.
func TestBuildBlocks_EmptyStreamingAssistant(t *testing.T) {
	msg := message.Message{
		ID:   "asst-empty-stream",
		Role: message.Assistant,
		// No Parts at all — simulates initial streaming state before first token
	}

	blocks := BuildBlocks([]message.Message{msg}, nil, nil, "")

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (StreamTail), got %d: %v", len(blocks), blockKinds(blocks))
	}
	if blocks[0].Kind != BlockStreamTail {
		t.Errorf("blocks[0]: want BlockStreamTail, got %d", blocks[0].Kind)
	}
	if blocks[0].Content != "" {
		t.Errorf("expected empty content for initial streaming, got %q", blocks[0].Content)
	}
}

// TestBuildBlocks_UnfinishedToolCall verifies that an unfinished tool call
// produces only a header block (no result block).
func TestBuildBlocks_UnfinishedToolCall(t *testing.T) {
	msg := message.Message{
		ID:   "asst-running",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{
				ID:       "tc-running",
				Name:     "Bash",
				Input:    `{"command":"sleep 5"}`,
				Type:     "tool_use",
				Finished: false, // still running
			},
		},
	}

	blocks := BuildBlocks([]message.Message{msg}, nil, nil, "")

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (header only), got %d: %v", len(blocks), blockKinds(blocks))
	}
	if blocks[0].Kind != BlockToolHeader {
		t.Errorf("blocks[0]: want BlockToolHeader, got %d", blocks[0].Kind)
	}
	if blocks[0].Meta.ToolFinished {
		t.Error("ToolFinished should be false for running tool")
	}
}

// TestBuildBlocks_ToolStateBeatsLegacyFinished verifies that queued/running
// state does not render terminal tool UI even if the legacy Finished field is set.
func TestBuildBlocks_ToolStateBeatsLegacyFinished(t *testing.T) {
	msg := message.Message{
		ID:   "asst-queued-legacy",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "Working on it"},
			message.ToolCall{
				ID:       "tc-queued",
				Name:     "Read",
				Input:    `{"file":"main.go"}`,
				Type:     "tool_use",
				Finished: true,
				State:    message.ToolCallQueued,
			},
		},
	}

	blocks := BuildBlocks([]message.Message{msg}, nil, nil, "")

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (markdown + header), got %d: %v", len(blocks), blockKinds(blocks))
	}
	if blocks[0].Kind != BlockStreamTail {
		t.Fatalf("blocks[0]: want BlockStreamTail, got %d", blocks[0].Kind)
	}
	if blocks[1].Kind != BlockToolHeader {
		t.Fatalf("blocks[1]: want BlockToolHeader, got %d", blocks[1].Kind)
	}
	if blocks[1].Meta.ToolFinished {
		t.Error("ToolFinished should stay false for queued tool state")
	}
	if blocks[1].Meta.ToolState != message.ToolCallQueued {
		t.Errorf("ToolState = %q, want queued", blocks[1].Meta.ToolState)
	}
}

// ─── Wave 3: Tool Grouping tests ────────────────────────────────────────────

// TestGroupConsecutiveTools_ThreeReads verifies that 3 consecutive Read tool calls
// are merged into a group header (collapsed by default).
func TestGroupConsecutiveTools_ThreeReads(t *testing.T) {
	msg := message.Message{
		ID:   "asst-group",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc-r1", Name: "Read", Input: `{"file":"a.go"}`, Finished: true},
			message.ToolCall{ID: "tc-r2", Name: "Read", Input: `{"file":"b.go"}`, Finished: true},
			message.ToolCall{ID: "tc-r3", Name: "Read", Input: `{"file":"c.go"}`, Finished: true},
		},
		Model: models.ModelID("claude-3-5-sonnet-20241022"),
	}
	msg.Parts = append(msg.Parts, message.Finish{})

	expanded := make(map[string]bool)
	blocks := BuildBlocksGrouped([]message.Message{msg}, nil, expanded, "")

	// Should produce: 1 group header (collapsed, no items)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (group header), got %d: %v", len(blocks), blockKinds(blocks))
	}
	if blocks[0].Kind != BlockToolGroupHeader {
		t.Errorf("blocks[0]: want BlockToolGroupHeader, got %d", blocks[0].Kind)
	}
	if blocks[0].Meta.GroupTotal != 3 {
		t.Errorf("GroupTotal = %d, want 3", blocks[0].Meta.GroupTotal)
	}
	if blocks[0].Meta.GroupFinished != 3 {
		t.Errorf("GroupFinished = %d, want 3", blocks[0].Meta.GroupFinished)
	}
}

// TestGroupConsecutiveTools_Expanded verifies that an expanded group emits
// header + group items.
func TestGroupConsecutiveTools_Expanded(t *testing.T) {
	msg := message.Message{
		ID:   "asst-group-exp",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc-e1", Name: "Grep", Input: `{"pattern":"foo"}`, Finished: true},
			message.ToolCall{ID: "tc-e2", Name: "Grep", Input: `{"pattern":"bar"}`, Finished: true},
		},
		Model: models.ModelID("claude-3-5-sonnet-20241022"),
	}
	msg.Parts = append(msg.Parts, message.Finish{})

	expanded := map[string]bool{"group_tc-e1": true}
	blocks := BuildBlocksGrouped([]message.Message{msg}, nil, expanded, "")

	// Should produce: 1 group header + 2 group items + 2 preview blocks = 5
	// Actually: header + (item + preview) × 2 = 5
	if len(blocks) < 3 {
		t.Fatalf("expected at least 3 blocks (header + 2 items), got %d: %v", len(blocks), blockKinds(blocks))
	}
	if blocks[0].Kind != BlockToolGroupHeader {
		t.Errorf("blocks[0]: want BlockToolGroupHeader, got %d", blocks[0].Kind)
	}
	if !blocks[0].Meta.GroupExpanded {
		t.Error("group header should be expanded")
	}
	if blocks[1].Kind != BlockToolGroupItem {
		t.Errorf("blocks[1]: want BlockToolGroupItem, got %d", blocks[1].Kind)
	}
}

// TestGroupConsecutiveTools_NoGroupForSingle verifies that a single tool call
// is NOT grouped (requires at least 2).
func TestGroupConsecutiveTools_NoGroupForSingle(t *testing.T) {
	msg := message.Message{
		ID:   "asst-single",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc-s1", Name: "Read", Input: `{"file":"a.go"}`, Finished: true},
		},
		Model: models.ModelID("claude-3-5-sonnet-20241022"),
	}
	msg.Parts = append(msg.Parts, message.Finish{})

	blocks := BuildBlocksGrouped([]message.Message{msg}, nil, make(map[string]bool), "")

	// Should produce: 1 header + 1 preview = 2 (no grouping)
	for _, b := range blocks {
		if b.Kind == BlockToolGroupHeader || b.Kind == BlockToolGroupItem {
			t.Errorf("single tool call should not be grouped, got kind %d", b.Kind)
		}
	}
}

// TestGroupConsecutiveTools_MixedTypes verifies that different tool types
// break the group (Read + Grep + Read should NOT form a group).
func TestGroupConsecutiveTools_MixedTypes(t *testing.T) {
	msg := message.Message{
		ID:   "asst-mixed",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc-m1", Name: "Read", Input: `{"file":"a.go"}`, Finished: true},
			message.ToolCall{ID: "tc-m2", Name: "Grep", Input: `{"pattern":"x"}`, Finished: true},
			message.ToolCall{ID: "tc-m3", Name: "Read", Input: `{"file":"b.go"}`, Finished: true},
		},
		Model: models.ModelID("claude-3-5-sonnet-20241022"),
	}
	msg.Parts = append(msg.Parts, message.Finish{})

	blocks := BuildBlocksGrouped([]message.Message{msg}, nil, make(map[string]bool), "")

	// No grouping: each tool call produces header + preview
	for _, b := range blocks {
		if b.Kind == BlockToolGroupHeader || b.Kind == BlockToolGroupItem {
			t.Errorf("mixed types should not be grouped, got kind %d", b.Kind)
		}
	}
}

// TestBuildBlocks_IsErrorPropagation verifies that IsError from ToolResult
// is propagated to BlockToolHeader.Meta.IsError so the renderer can show a red dot.
func TestBuildBlocks_IsErrorPropagation(t *testing.T) {
	assistantMsg := message.Message{
		ID:    "asst-err",
		Role:  message.Assistant,
		Model: models.ModelID("claude-3-5-sonnet-20241022"),
		Parts: []message.ContentPart{
			message.ToolCall{
				ID:       "tc-ok",
				Name:     "Bash",
				Input:    `{"command":"echo ok"}`,
				Type:     "tool_use",
				Finished: true,
			},
			message.ToolCall{
				ID:       "tc-fail",
				Name:     "Bash",
				Input:    `{"command":"false"}`,
				Type:     "tool_use",
				Finished: true,
			},
			message.Finish{Reason: message.FinishReasonToolUse},
		},
	}
	toolResultMsg := message.Message{
		ID:   "tool-err-result",
		Role: message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{ToolCallID: "tc-ok", Name: "Bash", Content: "ok"},
			message.ToolResult{ToolCallID: "tc-fail", Name: "Bash", Content: "exit 1", IsError: true},
		},
	}

	msgs := []message.Message{assistantMsg, toolResultMsg}
	toolMap := buildToolMap(msgs)
	blocks := BuildBlocks(msgs, toolMap, make(map[string]bool), "")

	// Find tool headers and verify IsError propagation
	var okHeader, failHeader *BlockVM
	for i := range blocks {
		if blocks[i].Kind == BlockToolHeader {
			switch blocks[i].Meta.ToolCallID {
			case "tc-ok":
				okHeader = &blocks[i]
			case "tc-fail":
				failHeader = &blocks[i]
			}
		}
	}
	if okHeader == nil || failHeader == nil {
		t.Fatal("expected to find both tool headers")
	}
	if okHeader.Meta.IsError {
		t.Error("successful tool header should have IsError=false")
	}
	if !failHeader.Meta.IsError {
		t.Error("failed tool header should have IsError=true")
	}
}
