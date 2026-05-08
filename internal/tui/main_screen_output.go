package tui

import (
	"io"
	"strings"
	"sync"
)

const (
	mainScreenFrameTokenPrefix = "\x1e\x1c"
	mainScreenFrameTokenSuffix = "\x1c\x1e"
	mainScreenFrameTokenZero   = byte('\x1d')
	mainScreenFrameTokenOne    = byte('\x1f')
	maxStagedMainScreenFrames  = 16
)

type MainScreenOutputController struct {
	mu       sync.Mutex
	seq      int64
	staged   map[int64]MainScreenFrameInput
	renderer *MainScreenRenderer
	last     MainScreenOutputStats
}

type MainScreenOutputStats struct {
	FrameSeq         int64
	ConsumedFrameSeq int64
	FullReset        bool
	ResetReason      string
	PrevLines        int
	NextLines        int
	ViewportHeight   int
	OffscreenReset   bool
}

func NewMainScreenOutputController(mode MainScreenResetMode) *MainScreenOutputController {
	return &MainScreenOutputController{
		staged:   make(map[int64]MainScreenFrameInput),
		renderer: NewMainScreenRenderer(mode),
	}
}

func (c *MainScreenOutputController) StageFrame(frame MainScreenFrameInput) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	seq := c.seq
	c.staged[seq] = frame
	c.dropStaleStagedFramesLocked(seq)
	return mainScreenFrameToken(seq)
}

func (c *MainScreenOutputController) WrapOutput(dst io.Writer) io.Writer {
	if fileDst, ok := dst.(mainScreenFileLikeWriter); ok {
		return &mainScreenFileLikeOutputWriter{
			controller: c,
			dst:        fileDst,
		}
	}
	return &mainScreenOutputWriter{controller: c, dst: dst}
}

func (c *MainScreenOutputController) consumeStaged(seq int64) (MainScreenRenderResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame, ok := c.staged[seq]
	if !ok {
		return MainScreenRenderResult{}, false
	}
	delete(c.staged, seq)
	result := c.renderer.Render(frame)
	c.last = MainScreenOutputStats{
		FrameSeq:         seq,
		ConsumedFrameSeq: seq,
		FullReset:        result.FullReset,
		ResetReason:      result.ResetReason,
		PrevLines:        result.PrevLines,
		NextLines:        result.NextLines,
		ViewportHeight:   result.ViewportHeight,
		OffscreenReset:   result.OffscreenReset,
	}
	return result, true
}

func (c *MainScreenOutputController) Stats() MainScreenOutputStats {
	if c == nil {
		return MainScreenOutputStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	stats := c.last
	if len(c.staged) > 0 {
		for seq, frame := range c.staged {
			if seq > stats.FrameSeq {
				stats.FrameSeq = seq
				stats.NextLines = len(splitMainScreenFrameLines(frame.Frame))
				stats.ViewportHeight = frame.ViewportHeight
			}
		}
	}
	return stats
}

type mainScreenOutputWriter struct {
	controller *MainScreenOutputController
	dst        io.Writer
}

func (w *mainScreenOutputWriter) Write(p []byte) (int, error) {
	raw := string(p)
	seq, hasToken := parseMainScreenFrameToken(raw)
	if !hasToken {
		_, err := w.dst.Write(p)
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}
	result, ok := w.controller.consumeStaged(seq)
	if !ok {
		return len(p), nil
	}
	_, err := io.WriteString(w.dst, result.Output)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func mainScreenFrameToken(seq int64) string {
	var sb strings.Builder
	sb.Grow(len(mainScreenFrameTokenPrefix) + 63 + len(mainScreenFrameTokenSuffix))
	sb.WriteString(mainScreenFrameTokenPrefix)
	started := false
	for bit := 62; bit >= 0; bit-- {
		if seq&(int64(1)<<bit) != 0 {
			started = true
		}
		if !started {
			continue
		}
		if seq&(int64(1)<<bit) != 0 {
			sb.WriteByte(mainScreenFrameTokenOne)
		} else {
			sb.WriteByte(mainScreenFrameTokenZero)
		}
	}
	if !started {
		sb.WriteByte(mainScreenFrameTokenZero)
	}
	sb.WriteString(mainScreenFrameTokenSuffix)
	return sb.String()
}

func parseMainScreenFrameToken(raw string) (int64, bool) {
	start := strings.Index(raw, mainScreenFrameTokenPrefix)
	if start < 0 {
		return 0, false
	}
	valueStart := start + len(mainScreenFrameTokenPrefix)
	if valueStart >= len(raw) {
		return 0, false
	}
	rest := raw[valueStart:]
	endOffset := strings.Index(rest, mainScreenFrameTokenSuffix)
	if endOffset <= 0 {
		return 0, false
	}
	payload := rest[:endOffset]
	var seq int64
	for i := 0; i < len(payload); i++ {
		if i >= 63 {
			return 0, false
		}
		seq <<= 1
		switch payload[i] {
		case mainScreenFrameTokenZero:
		case mainScreenFrameTokenOne:
			seq |= 1
		default:
			return 0, false
		}
	}
	return seq, true
}

func stripMainScreenFrameTokens(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	remaining := raw
	for {
		start := strings.Index(remaining, mainScreenFrameTokenPrefix)
		if start < 0 {
			b.WriteString(remaining)
			return b.String()
		}
		b.WriteString(remaining[:start])
		valueStart := start + len(mainScreenFrameTokenPrefix)
		rest := remaining[valueStart:]
		endOffset := strings.Index(rest, mainScreenFrameTokenSuffix)
		if endOffset <= 0 {
			b.WriteString(remaining[start:])
			return b.String()
		}
		if _, ok := parseMainScreenFrameToken(remaining[start : valueStart+endOffset+len(mainScreenFrameTokenSuffix)]); !ok {
			b.WriteString(remaining[start : valueStart+endOffset+len(mainScreenFrameTokenSuffix)])
			remaining = rest[endOffset+len(mainScreenFrameTokenSuffix):]
			continue
		}
		remaining = rest[endOffset+len(mainScreenFrameTokenSuffix):]
	}
}

func (c *MainScreenOutputController) dropStaleStagedFramesLocked(currentSeq int64) {
	cutoff := currentSeq - maxStagedMainScreenFrames
	for seq := range c.staged {
		if seq <= cutoff {
			delete(c.staged, seq)
		}
	}
}

type mainScreenFileLikeWriter interface {
	io.ReadWriteCloser
	Fd() uintptr
}

type mainScreenFileLikeOutputWriter struct {
	controller *MainScreenOutputController
	dst        mainScreenFileLikeWriter
}

func (w *mainScreenFileLikeOutputWriter) Write(p []byte) (int, error) {
	return (&mainScreenOutputWriter{controller: w.controller, dst: w.dst}).Write(p)
}

func (w *mainScreenFileLikeOutputWriter) Read(p []byte) (int, error) {
	return w.dst.Read(p)
}

func (w *mainScreenFileLikeOutputWriter) Close() error {
	return w.dst.Close()
}

func (w *mainScreenFileLikeOutputWriter) Fd() uintptr {
	return w.dst.Fd()
}
