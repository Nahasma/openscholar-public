package debug

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewTUITrace_DisabledWhenEmptyDir(t *testing.T) {
	tr, err := NewTUITrace("", false)
	if err != nil {
		t.Fatalf("NewTUITrace error: %v", err)
	}
	if tr != nil {
		t.Fatal("expected nil trace when sessionDir is empty")
	}
}

func TestTUITraceWrapOutputMirrorsWrites(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTUITrace(dir, true)
	if err != nil {
		t.Fatalf("NewTUITrace error: %v", err)
	}
	defer tr.Close()

	var dst bytes.Buffer
	w := tr.WrapOutput(&dst)
	chunk := []byte("\x1b[2Jhello\n")
	n, err := w.Write(chunk)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if n != len(chunk) {
		t.Fatalf("expected %d bytes written, got %d", len(chunk), n)
	}
	if dst.String() != string(chunk) {
		t.Fatalf("unexpected dst content: %q", dst.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, "tui.output.log"))
	if err != nil {
		t.Fatalf("read output log: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "\\x1b[2Jhello\\n") {
		t.Fatalf("expected escaped chunk in output log, got: %q", text)
	}
}

func TestTUITraceLogFrameDedupesConsecutiveContent(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTUITrace(dir, false)
	if err != nil {
		t.Fatalf("NewTUITrace error: %v", err)
	}
	defer tr.Close()

	meta := TUIFrameMeta{Reason: "view", State: "chat", Width: 80, Height: 24, Fullscreen: false}
	tr.LogFrame(meta, "same content")
	tr.LogFrame(meta, "same content")
	tr.LogFrame(meta, "different")

	data, err := os.ReadFile(filepath.Join(dir, "tui.frames.log"))
	if err != nil {
		t.Fatalf("read frames log: %v", err)
	}
	text := string(data)
	if got := strings.Count(text, "==== TUI FRAME ===="); got != 2 {
		t.Fatalf("expected 2 frame entries after dedupe, got %d", got)
	}
}

func TestTUITraceLogFrameKeepsMetadataOnlyChanges(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTUITrace(dir, false)
	if err != nil {
		t.Fatalf("NewTUITrace error: %v", err)
	}
	defer tr.Close()

	meta := TUIFrameMeta{Reason: "view", State: "chat", Width: 80, Height: 24}
	tr.LogFrame(meta, "same content")
	meta.ResizeEpoch = 2
	meta.MainScreenResetApplied = true
	meta.MainScreenResetReason = "resize-width"
	meta.MainScreenSliceAnchor = "msg-1"
	meta.MainScreenPrevLines = 20
	meta.MainScreenNextLines = 12
	meta.MainScreenFrameLines = 12
	meta.MainScreenViewportHeight = 10
	meta.MainScreenOffscreenReset = true
	tr.LogFrame(meta, "\x1b[2J\x1b[Hsame content")

	data, err := os.ReadFile(filepath.Join(dir, "tui.frames.log"))
	if err != nil {
		t.Fatalf("read frames log: %v", err)
	}
	text := string(data)
	if got := strings.Count(text, "==== TUI FRAME ===="); got != 2 {
		t.Fatalf("expected metadata-only frame change to bypass dedupe, got %d", got)
	}
	if !strings.Contains(text, "main_screen_reset_applied: true") {
		t.Fatalf("expected reset-applied metadata in trace, got: %q", text)
	}
	if !strings.Contains(text, "main_screen_reset_reason: resize-width") {
		t.Fatalf("expected reset reason metadata in trace, got: %q", text)
	}
	if !strings.Contains(text, "main_screen_slice_anchor: msg-1") {
		t.Fatalf("expected slice anchor metadata in trace, got: %q", text)
	}
	if !strings.Contains(text, "main_screen_next_lines: 12") {
		t.Fatalf("expected next-lines metadata in trace, got: %q", text)
	}
	if !strings.Contains(text, "main_screen_viewport_height: 10") {
		t.Fatalf("expected viewport-height metadata in trace, got: %q", text)
	}
}

type fakeFDWriter struct {
	readBuf  *bytes.Reader
	writeBuf bytes.Buffer
	fd       uintptr
	closed   bool
}

func (w *fakeFDWriter) Write(p []byte) (int, error) {
	return w.writeBuf.Write(p)
}

func (w *fakeFDWriter) Read(p []byte) (int, error) {
	if w.readBuf == nil {
		w.readBuf = bytes.NewReader(nil)
	}
	return w.readBuf.Read(p)
}

func (w *fakeFDWriter) Close() error {
	w.closed = true
	return nil
}

func (w *fakeFDWriter) Fd() uintptr {
	return w.fd
}

type partialWriter struct {
	n   int
	err error
	buf bytes.Buffer
}

func (w *partialWriter) Write(p []byte) (int, error) {
	n := w.n
	if n > len(p) {
		n = len(p)
	}
	if n > 0 {
		_, _ = w.buf.Write(p[:n])
	}
	return n, w.err
}

func TestTUITraceWrapOutputPreservesFileLikeInterface(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTUITrace(dir, false)
	if err != nil {
		t.Fatalf("NewTUITrace error: %v", err)
	}
	defer tr.Close()

	dst := &fakeFDWriter{
		readBuf: bytes.NewReader([]byte("hi")),
		fd:      123,
	}
	wrapped := tr.WrapOutput(dst)

	fileWrapped, ok := wrapped.(interface {
		Read([]byte) (int, error)
		Write([]byte) (int, error)
		Close() error
		Fd() uintptr
	})
	if !ok {
		t.Fatal("expected wrapped writer to implement file-like interface")
	}
	if got := fileWrapped.Fd(); got != dst.fd {
		t.Fatalf("expected fd %d, got %d", dst.fd, got)
	}
	readBuf := make([]byte, 2)
	readN, readErr := fileWrapped.Read(readBuf)
	if readErr != nil {
		t.Fatalf("read error: %v", readErr)
	}
	if readN != 2 || string(readBuf[:readN]) != "hi" {
		t.Fatalf("unexpected read result n=%d buf=%q", readN, string(readBuf[:readN]))
	}

	chunk := []byte("hello fd\n")
	n, err := fileWrapped.Write(chunk)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if n != len(chunk) {
		t.Fatalf("expected write n=%d, got %d", len(chunk), n)
	}
	if dst.writeBuf.String() != string(chunk) {
		t.Fatalf("unexpected dst content: %q", dst.writeBuf.String())
	}
	if err := fileWrapped.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if !dst.closed {
		t.Fatal("expected close to be forwarded")
	}

	data, err := os.ReadFile(filepath.Join(dir, "tui.output.log"))
	if err != nil {
		t.Fatalf("read output log: %v", err)
	}
	if !strings.Contains(string(data), "hello fd\\n") {
		t.Fatalf("expected mirrored output log to contain chunk, got %q", string(data))
	}
}

func TestTUITraceWrapOutputDoesNotInventFdForPlainWriter(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTUITrace(dir, false)
	if err != nil {
		t.Fatalf("NewTUITrace error: %v", err)
	}
	defer tr.Close()

	var dst bytes.Buffer
	wrapped := tr.WrapOutput(&dst)
	if _, ok := wrapped.(interface{ Fd() uintptr }); ok {
		t.Fatal("plain writer wrapper should not implement Fd")
	}
	if _, ok := wrapped.(io.ReadWriteCloser); ok {
		t.Fatal("plain writer wrapper should not implement ReadWriteCloser")
	}
}

func TestTUITraceWrapOutputPreservesPartialWriteAndError(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTUITrace(dir, false)
	if err != nil {
		t.Fatalf("NewTUITrace error: %v", err)
	}
	defer tr.Close()

	writeErr := errors.New("short write")
	dst := &partialWriter{n: 4, err: writeErr}
	wrapped := tr.WrapOutput(dst)

	chunk := []byte("abcdefg")
	n, err := wrapped.Write(chunk)
	if n != 4 {
		t.Fatalf("expected n=4, got %d", n)
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("expected error %v, got %v", writeErr, err)
	}

	if dst.buf.String() != "abcd" {
		t.Fatalf("expected dst to receive successful prefix only, got %q", dst.buf.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, "tui.output.log"))
	if err != nil {
		t.Fatalf("read output log: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "\"abcd\"") {
		t.Fatalf("expected output log to contain successful prefix, got %q", text)
	}
	if strings.Contains(text, "abcdefg") {
		t.Fatalf("output log should not contain full chunk on partial write, got %q", text)
	}
}

func TestTUITraceOutputLogRotatesAndCapsSize(t *testing.T) {
	dir := t.TempDir()
	tr, err := newTUITraceWithLimits(dir, false, 160, 2)
	if err != nil {
		t.Fatalf("newTUITraceWithLimits error: %v", err)
	}
	defer tr.Close()

	w := tr.WrapOutput(io.Discard)
	for i := 0; i < 20; i++ {
		_, _ = w.Write([]byte(fmt.Sprintf("chunk-%02d\n", i)))
	}

	checkBoundedLogFile(t, filepath.Join(dir, "tui.output.log"), 160)
	checkBoundedLogFile(t, filepath.Join(dir, "tui.output.log.1"), 160)
	checkBoundedLogFile(t, filepath.Join(dir, "tui.output.log.2"), 160)
	if _, err := os.Stat(filepath.Join(dir, "tui.output.log.3")); !os.IsNotExist(err) {
		t.Fatalf("unexpected extra backup file exists: %v", err)
	}
}

func TestTUITraceFrameLogRotatesAndCapsSize(t *testing.T) {
	dir := t.TempDir()
	tr, err := newTUITraceWithLimits(dir, false, 240, 2)
	if err != nil {
		t.Fatalf("newTUITraceWithLimits error: %v", err)
	}
	defer tr.Close()

	meta := TUIFrameMeta{Reason: "render", State: "chat", Width: 80, Height: 24}
	for i := 0; i < 12; i++ {
		meta.ResizeEpoch = i + 1 // keep dedupe from collapsing entries
		tr.LogFrame(meta, fmt.Sprintf("frame body %d", i))
	}

	checkBoundedLogFile(t, filepath.Join(dir, "tui.frames.log"), 240)
	checkBoundedLogFile(t, filepath.Join(dir, "tui.frames.log.1"), 240)
	checkBoundedLogFile(t, filepath.Join(dir, "tui.frames.log.2"), 240)
}

func TestTUITraceOversizedEntryIsTruncatedWithMarker(t *testing.T) {
	dir := t.TempDir()
	tr, err := newTUITraceWithLimits(dir, false, 120, 1)
	if err != nil {
		t.Fatalf("newTUITraceWithLimits error: %v", err)
	}
	defer tr.Close()

	w := tr.WrapOutput(io.Discard)
	_, _ = w.Write([]byte(strings.Repeat("x", 1000)))

	data, err := os.ReadFile(filepath.Join(dir, "tui.output.log"))
	if err != nil {
		t.Fatalf("read output log: %v", err)
	}
	if len(data) > 120 {
		t.Fatalf("expected capped output log <=120 bytes, got %d", len(data))
	}
	if !strings.Contains(string(data), "[... truncated ...]") {
		t.Fatalf("expected truncation marker in output log, got %q", string(data))
	}
}

func TestTUITraceRotatesOversizedExistingFileOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tui.output.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 200)), 0o644); err != nil {
		t.Fatalf("seed oversized output log: %v", err)
	}

	tr, err := newTUITraceWithLimits(dir, false, 120, 1)
	if err != nil {
		t.Fatalf("newTUITraceWithLimits error: %v", err)
	}
	defer tr.Close()

	checkBoundedLogFile(t, path, 120)
	backupPath := path + ".1"
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected oversized existing file to be rotated: %v", err)
	}
	checkBoundedLogFile(t, backupPath, 120)
}

func TestTUITraceRemovesPreexistingBackupsBeyondLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tui.output.log")
	for _, name := range []string{
		"tui.output.log",
		"tui.output.log.1",
		"tui.output.log.2",
		"tui.output.log.3",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("seed\n"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	tr, err := newTUITraceWithLimits(dir, false, 120, 2)
	if err != nil {
		t.Fatalf("newTUITraceWithLimits error: %v", err)
	}
	defer tr.Close()

	for _, backup := range []string{path + ".1", path + ".2"} {
		if _, err := os.Stat(backup); err != nil {
			t.Fatalf("expected retained backup %s: %v", backup, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("expected stale backup beyond limit to be removed: %v", err)
	}
}

func checkBoundedLogFile(t *testing.T, path string, capBytes int) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() > int64(capBytes) {
		t.Fatalf("expected %s <= %d bytes, got %d", path, capBytes, info.Size())
	}
}
