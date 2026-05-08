package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
	"github.com/Nahasma/openscholar-public/internal/message"
)

func TestGeometryBootstrapFallbackThenWindowOverrides(t *testing.T) {
	m := newTestModel()

	next, _ := m.Update(initialWindowSizeMsg{Width: defaultWindowWidth, Height: defaultWindowHeight, Source: geometrySourceFallback})
	updated := next.(Model)
	if updated.width != defaultWindowWidth || updated.height != defaultWindowHeight {
		t.Fatalf("fallback geometry = %dx%d, want %dx%d", updated.width, updated.height, defaultWindowWidth, defaultWindowHeight)
	}
	if updated.geometry.Source != geometrySourceFallback {
		t.Fatalf("source = %v, want fallback", updated.geometry.Source)
	}

	next, _ = updated.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated = next.(Model)
	if updated.width != 100 || updated.height != 30 {
		t.Fatalf("window geometry = %dx%d, want 100x30", updated.width, updated.height)
	}
	if updated.geometry.Source != geometrySourceWindow {
		t.Fatalf("source = %v, want window", updated.geometry.Source)
	}
	if updated.geometry.Epoch != updated.resizeEpoch {
		t.Fatalf("geometry epoch = %d, resize epoch = %d", updated.geometry.Epoch, updated.resizeEpoch)
	}
}

func TestFirstWindowSizeDoesNotEnterViewportOwnedAndRendersNaturalFrame(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated := next.(Model)
	if updated.chat.mainScreenViewportOwned {
		t.Fatal("first authoritative window size should not switch main-screen transcript to viewport-owned")
	}
	if updated.chat.mainScreenFrame != nil && updated.chat.mainScreenFrame.PendingReset {
		t.Fatal("first authoritative window size should not schedule visible reset")
	}

	updated.chat.messages = []message.Message{
		{
			ID:   "user-first-window",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "first completed turn"},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
		},
	}
	cmd := updated.updateViewportContent()
	printed := stripANSI(strings.Join(collectPrintedBodies(cmd), "\n"))
	if strings.TrimSpace(printed) != "" {
		t.Fatalf("completed transcript should not print to native scrollback, got %q", printed)
	}
	if view := stripANSI(updated.View()); !strings.Contains(view, "first completed turn") {
		t.Fatalf("completed transcript should render in natural frame, got %q", view)
	}
}

func TestWindowSizeAfterFallbackWithoutTranscriptDoesNotEnterViewportOwned(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)

	next, _ := m.Update(initialWindowSizeMsg{Width: defaultWindowWidth, Height: defaultWindowHeight, Source: geometrySourceFallback})
	updated := next.(Model)
	next, _ = updated.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated = next.(Model)
	if updated.chat.mainScreenViewportOwned {
		t.Fatal("first real window size after fallback should not switch an empty main-screen transcript to viewport-owned")
	}
	if updated.chat.mainScreenFrame != nil && updated.chat.mainScreenFrame.PendingReset {
		t.Fatal("empty transcript fallback->window should not schedule visible reset")
	}
}

func TestWindowSizeAfterFallbackProcessingWithoutTranscriptDoesNotEnterViewportOwned(t *testing.T) {
	m := newTestModel()
	m.setScreenMode(ScreenModeMain)

	next, _ := m.Update(initialWindowSizeMsg{Width: defaultWindowWidth, Height: defaultWindowHeight, Source: geometrySourceFallback})
	updated := next.(Model)
	updated.status.isProcessing = true
	next, _ = updated.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated = next.(Model)
	if updated.chat.mainScreenViewportOwned {
		t.Fatal("processing without transcript should not force viewport-owned mode on first real window size")
	}
	if updated.chat.mainScreenFrame != nil && updated.chat.mainScreenFrame.PendingReset {
		t.Fatal("processing without transcript should not schedule visible reset")
	}
}

func TestGeometryIgnoresInvalidAfterWindowSize(t *testing.T) {
	m := newTestModel()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated := next.(Model)
	epoch := updated.resizeEpoch

	next, _ = updated.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	updated = next.(Model)
	if updated.width != 100 || updated.height != 30 {
		t.Fatalf("invalid size overwrote real geometry: %dx%d", updated.width, updated.height)
	}
	if updated.resizeEpoch != epoch {
		t.Fatalf("resize epoch changed after invalid size: got %d want %d", updated.resizeEpoch, epoch)
	}
}

func TestGeometryFallbackDoesNotOverrideWindowSize(t *testing.T) {
	m := newTestModel()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated := next.(Model)

	next, _ = updated.Update(initialWindowSizeMsg{Width: defaultWindowWidth, Height: defaultWindowHeight, Source: geometrySourceFallback})
	updated = next.(Model)
	if updated.width != 100 || updated.height != 30 {
		t.Fatalf("fallback overwrote real geometry: %dx%d", updated.width, updated.height)
	}
	if updated.geometry.Source != geometrySourceWindow {
		t.Fatalf("source = %v, want window", updated.geometry.Source)
	}
}

func TestLayoutSnapshotEpochTracksResize(t *testing.T) {
	m := newTestModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated := next.(Model)

	snapshot := updated.currentLayoutSnapshot()
	if snapshot.Epoch != updated.resizeEpoch {
		t.Fatalf("snapshot epoch = %d, resize epoch = %d", snapshot.Epoch, updated.resizeEpoch)
	}

	next, _ = updated.Update(tea.WindowSizeMsg{Width: 90, Height: 26})
	updated = next.(Model)
	if updated.currentLayoutSnapshot().Epoch <= snapshot.Epoch {
		t.Fatalf("snapshot epoch did not grow after resize: before=%d after=%d", snapshot.Epoch, updated.currentLayoutSnapshot().Epoch)
	}
}

func TestNarrowGeometryKeepsOverlayDimensionsNonNegative(t *testing.T) {
	m := newTestModel()
	m.width = 1
	m.height = 3
	m.state = stateWorkspaceSelection
	if got := m.dialogContentWidth(); got != 0 {
		t.Fatalf("dialog content width = %d, want 0", got)
	}
	_ = m.buildDialogOverlay()

	overlay := &testOverlay{}
	m.features.OverlayStack = true
	m.overlays.Push(overlay)
	_ = m.buildDialogOverlay()
	if overlay.gotWidth < 0 || overlay.gotHeight < 0 {
		t.Fatalf("overlay saw negative dimensions: width=%d height=%d", overlay.gotWidth, overlay.gotHeight)
	}
}

func TestModelSelectOverlayNarrowWidthDoesNotPanic(t *testing.T) {
	loadWizardConfig(t)
	overlay := NewModelSelectOverlay(models.ProviderOpenAI, models.GPT41)
	_ = overlay.View(1, 3)
}
