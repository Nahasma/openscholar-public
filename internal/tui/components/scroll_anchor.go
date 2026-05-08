package components

// ScrollAnchor identifies a scroll position within a BlockList by stable block
// identity when possible, with block index kept as a fallback for legacy callers.
type ScrollAnchor struct {
	BlockID        string // stable block identity, preferred when resolving
	MsgID          string // parent message identity, fallback when the block changed
	BlockIdx       int    // index in BlockList, fallback when IDs cannot be found
	LineOffset     int    // line offset within the block
	BoundaryBefore bool   // true when anchored to the inter-message row before BlockIdx
	BoundaryOffset int    // row offset within a multi-row boundary
}

// AnchorForBlock builds an anchor for the given block index and line offset.
func AnchorForBlock(blocks []BlockVM, blockIdx, lineOffset int) ScrollAnchor {
	if len(blocks) == 0 {
		return ScrollAnchor{}
	}
	if blockIdx < 0 {
		blockIdx = 0
	}
	if blockIdx >= len(blocks) {
		blockIdx = len(blocks) - 1
	}
	if lineOffset < 0 {
		lineOffset = 0
	}
	block := blocks[blockIdx]
	return ScrollAnchor{
		BlockID:    block.ID,
		MsgID:      block.MsgID,
		BlockIdx:   blockIdx,
		LineOffset: lineOffset,
	}
}

// AnchorForBoundaryBefore builds an anchor for the separator row before a block.
func AnchorForBoundaryBefore(blocks []BlockVM, blockIdx int) ScrollAnchor {
	return AnchorForBoundaryBeforeOffset(blocks, blockIdx, 0)
}

// AnchorForBoundaryBeforeOffset builds an anchor for a specific separator row
// before a block.
func AnchorForBoundaryBeforeOffset(blocks []BlockVM, blockIdx, boundaryOffset int) ScrollAnchor {
	rows := boundaryRowsBeforeBlock(blocks, blockIdx)
	if rows <= 0 {
		return AnchorForBlock(blocks, blockIdx, 0)
	}
	anchor := AnchorForBlock(blocks, blockIdx, 0)
	anchor.BoundaryBefore = true
	if boundaryOffset < 0 {
		boundaryOffset = 0
	}
	if boundaryOffset >= rows {
		boundaryOffset = rows - 1
	}
	anchor.BoundaryOffset = boundaryOffset
	return anchor
}

// ResolveBlockIndex returns the best current block index for the anchor.
func (a ScrollAnchor) ResolveBlockIndex(blocks []BlockVM) int {
	if len(blocks) == 0 {
		return 0
	}
	if a.BlockID != "" {
		for i, block := range blocks {
			if block.ID == a.BlockID {
				return i
			}
		}
	}
	if a.MsgID != "" {
		if a.BlockIdx >= 0 && a.BlockIdx < len(blocks) && blocks[a.BlockIdx].MsgID == a.MsgID {
			return a.BlockIdx
		}
		for i, block := range blocks {
			if block.MsgID == a.MsgID {
				return i
			}
		}
	}
	if a.BlockIdx < 0 {
		return 0
	}
	if a.BlockIdx >= len(blocks) {
		return len(blocks) - 1
	}
	return a.BlockIdx
}

// Normalize refreshes the fallback index and clamps the line offset for the
// current block layout.
func (a ScrollAnchor) Normalize(hc *HeightCache, blocks []BlockVM, defaultH int) ScrollAnchor {
	if len(blocks) == 0 {
		return ScrollAnchor{}
	}
	idx := a.ResolveBlockIndex(blocks)
	idx, ok := nearestVisibleBlockIndex(hc, blocks, idx, defaultH)
	if !ok {
		return ScrollAnchor{}
	}
	if a.BoundaryBefore {
		if boundaryRowsBeforeBlock(blocks, idx) > 0 {
			return AnchorForBoundaryBeforeOffset(blocks, idx, a.BoundaryOffset)
		}
		return AnchorForBlock(blocks, idx, 0)
	}
	offset := a.LineOffset
	if offset < 0 {
		offset = 0
	}
	height, ok := DisplayHeight(hc, blocks[idx], defaultH)
	if !ok {
		return ScrollAnchor{}
	}
	if height < 1 {
		height = 1
	}
	if offset >= height {
		offset = height - 1
	}
	return AnchorForBlock(blocks, idx, offset)
}

// Resolve converts the anchor to an absolute line offset from the top of the
// block list, using the HeightCache for block heights.
func (a ScrollAnchor) Resolve(hc *HeightCache, blocks []BlockVM, defaultH int) int {
	if len(blocks) == 0 {
		return 0
	}
	normalized := a.Normalize(hc, blocks, defaultH)
	if normalized.BoundaryBefore {
		return hc.HeightBefore(blocks, normalized.BlockIdx, defaultH)
	}
	return hc.HeightBefore(blocks, normalized.BlockIdx, defaultH) + normalized.LineOffset
}

// ResolveDisplayLine converts the anchor to the rendered transcript line model,
// where inter-message boundary rows are counted separately from block heights.
func (a ScrollAnchor) ResolveDisplayLine(hc *HeightCache, blocks []BlockVM, defaultH int) int {
	if len(blocks) == 0 {
		return 0
	}
	normalized := a.Normalize(hc, blocks, defaultH)
	if normalized.BoundaryBefore {
		rows := boundaryRowsBeforeBlock(blocks, normalized.BlockIdx)
		return max(0, DisplayHeightBefore(hc, blocks, normalized.BlockIdx, defaultH)-rows+normalized.BoundaryOffset)
	}
	return DisplayHeightBefore(hc, blocks, normalized.BlockIdx, defaultH) + normalized.LineOffset
}

// DisplayHeightBefore returns rendered line height before idx, including
// inter-message boundary rows.
func DisplayHeightBefore(hc *HeightCache, blocks []BlockVM, idx int, defaultH int) int {
	return hc.HeightBefore(blocks, idx, defaultH) + countMessageBoundariesBefore(blocks, idx)
}

// DisplayTotalHeight returns rendered transcript height, including
// inter-message boundary rows.
func DisplayTotalHeight(hc *HeightCache, blocks []BlockVM, defaultH int) int {
	return hc.TotalHeight(blocks, defaultH) + countMessageBoundariesBefore(blocks, len(blocks))
}

// AnchorFromLine finds the ScrollAnchor for the given absolute line offset.
// If the line exceeds total height, it anchors to the last block.
func AnchorFromLine(hc *HeightCache, blocks []BlockVM, absLine int, defaultH int) ScrollAnchor {
	if absLine < 0 {
		absLine = 0
	}
	accumulated := 0
	for i, b := range blocks {
		h, visible := DisplayHeight(hc, b, defaultH)
		if !visible {
			continue
		}
		if accumulated+h > absLine {
			return AnchorForBlock(blocks, i, max(0, absLine-accumulated))
		}
		accumulated += h
	}
	// Past the end — anchor to last block
	if len(blocks) == 0 {
		return ScrollAnchor{}
	}
	lastIdx, ok := nearestVisibleBlockIndex(hc, blocks, len(blocks)-1, defaultH)
	if !ok {
		return ScrollAnchor{}
	}
	lastH, ok := DisplayHeight(hc, blocks[lastIdx], defaultH)
	if !ok {
		return ScrollAnchor{}
	}
	return AnchorForBlock(blocks, lastIdx, max(0, lastH-1))
}

func countMessageBoundariesBefore(blocks []BlockVM, idx int) int {
	if idx > len(blocks) {
		idx = len(blocks)
	}
	count := 0
	var prevVisible *BlockVM
	for i := 0; i <= idx && i < len(blocks); i++ {
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

func hasMessageBoundaryBefore(blocks []BlockVM, idx int) bool {
	return boundaryRowsBeforeBlock(blocks, idx) > 0
}

func boundaryRowsBeforeBlock(blocks []BlockVM, idx int) int {
	if idx <= 0 || idx >= len(blocks) || IsDisplayHidden(blocks[idx]) {
		return 0
	}
	for i := idx - 1; i >= 0; i-- {
		if IsDisplayHidden(blocks[i]) {
			continue
		}
		return BoundaryRowsBetween(blocks[i], blocks[idx])
	}
	return 0
}

func nearestVisibleBlockIndex(hc *HeightCache, blocks []BlockVM, idx int, defaultH int) (int, bool) {
	if len(blocks) == 0 {
		return 0, false
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(blocks) {
		idx = len(blocks) - 1
	}
	for i := idx; i < len(blocks); i++ {
		if _, ok := DisplayHeight(hc, blocks[i], defaultH); ok {
			return i, true
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if _, ok := DisplayHeight(hc, blocks[i], defaultH); ok {
			return i, true
		}
	}
	return 0, false
}

// AnchorFromDisplayLine finds the ScrollAnchor for a rendered transcript line,
// counting inter-message boundary rows separately from block heights.
func AnchorFromDisplayLine(hc *HeightCache, blocks []BlockVM, absLine int, defaultH int) ScrollAnchor {
	if absLine < 0 {
		absLine = 0
	}
	accumulated := 0
	var prevVisible *BlockVM
	for i, b := range blocks {
		h, visible := DisplayHeight(hc, b, defaultH)
		if !visible {
			continue
		}
		if prevVisible != nil {
			boundaryRows := BoundaryRowsBetween(*prevVisible, b)
			if boundaryRows > 0 {
				if absLine >= accumulated && absLine < accumulated+boundaryRows {
					return AnchorForBoundaryBeforeOffset(blocks, i, absLine-accumulated)
				}
				accumulated += boundaryRows
			}
		}

		if accumulated+h > absLine {
			return AnchorForBlock(blocks, i, max(0, absLine-accumulated))
		}
		accumulated += h
		prevVisible = &blocks[i]
	}
	if len(blocks) == 0 {
		return ScrollAnchor{}
	}
	lastIdx, ok := nearestVisibleBlockIndex(hc, blocks, len(blocks)-1, defaultH)
	if !ok {
		return ScrollAnchor{}
	}
	lastH, ok := DisplayHeight(hc, blocks[lastIdx], defaultH)
	if !ok {
		return ScrollAnchor{}
	}
	return AnchorForBlock(blocks, lastIdx, max(0, lastH-1))
}
