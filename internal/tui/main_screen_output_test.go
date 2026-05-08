package tui

import (
	"bytes"
	"io"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestMainScreenOutputController_StageFrameReturnsUniqueTokens(t *testing.T) {
	c := NewMainScreenOutputController(MainScreenResetModeFull)
	token1 := c.StageFrame(MainScreenFrameInput{
		Frame:          "FRAME",
		Width:          80,
		Height:         24,
		ViewportHeight: 10,
		ResizeEpoch:    1,
	})
	token2 := c.StageFrame(MainScreenFrameInput{
		Frame:          "FRAME2",
		Width:          80,
		Height:         24,
		ViewportHeight: 10,
		ResizeEpoch:    1,
	})
	if token1 == token2 {
		t.Fatalf("StageFrame token should be unique, got same token %q", token1)
	}
	seq1, ok := parseMainScreenFrameToken(token1)
	if !ok || seq1 <= 0 {
		t.Fatalf("token1 should be parseable with positive seq, token=%q seq=%d ok=%v", token1, seq1, ok)
	}
	seq2, ok := parseMainScreenFrameToken(token2)
	if !ok || seq2 <= seq1 {
		t.Fatalf("token2 should parse and increase seq: seq1=%d seq2=%d ok=%v", seq1, seq2, ok)
	}
	if width := xansi.StringWidth(token2); width != 0 {
		t.Fatalf("frame token must have zero terminal width to survive Bubble Tea truncation, width=%d token=%q", width, token2)
	}
}

func TestMainScreenOutputController_SequentialTokensConsumeMatchingFrames(t *testing.T) {
	c := NewMainScreenOutputController(MainScreenResetModeFull)
	token1 := c.StageFrame(MainScreenFrameInput{Frame: "ONE", Width: 80, Height: 24, ViewportHeight: 10, ResizeEpoch: 1})
	token2 := c.StageFrame(MainScreenFrameInput{Frame: "TWO", Width: 80, Height: 24, ViewportHeight: 10, ResizeEpoch: 1})
	var b bytes.Buffer
	w := c.WrapOutput(&b)

	if _, err := w.Write([]byte("prefix" + token1 + "suffix")); err != nil {
		t.Fatalf("write token1 failed: %v", err)
	}
	if got := b.String(); got != "ONE\x1b[K\x1b[80D" {
		t.Fatalf("token1 should render frame ONE, got %q", got)
	}

	b.Reset()
	if _, err := w.Write([]byte("x" + token2 + "y")); err != nil {
		t.Fatalf("write token2 failed: %v", err)
	}
	if got := b.String(); got != "TWO\x1b[K\x1b[80D" {
		t.Fatalf("token2 should render frame TWO, got %q", got)
	}
}

func TestMainScreenOutputController_OldTokenCannotConsumeLatestFrame(t *testing.T) {
	c := NewMainScreenOutputController(MainScreenResetModeFull)
	oldToken := c.StageFrame(MainScreenFrameInput{Frame: "OLD", Width: 80, Height: 24, ViewportHeight: 10, ResizeEpoch: 1})
	newToken := c.StageFrame(MainScreenFrameInput{Frame: "NEW", Width: 80, Height: 24, ViewportHeight: 10, ResizeEpoch: 1})
	var b bytes.Buffer
	w := c.WrapOutput(&b)

	if _, err := w.Write([]byte(oldToken)); err != nil {
		t.Fatalf("write old token failed: %v", err)
	}
	if got := b.String(); got != "OLD\x1b[K\x1b[80D" {
		t.Fatalf("old token consumed wrong frame: got %q", got)
	}

	b.Reset()
	if _, err := w.Write([]byte(newToken)); err != nil {
		t.Fatalf("write new token failed: %v", err)
	}
	if got := b.String(); got != "NEW\x1b[K\x1b[80D" {
		t.Fatalf("new token consumed wrong frame: got %q", got)
	}
}

func TestMainScreenOutputController_SuppressesWriteWhenStagedFrameMissing(t *testing.T) {
	c := NewMainScreenOutputController(MainScreenResetModeFull)
	token := c.StageFrame(MainScreenFrameInput{Frame: "FRAME", Width: 80, Height: 24, ViewportHeight: 10, ResizeEpoch: 1})
	seq, ok := parseMainScreenFrameToken(token)
	if !ok {
		t.Fatalf("expected parseable token %q", token)
	}
	var b bytes.Buffer
	w := c.WrapOutput(&b)
	_, _ = w.Write([]byte(token)) // consume once

	b.Reset()
	missingToken := mainScreenFrameToken(seq + 999)
	payload := "a\x1b[2C" + missingToken + "b"
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Fatalf("write missing token failed: %v", err)
	}
	if got := b.String(); got != "" {
		t.Fatalf("stale token frame write should be suppressed, got %q", got)
	}
	if strings.Contains(b.String(), "OS_MAIN_FRAME") {
		t.Fatalf("token marker leaked: %q", b.String())
	}
}

func TestMainScreenOutputController_PassthroughWithoutToken(t *testing.T) {
	c := NewMainScreenOutputController(MainScreenResetModeFull)
	var b bytes.Buffer
	w := c.WrapOutput(&b)
	_, err := w.Write([]byte("raw"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if got := b.String(); got != "raw" {
		t.Fatalf("unexpected passthrough output: %q", got)
	}
}

func TestMainScreenOutputController_ConsecutiveStageFrameOutputsDifferForBubbleTea(t *testing.T) {
	c := NewMainScreenOutputController(MainScreenResetModeFull)
	// Regression: distinct View strings prevent Bubble Tea lastRender short-circuit.
	view1 := c.StageFrame(MainScreenFrameInput{Frame: "A", Width: 80, Height: 24, ViewportHeight: 10, ResizeEpoch: 1})
	view2 := c.StageFrame(MainScreenFrameInput{Frame: "B", Width: 80, Height: 24, ViewportHeight: 10, ResizeEpoch: 1})
	if view1 == view2 {
		t.Fatalf("consecutive StageFrame outputs must differ: %q", view1)
	}
}

func TestMainScreenOutputController_FrameTokenSurvivesNarrowBubbleTeaTruncation(t *testing.T) {
	c := NewMainScreenOutputController(MainScreenResetModeFull)
	token := c.StageFrame(MainScreenFrameInput{Frame: "NARROW", Width: 1, Height: 24, ViewportHeight: 10, ResizeEpoch: 1})
	truncated := xansi.Truncate(token, 1, "")
	if truncated != token {
		t.Fatalf("zero-width token should not be truncated by Bubble Tea width guard: got %q want %q", truncated, token)
	}

	var b bytes.Buffer
	w := c.WrapOutput(&b)
	if _, err := w.Write([]byte(truncated + xansi.EraseLineRight + xansi.CursorBackward(1))); err != nil {
		t.Fatalf("write truncated token failed: %v", err)
	}
	if got := b.String(); got != "NARROW\x1b[D" {
		t.Fatalf("narrow token should render staged frame, got %q", got)
	}
}

type fakeFileLikeOutput struct {
	bytes.Buffer
	closed bool
}

func (f *fakeFileLikeOutput) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (f *fakeFileLikeOutput) Close() error {
	f.closed = true
	return nil
}

func (f *fakeFileLikeOutput) Fd() uintptr {
	return 42
}

func TestMainScreenOutputController_PreservesFileLikeOutput(t *testing.T) {
	c := NewMainScreenOutputController(MainScreenResetModeFull)
	dst := &fakeFileLikeOutput{}
	w := c.WrapOutput(dst)
	fileLike, ok := w.(interface {
		io.ReadWriteCloser
		Fd() uintptr
	})
	if !ok {
		t.Fatalf("wrapped output should preserve file-like interface, got %T", w)
	}
	if fileLike.Fd() != 42 {
		t.Fatalf("Fd() = %d, want 42", fileLike.Fd())
	}
	if err := fileLike.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !dst.closed {
		t.Fatal("close should forward to destination")
	}
}
