package components

import "strings"

const (
	// DefaultBlockHeight is the estimated height for unmeasured blocks.
	DefaultBlockHeight = 4
	// OverscanBlocks is the number of extra blocks rendered above/below viewport.
	OverscanBlocks = 2
)

// VirtualMessageList renders only visible blocks from a BlockList,
// using a ScrollAnchor for position tracking and HeightCache for measurements.
type VirtualMessageList struct {
	blocks      *BlockList
	heightCache *HeightCache
	anchor      ScrollAnchor
	viewportH   int
	autoFollow  bool // when true, always anchor to bottom

	lastFrameTopLine int
	lastTotalHeight  int
	lastFrameHeight  int
}

// VirtualFrame is the rendered transcript window for a virtual list pass.
// Content contains only the mounted frame, not top/bottom spacer rows.
type VirtualFrame struct {
	Content         string
	TopLine         int
	ViewportYOffset int
	TotalHeight     int
	ContentHeight   int
	StartBlock      int
	EndBlock        int
}

// NewVirtualMessageList creates a VirtualMessageList with the given dependencies.
func NewVirtualMessageList(blocks *BlockList, hc *HeightCache) *VirtualMessageList {
	return &VirtualMessageList{
		blocks:      blocks,
		heightCache: hc,
		autoFollow:  true,
	}
}

// SetViewportHeight updates the viewport height (call on terminal resize).
func (vl *VirtualMessageList) SetViewportHeight(h int) {
	vl.viewportH = h
}

// ViewportHeight returns the current viewport height.
func (vl *VirtualMessageList) ViewportHeight() int {
	return vl.viewportH
}

// Anchor returns the current scroll anchor.
func (vl *VirtualMessageList) Anchor() ScrollAnchor {
	return vl.anchor
}

// SetAnchor updates the current scroll anchor.
func (vl *VirtualMessageList) SetAnchor(anchor ScrollAnchor) {
	vl.anchor = anchor
}

// AutoFollow returns whether auto-follow is enabled.
func (vl *VirtualMessageList) AutoFollow() bool {
	return vl.autoFollow
}

// SetAutoFollow sets the auto-follow mode.
func (vl *VirtualMessageList) SetAutoFollow(on bool) {
	vl.autoFollow = on
}

// LastFrameTopLine returns the absolute display line for the first row in the
// most recently rendered frame.
func (vl *VirtualMessageList) LastFrameTopLine() int {
	return vl.lastFrameTopLine
}

// LastTotalHeight returns the total display height measured during the most
// recently rendered frame.
func (vl *VirtualMessageList) LastTotalHeight() int {
	return vl.lastTotalHeight
}

// LastFrameHeight returns the rendered height of the most recent mounted frame,
// excluding any non-transcript feedback rows appended by callers.
func (vl *VirtualMessageList) LastFrameHeight() int {
	return vl.lastFrameHeight
}

// AnchorFromFrameYOffset converts a viewport-local offset back into the global
// transcript coordinate model.
func (vl *VirtualMessageList) AnchorFromFrameYOffset(yOffset int) ScrollAnchor {
	if yOffset < 0 {
		yOffset = 0
	}
	if vl.lastFrameHeight > 0 && yOffset >= vl.lastFrameHeight {
		yOffset = vl.lastFrameHeight - 1
	}
	return AnchorFromDisplayLine(vl.heightCache, vl.blocks.All(), vl.lastFrameTopLine+yOffset, DefaultBlockHeight)
}

// VisibleRange returns the start (inclusive) and end (exclusive) block indices
// that should be rendered, including OverscanBlocks padding on each side.
func (vl *VirtualMessageList) VisibleRange() (start, end int) {
	n := vl.blocks.Len()
	if n == 0 || vl.viewportH <= 0 {
		return 0, 0
	}

	allBlocks := vl.blocks.All()

	// If auto-follow, anchor to bottom
	if vl.autoFollow {
		vl.anchorToBottom()
	}

	// Find the first visible block from the semantic anchor.
	vl.anchor = vl.anchor.Normalize(vl.heightCache, allBlocks, DefaultBlockHeight)
	anchorIdx := vl.anchor.BlockIdx

	// The anchor block is the top of the viewport.
	// Walk forward from anchor to fill viewportH lines.
	remaining := vl.viewportH
	endIdx := anchorIdx

	if vl.anchor.BoundaryBefore {
		rows := boundaryRowsBeforeBlock(allBlocks, anchorIdx)
		remaining -= max(0, rows-vl.anchor.BoundaryOffset)
	}

	// Subtract the visible portion of the anchor block
	if firstBlockH, visible := DisplayHeight(vl.heightCache, allBlocks[anchorIdx], DefaultBlockHeight); visible {
		remaining -= (firstBlockH - vl.anchor.LineOffset)
	}
	endIdx++

	// Continue adding blocks until viewport is filled
	for endIdx < n && remaining > 0 {
		h, visible := DisplayHeight(vl.heightCache, allBlocks[endIdx], DefaultBlockHeight)
		if !visible {
			endIdx++
			continue
		}
		if boundaryRows := boundaryRowsBeforeBlock(allBlocks, endIdx); boundaryRows > 0 {
			remaining -= boundaryRows
			if remaining <= 0 {
				break
			}
		}
		remaining -= h
		endIdx++
	}

	// Apply overscan
	start = anchorIdx - OverscanBlocks
	end = endIdx + OverscanBlocks

	// Clamp
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}

	return start, end
}

// countMessageBoundaries counts inter-message boundary rows in blocks[start:end).
func countMessageBoundaries(blocks []BlockVM, start, end int) int {
	count := 0
	var prevVisible *BlockVM
	for i := start; i < end; i++ {
		if IsDisplayHidden(blocks[i]) {
			continue
		}
		if prevVisible != nil {
			count += BoundaryRowsBetween(*prevVisible, blocks[i])
		}
		prevVisible = &blocks[i]
	}
	return count
}

// Render renders only the visible blocks, with empty-line padding for blocks
// above and below the visible range to maintain correct total height.
func (vl *VirtualMessageList) Render(ctx BlockRenderContext) string {
	n := vl.blocks.Len()
	if n == 0 {
		return ""
	}

	start, end := vl.VisibleRange()
	if start == end {
		return ""
	}

	allBlocks := vl.blocks.All()

	firstDisplayIdx := -1
	for i := start; i < end; i++ {
		if _, visible := DisplayHeight(vl.heightCache, allBlocks[i], DefaultBlockHeight); visible {
			firstDisplayIdx = i
			break
		}
	}
	if firstDisplayIdx < 0 {
		return ""
	}

	// Top padding: sum of heights before the first displayable block. The
	// overscan start may land on hidden blocks, so padding must be based on the
	// actual first rendered block to preserve the boundary row before it.
	topPadding := DisplayHeightBefore(vl.heightCache, allBlocks, firstDisplayIdx, DefaultBlockHeight)

	// Render visible blocks with message-boundary spacing
	var sb strings.Builder
	if topPadding > 0 {
		sb.WriteString(strings.Repeat("\n", topPadding))
	}

	// The boundary before the first rendered block is already represented by
	// topPadding, so boundary insertion starts between rendered blocks.
	var prevVisible *BlockVM
	for i := firstDisplayIdx; i < end; i++ {
		rendered := ctx.Render(&allBlocks[i])
		if rendered == "" {
			continue
		}
		if prevVisible != nil {
			if rows := BoundaryRowsBetween(*prevVisible, allBlocks[i]); rows > 0 {
				sb.WriteString(strings.Repeat("\n", rows))
			}
		}
		sb.WriteString(rendered)
		vl.heightCache.Set(allBlocks[i].ID, allBlocks[i].Height)
		prevVisible = &allBlocks[i]
	}

	// Bottom padding is the remaining display height after the visible range.
	consumedH := DisplayHeightBefore(vl.heightCache, allBlocks, end, DefaultBlockHeight) -
		boundaryRowsBeforeBlock(allBlocks, end)
	bottomH := DisplayTotalHeight(vl.heightCache, allBlocks, DefaultBlockHeight) -
		consumedH
	if bottomH < 0 {
		bottomH = 0
	}
	if bottomH > 0 {
		sb.WriteString(strings.Repeat("\n", bottomH))
	}

	return sb.String()
}

// Frame renders the mounted virtual window without materializing spacer rows
// for the off-screen transcript. The caller should set the terminal viewport
// content to Content and its local YOffset to ViewportYOffset.
func (vl *VirtualMessageList) Frame(ctx BlockRenderContext) VirtualFrame {
	n := vl.blocks.Len()
	if n == 0 {
		vl.lastFrameTopLine = 0
		vl.lastTotalHeight = 0
		vl.lastFrameHeight = 0
		return VirtualFrame{}
	}

	start, end := vl.VisibleRange()
	if start == end {
		vl.lastFrameTopLine = 0
		vl.lastTotalHeight = DisplayTotalHeight(vl.heightCache, vl.blocks.All(), DefaultBlockHeight)
		vl.lastFrameHeight = 0
		return VirtualFrame{TotalHeight: vl.lastTotalHeight}
	}

	allBlocks := vl.blocks.All()
	firstDisplayIdx := -1
	for i := start; i < end; i++ {
		if _, visible := DisplayHeight(vl.heightCache, allBlocks[i], DefaultBlockHeight); visible {
			firstDisplayIdx = i
			break
		}
	}
	if firstDisplayIdx < 0 {
		vl.lastFrameTopLine = 0
		vl.lastTotalHeight = DisplayTotalHeight(vl.heightCache, allBlocks, DefaultBlockHeight)
		vl.lastFrameHeight = 0
		return VirtualFrame{TotalHeight: vl.lastTotalHeight}
	}

	frameTopLine := DisplayHeightBefore(vl.heightCache, allBlocks, firstDisplayIdx, DefaultBlockHeight)
	var sb strings.Builder
	if rows := boundaryRowsBeforeBlock(allBlocks, firstDisplayIdx); rows > 0 {
		frameTopLine -= rows
		sb.WriteString(strings.Repeat("\n", rows))
	}

	var prevVisible *BlockVM
	for i := firstDisplayIdx; i < end; i++ {
		rendered := ctx.Render(&allBlocks[i])
		if rendered == "" {
			continue
		}
		if prevVisible != nil {
			if rows := BoundaryRowsBetween(*prevVisible, allBlocks[i]); rows > 0 {
				sb.WriteString(strings.Repeat("\n", rows))
			}
		}
		sb.WriteString(rendered)
		vl.heightCache.Set(allBlocks[i].ID, allBlocks[i].Height)
		prevVisible = &allBlocks[i]
	}

	content := sb.String()
	contentHeight := MeasureRenderedHeight(content)
	totalHeight := DisplayTotalHeight(vl.heightCache, allBlocks, DefaultBlockHeight)
	anchorLine := vl.anchor.ResolveDisplayLine(vl.heightCache, allBlocks, DefaultBlockHeight)
	viewportYOffset := anchorLine - frameTopLine
	maxFrameOffset := max(0, contentHeight-vl.viewportH)
	viewportYOffset = min(max(0, viewportYOffset), maxFrameOffset)

	vl.lastFrameTopLine = frameTopLine
	vl.lastTotalHeight = totalHeight
	vl.lastFrameHeight = contentHeight

	return VirtualFrame{
		Content:         content,
		TopLine:         frameTopLine,
		ViewportYOffset: viewportYOffset,
		TotalHeight:     totalHeight,
		ContentHeight:   contentHeight,
		StartBlock:      firstDisplayIdx,
		EndBlock:        end,
	}
}

// ScrollToBottom anchors to the last block, setting auto-follow.
func (vl *VirtualMessageList) ScrollToBottom() {
	vl.autoFollow = true
	vl.anchorToBottom()
}

// ScrollBy scrolls by delta lines (positive = down, negative = up).
// Disables auto-follow when scrolling up.
func (vl *VirtualMessageList) ScrollBy(delta int) {
	n := vl.blocks.Len()
	if n == 0 {
		return
	}

	allBlocks := vl.blocks.All()

	// Convert current anchor to rendered-display line.
	absLine := vl.anchor.ResolveDisplayLine(vl.heightCache, allBlocks, DefaultBlockHeight)
	absLine += delta

	// Clamp to valid range
	totalH := DisplayTotalHeight(vl.heightCache, allBlocks, DefaultBlockHeight)
	maxLine := max(totalH-vl.viewportH, 0)
	absLine = max(absLine, 0)
	absLine = min(absLine, maxLine)

	vl.anchor = AnchorFromDisplayLine(vl.heightCache, allBlocks, absLine, DefaultBlockHeight)

	// Check if we're at the bottom
	if absLine >= maxLine {
		vl.autoFollow = true
	} else if delta < 0 {
		vl.autoFollow = false
	}
}

// ScrollToBlock jumps to the specified block index (for search navigation).
// Disables auto-follow.
func (vl *VirtualMessageList) ScrollToBlock(blockIdx int) {
	n := vl.blocks.Len()
	if n == 0 {
		return
	}
	blockIdx = max(blockIdx, 0)
	blockIdx = min(blockIdx, n-1)

	vl.anchor = AnchorForBlock(vl.blocks.All(), blockIdx, 0)
	vl.autoFollow = false
}

// anchorToBottom positions the anchor so the last block is at the bottom of viewport.
func (vl *VirtualMessageList) anchorToBottom() {
	n := vl.blocks.Len()
	if n == 0 {
		return
	}

	allBlocks := vl.blocks.All()
	totalH := DisplayTotalHeight(vl.heightCache, allBlocks, DefaultBlockHeight)
	topLine := max(totalH-vl.viewportH, 0)

	vl.anchor = AnchorFromDisplayLine(vl.heightCache, allBlocks, topLine, DefaultBlockHeight)
}
