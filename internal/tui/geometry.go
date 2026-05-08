package tui

const (
	defaultWindowWidth  = 80
	defaultWindowHeight = 24
)

type geometrySource uint8

const (
	geometrySourceUnknown geometrySource = iota
	geometrySourceFallback
	geometrySourceBootstrap
	geometrySourceWindow
)

// GeometryState records the sanitized terminal geometry used by layout and
// rendering. Width/height remain mirrored on Model for existing call sites.
type GeometryState struct {
	Width  int
	Height int
	Source geometrySource
	Epoch  int
	Valid  bool
}

type fullscreenFrameState struct {
	Dirty       bool
	ResizeEpoch int
	LastWidth   int
	LastHeight  int
	LastEpoch   int
	HasFrame    bool
}

type initialWindowSizeMsg struct {
	Width  int
	Height int
	Source geometrySource
}

func (m *Model) markFullscreenFrameDirty() {
	frame := m.ensureFullscreenFrameState()
	frame.Dirty = true
	frame.ResizeEpoch = m.resizeEpoch
}

func (m *Model) recordFullscreenFramePainted(width, height, epoch int) {
	frame := m.ensureFullscreenFrameState()
	frame.LastWidth = width
	frame.LastHeight = height
	frame.LastEpoch = epoch
	frame.HasFrame = true
	if frame.Dirty &&
		width == m.width &&
		height == m.height &&
		epoch == m.resizeEpoch {
		frame.Dirty = false
	}
}

func (m *Model) ensureFullscreenFrameState() *fullscreenFrameState {
	if m.fullscreenFrame == nil {
		m.fullscreenFrame = &fullscreenFrameState{}
	}
	return m.fullscreenFrame
}

func (m *Model) ensureMainScreenFrameState() *mainScreenFrameState {
	if m.chat.mainScreenFrame == nil {
		m.chat.mainScreenFrame = &mainScreenFrameState{}
	}
	return m.chat.mainScreenFrame
}

func (m *Model) applyWindowSize(width, height int, source geometrySource) (prevWidth int, prevHeight int, changed bool) {
	prevWidth, prevHeight = m.width, m.height
	if source == geometrySourceUnknown {
		source = geometrySourceWindow
	}

	if width <= 0 || height <= 0 {
		if m.geometry.Valid {
			return prevWidth, prevHeight, false
		}
		width, height = defaultWindowWidth, defaultWindowHeight
		source = geometrySourceFallback
	}

	if source != geometrySourceWindow && m.geometry.Valid && m.geometry.Source == geometrySourceWindow {
		return prevWidth, prevHeight, false
	}
	if source == geometrySourceFallback && m.geometry.Valid && m.geometry.Source != geometrySourceFallback {
		return prevWidth, prevHeight, false
	}

	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	if m.width == width && m.height == height && m.width > 0 && m.height > 0 {
		if !m.geometry.Valid {
			m.geometry = GeometryState{
				Width:  width,
				Height: height,
				Source: source,
				Epoch:  m.resizeEpoch,
				Valid:  true,
			}
		} else if source > m.geometry.Source {
			m.geometry.Source = source
		}
		return prevWidth, prevHeight, false
	}

	m.width = width
	m.height = height
	m.resizeEpoch++
	m.geometry = GeometryState{
		Width:  width,
		Height: height,
		Source: source,
		Epoch:  m.resizeEpoch,
		Valid:  true,
	}
	return prevWidth, prevHeight, true
}

func (m *Model) syncGeometryToControls() {
	m.composer.input.SetWidth(m.width)
	m.search.historySearch.SetWidth(m.width)
	m.search.textSearch.SetWidth(m.width)
}

func (m Model) dialogContentWidth() int {
	width := m.width - 4
	if width < 0 {
		return 0
	}
	return width
}

func innerDialogWidth(width int) int {
	width -= 4
	if width < 0 {
		return 0
	}
	return width
}
