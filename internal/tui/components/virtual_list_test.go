package components

import (
	"fmt"
	"strings"
	"testing"
)

// makeTestBlocks creates n test blocks with sequential IDs and pre-set heights in cache.
func makeTestBlocks(n int, heights []int, hc *HeightCache) *BlockList {
	bl := NewBlockList()
	blocks := make([]BlockVM, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("msg%d-0", i)
		h := DefaultBlockHeight
		if i < len(heights) {
			h = heights[i]
		}
		blocks[i] = BlockVM{
			ID:      id,
			MsgID:   fmt.Sprintf("msg%d", i),
			Kind:    BlockUser,
			Content: fmt.Sprintf("Block %d content", i),
			Dirty:   false, // avoid actual rendering
		}
		hc.Set(id, h)
	}
	bl.RebuildAll(blocks)
	return bl
}

func TestVisibleRange_Empty(t *testing.T) {
	hc := NewHeightCache()
	bl := NewBlockList()
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(20)

	start, end := vl.VisibleRange()
	if start != 0 || end != 0 {
		t.Errorf("empty list: expected (0,0), got (%d,%d)", start, end)
	}
}

func TestVisibleRange_ZeroViewport(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(10, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	// viewportH = 0 (default)

	start, end := vl.VisibleRange()
	if start != 0 || end != 0 {
		t.Errorf("zero viewport: expected (0,0), got (%d,%d)", start, end)
	}
}

func TestVisibleRange_FewBlocks(t *testing.T) {
	hc := NewHeightCache()
	// 3 blocks, each height 4 = total 12, viewport 20
	bl := makeTestBlocks(3, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(20)

	start, end := vl.VisibleRange()
	// All blocks should be visible (total height < viewport)
	if start != 0 {
		t.Errorf("expected start=0, got %d", start)
	}
	if end != 3 {
		t.Errorf("expected end=3, got %d", end)
	}
}

func TestVisibleRange_ManyBlocks(t *testing.T) {
	hc := NewHeightCache()
	// 50 blocks, each height 4 = total 200, viewport 20
	bl := makeTestBlocks(50, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(20)

	// Auto-follow → anchored to bottom
	start, end := vl.VisibleRange()

	// Should not include all 50 blocks
	visibleCount := end - start
	if visibleCount >= 50 {
		t.Errorf("expected fewer than 50 visible blocks, got %d", visibleCount)
	}

	// End should be 50 (includes last block + overscan clamped)
	if end != 50 {
		t.Errorf("auto-follow: expected end=50, got %d", end)
	}

	// The visible range should be reasonable (viewport/blockH + 2*overscan)
	expectedMax := (20/DefaultBlockHeight + 1) + 2*OverscanBlocks + 1
	if visibleCount > expectedMax {
		t.Errorf("visible count %d exceeds expected max %d", visibleCount, expectedMax)
	}
}

func TestVisibleRange_WithOverscan(t *testing.T) {
	hc := NewHeightCache()
	// 20 blocks, each height 4, viewport 12 (= 3 blocks fit)
	bl := makeTestBlocks(20, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(12)

	// Scroll to middle
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 10, LineOffset: 0}

	start, end := vl.VisibleRange()

	// Start should be 10 - OverscanBlocks = 8
	if start != 10-OverscanBlocks {
		t.Errorf("expected start=%d, got %d", 10-OverscanBlocks, start)
	}

	// End should be around 13 + OverscanBlocks = 15
	// (blocks 10,11,12 fill viewport, + 2 overscan)
	if end < 13 || end > 16 {
		t.Errorf("expected end around 15, got %d", end)
	}
}

func TestVisibleRange_OverscanClamped(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(5, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(8)

	// Anchor at block 0
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 0, LineOffset: 0}

	start, end := vl.VisibleRange()

	// Start overscan should be clamped to 0
	if start != 0 {
		t.Errorf("expected start=0 (clamped), got %d", start)
	}
	if end > 5 {
		t.Errorf("expected end<=5, got %d", end)
	}
}

func TestScrollToBottom(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(20, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(12)
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 0, LineOffset: 0}

	vl.ScrollToBottom()

	if !vl.AutoFollow() {
		t.Error("expected autoFollow=true after ScrollToBottom")
	}

	_, end := vl.VisibleRange()
	if end != 20 {
		t.Errorf("expected end=20 after ScrollToBottom, got %d", end)
	}
}

func TestScrollBy_Down(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(20, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(12)
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 0, LineOffset: 0}

	vl.ScrollBy(8) // scroll down 8 lines = 2 blocks

	absLine := vl.anchor.ResolveDisplayLine(hc, bl.All(), DefaultBlockHeight)
	if absLine != 8 {
		t.Errorf("expected absLine=8 after ScrollBy(8), got %d", absLine)
	}
}

func TestScrollBy_Up(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(20, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(12)
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 5, LineOffset: 0}

	vl.ScrollBy(-4) // scroll up 4 lines

	if vl.AutoFollow() {
		t.Error("expected autoFollow=false after scrolling up")
	}

	absLine := vl.anchor.ResolveDisplayLine(hc, bl.All(), DefaultBlockHeight)
	// Was at block 5 (display line 25, including boundaries), scroll up 4 → line 21.
	if absLine != 21 {
		t.Errorf("expected absLine=21, got %d", absLine)
	}
}

func TestScrollBy_ClampTop(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(10, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(12)
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 1, LineOffset: 0}

	vl.ScrollBy(-100) // scroll way past top

	absLine := vl.anchor.ResolveDisplayLine(hc, bl.All(), DefaultBlockHeight)
	if absLine != 0 {
		t.Errorf("expected clamped to 0, got %d", absLine)
	}
}

func TestScrollBy_ClampBottom(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(10, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(12)
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 5, LineOffset: 0}

	vl.ScrollBy(1000) // scroll way past bottom

	if !vl.AutoFollow() {
		t.Error("expected autoFollow=true after scrolling to bottom")
	}
}

func TestScrollToBlock(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(20, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(12)

	vl.ScrollToBlock(10)

	if vl.AutoFollow() {
		t.Error("expected autoFollow=false after ScrollToBlock")
	}
	if vl.anchor.BlockIdx != 10 {
		t.Errorf("expected anchor at block 10, got %d", vl.anchor.BlockIdx)
	}
	if vl.anchor.LineOffset != 0 {
		t.Errorf("expected LineOffset=0, got %d", vl.anchor.LineOffset)
	}
}

func TestScrollToBlock_Clamped(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(5, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(12)

	vl.ScrollToBlock(100) // past end
	if vl.anchor.BlockIdx != 4 {
		t.Errorf("expected clamped to 4, got %d", vl.anchor.BlockIdx)
	}

	vl.ScrollToBlock(-5) // before start
	if vl.anchor.BlockIdx != 0 {
		t.Errorf("expected clamped to 0, got %d", vl.anchor.BlockIdx)
	}
}

func TestAutoFollow_AnchorToBottom(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(30, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(12)
	vl.autoFollow = true

	_, end := vl.VisibleRange()
	// Auto-follow should always include the last block
	if end != 30 {
		t.Errorf("auto-follow: expected end=30, got %d", end)
	}
}

func TestRender_OnlyVisibleBlocks(t *testing.T) {
	hc := NewHeightCache()
	// 10 blocks, each height 3 lines, viewport 6 (fits ~2 blocks)
	heights := make([]int, 10)
	for i := range heights {
		heights[i] = 3
	}
	bl := makeTestBlocks(10, heights, hc)

	// Pre-set rendered content to avoid actual rendering
	allBlocks := bl.All()
	for i := range allBlocks {
		allBlocks[i].Dirty = false
		allBlocks[i].Rendered = fmt.Sprintf("BLOCK_%d_LINE1\nBLOCK_%d_LINE2\nBLOCK_%d_LINE3", i, i, i)
		allBlocks[i].WidthKey = 80
		allBlocks[i].Height = 3
	}
	bl.RebuildAll(allBlocks)
	// Re-set heights after rebuild
	for i := range allBlocks {
		hc.Set(allBlocks[i].ID, 3)
	}

	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(6)
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 4, LineOffset: 0}

	ctx := BlockRenderContext{Width: 80}
	output := vl.Render(ctx)

	// Should contain blocks around index 4
	if !strings.Contains(output, "BLOCK_4_LINE1") {
		t.Error("expected output to contain BLOCK_4 content")
	}

	// Should NOT contain blocks far from viewport
	if strings.Contains(output, "BLOCK_0_LINE1") {
		t.Error("expected output NOT to contain BLOCK_0 content")
	}
}

func TestRender_TopBottomPadding(t *testing.T) {
	hc := NewHeightCache()
	// 10 blocks, each height 2, viewport 4
	heights := make([]int, 10)
	for i := range heights {
		heights[i] = 2
	}
	bl := makeTestBlocks(10, heights, hc)

	allBlocks := bl.All()
	for i := range allBlocks {
		allBlocks[i].Dirty = false
		allBlocks[i].Rendered = fmt.Sprintf("B%d\nB%d", i, i)
		allBlocks[i].WidthKey = 80
		allBlocks[i].Height = 2
	}
	bl.RebuildAll(allBlocks)
	for i := range allBlocks {
		hc.Set(allBlocks[i].ID, 2)
	}

	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(4)
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 5, LineOffset: 0} // middle

	ctx := BlockRenderContext{Width: 80}
	output := vl.Render(ctx)

	// Verify top padding exists (blocks 0..start have heights summed as newlines)
	// and bottom padding exists
	start, end := vl.VisibleRange()
	topExpected := vl.heightCache.HeightBefore(bl.All(), start, DefaultBlockHeight)
	if topExpected > 0 && !strings.HasPrefix(output, "\n") {
		t.Error("expected top padding newlines")
	}

	// Verify rendered content from anchor block is present
	if !strings.Contains(output, "B5") {
		t.Error("expected anchor block content B5")
	}

	// Verify blocks outside visible range are not rendered
	if strings.Contains(output, "B0\n") {
		t.Error("expected block 0 to be padding, not rendered content")
	}

	_ = end
}

func TestFrame_DoesNotMaterializeSpacerRows(t *testing.T) {
	hc := NewHeightCache()
	heights := make([]int, 20)
	for i := range heights {
		heights[i] = 2
	}
	bl := makeTestBlocks(20, heights, hc)

	allBlocks := bl.All()
	for i := range allBlocks {
		allBlocks[i].Dirty = false
		allBlocks[i].Rendered = fmt.Sprintf("F%d-A\nF%d-B", i, i)
		allBlocks[i].WidthKey = 80
		allBlocks[i].Height = 2
	}
	bl.RebuildAll(allBlocks)
	for i := range allBlocks {
		hc.Set(allBlocks[i].ID, 2)
	}

	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(4)
	vl.SetAutoFollow(false)
	vl.SetAnchor(AnchorForBlock(bl.All(), 10, 0))

	frame := vl.Frame(BlockRenderContext{Width: 80})
	frame = vl.Frame(BlockRenderContext{Width: 80})

	if frame.TopLine <= 0 {
		t.Fatalf("expected frame to track non-zero global top line, got %+v", frame)
	}
	if frame.ContentHeight >= frame.TotalHeight {
		t.Fatalf("frame should mount less than total transcript, contentHeight=%d totalHeight=%d", frame.ContentHeight, frame.TotalHeight)
	}
	if strings.HasPrefix(frame.Content, "\n\n\n\n\n") {
		t.Fatalf("frame should not materialize large top spacer rows, got prefix %q", frame.Content[:min(len(frame.Content), 20)])
	}
	if !strings.Contains(frame.Content, "F10-A") {
		t.Fatalf("frame should include anchor block content, got %q", frame.Content)
	}

	anchor := vl.AnchorFromFrameYOffset(frame.ViewportYOffset)
	if anchor.MsgID != "msg10" {
		t.Fatalf("frame-local y offset should resolve back to anchor message, got %+v frame=%+v", anchor, frame)
	}

	feedbackAnchor := vl.AnchorFromFrameYOffset(frame.ContentHeight + 5)
	frameBottomAnchor := vl.AnchorFromFrameYOffset(frame.ContentHeight - 1)
	if feedbackAnchor.MsgID != frameBottomAnchor.MsgID || feedbackAnchor.LineOffset != frameBottomAnchor.LineOffset {
		t.Fatalf("feedback y offset should clamp to mounted frame bottom, got %+v want %+v frame=%+v", feedbackAnchor, frameBottomAnchor, frame)
	}
}

func TestRender_Empty(t *testing.T) {
	hc := NewHeightCache()
	bl := NewBlockList()
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(20)

	ctx := BlockRenderContext{Width: 80}
	output := vl.Render(ctx)
	if output != "" {
		t.Errorf("expected empty output, got %q", output)
	}
}

func TestRender_TopPaddingIncludesBoundaryWhenOverscanStartsOnHiddenBlock(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "b0", MsgID: "m0", Rendered: "B0", WidthKey: 80, Height: 1},
		{ID: "hidden-m1", MsgID: "m1", Kind: BlockThinking},
		{ID: "b2", MsgID: "m2", Rendered: "B2", WidthKey: 80, Height: 1},
		{ID: "b3", MsgID: "m3", Rendered: "B3", WidthKey: 80, Height: 1},
		{ID: "b4", MsgID: "m4", Rendered: "B4", WidthKey: 80, Height: 1},
	}
	MarkDisplayHidden(&blocks[1], 80)
	for _, block := range blocks {
		if !IsDisplayHidden(block) {
			hc.Set(block.ID, block.Height)
		}
	}
	bl := NewBlockList()
	bl.RebuildAll(blocks)

	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(1)
	vl.SetAutoFollow(false)
	vl.SetAnchor(AnchorForBlock(blocks, 3, 0))

	start, _ := vl.VisibleRange()
	if start != 1 {
		t.Fatalf("test setup expected overscan start on hidden block, got start=%d", start)
	}

	output := vl.Render(BlockRenderContext{Width: 80})
	if !strings.HasPrefix(output, "\n\nB2") {
		t.Fatalf("top padding should include height before b0 and boundary before b2, got %q", output)
	}
}

func TestVirtualMessageList_UserAssistantBoundaryRowsAffectRenderAndTotal(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "u0", MsgID: "u0", Kind: BlockUser, Rendered: "U0\n", WidthKey: 80, Height: 1},
		{ID: "a0", MsgID: "a0", Kind: BlockAssistantMarkdown, Rendered: "A0\n", WidthKey: 80, Height: 1},
		{ID: "u1", MsgID: "u1", Kind: BlockUser, Rendered: "U1\n", WidthKey: 80, Height: 1},
	}
	for _, block := range blocks {
		hc.Set(block.ID, block.Height)
	}
	bl := NewBlockList()
	bl.RebuildAll(blocks)

	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(20)
	vl.SetAutoFollow(false)
	vl.SetAnchor(AnchorForBlock(blocks, 0, 0))

	if got := DisplayTotalHeight(hc, blocks, DefaultBlockHeight); got != 5 {
		t.Fatalf("display total with user<->assistant boundaries got %d, want 5", got)
	}
	if got := countMessageBoundaries(blocks, 0, len(blocks)); got != 2 {
		t.Fatalf("boundary row count got %d, want 2", got)
	}

	out := vl.Render(BlockRenderContext{Width: 80})
	if !strings.Contains(out, "U0\n\nA0\n\nU1") {
		t.Fatalf("expected one blank row between user and assistant blocks, got %q", out)
	}
}

func TestVirtualMessageList_RenderBottomPaddingIncludesBoundaryBeforeEnd(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "sys0", MsgID: "sys0", Kind: BlockSystem, Rendered: "S0\n", WidthKey: 80, Height: 1},
		{ID: "sys1", MsgID: "sys1", Kind: BlockSystem, Rendered: "S1\n", WidthKey: 80, Height: 1},
		{ID: "u0", MsgID: "u0", Kind: BlockUser, Rendered: "U0\n", WidthKey: 80, Height: 1},
		{ID: "a0", MsgID: "a0", Kind: BlockAssistantMarkdown, Rendered: "A0\n", WidthKey: 80, Height: 1},
	}
	for _, block := range blocks {
		hc.Set(block.ID, block.Height)
	}
	bl := NewBlockList()
	bl.RebuildAll(blocks)

	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(1)
	vl.SetAutoFollow(false)
	vl.SetAnchor(AnchorForBlock(blocks, 0, 0))

	if _, end := vl.VisibleRange(); end != 3 {
		t.Fatalf("test setup expected visible range to end before assistant block, got end=%d", end)
	}

	out := vl.Render(BlockRenderContext{Width: 80})
	if got, want := strings.Count(out, "\n"), DisplayTotalHeight(hc, blocks, DefaultBlockHeight); got != want {
		t.Fatalf("rendered height should preserve total display height including boundary before end, got %d want %d output=%q", got, want, out)
	}
	if !strings.HasSuffix(out, "U0\n\n\n") {
		t.Fatalf("expected bottom spacer to include 1-row boundary and assistant block height, got %q", out)
	}
}

func TestVirtualMessageList_FrameAnchorsBoundaryRow(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "u0", MsgID: "u0", Kind: BlockUser, Rendered: "U0\n", WidthKey: 80, Height: 1},
		{ID: "a0", MsgID: "a0", Kind: BlockAssistantMarkdown, Rendered: "A0\n", WidthKey: 80, Height: 1},
		{ID: "u1", MsgID: "u1", Kind: BlockUser, Rendered: "U1\n", WidthKey: 80, Height: 1},
	}
	for _, block := range blocks {
		hc.Set(block.ID, block.Height)
	}
	bl := NewBlockList()
	bl.RebuildAll(blocks)

	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(3)
	vl.SetAutoFollow(false)
	vl.SetAnchor(AnchorForBoundaryBefore(blocks, 1))

	frame := vl.Frame(BlockRenderContext{Width: 80})
	if frame.ViewportYOffset != 1 {
		t.Fatalf("expected viewport offset to resolve to boundary row, got %d frame=%+v content=%q", frame.ViewportYOffset, frame, frame.Content)
	}
	if frame.TopLine != 0 {
		t.Fatalf("expected frame to start at transcript top, got top line %d", frame.TopLine)
	}
}

func TestVirtualMessageList_VariableHeights(t *testing.T) {
	hc := NewHeightCache()
	// Mix of heights: [1, 10, 2, 8, 3, 5, 1, 1, 12, 2]
	heights := []int{1, 10, 2, 8, 3, 5, 1, 1, 12, 2}
	bl := makeTestBlocks(10, heights, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(15)

	// Anchor at block 3 (abs line = 1+10+2 = 13)
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 3, LineOffset: 0}

	start, end := vl.VisibleRange()

	// start should be 3-OverscanBlocks = 1
	if start != 1 {
		t.Errorf("expected start=1, got %d", start)
	}
	// Block 3 (h=8) + block 4 (h=3) + block 5 (h=5) = 16 > 15, so endIdx=6
	// end = 6 + OverscanBlocks = 8
	if end < 7 || end > 9 {
		t.Errorf("expected end around 8, got %d", end)
	}
}

func TestNewVirtualMessageList_Defaults(t *testing.T) {
	hc := NewHeightCache()
	bl := NewBlockList()
	vl := NewVirtualMessageList(bl, hc)

	if !vl.AutoFollow() {
		t.Error("expected autoFollow=true by default")
	}
	if vl.ViewportHeight() != 0 {
		t.Errorf("expected viewportH=0, got %d", vl.ViewportHeight())
	}
	anchor := vl.Anchor()
	if anchor.BlockIdx != 0 || anchor.LineOffset != 0 {
		t.Errorf("expected zero anchor, got %+v", anchor)
	}
}

func TestVirtualMessageList_SetAnchor(t *testing.T) {
	hc := NewHeightCache()
	bl := makeTestBlocks(6, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(8)

	anchor := ScrollAnchor{BlockIdx: 3, LineOffset: 1}
	vl.SetAnchor(anchor)

	got := vl.Anchor()
	if got.BlockIdx != anchor.BlockIdx || got.LineOffset != anchor.LineOffset {
		t.Fatalf("anchor mismatch: got %+v want %+v", got, anchor)
	}
}

func TestVisibleRange_SkipsHiddenAnchorBlocks(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "b0", MsgID: "m0", Height: 2},
		{ID: "hidden-1", MsgID: "m1", Kind: BlockThinking},
		{ID: "b2", MsgID: "m1", Height: 3},
		{ID: "b3", MsgID: "m2", Height: 2},
	}
	MarkDisplayHidden(&blocks[1], 80)
	for _, b := range blocks {
		if !IsDisplayHidden(b) {
			hc.Set(b.ID, b.Height)
		}
	}
	bl := NewBlockList()
	bl.RebuildAll(blocks)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(4)
	vl.SetAutoFollow(false)
	vl.SetAnchor(AnchorForBlock(blocks, 1, 0))

	start, end := vl.VisibleRange()
	if start != 0 || end != 4 {
		t.Fatalf("range should include overscan while normalizing away hidden anchor, got start=%d end=%d", start, end)
	}
	if got := vl.Anchor(); got.BlockID == "hidden-1" || got.BlockID != "b2" {
		t.Fatalf("hidden anchor should normalize to next visible block, got %+v", got)
	}
}

func TestScrollBy_EmptyList(t *testing.T) {
	hc := NewHeightCache()
	bl := NewBlockList()
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(20)

	// Should not panic
	vl.ScrollBy(10)
	vl.ScrollBy(-10)
	vl.ScrollToBottom()
	vl.ScrollToBlock(5)
}

func BenchmarkVisibleRange_500Blocks(b *testing.B) {
	hc := NewHeightCache()
	bl := makeTestBlocks(500, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(40)
	vl.autoFollow = false
	vl.anchor = ScrollAnchor{BlockIdx: 250, LineOffset: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vl.VisibleRange()
	}
}

func BenchmarkVisibleRange_AutoFollow_500(b *testing.B) {
	hc := NewHeightCache()
	bl := makeTestBlocks(500, nil, hc)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(40)
	vl.autoFollow = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vl.VisibleRange()
	}
}
