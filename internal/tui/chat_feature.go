package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

const (
	nonFullscreenMessageCap     = 200
	nonFullscreenMessageCapStep = 50
)

type messageSliceAnchor struct {
	MessageID string
	Idx       int
}

type streamLineFlushState struct {
	MessageID          string
	FlushWidth         int
	CommittedSourceEnd int
}

type mainScreenFrameState struct {
	PendingReset   bool
	AppliedReset   bool
	Reason         string
	ResizeEpoch    int
	Width          int
	Height         int
	SliceAnchor    string
	PrevLines      int
	PrevViewport   int
	ScrollbackRows int
	FreezeCount    int
}

// ExpandState represents the expand/collapse state of a tool call or thinking block.
type ExpandState uint8

const (
	ExpandCollapsed ExpandState = iota // Single-line summary (default)
	ExpandInline    ExpandState = 1    // Inline expand: params + first N lines of result
	ExpandOverlay   ExpandState = 2    // Overlay full view (future extension)
)

// ChatFeature groups all chat viewport, message, and selection state.
type ChatFeature struct {
	messages          []message.Message
	toolMessages      map[string]message.Message        // tool_call_id → Tool role message
	expandedToolCalls map[string]bool                   // legacy: expanded tool call IDs
	expandedNodes     map[string]components.ExpandState // Wave 2: three-state expand
	dirtyMessageIDs   map[string]bool                   // messages that need re-rendering
	ticksSinceLastMsg int                               // fallback safety net counter

	viewport    viewport.Model
	scrollMode  ScrollMode // auto-follow vs manual-locked
	renderCache *components.RenderCache

	focusedToolCallID      string      // currently focused tool call ID for navigation
	copyModeBlocks         []CodeBlock // code blocks available for selection in copy mode
	selection              TextSelection
	contentLines           []string // cached content lines (synced with viewport)
	inlineErrors           []components.InlineError
	renderedContent        string // latest transcript render for non-fullscreen mode
	transcriptExpanded     bool   // global transcript/thinking expansion toggle
	flushedAnchor          *messageSliceAnchor
	liveTailAnchor         *messageSliceAnchor
	activeStreamFlush      *streamLineFlushState
	mainScreenIntroFlushed bool
	mainScreenIntroCache   string
	mainScreenIntroWidth   int
	mainScreenIntroCached  bool
	// Legacy main-screen ownership fields are cleared on mode/session resets.
	// Resize no longer sets or reads them; terminal-native scrollback is driven
	// by MainScreenRenderer frame growth.
	mainScreenViewportOwned bool
	mainScreenOwnedStart    *messageSliceAnchor
	mainScreenOwnedStream   *streamLineFlushState
	mainScreenFrame         *mainScreenFrameState
	pendingAnchor           *components.ScrollAnchor
	resumePinnedStart       int
	resumePinned            bool

	// Phase 2: Block-based rendering (behind BlockRenderer feature flag)
	blockList   *components.BlockList
	heightCache *components.HeightCache

	// Phase 3: Virtual scrolling (behind VirtualScroll feature flag)
	virtualList *components.VirtualMessageList
}

// IsExpanded returns whether a node is expanded (either legacy bool or Wave 2 ExpandState).
func (c *ChatFeature) IsExpanded(id string) bool {
	return c.GetExpandState(id) >= components.ExpandInline
}

// GetExpandState returns the ExpandState for a node.
// expandedNodes is the single source of truth; transcriptExpanded provides global override.
func (c *ChatFeature) GetExpandState(id string) components.ExpandState {
	if c.expandedNodes != nil {
		if state, ok := c.expandedNodes[id]; ok {
			return state
		}
	}
	if c.transcriptExpanded {
		return components.ExpandInline
	}
	return components.ExpandCollapsed
}

// NewChatFeature creates a ChatFeature with initialized maps.
func NewChatFeature() ChatFeature {
	vp := viewport.New(0, 0)
	bl := components.NewBlockList()
	hc := components.NewHeightCache()
	return ChatFeature{
		toolMessages:      make(map[string]message.Message),
		expandedToolCalls: make(map[string]bool),
		expandedNodes:     make(map[string]components.ExpandState),
		dirtyMessageIDs:   make(map[string]bool),
		viewport:          vp,
		scrollMode:        ScrollAutoFollow,
		renderCache:       components.NewRenderCache(),
		blockList:         bl,
		heightCache:       hc,
		virtualList:       components.NewVirtualMessageList(bl, hc),
	}
}

// Reset clears all chat state.
func (c *ChatFeature) Reset() {
	*c = NewChatFeature()
}
