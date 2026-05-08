package components

import (
	"fmt"
	"math/rand"
	"testing"
)

// makeBenchBlocks creates n test BlockVMs with pre-set heights in cache.
// Named differently from the existing makeTestBlocks to avoid redeclaration
// (makeTestBlocks in virtual_list_test.go has a different signature).
func makeBenchBlocks(n int) ([]BlockVM, *HeightCache) {
	hc := NewHeightCache()
	blocks := make([]BlockVM, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("bench-msg%d-0", i)
		blocks[i] = BlockVM{
			ID:      id,
			MsgID:   fmt.Sprintf("bench-msg%d", i),
			Kind:    BlockUser,
			Content: fmt.Sprintf("Benchmark block %d content line", i),
			// Pre-rendered so RenderBlock is a cache-hit.
			Rendered: fmt.Sprintf("Benchmark block %d\n", i),
			WidthKey: 80,
			Height:   DefaultBlockHeight,
			Dirty:    false,
		}
		hc.Set(id, DefaultBlockHeight)
	}
	return blocks, hc
}

// makeBenchBlockList creates a *BlockList containing n blocks with heights cached.
func makeBenchBlockList(n int) (*BlockList, *HeightCache) {
	blocks, hc := makeBenchBlocks(n)
	bl := NewBlockList()
	bl.RebuildAll(blocks)
	return bl, hc
}

// makeBenchRenderContext creates a minimal BlockRenderContext for benchmarks.
func makeBenchRenderContext(width int) BlockRenderContext {
	return BlockRenderContext{Width: width}
}

// BenchmarkVirtualListRender_500 measures the cost of Render() with 500 blocks
// and a 50-line viewport (virtual: only ~13 blocks rendered + overscan).
func BenchmarkVirtualListRender_500(b *testing.B) {
	b.ReportAllocs()
	bl, hc := makeBenchBlockList(500)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(50)
	// Pin to middle so we exercise both top- and bottom-padding paths.
	vl.SetAutoFollow(false)
	vl.anchor = ScrollAnchor{BlockIdx: 250, LineOffset: 0}

	ctx := makeBenchRenderContext(80)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vl.Render(ctx)
	}
}

// BenchmarkFullRender_500 is the full-render baseline: iterates every block
// without virtual windowing, so we can compare against BenchmarkVirtualListRender_500.
func BenchmarkFullRender_500(b *testing.B) {
	b.ReportAllocs()
	bl, _ := makeBenchBlockList(500)
	ctx := makeBenchRenderContext(80)
	allBlocks := bl.All()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range allBlocks {
			_ = RenderBlock(&allBlocks[j], ctx)
		}
	}
}

// BenchmarkVirtualListScrollStep_500 measures the cost of a single ScrollBy(1)
// call on a 500-block list.
func BenchmarkVirtualListScrollStep_500(b *testing.B) {
	b.ReportAllocs()
	bl, hc := makeBenchBlockList(500)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(50)
	vl.SetAutoFollow(false)
	vl.anchor = ScrollAnchor{BlockIdx: 10, LineOffset: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vl.ScrollBy(1)
	}
}

// BenchmarkResizeReflow_500 measures the cost of updating the viewport height
// (resize) and immediately computing VisibleRange on a 500-block list.
func BenchmarkResizeReflow_500(b *testing.B) {
	b.ReportAllocs()
	bl, hc := makeBenchBlockList(500)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetAutoFollow(false)
	vl.anchor = ScrollAnchor{BlockIdx: 200, LineOffset: 0}

	heights := []int{40, 50, 60, 80}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vl.SetViewportHeight(heights[i%len(heights)])
		_, _ = vl.VisibleRange()
	}
}

// BenchmarkSearchJump_Virtual_500 measures the cost of ScrollToBlock() + VisibleRange()
// on a 500-block list, simulating random search-result navigation.
func BenchmarkSearchJump_Virtual_500(b *testing.B) {
	b.ReportAllocs()
	bl, hc := makeBenchBlockList(500)
	vl := NewVirtualMessageList(bl, hc)
	vl.SetViewportHeight(50)

	// Pre-generate random target indices to avoid rand overhead in the hot path.
	targets := make([]int, 1024)
	r := rand.New(rand.NewSource(42))
	for i := range targets {
		targets[i] = r.Intn(500)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vl.ScrollToBlock(targets[i%len(targets)])
		_, _ = vl.VisibleRange()
	}
}
