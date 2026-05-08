package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/tui/components"
)

// ---------------------------------------------------------------------------
// StatusRegion
// ---------------------------------------------------------------------------

func TestStatusRegion_DefaultHidden(t *testing.T) {
	r := StatusRegion{}
	for _, w := range []int{0, 40, 80, 200} {
		if h := r.PreferredHeight(w); h != 0 {
			t.Errorf("StatusRegion.PreferredHeight(%d) = %d, want 0", w, h)
		}
	}
	if r.MinHeight() != 0 {
		t.Errorf("StatusRegion.MinHeight() = %d, want 0", r.MinHeight())
	}
}

// ---------------------------------------------------------------------------
// ChatRegion
// ---------------------------------------------------------------------------

func TestChatRegion_MinHeight(t *testing.T) {
	r := ChatRegion{}
	if r.MinHeight() != 0 {
		t.Errorf("ChatRegion.MinHeight() = %d, want 0", r.MinHeight())
	}
	// PreferredHeight is 0 (elastic); the actual height is computed by Layout.
	if r.PreferredHeight(80) != 0 {
		t.Errorf("ChatRegion.PreferredHeight(80) = %d, want 0 (elastic)", r.PreferredHeight(80))
	}
}

// ---------------------------------------------------------------------------
// fixedHeightRegion
// ---------------------------------------------------------------------------

func TestFixedHeightRegion(t *testing.T) {
	r := fixedHeightRegion(7)
	if r.PreferredHeight(80) != 7 {
		t.Errorf("fixedHeightRegion(7).PreferredHeight = %d, want 7", r.PreferredHeight(80))
	}
	if r.MinHeight() != 0 {
		t.Errorf("fixedHeightRegion.MinHeight() = %d, want 0", r.MinHeight())
	}
}

// ---------------------------------------------------------------------------
// LayoutManager.Layout
// ---------------------------------------------------------------------------

// simpleRegion is a test helper that reports fixed preferred and min heights.
type simpleRegion struct {
	preferred int
	min       int
}

func (s simpleRegion) PreferredHeight(_ int) int { return s.preferred }
func (s simpleRegion) MinHeight() int            { return s.min }
func (s simpleRegion) View(_, _ int) string      { return "" }

type testOverlay struct {
	gotWidth  int
	gotHeight int
}

func (o *testOverlay) ID() string        { return "test-overlay" }
func (o *testOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *testOverlay) BlocksInput() bool { return true }
func (o *testOverlay) Update(tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	return o, nil, nil
}
func (o *testOverlay) View(width, height int) string {
	o.gotWidth = width
	o.gotHeight = height
	return "overlay"
}

func TestLayoutManager_BasicDistribution(t *testing.T) {
	lm := LayoutManager{
		header:   simpleRegion{preferred: 1, min: 0},
		chat:     simpleRegion{preferred: 0, min: 3},
		overlay:  nil,
		composer: simpleRegion{preferred: 3, min: 1},
		status:   simpleRegion{preferred: 1, min: 1},
	}
	// Terminal: 80 × 24
	result := lm.Layout(80, 24)

	if result.StatusHeight != 1 {
		t.Errorf("StatusHeight = %d, want 1", result.StatusHeight)
	}
	if result.ComposerHeight != 3 {
		t.Errorf("ComposerHeight = %d, want 3", result.ComposerHeight)
	}
	if result.OverlayHeight != 0 {
		t.Errorf("OverlayHeight = %d, want 0 (no overlay)", result.OverlayHeight)
	}
	// chatH = 24 - 1(header) - 1(status) - 3(composer) - 0(overlay) = 19
	if result.ChatHeight != 19 {
		t.Errorf("ChatHeight = %d, want 19", result.ChatHeight)
	}
}

func TestLayoutManager_WithOverlay(t *testing.T) {
	lm := LayoutManager{
		header:   simpleRegion{preferred: 1, min: 0},
		chat:     simpleRegion{preferred: 0, min: 3},
		overlay:  simpleRegion{preferred: 10, min: 0},
		composer: simpleRegion{preferred: 2, min: 1},
		status:   simpleRegion{preferred: 1, min: 1},
	}
	result := lm.Layout(80, 30)

	// Overlay replaces composer: chatH = 30 - 1(header) - 1(status) - 10 = 18
	if result.ChatHeight != 18 {
		t.Errorf("ChatHeight = %d, want 18", result.ChatHeight)
	}
	if result.OverlayHeight != 10 {
		t.Errorf("OverlayHeight = %d, want 10", result.OverlayHeight)
	}
	if result.ComposerHeight != 0 {
		t.Errorf("ComposerHeight = %d, want 0 when overlay is active", result.ComposerHeight)
	}
}

func TestLayoutManager_ChatClampedToMinHeight(t *testing.T) {
	// Composer + overlay + status consume most of the terminal.
	lm := LayoutManager{
		header:   simpleRegion{preferred: 1, min: 0},
		chat:     simpleRegion{preferred: 0, min: 3},
		overlay:  simpleRegion{preferred: 15, min: 0},
		composer: simpleRegion{preferred: 5, min: 1},
		status:   simpleRegion{preferred: 1, min: 1},
	}
	// Overlay replaces composer: remaining for chat = 20 - 1(header) - 1(status) - 15 = 3
	result := lm.Layout(80, 20)

	if result.ChatHeight < 0 {
		t.Errorf("ChatHeight = %d, should be >= 0 (min height)", result.ChatHeight)
	}
	if result.ChatHeight != 3 {
		t.Errorf("ChatHeight = %d, want 3", result.ChatHeight)
	}
}

func TestLayoutManager_SumConsistency(t *testing.T) {
	// Verify that the reported heights make sense relative to the total terminal height.
	lm := LayoutManager{
		header:   simpleRegion{preferred: 1, min: 0},
		chat:     simpleRegion{preferred: 0, min: 3},
		overlay:  nil,
		composer: simpleRegion{preferred: 4, min: 1},
		status:   simpleRegion{preferred: 1, min: 1},
	}
	total := 40
	result := lm.Layout(80, total)

	sum := result.HeaderHeight + result.ChatHeight + result.ComposerHeight + result.OverlayHeight + result.StatusHeight
	// sum should equal total when clamping was not triggered.
	if sum != total {
		t.Errorf("heights sum = %d, want %d (total)", sum, total)
	}
}

func TestLayoutManager_AllowsZeroChatHeightOnTinyTerminal(t *testing.T) {
	lm := LayoutManager{
		header:   simpleRegion{preferred: 1, min: 0},
		chat:     simpleRegion{preferred: 0, min: 0},
		overlay:  nil,
		composer: simpleRegion{preferred: 4, min: 1},
		status:   simpleRegion{preferred: 1, min: 0},
	}

	result := lm.Layout(80, 4)
	if result.ChatHeight != 0 {
		t.Fatalf("ChatHeight = %d, want 0 on tiny terminal", result.ChatHeight)
	}
}

func TestLayoutManager_Snapshot_TinyHeightKeepsNonNegativeTranscript(t *testing.T) {
	lm := LayoutManager{
		header:   simpleRegion{preferred: 1, min: 0},
		chat:     simpleRegion{preferred: 0, min: 0},
		overlay:  nil,
		composer: simpleRegion{preferred: 4, min: 0},
		status:   simpleRegion{preferred: 1, min: 1},
	}

	s := lm.Snapshot(80, 2)
	if s.TranscriptHeight < 0 {
		t.Fatalf("TranscriptHeight = %d, want >= 0", s.TranscriptHeight)
	}
	if s.StatusHeight != 1 {
		t.Fatalf("StatusHeight = %d, want fixed status row 1", s.StatusHeight)
	}
	if s.BottomHeight != 0 {
		t.Fatalf("BottomHeight = %d, want 0 when tiny height is exhausted by header+status", s.BottomHeight)
	}
}

func TestLayoutManager_Snapshot_EmptyBottomDoesNotConsumeTranscript(t *testing.T) {
	lm := LayoutManager{
		header:   simpleRegion{preferred: 1, min: 0},
		chat:     simpleRegion{preferred: 0, min: 0},
		overlay:  nil,
		composer: simpleRegion{preferred: 0, min: 0},
		status:   simpleRegion{preferred: 1, min: 1},
	}

	s := lm.Snapshot(80, 10)
	if s.BottomHeight != 0 {
		t.Fatalf("BottomHeight = %d, want 0 for empty composer and no overlay", s.BottomHeight)
	}
	if s.TranscriptHeight != 8 {
		t.Fatalf("TranscriptHeight = %d, want 8", s.TranscriptHeight)
	}
}

func TestLayoutManager_Snapshot_StatusRowPreferredOverBottom(t *testing.T) {
	lm := LayoutManager{
		header:   simpleRegion{preferred: 1, min: 0},
		chat:     simpleRegion{preferred: 0, min: 0},
		overlay:  nil,
		composer: simpleRegion{preferred: 10, min: 0},
		status:   simpleRegion{preferred: 1, min: 1},
	}

	s := lm.Snapshot(80, 3)
	if s.StatusHeight != 1 {
		t.Fatalf("StatusHeight = %d, want 1", s.StatusHeight)
	}
	if s.BottomHeight != 1 {
		t.Fatalf("BottomHeight = %d, want remaining 1 row", s.BottomHeight)
	}
	if s.TranscriptHeight != 0 {
		t.Fatalf("TranscriptHeight = %d, want 0", s.TranscriptHeight)
	}
}

func TestLayoutManager_Snapshot_ResizeRecalculatesHeights(t *testing.T) {
	lm := LayoutManager{
		header:   simpleRegion{preferred: 1, min: 0},
		chat:     simpleRegion{preferred: 0, min: 0},
		overlay:  nil,
		composer: simpleRegion{preferred: 3, min: 0},
		status:   simpleRegion{preferred: 1, min: 1},
	}

	large := lm.Snapshot(100, 24)
	small := lm.Snapshot(60, 8)

	if large.TranscriptHeight != 19 {
		t.Fatalf("large TranscriptHeight = %d, want 19", large.TranscriptHeight)
	}
	if small.TranscriptHeight != 3 {
		t.Fatalf("small TranscriptHeight = %d, want 3", small.TranscriptHeight)
	}
	if large.StatusHeight != 1 || small.StatusHeight != 1 {
		t.Fatalf("status row should stay fixed at 1, got large=%d small=%d", large.StatusHeight, small.StatusHeight)
	}
}

func TestBuildLayoutManager_OverlayUsesFinalBottomBudget(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.features.OverlayStack = true
	m.status.overlayName = "test"

	overlay := &testOverlay{}
	m.overlays.Push(overlay)

	_ = buildLayoutManager(m)

	want := m.height - m.headerLayoutHeight() - m.statusLayoutHeight()
	if overlay.gotWidth != m.width {
		t.Fatalf("overlay width = %d, want %d", overlay.gotWidth, m.width)
	}
	if overlay.gotHeight != want {
		t.Fatalf("overlay height = %d, want %d", overlay.gotHeight, want)
	}
}

func TestOverlayAvailableHeightKeepsStatusDroppableOnTinyTerminal(t *testing.T) {
	m := newTestModel()
	m.width = 40
	m.height = 7
	m.status.overlayName = "permission"

	got := m.overlayAvailableHeight()
	want := m.height - m.headerLayoutHeight() - m.statusLayoutHeight()
	if got != want {
		t.Fatalf("overlay available height = %d, want %d", got, want)
	}
	if got < 0 {
		t.Fatalf("overlay available height should not be negative, got %d", got)
	}
}

func TestPermissionOverlayFitsHeaderStatusBudgetOnTinyTerminal(t *testing.T) {
	req := permission.PermissionRequest{
		ToolName:    "Write",
		Description: "Create file",
		Action:      "write",
		Path:        "/tmp/tiny.txt",
		Params:      map[string]any{"content": "one\ntwo\nthree\nfour\nfive"},
	}

	m := newTestModel()
	m.width = 40
	m.height = 7
	m.status.overlayName = "permission"
	m.state = statePermission
	m.dlg.perm.pending = &req

	overlay := m.buildDialogOverlay()
	if got, maxHeight := lineCount(overlay), m.overlayAvailableHeight(); got > maxHeight {
		t.Fatalf("legacy permission overlay height = %d, want <= %d; overlay=%q", got, maxHeight, overlay)
	}

	m.features.OverlayStack = true
	m.overlays.Push(NewPermissionOverlay(&req))
	stackOverlay := m.buildDialogOverlay()
	if got, maxHeight := lineCount(stackOverlay), m.currentLayoutSnapshot().BottomHeight; got > maxHeight {
		t.Fatalf("stack permission overlay height = %d, want <= final bottom height %d; overlay=%q", got, maxHeight, stackOverlay)
	}
}

func TestBuildLayoutManager_ZeroHeightOverlayStackDoesNotReserveOverlay(t *testing.T) {
	req := permission.PermissionRequest{
		ToolName:    "Write",
		Description: "Create file",
		Action:      "write",
		Params:      map[string]any{"content": "one\ntwo"},
	}

	m := newTestModel()
	m.width = 40
	m.height = m.headerLayoutHeight() + 1
	m.features.OverlayStack = true
	m.features.LayoutRegion = true
	m.status.overlayName = "permission"
	m.overlays.Push(NewPermissionOverlay(&req))

	lm := buildLayoutManager(m)
	result := lm.Layout(m.width, m.height)
	if result.OverlayHeight != 0 {
		t.Fatalf("zero-height overlay should not reserve rows, got overlay height %d", result.OverlayHeight)
	}
	if result.ComposerHeight == 0 {
		t.Fatalf("empty overlay render should not suppress composer height in layout result: %+v", result)
	}
}

func TestOverlayAvailableHeightReservesStatusRow(t *testing.T) {
	m := newTestModel()
	m.width = 40
	m.height = 6
	m.features.StatusV2 = true

	want := m.height - m.headerLayoutHeight() - m.statusLayoutHeight()
	if got := m.overlayAvailableHeight(); got != want {
		t.Fatalf("overlayAvailableHeight=%d, want %d with status reservation", got, want)
	}
}

func TestHeaderLayoutHeight_ByScreenMode(t *testing.T) {
	main := newTestModel()
	main.setScreenMode(ScreenModeMain)
	main.width = 80
	main.height = 24
	main.recalcLayout()
	if got := main.headerLayoutHeight(); got != 0 {
		t.Fatalf("main-screen headerLayoutHeight=%d, want 0", got)
	}

	full := newTestModel()
	full.setScreenMode(ScreenModeFullscreen)
	full.width = 80
	full.height = 24
	full.recalcLayout()
	if got := full.headerLayoutHeight(); got <= 0 {
		t.Fatalf("fullscreen headerLayoutHeight=%d, want > 0", got)
	}
}

func TestSessionBrowserOverlayFitsHeaderStatusBudgetOnTinyTerminal(t *testing.T) {
	sessions := []session.Session{{ID: "s1", Title: "Tiny", MessageCount: 1}}

	m := newTestModel()
	m.width = 40
	m.height = 6
	m.status.overlayName = "sessions"
	m.state = stateSessionBrowser
	m.dlg.sessionBrowser.list = sessions

	overlay := m.buildDialogOverlay()
	if got, maxHeight := lineCount(overlay), m.overlayAvailableHeight(); got > maxHeight {
		t.Fatalf("legacy session browser height = %d, want <= %d; overlay=%q", got, maxHeight, overlay)
	}

	m.features.OverlayStack = true
	m.overlays.Push(NewSessionBrowserOverlay(sessions))
	stackOverlay := m.buildDialogOverlay()
	if got, maxHeight := lineCount(stackOverlay), m.currentLayoutSnapshot().BottomHeight; got > maxHeight {
		t.Fatalf("stack session browser height = %d, want <= final bottom height %d; overlay=%q", got, maxHeight, stackOverlay)
	}

	if NewSessionBrowserOverlay(sessions).View(m.width, 0) != "" {
		t.Fatal("zero-height session browser overlay should render empty")
	}
}

// ---------------------------------------------------------------------------
// ComposerRegion with nil Model
// ---------------------------------------------------------------------------

func TestComposerRegion_NilModel(t *testing.T) {
	r := ComposerRegion{m: nil}
	h := r.PreferredHeight(80)
	if h != 1 {
		t.Errorf("ComposerRegion(nil).PreferredHeight = %d, want 1", h)
	}
}

// ---------------------------------------------------------------------------
// ComposerRegion uses InputModel.Height()
// ---------------------------------------------------------------------------

func TestComposerRegion_EmptyInput_UsesInputHeight(t *testing.T) {
	c := NewComposerFeature()
	// InputModel.Height() returns lines + 2 (separators), minimum 3 lines content
	// So minimum is 3 + 2 = 5 for an empty input.
	h := c.input.Height()
	if h < 1 {
		t.Errorf("InputModel.Height() (empty) = %d, want >= 1", h)
	}
}

func TestComposerRegion_PreferredHeightMatchesRenderedBottomSection(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 24
	m.status.isProcessing = true
	m.status.processing = ProcessingState{
		Phase:     PhaseToolRunning,
		StartedAt: time.Now(),
	}

	r := ComposerRegion{m: m}
	want := strings.Count(m.buildBottomSection(), "\n") + 1
	if got := r.PreferredHeight(m.width); got != want {
		t.Fatalf("ComposerRegion.PreferredHeight() = %d, want rendered bottom section lines %d", got, want)
	}
}

func TestComposerLayoutHeight_MultilineInputMatchesRenderedBottomSection(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.composer.input.SetWidth(m.width)
	m.composer.input.SetValue("line one\nline two\nline three")

	bottom := stripANSI(m.buildBottomSection())
	if !strings.Contains(bottom, "line one") || !strings.Contains(bottom, "line two") || !strings.Contains(bottom, "line three") {
		t.Fatalf("bottom section missing multiline input:\n%s", bottom)
	}
	if strings.Count(bottom, components.FigPrompt) != 1 {
		t.Fatalf("prompt count in bottom section = %d, want 1\n%s", strings.Count(bottom, components.FigPrompt), bottom)
	}

	want := strings.Count(strings.TrimRight(bottom, "\n"), "\n") + 1
	if got := m.composerLayoutHeight(); got != want {
		t.Fatalf("composerLayoutHeight() = %d, want bottom section lines %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// OverlayRegion with nil OverlayManager
// ---------------------------------------------------------------------------

func TestOverlayRegion_Zero(t *testing.T) {
	r := OverlayRegion{renderedLines: 0}
	if h := r.PreferredHeight(80); h != 0 {
		t.Errorf("OverlayRegion(0).PreferredHeight = %d, want 0", h)
	}
}

func TestOverlayRegion_PreRendered(t *testing.T) {
	r := OverlayRegion{renderedLines: 12}
	if h := r.PreferredHeight(80); h != 12 {
		t.Errorf("OverlayRegion(12).PreferredHeight = %d, want 12", h)
	}
}

// ---------------------------------------------------------------------------
// Feature flag: LayoutRegion in TUIFeatures
// ---------------------------------------------------------------------------

func TestFeaturesFromEnv_LayoutRegion(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		t.Setenv("OS_TUI_LAYOUT_REGION", "")
		f := FeaturesFromEnv()
		if f.LayoutRegion {
			t.Error("LayoutRegion should be disabled when env var is empty")
		}
	})

	t.Run("enabled", func(t *testing.T) {
		t.Setenv("OS_TUI_LAYOUT_REGION", "1")
		f := FeaturesFromEnv()
		if !f.LayoutRegion {
			t.Error("LayoutRegion should be enabled when OS_TUI_LAYOUT_REGION=1")
		}
	})

	t.Run("not_affected_by_other_flags", func(t *testing.T) {
		t.Setenv("OS_TUI_LAYOUT_REGION", "")
		t.Setenv("OS_TUI_BLOCK_RENDERER", "1")
		f := FeaturesFromEnv()
		if f.LayoutRegion {
			t.Error("LayoutRegion should remain disabled when only BlockRenderer is set")
		}
		if !f.BlockRenderer {
			t.Error("BlockRenderer should be enabled")
		}
	})
}

func TestDefaultFeatures_LayoutRegionDisabled(t *testing.T) {
	f := DefaultFeatures()
	if f.LayoutRegion {
		t.Error("DefaultFeatures should have LayoutRegion disabled")
	}
}

func TestFeaturesFromEnv_VimMode(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		t.Setenv("OS_TUI_VIM_MODE", "")
		f := FeaturesFromEnv()
		if f.VimMode {
			t.Error("VimMode should be disabled when env var is empty")
		}
	})

	t.Run("enabled", func(t *testing.T) {
		t.Setenv("OS_TUI_VIM_MODE", "1")
		f := FeaturesFromEnv()
		if !f.VimMode {
			t.Error("VimMode should be enabled when OS_TUI_VIM_MODE=1")
		}
	})
}
