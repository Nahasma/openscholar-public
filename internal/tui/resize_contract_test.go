package tui

import (
	"context"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/message"
)

func TestNewFullscreenFromEnvMarksInitialFrameDirty(t *testing.T) {
	t.Setenv("OS_TUI_FULLSCREEN", "1")

	m := New(context.Background(), &app.App{}, false, nil)
	if !m.isFullscreenMode() {
		t.Fatal("fullscreen env should initialize fullscreen terminal state")
	}
	state := m.ensureFullscreenFrameState()
	if !state.Dirty {
		t.Fatal("env-driven fullscreen startup should mark frame dirty before first paint")
	}
}

func TestFullscreenEnterMarksRepaintDirtyUntilFramePainted(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.recalcLayout()

	changed, _ := m.switchScreenMode(ScreenModeFullscreen)
	if !changed {
		t.Fatal("expected screen mode switch to fullscreen")
	}
	if !m.ensureFullscreenFrameState().Dirty {
		t.Fatal("switching into fullscreen should mark frame dirty")
	}

	next, _ := m.Update(screenModeAppliedMsg{target: ScreenModeFullscreen, prevWidth: 80})
	updated := next.(Model)
	if !updated.ensureFullscreenFrameState().Dirty {
		t.Fatal("fullscreen should remain dirty until a full frame is rendered")
	}

	_ = updated.View()
	state := updated.ensureFullscreenFrameState()
	if state.Dirty {
		t.Fatal("rendering fullscreen frame should clear dirty flag")
	}
	if !state.HasFrame {
		t.Fatal("fullscreen frame metadata should record first paint")
	}
	if state.LastWidth != updated.width || state.LastHeight != updated.height {
		t.Fatalf("last fullscreen frame size = %dx%d, want %dx%d", state.LastWidth, state.LastHeight, updated.width, updated.height)
	}
}

func TestFullscreenResizeMarksDirtyEpochAndClearsAfterView(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	_ = m.View()

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	updated := next.(Model)
	state := updated.ensureFullscreenFrameState()
	if !state.Dirty {
		t.Fatal("effective fullscreen resize should mark frame dirty")
	}
	if state.ResizeEpoch != updated.resizeEpoch {
		t.Fatalf("fullscreen resize epoch = %d, want %d", state.ResizeEpoch, updated.resizeEpoch)
	}
	if findMsgTypeIndex(collectCmdTypeNames(cmd), "clearScreenMsg") >= 0 {
		t.Fatalf("fullscreen resize should not emit clearScreenMsg, got %#v", collectCmdTypeNames(cmd))
	}

	_ = updated.View()
	if state.Dirty {
		t.Fatal("fullscreen resize dirty flag should clear after rendering current frame")
	}
	if state.LastWidth != 79 || state.LastHeight != 24 {
		t.Fatalf("last fullscreen frame size = %dx%d, want 79x24", state.LastWidth, state.LastHeight)
	}
}

func TestInitialFullscreenWindowSizeRefreshesDirtyEpoch(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)

	next, _ := m.Update(initialWindowSizeMsg{Width: 100, Height: 30, Source: geometrySourceBootstrap})
	updated := next.(Model)
	state := updated.ensureFullscreenFrameState()
	if !state.Dirty {
		t.Fatal("initial fullscreen geometry should keep frame dirty until paint")
	}
	if state.ResizeEpoch != updated.resizeEpoch {
		t.Fatalf("dirty epoch = %d, want resize epoch %d", state.ResizeEpoch, updated.resizeEpoch)
	}

	_ = updated.View()
	if state.Dirty {
		t.Fatal("initial fullscreen dirty flag should clear after first paint")
	}
}

func TestFullscreenSameSizeResizeDoesNotChangeDirtyState(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 80
	m.height = 24
	m.recalcLayout()
	_ = m.View()

	before := *m.ensureFullscreenFrameState()
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatalf("same-size fullscreen resize should return nil command, got %v", cmd)
	}
	updated := next.(Model)
	state := updated.ensureFullscreenFrameState()
	if state.Dirty != before.Dirty {
		t.Fatalf("same-size fullscreen resize changed dirty state from %v to %v", before.Dirty, state.Dirty)
	}
	if state.ResizeEpoch != before.ResizeEpoch {
		t.Fatalf("same-size fullscreen resize changed resize epoch from %d to %d", before.ResizeEpoch, state.ResizeEpoch)
	}
}

func TestFullscreenFrameContractRowsAndWidthAndMetadata(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeFullscreen)
	m.width = 34
	m.height = 7
	m.recalcLayout()

	for i := 0; i < 16; i++ {
		m.chat.messages = append(m.chat.messages, message.Message{
			ID:   fmt.Sprintf("frame-contract-%02d", i),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("frame-contract message %02d long long long long line", i)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		})
	}
	_ = m.updateViewportContent()

	rendered := m.View()
	assertRenderedFrameFits(t, rendered, 7, 34)
	state := m.ensureFullscreenFrameState()
	if state.LastWidth != 34 || state.LastHeight != 7 {
		t.Fatalf("recorded fullscreen frame size = %dx%d, want 34x7", state.LastWidth, state.LastHeight)
	}
}

func TestMainScreenResizeDoesNotEnterViewportOwnedPath(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)
	m.width = 236
	m.height = 18
	m.recalcLayout()
	m.chat.messages = []message.Message{{
		ID:   "r0",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "resize sequence baseline"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	}}
	_ = m.updateViewportContent()
	for _, w := range []int{228, 229, 230, 226} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 18})
		updated := next.(Model)
		m = &updated
		if m.chat.mainScreenViewportOwned {
			t.Fatalf("width=%d unexpectedly entered viewport-owned path", w)
		}
		if stats := m.mainOutput.Stats(); stats.FrameSeq > 0 && (stats.ResetReason == "pending-commit" || stats.ResetReason == "suppress") {
			t.Fatalf("width=%d should not use commit/suppress path, stats=%+v", w, stats)
		}
	}
}

func TestResolveSliceAnchorStartFallsBackToIdxAndRefreshesID(t *testing.T) {
	msgs := []message.Message{
		{ID: "m0"},
		{ID: "m1"},
		{ID: "m2"},
	}
	start, refreshed := resolveSliceAnchorStart(msgs, &messageSliceAnchor{
		MessageID: "missing",
		Idx:       1,
	}, true)
	if start != 2 {
		t.Fatalf("flushed-boundary fallback start = %d, want 2", start)
	}
	if refreshed == nil || refreshed.MessageID != "m1" || refreshed.Idx != 1 {
		t.Fatalf("fallback should refresh anchor to current idx message, got %+v", refreshed)
	}
}
