package components

import "testing"

func TestScrollAnchor_Resolve(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "b0", MsgID: "m0", Height: 3},
		{ID: "b1", MsgID: "m1", Height: 5},
		{ID: "b2", MsgID: "m2", Height: 2},
	}
	for _, b := range blocks {
		hc.Set(b.ID, b.Height)
	}

	tests := []struct {
		anchor ScrollAnchor
		want   int
	}{
		{ScrollAnchor{BlockIdx: 0, LineOffset: 0}, 0},
		{ScrollAnchor{BlockIdx: 0, LineOffset: 2}, 2},
		{ScrollAnchor{BlockIdx: 1, LineOffset: 0}, 3},
		{ScrollAnchor{BlockIdx: 1, LineOffset: 3}, 6},
		{ScrollAnchor{BlockIdx: 2, LineOffset: 1}, 9},
	}

	for _, tt := range tests {
		got := tt.anchor.Resolve(hc, blocks, 1)
		if got != tt.want {
			t.Errorf("Resolve(%+v) = %d, want %d", tt.anchor, got, tt.want)
		}
	}
}

func TestScrollAnchor_ResolvePrefersBlockIDOverStaleIndex(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "b0", MsgID: "m0", Height: 3},
		{ID: "b1", MsgID: "m1", Height: 5},
		{ID: "b2", MsgID: "m2", Height: 2},
	}
	for _, b := range blocks {
		hc.Set(b.ID, b.Height)
	}

	anchor := ScrollAnchor{
		BlockID:    "b2",
		MsgID:      "m2",
		BlockIdx:   0,
		LineOffset: 1,
	}

	if got := anchor.Resolve(hc, blocks, 1); got != 9 {
		t.Fatalf("Resolve should use BlockID before stale index, got %d want 9", got)
	}
}

func TestScrollAnchor_ResolveFallsBackToMsgID(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "new-0", MsgID: "new", Height: 4},
		{ID: "m1-new", MsgID: "m1", Height: 3},
		{ID: "m2-0", MsgID: "m2", Height: 2},
	}
	for _, b := range blocks {
		hc.Set(b.ID, b.Height)
	}

	anchor := ScrollAnchor{
		BlockID:    "old-missing-block",
		MsgID:      "m1",
		BlockIdx:   2,
		LineOffset: 20,
	}

	if got := anchor.Resolve(hc, blocks, 1); got != 6 {
		t.Fatalf("Resolve should fall back to MsgID and clamp line offset, got %d want 6", got)
	}
}

func TestAnchorFromLine(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "b0", MsgID: "m0", Height: 3},
		{ID: "b1", MsgID: "m1", Height: 5},
		{ID: "b2", MsgID: "m2", Height: 2},
	}
	for _, b := range blocks {
		hc.Set(b.ID, b.Height)
	}

	tests := []struct {
		line    int
		wantIdx int
		wantOff int
	}{
		{0, 0, 0},
		{2, 0, 2},
		{3, 1, 0},
		{7, 1, 4},
		{8, 2, 0},
		{9, 2, 1},
		{100, 2, 1}, // past end
	}

	for _, tt := range tests {
		got := AnchorFromLine(hc, blocks, tt.line, 1)
		if got.BlockIdx != tt.wantIdx || got.LineOffset != tt.wantOff {
			t.Errorf("AnchorFromLine(%d) = {%d,%d}, want {%d,%d}",
				tt.line, got.BlockIdx, got.LineOffset, tt.wantIdx, tt.wantOff)
		}
		if blocks[got.BlockIdx].ID != got.BlockID || blocks[got.BlockIdx].MsgID != got.MsgID {
			t.Errorf("AnchorFromLine(%d) semantic IDs = {%q,%q}, want {%q,%q}",
				tt.line, got.BlockID, got.MsgID, blocks[got.BlockIdx].ID, blocks[got.BlockIdx].MsgID)
		}
	}
}

func TestAnchorFromLine_Empty(t *testing.T) {
	hc := NewHeightCache()
	anchor := AnchorFromLine(hc, nil, 0, 1)
	if anchor.BlockIdx != 0 || anchor.LineOffset != 0 {
		t.Errorf("empty anchor = %+v, want {0,0}", anchor)
	}
}

func TestAnchorFromLine_NegativeProtection(t *testing.T) {
	hc := NewHeightCache()
	// Block with height 0 — accumulated never advances, so LineOffset could be negative.
	blocks := []BlockVM{
		{ID: "b0", MsgID: "m0", Height: 0},
		{ID: "b1", MsgID: "m1", Height: 3},
	}
	for _, b := range blocks {
		hc.Set(b.ID, b.Height)
	}

	// absLine=0, block b0 has h=0 so accumulated+h (0) is NOT > 0; falls through.
	// block b1 has h=3 and accumulated=0, so accumulated+h (3) > 0 → LineOffset = 0-0 = 0 (fine).
	anchor := AnchorFromLine(hc, blocks, 0, 1)
	if anchor.LineOffset < 0 {
		t.Errorf("LineOffset should not be negative, got %d", anchor.LineOffset)
	}

	// lastH=0 fallback: LineOffset = max(0, 0-1) = 0
	blocksZero := []BlockVM{{ID: "z0", MsgID: "z", Height: 0}}
	hc.Set("z0", 0)
	anchor2 := AnchorFromLine(hc, blocksZero, 5, 1)
	if anchor2.LineOffset < 0 {
		t.Errorf("fallback LineOffset should not be negative, got %d", anchor2.LineOffset)
	}
}

func TestScrollAnchor_Roundtrip(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "b0", MsgID: "m0", Height: 4},
		{ID: "b1", MsgID: "m1", Height: 6},
		{ID: "b2", MsgID: "m2", Height: 3},
	}
	for _, b := range blocks {
		hc.Set(b.ID, b.Height)
	}

	// For each absolute line, roundtrip through AnchorFromLine → Resolve
	totalHeight := hc.TotalHeight(blocks, 1)
	for line := 0; line < totalHeight; line++ {
		anchor := AnchorFromLine(hc, blocks, line, 1)
		resolved := anchor.Resolve(hc, blocks, 1)
		if resolved != line {
			t.Errorf("roundtrip line %d: anchor=%+v resolved=%d", line, anchor, resolved)
		}
	}
}

func TestScrollAnchor_DisplayLineRoundtripIncludesBoundaryRows(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "b0", MsgID: "m0", Height: 2},
		{ID: "b1", MsgID: "m1", Height: 3},
		{ID: "b2", MsgID: "m1", Height: 2},
		{ID: "b3", MsgID: "m2", Height: 1},
	}
	for _, b := range blocks {
		hc.Set(b.ID, b.Height)
	}

	totalHeight := DisplayTotalHeight(hc, blocks, 1)
	for line := 0; line < totalHeight; line++ {
		anchor := AnchorFromDisplayLine(hc, blocks, line, 1)
		resolved := anchor.ResolveDisplayLine(hc, blocks, 1)
		if resolved != line {
			t.Fatalf("display roundtrip line %d: anchor=%+v resolved=%d", line, anchor, resolved)
		}
	}

	boundaryAnchor := AnchorFromDisplayLine(hc, blocks, 2, 1)
	if !boundaryAnchor.BoundaryBefore || boundaryAnchor.BlockID != "b1" {
		t.Fatalf("line 2 should anchor to boundary before b1, got %+v", boundaryAnchor)
	}
}

func TestScrollAnchor_DisplayLineSkipsHiddenBlocks(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "b0", MsgID: "m0", Height: 2},
		{ID: "hidden-1", MsgID: "m1", Kind: BlockThinking},
		{ID: "b2", MsgID: "m1", Height: 3},
		{ID: "hidden-2", MsgID: "m2", Kind: BlockThinking},
		{ID: "b4", MsgID: "m2", Height: 1},
	}
	MarkDisplayHidden(&blocks[1], 80)
	MarkDisplayHidden(&blocks[3], 80)
	for _, b := range blocks {
		if !IsDisplayHidden(b) {
			hc.Set(b.ID, b.Height)
		}
	}

	if got := DisplayTotalHeight(hc, blocks, 1); got != 8 {
		t.Fatalf("display total should skip hidden block heights but keep visible message boundaries, got %d want 8", got)
	}
	if got := DisplayHeightBefore(hc, blocks, 2, 1); got != 3 {
		t.Fatalf("height before b2 should skip hidden-1 and include boundary before b2, got %d want 3", got)
	}
	if got := DisplayHeightBefore(hc, blocks, 4, 1); got != 7 {
		t.Fatalf("height before b4 should skip hidden-2 and include boundary before b4, got %d want 7", got)
	}

	for line := 0; line < DisplayTotalHeight(hc, blocks, 1); line++ {
		anchor := AnchorFromDisplayLine(hc, blocks, line, 1)
		if anchor.BlockID == "hidden-1" || anchor.BlockID == "hidden-2" {
			t.Fatalf("line %d anchored to hidden block: %+v", line, anchor)
		}
		if resolved := anchor.ResolveDisplayLine(hc, blocks, 1); resolved != line {
			t.Fatalf("display roundtrip with hidden blocks line %d: anchor=%+v resolved=%d", line, anchor, resolved)
		}
	}
}

func TestScrollAnchor_DisplayLineRoundtrip_UserAssistantSingleRowBoundary(t *testing.T) {
	hc := NewHeightCache()
	blocks := []BlockVM{
		{ID: "u0", MsgID: "u0", Kind: BlockUser, Height: 1},
		{ID: "a0", MsgID: "a0", Kind: BlockAssistantMarkdown, Height: 1},
		{ID: "u1", MsgID: "u1", Kind: BlockUser, Height: 1},
	}
	for _, b := range blocks {
		hc.Set(b.ID, b.Height)
	}

	if got := DisplayTotalHeight(hc, blocks, 1); got != 5 {
		t.Fatalf("display total with 1+1 boundary rows got %d, want 5", got)
	}
	if got := DisplayHeightBefore(hc, blocks, 1, 1); got != 2 {
		t.Fatalf("height before assistant should include 1-row boundary, got %d want 2", got)
	}
	if got := DisplayHeightBefore(hc, blocks, 2, 1); got != 4 {
		t.Fatalf("height before second user should include two 1-row boundaries, got %d want 4", got)
	}

	for line := 0; line < DisplayTotalHeight(hc, blocks, 1); line++ {
		anchor := AnchorFromDisplayLine(hc, blocks, line, 1)
		resolved := anchor.ResolveDisplayLine(hc, blocks, 1)
		if resolved != line {
			t.Fatalf("roundtrip line %d: anchor=%+v resolved=%d", line, anchor, resolved)
		}
	}

	boundaryRow := AnchorFromDisplayLine(hc, blocks, 1, 1)
	if !boundaryRow.BoundaryBefore || boundaryRow.BoundaryOffset != 0 {
		t.Fatalf("line 1 should anchor to boundary row before assistant, got %+v", boundaryRow)
	}
}
