package components

import (
	"fmt"
	"strings"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// makeIntegrationList builds a complete VirtualMessageList + HeightCache +
// BlockList with n pre-rendered blocks of uniform height blockH.
func makeIntegrationList(n, blockH, viewportH int) (*VirtualMessageList, *BlockList, *HeightCache) {
	hc := NewHeightCache()
	blocks := make([]BlockVM, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("int-msg%d-0", i)
		// Build a rendered string with blockH newlines so RenderBlock returns
		// a cache-hit of the correct measured height.
		lines := make([]string, blockH)
		for l := 0; l < blockH; l++ {
			lines[l] = fmt.Sprintf("block%d-line%d", i, l)
		}
		rendered := strings.Join(lines, "\n")
		blocks[i] = BlockVM{
			ID:       id,
			MsgID:    fmt.Sprintf("int-msg%d", i),
			Kind:     BlockUser,
			Content:  fmt.Sprintf("block %d", i),
			Rendered: rendered,
			WidthKey: 80,
			Height:   blockH,
			Dirty:    false,
		}
		hc.Set(id, blockH)
	}
	bl := NewBlockList()
	bl.RebuildAll(blocks)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(viewportH)
	return vl, bl, hc
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestVirtualScrollIntegration_RenderOnlyVisible verifies that Render() only
// produces content for blocks inside VisibleRange; blocks outside appear only
// as blank padding lines.
func TestVirtualScrollIntegration_RenderOnlyVisible(t *testing.T) {
	const (
		n         = 50
		blockH    = 4
		viewportH = 20 // ~5 blocks fit
	)
	vl, bl, hc := makeIntegrationList(n, blockH, viewportH)
	vl.SetAutoFollow(false)
	vl.anchor = ScrollAnchor{BlockIdx: 20, LineOffset: 0}

	ctx := BlockRenderContext{Width: 80}
	output := vl.Render(ctx)

	start, end := vl.VisibleRange()

	// Every block inside the visible window must appear in output.
	allBlocks := bl.All()
	for i := start; i < end; i++ {
		label := fmt.Sprintf("block%d-line0", i)
		if !strings.Contains(output, label) {
			t.Errorf("visible block %d (%s) not found in Render output", i, label)
		}
	}

	// Blocks well outside the visible window (with margin > OverscanBlocks)
	// should NOT appear as rendered content — they should be blank newlines.
	for i := 0; i < start-OverscanBlocks-1; i++ {
		label := fmt.Sprintf("block%d-line0", i)
		if strings.Contains(output, label) {
			t.Errorf("non-visible block %d (%s) unexpectedly found in Render output", i, label)
		}
	}

	// Verify top padding is present when start > 0.
	topPadding := hc.HeightBefore(allBlocks, start, DefaultBlockHeight)
	if start > 0 && topPadding > 0 {
		if !strings.HasPrefix(output, "\n") {
			t.Error("expected top padding newlines at the start of Render output")
		}
	}
}

// TestVirtualScrollIntegration_ScrollPreservesAnchor verifies that after
// ScrollBy the anchor resolves to the expected absolute line position.
func TestVirtualScrollIntegration_ScrollPreservesAnchor(t *testing.T) {
	const (
		n         = 40
		blockH    = 5
		viewportH = 15
	)
	vl, bl, hc := makeIntegrationList(n, blockH, viewportH)
	vl.SetAutoFollow(false)
	vl.anchor = ScrollAnchor{BlockIdx: 0, LineOffset: 0}

	// Scroll down by 10 lines.
	vl.ScrollBy(10)
	absLine := vl.anchor.ResolveDisplayLine(hc, bl.All(), DefaultBlockHeight)
	if absLine != 10 {
		t.Errorf("after ScrollBy(10) expected absLine=10, got %d", absLine)
	}

	// Scroll up by 3 lines.
	vl.ScrollBy(-3)
	absLine = vl.anchor.ResolveDisplayLine(hc, bl.All(), DefaultBlockHeight)
	if absLine != 7 {
		t.Errorf("after ScrollBy(-3) expected absLine=7, got %d", absLine)
	}

	// Scroll up past the top — should clamp to 0.
	vl.ScrollBy(-1000)
	absLine = vl.anchor.ResolveDisplayLine(hc, bl.All(), DefaultBlockHeight)
	if absLine != 0 {
		t.Errorf("after over-scroll up expected absLine=0, got %d", absLine)
	}

	if vl.AutoFollow() {
		t.Error("AutoFollow should be false after manual scroll up")
	}
}

// TestVirtualScrollIntegration_AutoFollowOnNewMessage verifies that when
// AutoFollow is enabled the visible range always includes the last block, and
// that adding new messages keeps the view anchored to the bottom.
func TestVirtualScrollIntegration_AutoFollowOnNewMessage(t *testing.T) {
	const (
		n         = 20
		blockH    = 4
		viewportH = 16
	)
	vl, bl, hc := makeIntegrationList(n, blockH, viewportH)
	// AutoFollow is true by default.

	_, end := vl.VisibleRange()
	if end != n {
		t.Errorf("auto-follow: initial end expected %d, got %d", n, end)
	}

	// Simulate "new message arrived": add more blocks.
	extra := make([]BlockVM, n+5)
	copy(extra, bl.All())
	for i := n; i < n+5; i++ {
		id := fmt.Sprintf("int-msg%d-0", i)
		extra[i] = BlockVM{
			ID:       id,
			MsgID:    fmt.Sprintf("int-msg%d", i),
			Kind:     BlockUser,
			Content:  fmt.Sprintf("new block %d", i),
			Rendered: fmt.Sprintf("new block %d line0\nnew block %d line1", i, i),
			WidthKey: 80,
			Height:   blockH,
			Dirty:    false,
		}
		hc.Set(id, blockH)
	}
	bl.RebuildAll(extra)

	_, end = vl.VisibleRange()
	if end != n+5 {
		t.Errorf("auto-follow after append: expected end=%d, got %d", n+5, end)
	}

	// Manually scroll up → AutoFollow should turn off.
	vl.ScrollBy(-blockH)
	if vl.AutoFollow() {
		t.Error("AutoFollow should be false after manual scroll up")
	}

	// Scrolling back to bottom should re-enable AutoFollow.
	vl.ScrollToBottom()
	if !vl.AutoFollow() {
		t.Error("AutoFollow should be true after ScrollToBottom")
	}
	_, end = vl.VisibleRange()
	if end != n+5 {
		t.Errorf("after ScrollToBottom expected end=%d, got %d", n+5, end)
	}
}

// TestVirtualScrollIntegration_ScrollToBlockAccuracy verifies that
// ScrollToBlock positions the anchor exactly on the requested block index.
func TestVirtualScrollIntegration_ScrollToBlockAccuracy(t *testing.T) {
	const (
		n         = 100
		blockH    = 3
		viewportH = 12
	)
	vl, _, _ := makeIntegrationList(n, blockH, viewportH)

	targets := []int{0, 1, 49, 50, 98, 99}
	for _, target := range targets {
		vl.ScrollToBlock(target)

		if vl.AutoFollow() {
			t.Errorf("ScrollToBlock(%d): expected AutoFollow=false", target)
		}
		if vl.anchor.BlockIdx != target {
			t.Errorf("ScrollToBlock(%d): anchor.BlockIdx=%d", target, vl.anchor.BlockIdx)
		}
		if vl.anchor.LineOffset != 0 {
			t.Errorf("ScrollToBlock(%d): anchor.LineOffset=%d (expected 0)", target, vl.anchor.LineOffset)
		}

		// The requested block should appear inside VisibleRange.
		start, end := vl.VisibleRange()
		if target < start || target >= end {
			t.Errorf("ScrollToBlock(%d): block not in VisibleRange [%d,%d)", target, start, end)
		}
	}
}

// TestVirtualScrollIntegration_ResizePreservesPosition verifies that changing
// the viewport height via SetViewportHeight updates VisibleRange without
// corrupting the anchor position.
func TestVirtualScrollIntegration_ResizePreservesPosition(t *testing.T) {
	const (
		n         = 60
		blockH    = 4
		viewportH = 20
	)
	vl, bl, hc := makeIntegrationList(n, blockH, viewportH)
	vl.SetAutoFollow(false)
	vl.anchor = ScrollAnchor{BlockIdx: 30, LineOffset: 0}

	// Record anchor's absolute line before resize.
	beforeLine := vl.anchor.Resolve(hc, bl.All(), DefaultBlockHeight)

	// Resize to a smaller viewport.
	vl.SetViewportHeight(10)
	afterLine := vl.anchor.Resolve(hc, bl.All(), DefaultBlockHeight)

	if beforeLine != afterLine {
		t.Errorf("resize changed anchor line: before=%d after=%d", beforeLine, afterLine)
	}

	// The visible range should still include the anchor block.
	start, end := vl.VisibleRange()
	anchorIdx := vl.anchor.BlockIdx
	if anchorIdx < start || anchorIdx >= end {
		t.Errorf("anchor block %d not in VisibleRange [%d,%d) after resize", anchorIdx, start, end)
	}

	// Resize to a larger viewport.
	vl.SetViewportHeight(40)
	afterLine2 := vl.anchor.Resolve(hc, bl.All(), DefaultBlockHeight)
	if beforeLine != afterLine2 {
		t.Errorf("large resize changed anchor line: before=%d after=%d", beforeLine, afterLine2)
	}
}

// TestVirtualScrollIntegration_HeightCacheConsistency verifies that after
// Render() the HeightCache reflects the actual rendered heights of visible
// blocks (as measured by RenderBlock).
func TestVirtualScrollIntegration_HeightCacheConsistency(t *testing.T) {
	const (
		n         = 20
		blockH    = 3
		viewportH = 12
	)
	vl, bl, hc := makeIntegrationList(n, blockH, viewportH)
	vl.SetAutoFollow(false)
	vl.anchor = ScrollAnchor{BlockIdx: 5, LineOffset: 0}

	ctx := BlockRenderContext{Width: 80}
	_ = vl.Render(ctx)

	start, end := vl.VisibleRange()
	allBlocks := bl.All()
	for i := start; i < end; i++ {
		b := &allBlocks[i]
		cached, ok := hc.Get(b.ID)
		if !ok {
			t.Errorf("block %d (%s) has no cached height after Render", i, b.ID)
			continue
		}
		expected := blockH
		if cached != expected {
			t.Errorf("block %d: expected cached height=%d, got %d", i, expected, cached)
		}
	}
}

// TestVirtualScrollIntegration_AnchorFromLineClamp verifies that AnchorFromLine
// handles boundary conditions: absLine=0, absLine=totalHeight, and
// absLine > totalHeight all return valid anchors within bounds.
func TestVirtualScrollIntegration_AnchorFromLineClamp(t *testing.T) {
	const (
		n      = 10
		blockH = 5
	)
	hc := NewHeightCache()
	blocks := make([]BlockVM, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("clamp-msg%d-0", i)
		blocks[i] = BlockVM{ID: id, MsgID: fmt.Sprintf("clamp-msg%d", i)}
		hc.Set(id, blockH)
	}
	bl := NewBlockList()
	bl.RebuildAll(blocks)
	allBlocks := bl.All()
	totalH := hc.TotalHeight(allBlocks, DefaultBlockHeight)

	// absLine = 0 → should land on block 0, offset 0.
	a0 := AnchorFromLine(hc, allBlocks, 0, DefaultBlockHeight)
	if a0.BlockIdx != 0 || a0.LineOffset != 0 {
		t.Errorf("absLine=0: expected {0,0}, got {%d,%d}", a0.BlockIdx, a0.LineOffset)
	}

	// absLine = totalH - 1 → should land on last block, last line.
	aLast := AnchorFromLine(hc, allBlocks, totalH-1, DefaultBlockHeight)
	if aLast.BlockIdx != n-1 {
		t.Errorf("absLine=totalH-1: expected BlockIdx=%d, got %d", n-1, aLast.BlockIdx)
	}
	if aLast.LineOffset < 0 {
		t.Errorf("absLine=totalH-1: LineOffset should be >=0, got %d", aLast.LineOffset)
	}

	// absLine > totalH → should clamp to last block without panic.
	aOver := AnchorFromLine(hc, allBlocks, totalH+100, DefaultBlockHeight)
	if aOver.BlockIdx < 0 || aOver.BlockIdx >= n {
		t.Errorf("absLine>totalH: BlockIdx %d out of [0,%d)", aOver.BlockIdx, n)
	}

	// absLine = 0 on empty slice → should return zero anchor without panic.
	aEmpty := AnchorFromLine(hc, []BlockVM{}, 0, DefaultBlockHeight)
	if aEmpty.BlockIdx != 0 || aEmpty.LineOffset != 0 {
		t.Errorf("empty blocks: expected zero anchor, got {%d,%d}", aEmpty.BlockIdx, aEmpty.LineOffset)
	}
}
