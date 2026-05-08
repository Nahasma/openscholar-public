package tui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

type MainScreenFrameInput struct {
	Frame          string
	Width          int
	Height         int
	ViewportHeight int
	ResizeEpoch    int
	SliceAnchor    string
	Reason         string
}

type mainScreenFrameStateLite struct {
	Width          int
	Height         int
	ViewportHeight int
	LineCount      int
	ScrollbackRows int
	ResizeEpoch    int
	SliceAnchor    string
	Lines          []string
}

func (s mainScreenFrameStateLite) visibleWindow() (start, rows int) {
	start = max(0, s.ScrollbackRows)
	if start > s.LineCount {
		start = s.LineCount
	}
	rows = s.LineCount - start
	if rows <= 0 {
		rows = 1
	}
	return start, rows
}

type MainScreenRenderResult struct {
	Output         string
	FullReset      bool
	ResetReason    string
	PrevLines      int
	NextLines      int
	ViewportHeight int
	OffscreenReset bool
	FrameUpdated   mainScreenFrameStateLite
}

type MainScreenRenderer struct {
	resetMode MainScreenResetMode
	prev      *mainScreenFrameStateLite
}

func NewMainScreenRenderer(mode MainScreenResetMode) *MainScreenRenderer {
	return &MainScreenRenderer{resetMode: mode}
}

func ClearTerminalSequence(mode MainScreenResetMode) string {
	if mode == MainScreenResetModeFull {
		return "\x1b[2J\x1b[3J\x1b[H"
	}
	return "\x1b[2J\x1b[H"
}

func splitMainScreenFrameLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

func (r *MainScreenRenderer) Render(next MainScreenFrameInput) MainScreenRenderResult {
	nextLines := splitMainScreenFrameLines(next.Frame)
	viewportHeight := next.ViewportHeight
	if viewportHeight <= 0 {
		viewportHeight = next.Height
	}
	nextState := mainScreenFrameStateLite{
		Width:          next.Width,
		Height:         next.Height,
		ViewportHeight: viewportHeight,
		LineCount:      len(nextLines),
		ScrollbackRows: 0,
		ResizeEpoch:    next.ResizeEpoch,
		SliceAnchor:    next.SliceAnchor,
		Lines:          nextLines,
	}
	if nextState.ViewportHeight > 0 {
		nextState.ScrollbackRows = max(0, nextState.LineCount-nextState.ViewportHeight)
	}

	out := MainScreenRenderResult{
		Output:         paintMainScreenLines(nextLines, next.Width),
		PrevLines:      0,
		NextLines:      nextState.LineCount,
		ViewportHeight: nextState.ViewportHeight,
		FrameUpdated:   nextState,
	}
	if r.prev == nil {
		if next.Reason == "return-main" {
			out.FullReset = true
			out.ResetReason = "return-main"
			out.Output = ClearTerminalSequence(r.resetMode) + paintMainScreenLines(nextLines, next.Width)
		}
		r.prev = &nextState
		return out
	}

	out.PrevLines = r.prev.LineCount
	fullReason := r.fullResetReason(next, nextState)
	if fullReason != "" {
		out.FullReset = true
		out.ResetReason = fullReason
		out.OffscreenReset = fullReason == "offscreen"
		out.Output = ClearTerminalSequence(r.resetMode) + paintMainScreenLines(nextLines, next.Width)
	} else {
		out.Output = r.repaintVisibleRows(nextState)
	}
	r.prev = &nextState
	return out
}

func (r *MainScreenRenderer) fullResetReason(next MainScreenFrameInput, nextState mainScreenFrameStateLite) string {
	if r.prev == nil {
		return ""
	}
	sliceChanged := r.prev.SliceAnchor != nextState.SliceAnchor
	switch {
	case next.Reason == "return-main":
		return "return-main"
	case r.prev.Width != 0 && r.prev.Width != nextState.Width:
		return "resize-width"
	case r.prev.ViewportHeight > nextState.ViewportHeight:
		return "resize-height-shrink"
	case !sliceChanged && nextState.ScrollbackRows < r.prev.ScrollbackRows:
		return "offscreen"
	case !sliceChanged && r.prev.ScrollbackRows > 0 && nextState.LineCount <= nextState.ViewportHeight && nextState.LineCount < r.prev.LineCount:
		return "offscreen"
	case !sliceChanged && r.diffTouchesScrollback(nextState):
		return "offscreen"
	}
	return ""
}

func (r *MainScreenRenderer) diffTouchesScrollback(nextState mainScreenFrameStateLite) bool {
	if r.prev == nil || r.prev.ScrollbackRows <= 0 {
		return false
	}
	limit := min(r.prev.ScrollbackRows, max(len(r.prev.Lines), len(nextState.Lines)))
	for i := 0; i < limit; i++ {
		prevLine := ""
		if i < len(r.prev.Lines) {
			prevLine = r.prev.Lines[i]
		}
		nextLine := ""
		if i < len(nextState.Lines) {
			nextLine = nextState.Lines[i]
		}
		if prevLine != nextLine {
			return true
		}
	}
	return false
}

func (r *MainScreenRenderer) repaintVisibleRows(nextState mainScreenFrameStateLite) string {
	if r.prev == nil {
		return paintMainScreenLines(nextState.Lines, nextState.Width)
	}
	nextVisibleStart, nextVisibleRows := nextState.visibleWindow()
	prevVisibleStart, prevVisibleRows := r.prev.visibleWindow()
	start := prevVisibleStart
	if r.prev.SliceAnchor != nextState.SliceAnchor {
		start = max(0, nextState.LineCount-prevVisibleRows)
	} else if nextVisibleStart <= prevVisibleStart {
		start = nextVisibleStart
	}
	var sb strings.Builder
	rowsUp := prevVisibleRows - 1
	if rowsUp > 0 {
		sb.WriteString(xansi.CursorUp(rowsUp))
	}
	sb.WriteString(paintMainScreenLines(nextState.Lines[start:], nextState.Width))
	nextVisibleRows = max(1, nextState.LineCount-start)
	if prevVisibleRows > nextVisibleRows {
		sb.WriteString(clearMainScreenRowsBelowPaintedFrame())
	}
	return sb.String()
}

func clearMainScreenRowsBelowPaintedFrame() string {
	return xansi.CursorDown(1) + xansi.EraseScreenBelow + xansi.CursorUp(1)
}

func paintMainScreenLines(lines []string, width int) string {
	if len(lines) == 0 {
		return " "
	}
	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(line)
		if width > 0 && xansi.StringWidth(line) < width {
			sb.WriteString(xansi.EraseLineRight)
		}
		if i < len(lines)-1 {
			sb.WriteString("\r\n")
		}
	}
	if width > 0 {
		sb.WriteString(xansi.CursorBackward(width))
	} else {
		sb.WriteByte('\r')
	}
	return sb.String()
}
