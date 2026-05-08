package tui

// Region represents a rectangular area in the TUI layout.
// Each region can declare its preferred and minimum heights, and render itself.
// This enables declarative height calculation without full pre-rendering.
type Region interface {
	// PreferredHeight returns the desired number of terminal lines for this region
	// at the given terminal width.
	PreferredHeight(width int) int
	// MinHeight returns the smallest acceptable height for this region.
	MinHeight() int
	// View renders the region content for the given dimensions.
	View(width, height int) string
}

// LayoutResult holds the calculated heights for each region after a Layout() call.
type LayoutResult struct {
	HeaderHeight   int
	StatusHeight   int
	ComposerHeight int
	OverlayHeight  int
	ChatHeight     int
}

// LayoutSnapshot captures the resolved geometry for one layout pass.
// It is an internal contract between layout planning and rendering paths.
type LayoutSnapshot struct {
	Width            int
	Height           int
	Epoch            int
	HeaderHeight     int
	TranscriptHeight int
	BottomHeight     int
	StatusHeight     int
	ComposerHeight   int
	OverlayHeight    int
}

// LayoutManager computes how to distribute terminal height among the four
// regions: chat viewport, overlay, composer (input + pickers), and status bar.
//
// Usage (as a temporary local variable in recalcLayout):
//
//	lm := LayoutManager{chat: chatR, overlay: overlayR, composer: composerR, status: statusR}
//	result := lm.Layout(m.width, m.height)
type LayoutManager struct {
	header   Region // fixed top header bar
	chat     Region // fills remaining space
	overlay  Region // overlay stack top — nil when no overlay is active
	composer Region // input box + optional picker rows
	status   Region // status bar (0/1 line based on state)
}

// Snapshot resolves a full-screen layout in one pass and returns a stable
// region contract (header/transcript/bottom/status).
func (lm *LayoutManager) Snapshot(totalWidth, totalHeight int) LayoutSnapshot {
	if totalWidth < 0 {
		totalWidth = 0
	}
	if totalHeight < 0 {
		totalHeight = 0
	}

	headerH := 0
	if lm.header != nil {
		headerH = nonNegative(lm.header.PreferredHeight(totalWidth))
	}
	if headerH > totalHeight {
		headerH = totalHeight
	}
	remaining := totalHeight - headerH

	composerPreferred := 0
	composerMin := 0
	if lm.composer != nil {
		composerPreferred = nonNegative(lm.composer.PreferredHeight(totalWidth))
		composerMin = nonNegative(lm.composer.MinHeight())
	}

	overlayPreferred := 0
	overlayMin := 0
	if lm.overlay != nil {
		overlayPreferred = nonNegative(lm.overlay.PreferredHeight(totalWidth))
		overlayMin = nonNegative(lm.overlay.MinHeight())
	}

	statusPreferred := 0
	statusMin := 0
	if lm.status != nil {
		statusPreferred = nonNegative(lm.status.PreferredHeight(totalWidth))
		statusMin = nonNegative(lm.status.MinHeight())
	}

	// Overlay/dialog replaces composer as the bottom section.
	bottomPreferred := composerPreferred
	bottomMin := composerMin
	overlayH := 0
	composerH := 0
	if overlayPreferred > 0 {
		bottomPreferred = overlayPreferred
		bottomMin = overlayMin
	}

	statusMin = clampToAvailable(statusMin, statusPreferred)
	bottomMin = clampToAvailable(bottomMin, bottomPreferred)

	// Apply minimum guarantees first, then distribute the remaining rows to
	// preferred heights. This prevents status/bottom from stealing each other's
	// guaranteed minimum rows while keeping them shrinkable when min is 0.
	statusH := clampToAvailable(statusMin, remaining)
	remaining -= statusH

	bottomH := clampToAvailable(bottomMin, remaining)
	remaining -= bottomH

	statusExtra := clampToAvailable(statusPreferred-statusH, remaining)
	statusH += statusExtra
	remaining -= statusExtra

	bottomExtra := clampToAvailable(bottomPreferred-bottomH, remaining)
	bottomH += bottomExtra
	remaining -= bottomExtra

	if overlayPreferred > 0 {
		overlayH = bottomH
	} else {
		composerH = bottomH
	}

	transcriptH := remaining
	if transcriptH < 0 {
		transcriptH = 0
	}

	minChat := 0
	if lm.chat != nil {
		minChat = nonNegative(lm.chat.MinHeight())
	}
	if transcriptH < minChat {
		transcriptH = minChat
	}

	return LayoutSnapshot{
		Width:            totalWidth,
		Height:           totalHeight,
		HeaderHeight:     headerH,
		TranscriptHeight: transcriptH,
		BottomHeight:     bottomH,
		StatusHeight:     statusH,
		ComposerHeight:   composerH,
		OverlayHeight:    overlayH,
	}
}

// Layout distributes totalHeight among the four regions.
// It keeps backward compatibility with existing call sites that consume
// chat/composer/overlay/status heights.
func (lm *LayoutManager) Layout(totalWidth, totalHeight int) LayoutResult {
	snapshot := lm.Snapshot(totalWidth, totalHeight)
	return LayoutResult{
		HeaderHeight:   snapshot.HeaderHeight,
		StatusHeight:   snapshot.StatusHeight,
		ComposerHeight: snapshot.ComposerHeight,
		OverlayHeight:  snapshot.OverlayHeight,
		ChatHeight:     snapshot.TranscriptHeight,
	}
}

func nonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func clampToAvailable(v, available int) int {
	if available <= 0 {
		return 0
	}
	if v <= 0 {
		return 0
	}
	if v > available {
		return available
	}
	return v
}

// ---------------------------------------------------------------------------
// Concrete Region implementations
// ---------------------------------------------------------------------------

// StatusRegion is the bottom status bar.
type StatusRegion struct {
	m *Model
}

func (r StatusRegion) PreferredHeight(_ int) int {
	if r.m == nil {
		return 0
	}
	if r.m.shouldRenderStatusBar() {
		return 1
	}
	return 0
}
func (StatusRegion) MinHeight() int       { return 0 }
func (StatusRegion) View(_, _ int) string { return "" }

// HeaderRegion is the top header bar.
type HeaderRegion struct {
	m *Model
}

func (r HeaderRegion) PreferredHeight(_ int) int {
	if r.m == nil {
		return 0
	}
	return r.m.headerLayoutHeight()
}
func (HeaderRegion) MinHeight() int       { return 0 }
func (HeaderRegion) View(_, _ int) string { return "" }

// ChatRegion fills all remaining vertical space.
// MinHeight is 0 so very small terminals keep prompt/overlay rows visible.
type ChatRegion struct{}

func (ChatRegion) PreferredHeight(_ int) int { return 0 } // elastic — determined by Layout
func (ChatRegion) MinHeight() int            { return 0 }
func (ChatRegion) View(_, _ int) string      { return "" }

// ComposerRegion wraps the composer feature and estimates the height of the
// input area plus any active picker (command picker or file picker).
//
// It holds a reference to the Model so it can inspect live picker state
// without coupling layout.go to every individual field.
type ComposerRegion struct {
	m *Model // read-only reference; not mutated
}

// PreferredHeight returns the number of lines the composer section currently needs.
//
// It always delegates to buildBottomSection() and counts the rendered lines.
// This keeps LayoutRegion height accounting aligned with the rendered prompt
// area (search chrome, input, picker, footer) without measuring a full string.
func (r ComposerRegion) PreferredHeight(width int) int {
	if r.m == nil {
		return 1
	}
	_ = width // width is consumed by Model.width when composing the VM.
	return r.m.composerLayoutHeight()
}

func (r ComposerRegion) MinHeight() int { return 1 }
func (r ComposerRegion) View(_, _ int) string {
	// Rendering is handled by the existing buildBottomSection path; this Region
	// is used only for height estimation during layout calculation.
	return ""
}

// OverlayRegion wraps the OverlayManager and reports the height of the topmost
// overlay. It uses the pre-rendered overlay string (measured via line count)
// to avoid the estimation errors that arise from calling View() with a
// provisional height value.
type OverlayRegion struct {
	// renderedLines is the line count obtained from a full pre-render of the
	// top overlay. It is set by buildLayoutManager which does a single
	// View() call at the full terminal height, the same as the legacy path.
	renderedLines int
}

func (r OverlayRegion) PreferredHeight(_ int) int { return r.renderedLines }
func (r OverlayRegion) MinHeight() int            { return 0 }
func (r OverlayRegion) View(_, _ int) string      { return "" }

// ---------------------------------------------------------------------------
// Integration helper: build a LayoutManager from the current Model state.
// ---------------------------------------------------------------------------

func (m *Model) currentLayoutSnapshot() LayoutSnapshot {
	if m == nil {
		return LayoutSnapshot{}
	}
	lm := buildLayoutManager(m)
	snapshot := lm.Snapshot(m.width, m.height)
	snapshot.Epoch = m.resizeEpoch
	return snapshot
}

// buildLayoutManager constructs a LayoutManager using live Model references.
// The returned manager is used as a temporary local variable in recalcLayoutRegion.
func buildLayoutManager(m *Model) LayoutManager {
	var overlayRegion Region
	if m.features.OverlayStack && !m.overlays.IsEmpty() {
		// Pre-render overlay at the same bottom-section height budget used by
		// the final fullscreen layout so line counting matches actual rendering.
		rendered := m.overlays.Top().View(m.width, m.overlayAvailableHeight())
		lines := lineCount(rendered)
		overlayRegion = OverlayRegion{renderedLines: lines}
	} else if !m.features.OverlayStack {
		// Legacy state-switch path: measure overlay height via buildDialogOverlay.
		overlay := m.buildDialogOverlay()
		if overlay != "" {
			overlayRegion = fixedHeightRegion(lineCount(overlay))
		}
	}

	return LayoutManager{
		header:   HeaderRegion{m: m},
		chat:     ChatRegion{},
		overlay:  overlayRegion,
		composer: ComposerRegion{m: m},
		status:   StatusRegion{m: m},
	}
}

// fixedHeightRegion is a minimal Region that always reports a constant height.
// Used when the height is already known (e.g. pre-rendered overlay string).
type fixedHeightRegion int

func (f fixedHeightRegion) PreferredHeight(_ int) int { return int(f) }
func (f fixedHeightRegion) MinHeight() int            { return 0 }
func (f fixedHeightRegion) View(_, _ int) string      { return "" }
