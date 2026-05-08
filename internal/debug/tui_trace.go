package debug

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
)

const (
	defaultTUITraceMaxLogBytes = 1 << 20 // 1 MiB per active trace file.
	defaultTUITraceBackups     = 2
	logTruncationMarker        = "\n[... truncated ...]\n"
)

// TUIFrameMeta describes the rendering context for a TUI frame entry.
type TUIFrameMeta struct {
	Reason                    string
	State                     string
	Width                     int
	Height                    int
	Fullscreen                bool
	ResizeEpoch               int
	MainScreenViewportOwned   bool
	MainScreenResetPending    bool
	MainScreenResetApplied    bool
	FlushedAnchor             string
	LiveTailAnchor            string
	OwnedStartAnchor          string
	StreamFlush               string
	OwnedStreamFlush          string
	MainScreenResetReason     string
	MainScreenSliceAnchor     string
	MainScreenPrevLines       int
	MainScreenNextLines       int
	MainScreenFrameLines      int
	MainScreenViewportHeight  int
	MainScreenRendererEnabled bool
	MainScreenResetMode       string
	MainScreenFrameSeq        int64
	MainScreenFullReset       bool
	MainScreenOffscreenReset  bool
}

// TUITrace captures TUI output and readable frame snapshots for debugging.
// All methods are nil-safe.
type TUITrace struct {
	fullscreen bool

	mu sync.Mutex

	outputFile *rotatingLogFile
	framesFile *rotatingLogFile

	outputSeq int64
	frameSeq  int64

	lastFrame string
}

// NewTUITrace creates log writers under sessionDir:
// - tui.output.log: escaped terminal output stream
// - tui.frames.log: ANSI-stripped readable frame snapshots
//
// Returns nil when sessionDir is empty.
func NewTUITrace(sessionDir string, fullscreen bool) (*TUITrace, error) {
	return newTUITraceWithLimits(sessionDir, fullscreen, defaultTUITraceMaxLogBytes, defaultTUITraceBackups)
}

func newTUITraceWithLimits(sessionDir string, fullscreen bool, maxBytes int64, backups int) (*TUITrace, error) {
	if sessionDir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, err
	}

	outPath := filepath.Join(sessionDir, "tui.output.log")
	framesPath := filepath.Join(sessionDir, "tui.frames.log")

	out, err := openRotatingLogFile(outPath, maxBytes, backups)
	if err != nil {
		return nil, err
	}

	frames, err := openRotatingLogFile(framesPath, maxBytes, backups)
	if err != nil {
		_ = out.Close()
		return nil, err
	}

	return &TUITrace{
		fullscreen: fullscreen,
		outputFile: out,
		framesFile: frames,
	}, nil
}

// WrapOutput returns an io.Writer that mirrors Bubble Tea terminal output into
// tui.output.log while preserving original writes to dst.
func (t *TUITrace) WrapOutput(dst io.Writer) io.Writer {
	if t == nil {
		return dst
	}
	if fileDst, ok := dst.(fileLikeWriter); ok {
		return &traceFileLikeOutputWriter{
			trace: t,
			dst:   fileDst,
		}
	}
	return &traceOutputWriter{
		trace: t,
		dst:   dst,
	}
}

// LogFrame records a readable frame snapshot. Duplicate consecutive frames are skipped.
func (t *TUITrace) LogFrame(meta TUIFrameMeta, rendered string) {
	if t == nil {
		return
	}
	t.logFrame("frame", meta, rendered, true)
}

// LogScrollback records readable text that has been committed to terminal scrollback.
func (t *TUITrace) LogScrollback(meta TUIFrameMeta, rendered string) {
	if t == nil {
		return
	}
	t.logFrame("scrollback", meta, rendered, false)
}

func frameDedupeKey(plain string, meta TUIFrameMeta) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(plain))
	sb.WriteByte('\x00')
	sb.WriteString(meta.Reason)
	sb.WriteByte('\x00')
	sb.WriteString(meta.State)
	sb.WriteByte('\x00')
	sb.WriteString(strconv.Itoa(meta.Width))
	sb.WriteByte('x')
	sb.WriteString(strconv.Itoa(meta.Height))
	sb.WriteByte('\x00')
	sb.WriteString(strconv.FormatBool(meta.Fullscreen))
	sb.WriteByte('\x00')
	sb.WriteString(strconv.Itoa(meta.ResizeEpoch))
	sb.WriteByte('\x00')
	sb.WriteString(strconv.FormatBool(meta.MainScreenViewportOwned))
	sb.WriteByte('\x00')
	sb.WriteString(strconv.FormatBool(meta.MainScreenResetPending))
	sb.WriteByte('\x00')
	sb.WriteString(strconv.FormatBool(meta.MainScreenResetApplied))
	sb.WriteByte('\x00')
	sb.WriteString(meta.FlushedAnchor)
	sb.WriteByte('\x00')
	sb.WriteString(meta.LiveTailAnchor)
	sb.WriteByte('\x00')
	sb.WriteString(meta.OwnedStartAnchor)
	sb.WriteByte('\x00')
	sb.WriteString(meta.StreamFlush)
	sb.WriteByte('\x00')
	sb.WriteString(meta.OwnedStreamFlush)
	sb.WriteByte('\x00')
	sb.WriteString(meta.MainScreenResetReason)
	sb.WriteByte('\x00')
	sb.WriteString(meta.MainScreenSliceAnchor)
	sb.WriteByte('\x00')
	sb.WriteString(strconv.Itoa(meta.MainScreenPrevLines))
	sb.WriteByte('\x00')
	sb.WriteString(strconv.Itoa(meta.MainScreenNextLines))
	sb.WriteByte('\x00')
	sb.WriteString(strconv.Itoa(meta.MainScreenFrameLines))
	sb.WriteByte('\x00')
	sb.WriteString(strconv.Itoa(meta.MainScreenViewportHeight))
	return sb.String()
}

func (t *TUITrace) logFrame(kind string, meta TUIFrameMeta, rendered string, dedupe bool) {
	if t == nil {
		return
	}
	plain := xansi.Strip(rendered)

	t.mu.Lock()
	defer t.mu.Unlock()

	if dedupe {
		normalized := frameDedupeKey(plain, meta)
		if normalized == t.lastFrame {
			return
		}
		t.lastFrame = normalized
	}

	t.frameSeq++
	var sb strings.Builder
	sb.WriteString("==== TUI ")
	sb.WriteString(strings.ToUpper(kind))
	sb.WriteString(" ====\n")
	sb.WriteString("ts: ")
	sb.WriteString(time.Now().UTC().Format(time.RFC3339Nano))
	sb.WriteString("\n")
	sb.WriteString("seq: ")
	sb.WriteString(strconv.FormatInt(t.frameSeq, 10))
	sb.WriteString("\n")
	sb.WriteString("reason: ")
	sb.WriteString(meta.Reason)
	sb.WriteString("\n")
	sb.WriteString("state: ")
	sb.WriteString(meta.State)
	sb.WriteString("\n")
	sb.WriteString("width: ")
	sb.WriteString(strconv.Itoa(meta.Width))
	sb.WriteString("\n")
	sb.WriteString("height: ")
	sb.WriteString(strconv.Itoa(meta.Height))
	sb.WriteString("\n")
	sb.WriteString("fullscreen: ")
	sb.WriteString(strconv.FormatBool(meta.Fullscreen))
	sb.WriteString("\n")
	sb.WriteString("resize_epoch: ")
	sb.WriteString(strconv.Itoa(meta.ResizeEpoch))
	sb.WriteString("\n")
	sb.WriteString("main_screen_viewport_owned: ")
	sb.WriteString(strconv.FormatBool(false))
	sb.WriteString("\n")
	sb.WriteString("main_screen_reset_pending: ")
	sb.WriteString(strconv.FormatBool(meta.MainScreenResetPending))
	sb.WriteString("\n")
	sb.WriteString("main_screen_reset_applied: ")
	sb.WriteString(strconv.FormatBool(meta.MainScreenResetApplied))
	sb.WriteString("\n")
	sb.WriteString("flushed_anchor: ")
	sb.WriteString(meta.FlushedAnchor)
	sb.WriteString("\n")
	sb.WriteString("live_tail_anchor: ")
	sb.WriteString(meta.LiveTailAnchor)
	sb.WriteString("\n")
	sb.WriteString("owned_start_anchor: ")
	sb.WriteString(meta.OwnedStartAnchor)
	sb.WriteString("\n")
	sb.WriteString("stream_flush: ")
	sb.WriteString(meta.StreamFlush)
	sb.WriteString("\n")
	sb.WriteString("owned_stream_flush: ")
	sb.WriteString(meta.OwnedStreamFlush)
	sb.WriteString("\n")
	sb.WriteString("main_screen_reset_reason: ")
	sb.WriteString(meta.MainScreenResetReason)
	sb.WriteString("\n")
	sb.WriteString("main_screen_slice_anchor: ")
	sb.WriteString(meta.MainScreenSliceAnchor)
	sb.WriteString("\n")
	sb.WriteString("main_screen_prev_lines: ")
	sb.WriteString(strconv.Itoa(meta.MainScreenPrevLines))
	sb.WriteString("\n")
	sb.WriteString("main_screen_next_lines: ")
	sb.WriteString(strconv.Itoa(meta.MainScreenNextLines))
	sb.WriteString("\n")
	sb.WriteString("main_screen_frame_lines: ")
	sb.WriteString(strconv.Itoa(meta.MainScreenFrameLines))
	sb.WriteString("\n")
	sb.WriteString("main_screen_viewport_height: ")
	sb.WriteString(strconv.Itoa(meta.MainScreenViewportHeight))
	sb.WriteString("\n")
	sb.WriteString("main_screen_offscreen_freeze_count: ")
	sb.WriteString(strconv.Itoa(0))
	sb.WriteString("\n")
	sb.WriteString("main_screen_renderer_enabled: ")
	sb.WriteString(strconv.FormatBool(meta.MainScreenRendererEnabled))
	sb.WriteString("\n")
	sb.WriteString("main_screen_reset_mode: ")
	sb.WriteString(meta.MainScreenResetMode)
	sb.WriteString("\n")
	sb.WriteString("main_screen_frame_seq: ")
	sb.WriteString(strconv.FormatInt(meta.MainScreenFrameSeq, 10))
	sb.WriteString("\n")
	sb.WriteString("main_screen_full_reset: ")
	sb.WriteString(strconv.FormatBool(meta.MainScreenFullReset))
	sb.WriteString("\n")
	sb.WriteString("main_screen_offscreen_reset: ")
	sb.WriteString(strconv.FormatBool(meta.MainScreenOffscreenReset))
	sb.WriteString("\n")
	sb.WriteString("--\n")
	sb.WriteString(plain)
	sb.WriteString("\n\n")

	_ = t.framesFile.AppendString(sb.String())
}

func (t *TUITrace) logOutputChunk(p []byte) {
	if t == nil || len(p) == 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.outputSeq++
	var sb strings.Builder
	sb.WriteString("==== TUI OUTPUT ====\n")
	sb.WriteString("ts: ")
	sb.WriteString(time.Now().UTC().Format(time.RFC3339Nano))
	sb.WriteString("\n")
	sb.WriteString("seq: ")
	sb.WriteString(strconv.FormatInt(t.outputSeq, 10))
	sb.WriteString("\n")
	sb.WriteString("bytes: ")
	sb.WriteString(strconv.Itoa(len(p)))
	sb.WriteString("\n")
	sb.WriteString("fullscreen: ")
	sb.WriteString(strconv.FormatBool(t.fullscreen))
	sb.WriteString("\n")
	sb.WriteString("--\n")
	sb.WriteString(strconv.QuoteToASCII(string(p)))
	sb.WriteString("\n\n")
	_ = t.outputFile.AppendString(sb.String())
}

// Close closes both trace files.
func (t *TUITrace) Close() error {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	var errs []error
	if t.outputFile != nil {
		if err := t.outputFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if t.framesFile != nil {
		if err := t.framesFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("close tui trace: %v", errs)
}

type rotatingLogFile struct {
	path       string
	maxBytes   int64
	maxBackups int
	file       *os.File
	size       int64
}

func openRotatingLogFile(path string, maxBytes int64, maxBackups int) (*rotatingLogFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	lf := &rotatingLogFile{
		path:       path,
		maxBytes:   maxBytes,
		maxBackups: maxBackups,
		file:       f,
		size:       info.Size(),
	}
	if maxBytes > 0 && lf.size > maxBytes {
		if err := lf.rotate(); err != nil {
			_ = lf.Close()
			return nil, err
		}
	}
	if err := lf.enforceBackupBounds(); err != nil {
		_ = lf.Close()
		return nil, err
	}
	return lf, nil
}

func (f *rotatingLogFile) enforceBackupBounds() error {
	if err := removeBackupsBeyondLimit(f.path, f.maxBackups); err != nil {
		return err
	}
	if f.maxBackups <= 0 {
		return nil
	}
	for i := 1; i <= f.maxBackups; i++ {
		if err := capExistingLogFile(fmt.Sprintf("%s.%d", f.path, i), f.maxBytes); err != nil {
			return err
		}
	}
	return nil
}

func (f *rotatingLogFile) AppendString(s string) error {
	if f == nil || f.file == nil || s == "" {
		return nil
	}
	data := []byte(s)
	data = truncateLogEntryBytes(data, f.maxBytes)
	if f.maxBytes > 0 && f.size+int64(len(data)) > f.maxBytes {
		if err := f.rotate(); err != nil {
			return err
		}
	}
	n, err := f.file.Write(data)
	f.size += int64(n)
	return err
}

func truncateLogEntryBytes(data []byte, maxBytes int64) []byte {
	if maxBytes <= 0 || int64(len(data)) <= maxBytes {
		return data
	}
	marker := []byte(logTruncationMarker)
	if int64(len(marker)) >= maxBytes {
		return marker[:maxBytes]
	}
	keep := int(maxBytes) - len(marker)
	out := make([]byte, 0, maxBytes)
	out = append(out, data[:keep]...)
	out = append(out, marker...)
	return out
}

func (f *rotatingLogFile) rotate() error {
	if f.file != nil {
		if err := f.file.Close(); err != nil {
			return err
		}
		f.file = nil
	}

	if f.maxBackups <= 0 {
		if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := f.enforceBackupBounds(); err != nil {
			return err
		}
		return f.reopen()
	}

	oldest := fmt.Sprintf("%s.%d", f.path, f.maxBackups)
	_ = os.Remove(oldest)
	for i := f.maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", f.path, i)
		dst := fmt.Sprintf("%s.%d", f.path, i+1)
		if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(f.path, f.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := f.enforceBackupBounds(); err != nil {
		return err
	}
	return f.reopen()
}

func (f *rotatingLogFile) reopen() error {
	nf, err := os.OpenFile(f.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	f.file = nf
	f.size = 0
	return nil
}

func (f *rotatingLogFile) Close() error {
	if f == nil || f.file == nil {
		return nil
	}
	return f.file.Close()
}

func removeBackupsBeyondLimit(path string, maxBackups int) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	prefix := base + "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
		if err != nil {
			continue
		}
		if maxBackups <= 0 || n > maxBackups {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func capExistingLogFile(path string, maxBytes int64) error {
	if maxBytes <= 0 {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() <= maxBytes {
		return nil
	}

	marker := []byte(logTruncationMarker)
	if int64(len(marker)) >= maxBytes {
		return os.WriteFile(path, marker[:maxBytes], 0o644)
	}

	keep := int(maxBytes) - len(marker)
	buf := make([]byte, keep)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	n, readErr := io.ReadFull(file, buf)
	closeErr := file.Close()
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}

	out := append([]byte{}, buf[:n]...)
	out = append(out, marker...)
	return os.WriteFile(path, out, 0o644)
}

type traceOutputWriter struct {
	trace *TUITrace
	dst   io.Writer
}

func (w *traceOutputWriter) Write(p []byte) (int, error) {
	return writeAndMirror(w.trace, w.dst, p)
}

type fileLikeWriter interface {
	io.ReadWriteCloser
	Fd() uintptr
}

type traceFileLikeOutputWriter struct {
	trace *TUITrace
	dst   fileLikeWriter
}

func (w *traceFileLikeOutputWriter) Write(p []byte) (int, error) {
	return writeAndMirror(w.trace, w.dst, p)
}

func (w *traceFileLikeOutputWriter) Read(p []byte) (int, error) {
	return w.dst.Read(p)
}

func (w *traceFileLikeOutputWriter) Close() error {
	return w.dst.Close()
}

func (w *traceFileLikeOutputWriter) Fd() uintptr {
	return w.dst.Fd()
}

func writeAndMirror(trace *TUITrace, dst io.Writer, p []byte) (int, error) {
	n, err := dst.Write(p)
	if n > 0 {
		trace.logOutputChunk(p[:n])
	}
	return n, err
}
