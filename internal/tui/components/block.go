package components

import (
	"fmt"

	"github.com/Nahasma/openscholar-public/internal/message"
)

// ExpandState represents the expand/collapse state of a tool call or thinking block.
type ExpandState uint8

const (
	ExpandCollapsed ExpandState = iota // Single-line summary (default)
	ExpandInline    ExpandState = 1    // Inline expand: params + first N lines of result
	ExpandOverlay   ExpandState = 2    // Overlay full view (future extension)
)

// BlockKind identifies the visual type of a chat block.
type BlockKind int

const (
	BlockUser              BlockKind = iota
	BlockAssistantMarkdown           // finished markdown content
	BlockThinking                    // reasoning/thinking content
	BlockToolHeader                  // tool call header line (done or running)
	BlockToolPreview                 // collapsed tool result preview
	BlockToolDetail                  // expanded tool result detail
	BlockInlineError                 // inline error message
	BlockStreamTail                  // streaming tail (breathing dots)
	BlockCompactBoundary             // compact summary boundary
	BlockToolGroupHeader             // tool group header (collapsed/expanded)
	BlockToolGroupItem               // individual item within a tool group
	BlockSystem                      // command/system output retained in transcript
	BlockCommandActivity             // UI-only command activity record
)

// BlockVM is the view-model for a single renderable block in the chat viewport.
type BlockVM struct {
	ID       string // unique: msgID + "-" + index
	MsgID    string // parent message ID
	Kind     BlockKind
	Content  string // source content (pre-render)
	Rendered string // cached rendered output
	WidthKey int    // terminal width at render time
	Height   int    // measured line height (0 = unmeasured)
	Dirty    bool   // needs re-rendering

	// Metadata carries kind-specific data (tool call ID, tool name, etc.)
	Meta BlockMeta
}

// BlockAttachment represents a binary content attachment on a user message.
type BlockAttachment struct {
	FileName string
}

// BlockMeta holds kind-specific metadata for a block.
type BlockMeta struct {
	ToolCallID            string                // for tool-related blocks
	ToolName              string                // tool display name
	ToolInput             string                // raw tool input JSON
	ToolFinished          bool                  // whether the tool call has finished
	ToolState             message.ToolCallState // Explicit tool lifecycle state (queued/running/completed/errored/canceled)
	ToolIdx               int                   // index among sibling tool calls (for animation phase)
	IsExpanded            bool                  // whether the block is in expanded state
	IsFocused             bool                  // whether the block has focus indicator
	IsError               bool                  // for error-related blocks
	SemanticKey           string                // stable key for expand/focus (e.g. "thinking_<msgID>", "summary_<msgID>")
	Attachments           []BlockAttachment     // binary content attachments (user messages)
	IsSummary             bool                  // whether this user block is a conversation summary
	IsFirstContentBlock   bool                  // CC alignment: first text block of assistant message gets ⏺ dot
	IsConsecutiveThinking bool                  // consecutive thinking block (2nd+ in a row — suppress label when collapsed)
	IsMessageFinished     bool                  // whether the parent message is finished (for hiding thinking after completion)

	// Wave 3: Tool group metadata
	GroupToolName string  // tool name for the group (e.g. "Read")
	GroupTotal    int     // total items in the group
	GroupFinished int     // finished items in the group
	GroupQueued   int     // queued items in the group
	GroupRunning  int     // running items in the group
	GroupElapsed  float64 // total elapsed seconds for the group
	GroupExpanded bool    // whether the group is expanded
	GroupHint     string  // CC alignment: hint text for collapsed group header (last tool's param)

	CommandInvocation string // command activity invocation, e.g. /config
	CommandSummary    string // command activity summary text
}

// MarkDirty flags the block for re-rendering.
func (b *BlockVM) MarkDirty() {
	b.Dirty = true
	b.Rendered = ""
	b.Height = 0
}

// BlockList manages an ordered collection of BlockVMs with index lookups.
type BlockList struct {
	blocks  []BlockVM
	byID    map[string]int   // blockID → index
	byMsgID map[string][]int // msgID → block indices
}

// NewBlockList creates an empty BlockList.
func NewBlockList() *BlockList {
	return &BlockList{
		byID:    make(map[string]int),
		byMsgID: make(map[string][]int),
	}
}

// Len returns the number of blocks.
func (bl *BlockList) Len() int { return len(bl.blocks) }

// Get returns a pointer to the block at index i, or nil if out of range.
func (bl *BlockList) Get(i int) *BlockVM {
	if i < 0 || i >= len(bl.blocks) {
		return nil
	}
	return &bl.blocks[i]
}

// GetByID returns a pointer to the block with the given ID, or nil.
func (bl *BlockList) GetByID(id string) *BlockVM {
	idx, ok := bl.byID[id]
	if !ok {
		return nil
	}
	return &bl.blocks[idx]
}

// BlocksForMsg returns the indices of all blocks belonging to the given message.
func (bl *BlockList) BlocksForMsg(msgID string) []int {
	return bl.byMsgID[msgID]
}

// ReplaceMessage removes all blocks for msgID and inserts newBlocks in their place.
func (bl *BlockList) ReplaceMessage(msgID string, newBlocks []BlockVM) {
	oldIndices := bl.byMsgID[msgID]
	if len(oldIndices) == 0 {
		// Append at end
		startIdx := len(bl.blocks)
		bl.blocks = append(bl.blocks, newBlocks...)
		indices := make([]int, len(newBlocks))
		for i, b := range newBlocks {
			idx := startIdx + i
			indices[i] = idx
			bl.byID[b.ID] = idx
		}
		bl.byMsgID[msgID] = indices
		return
	}

	// Replace in-place: remove old, insert new at the same position
	insertAt := oldIndices[0]
	removeCount := len(oldIndices)

	// Remove old IDs from byID
	for _, idx := range oldIndices {
		delete(bl.byID, bl.blocks[idx].ID)
	}

	// Splice: replace old blocks with new blocks
	tail := make([]BlockVM, len(bl.blocks[insertAt+removeCount:]))
	copy(tail, bl.blocks[insertAt+removeCount:])
	bl.blocks = append(bl.blocks[:insertAt], newBlocks...)
	bl.blocks = append(bl.blocks, tail...)

	// Rebuild indices from insertAt onward
	bl.rebuildIndicesFrom(insertAt)
}

// RebuildAll replaces all blocks with a new set.
func (bl *BlockList) RebuildAll(blocks []BlockVM) {
	bl.blocks = blocks
	bl.byID = make(map[string]int, len(blocks))
	bl.byMsgID = make(map[string][]int)
	for i, b := range bl.blocks {
		bl.byID[b.ID] = i
		bl.byMsgID[b.MsgID] = append(bl.byMsgID[b.MsgID], i)
	}
}

// All returns a slice of all blocks (for iteration).
func (bl *BlockList) All() []BlockVM { return bl.blocks }

// rebuildIndicesFrom rebuilds the byID and byMsgID maps from index start onward.
func (bl *BlockList) rebuildIndicesFrom(start int) {
	// Clear byMsgID entries that reference indices >= start
	for msgID, indices := range bl.byMsgID {
		var kept []int
		for _, idx := range indices {
			if idx < start {
				kept = append(kept, idx)
			}
		}
		if len(kept) == 0 {
			delete(bl.byMsgID, msgID)
		} else {
			bl.byMsgID[msgID] = kept
		}
	}
	// Re-index from start
	for i := start; i < len(bl.blocks); i++ {
		b := &bl.blocks[i]
		bl.byID[b.ID] = i
		bl.byMsgID[b.MsgID] = append(bl.byMsgID[b.MsgID], i)
	}
}

// MakeBlockID generates a unique block ID from message ID and index.
func MakeBlockID(msgID string, index int) string {
	return fmt.Sprintf("%s-%d", msgID, index)
}

// IsAssistantOwnedBlock reports whether the block belongs to assistant-owned
// transcript content for boundary spacing purposes.
func IsAssistantOwnedBlock(block BlockVM) bool {
	switch block.Kind {
	case BlockAssistantMarkdown, BlockStreamTail, BlockThinking,
		BlockToolHeader, BlockToolPreview, BlockToolDetail,
		BlockToolGroupHeader, BlockToolGroupItem:
		return true
	default:
		return false
	}
}

// BoundaryRowsBetween returns the number of transcript boundary rows to insert
// between two adjacent visible blocks.
func BoundaryRowsBetween(prev, curr BlockVM) int {
	if prev.MsgID == "" || curr.MsgID == "" {
		return 0
	}
	if prev.MsgID == curr.MsgID {
		return 0
	}
	if IsDisplayHidden(prev) || IsDisplayHidden(curr) {
		return 0
	}
	if (prev.Kind == BlockUser && IsAssistantOwnedBlock(curr)) || (curr.Kind == BlockUser && IsAssistantOwnedBlock(prev)) {
		return 1
	}
	return 1
}
