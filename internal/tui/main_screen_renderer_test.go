package tui

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClearTerminalSequence_DefaultFull(t *testing.T) {
	got := ClearTerminalSequence(MainScreenResetModeFull)
	if !strings.Contains(got, "\x1b[3J") {
		t.Fatalf("full mode must include ESC[3J, got %q", got)
	}
}

func TestClearTerminalSequence_Visible(t *testing.T) {
	got := ClearTerminalSequence(MainScreenResetModeVisible)
	if strings.Contains(got, "\x1b[3J") {
		t.Fatalf("visible mode must not include ESC[3J, got %q", got)
	}
}

func TestMainScreenRenderer_FullResetOnWidthChange(t *testing.T) {
	r := NewMainScreenRenderer(MainScreenResetModeFull)
	_ = r.Render(MainScreenFrameInput{Frame: "a", Width: 80, Height: 24, ViewportHeight: 10, ResizeEpoch: 1})
	out := r.Render(MainScreenFrameInput{Frame: "b", Width: 79, Height: 24, ViewportHeight: 10, ResizeEpoch: 2})
	if !out.FullReset || out.ResetReason != "resize-width" {
		t.Fatalf("expected resize-width full reset, got full=%v reason=%q", out.FullReset, out.ResetReason)
	}
	if !strings.Contains(out.Output, "\x1b[3J") {
		t.Fatalf("full reset output must clear scrollback, got %q", out.Output)
	}
}

func TestMainScreenRenderer_SameSizeRepaintDoesNotClearScrollback(t *testing.T) {
	r := NewMainScreenRenderer(MainScreenResetModeFull)
	_ = r.Render(MainScreenFrameInput{Frame: "a\nb", Width: 80, Height: 24, ResizeEpoch: 1})
	out := r.Render(MainScreenFrameInput{Frame: "a\nc", Width: 80, Height: 24, ResizeEpoch: 1})
	if out.FullReset {
		t.Fatalf("same-size repaint should not full reset: %+v", out)
	}
	if strings.Contains(out.Output, "\x1b[3J") || strings.Contains(out.Output, "\x1b[2J") {
		t.Fatalf("same-size repaint should not clear terminal, got %q", out.Output)
	}
	if !strings.Contains(out.Output, "\x1b[A") {
		t.Fatalf("same-size repaint should move back to previous frame top, got %q", out.Output)
	}
}

func TestMainScreenRenderer_OffscreenDiffForcesFullReset(t *testing.T) {
	r := NewMainScreenRenderer(MainScreenResetModeFull)
	_ = r.Render(MainScreenFrameInput{Frame: "a\nb\nc\nd\ne", Width: 80, Height: 3, ResizeEpoch: 1})
	out := r.Render(MainScreenFrameInput{Frame: "changed\nb\nc\nd\ne", Width: 80, Height: 3, ResizeEpoch: 1})
	if !out.FullReset || out.ResetReason != "offscreen" {
		t.Fatalf("offscreen diff should full reset, got full=%v reason=%q", out.FullReset, out.ResetReason)
	}
}

func TestMainScreenRenderer_ExactViewportTopRowDiffDoesNotClearScrollback(t *testing.T) {
	r := NewMainScreenRenderer(MainScreenResetModeFull)
	_ = r.Render(MainScreenFrameInput{Frame: "a\nb\nc", Width: 80, Height: 3, ResizeEpoch: 1})
	out := r.Render(MainScreenFrameInput{Frame: "changed\nb\nc", Width: 80, Height: 3, ResizeEpoch: 1})
	if out.FullReset {
		t.Fatalf("exact-height same-size repaint should not full reset, got reason=%q", out.ResetReason)
	}
	if strings.Contains(out.Output, "\x1b[3J") {
		t.Fatalf("ordinary exact-height repaint must not clear scrollback, got %q", out.Output)
	}
}

func TestMainScreenRenderer_ReturnMainFirstFrameFullResets(t *testing.T) {
	r := NewMainScreenRenderer(MainScreenResetModeFull)
	out := r.Render(MainScreenFrameInput{Frame: "main", Width: 80, Height: 24, ResizeEpoch: 1, Reason: "return-main"})
	if !out.FullReset || out.ResetReason != "return-main" {
		t.Fatalf("return-main first frame should full reset, got full=%v reason=%q", out.FullReset, out.ResetReason)
	}
	if !strings.Contains(out.Output, "\x1b[3J") {
		t.Fatalf("return-main full reset should clear scrollback in full mode, got %q", out.Output)
	}
}

func TestMainScreenRenderer_VisibleResetModeOmitsScrollbackClear(t *testing.T) {
	r := NewMainScreenRenderer(MainScreenResetModeVisible)
	_ = r.Render(MainScreenFrameInput{Frame: "a", Width: 80, Height: 24, ResizeEpoch: 1})
	out := r.Render(MainScreenFrameInput{Frame: "b", Width: 79, Height: 24, ResizeEpoch: 2})
	if !out.FullReset {
		t.Fatal("width change should full reset")
	}
	if strings.Contains(out.Output, "\x1b[3J") {
		t.Fatalf("visible mode must not clear scrollback, got %q", out.Output)
	}
}

func TestMainScreenRenderer_RepaintCleanupPreservesLastLiveRow(t *testing.T) {
	r := NewMainScreenRenderer(MainScreenResetModeVisible)
	term := newMainScreenTestTerminal(12, 8)

	first := r.Render(MainScreenFrameInput{Frame: "old-a\nold-b\nold-c\nold-d\nold-e", Width: 12, Height: 24, ResizeEpoch: 1})
	term.Write(first.Output)

	out := r.Render(MainScreenFrameInput{
		Frame:       "new-a\nnew-b",
		Width:       12,
		Height:      24,
		ResizeEpoch: 1,
	})
	if !strings.Contains(out.Output, clearMainScreenRowsBelowPaintedFrame()) {
		t.Fatalf("expected residual-row cleanup sequence, got %q", out.Output)
	}
	term.Write(out.Output)

	lines := term.VisibleLines()
	if lines[0] != "new-a" || lines[1] != "new-b" {
		t.Fatalf("unexpected visible rows after repaint: %#v", lines[:2])
	}
	if visible := strings.Join(lines, "\n"); strings.Contains(visible, "old-d") || strings.Contains(visible, "old-e") {
		t.Fatalf("residual old frame rows should be cleared, got %#v", lines)
	}
}

func TestMainScreenRenderer_ResetThenFrame(t *testing.T) {
	r := NewMainScreenRenderer(MainScreenResetModeVisible)
	_ = r.Render(MainScreenFrameInput{Frame: "old", Width: 80, Height: 24, ResizeEpoch: 1})
	out := r.Render(MainScreenFrameInput{
		Frame:       "new",
		Width:       79,
		Height:      24,
		ResizeEpoch: 2,
	})
	if !out.FullReset || out.ResetReason != "resize-width" {
		t.Fatalf("expected width reset, got full=%v reason=%q", out.FullReset, out.ResetReason)
	}
	wantPrefix := ClearTerminalSequence(MainScreenResetModeVisible)
	if !strings.HasPrefix(out.Output, wantPrefix) {
		t.Fatalf("reset output should start clear sequence, got %q", out.Output)
	}
	if strings.Contains(out.Output, "\x1b[3J") {
		t.Fatalf("visible reset mode should not include scrollback clear, got %q", out.Output)
	}
}

func TestMainScreenRenderer_StageFrameWrapOutput_NoFakeDuplicateRowsForCJKLongThenShortReply(t *testing.T) {
	controller := NewMainScreenOutputController(MainScreenResetModeVisible)
	term := newMainScreenTestTerminal(40, 8)
	writer := controller.WrapOutput(&strings.Builder{})

	longReplyLines := []string{
		"## 调试结论",
		"- arXiv API 连接超时",
		"- 如果想更深入覆盖，可以加超时重试",
		"- 再加一个 429 回退测试",
	}
	shortReply := "好的，用 WebSearch 来搜。"
	messages := []string{
		strings.Join(longReplyLines, "\n"),
		shortReply,
	}
	renderedContent := strings.Join(append([]string{
		"用户：请排查重复行",
		"助手：",
	}, longReplyLines...), "\n")

	if got := strings.Count(strings.Join(messages, "\n"), "arXiv API 连接超时"); got != 1 {
		t.Fatalf("messages layer duplicate arXiv line: %d", got)
	}
	if got := strings.Count(renderedContent, "如果想更深入覆盖"); got != 1 {
		t.Fatalf("renderedContent layer duplicate deep-coverage line: %d", got)
	}

	frames := []MainScreenFrameInput{
		{
			Frame: strings.Join([]string{
				"用户：请排查重复行",
				"助手：",
				"## 调试结论",
				"- arXiv API 连接超时",
				"- 如果想更深入覆盖，可以加超时重试",
				"状态：处理中",
				"进度：72%",
			}, "\n"),
			Width:          40,
			Height:         8,
			ViewportHeight: 8,
			ResizeEpoch:    1,
		},
		{
			Frame: strings.Join([]string{
				"用户：请排查重复行",
				"助手：",
				"## 调试结论",
				"- arXiv API 连接超时",
				"- 如果想更深入覆盖，可以加超时重试",
				"- 再加一个 429 回退测试",
				"- 追加一条边界行",
				"状态：处理中",
				"进度：98%",
			}, "\n"),
			Width:          40,
			Height:         8,
			ViewportHeight: 8,
			ResizeEpoch:    1,
		},
		{
			Frame: strings.Join([]string{
				"用户：请排查重复行",
				"助手：",
				"## 调试结论",
				"- arXiv API 连接超时",
				"- 如果想更深入覆盖，可以加超时重试",
				"- 再加一个 429 回退测试",
				shortReply,
				"状态：空闲",
			}, "\n"),
			Width:          40,
			Height:         8,
			ViewportHeight: 8,
			ResizeEpoch:    1,
		},
	}

	for i, frame := range frames {
		token := controller.StageFrame(frame)
		var out strings.Builder
		writer = controller.WrapOutput(&out)
		if _, err := writer.Write([]byte(token)); err != nil {
			t.Fatalf("frame %d write failed: %v", i, err)
		}
		term.Write(out.String())
	}

	visible := strings.Join(term.VisibleLines(), "\n")
	if got := strings.Count(visible, "arXiv API 连接超时"); got != 1 {
		t.Fatalf("terminal surface should contain arXiv line once, got %d; surface=%q", got, visible)
	}
	if got := strings.Count(visible, "如果想更深入覆盖"); got != 1 {
		t.Fatalf("terminal surface should contain deep-coverage line once, got %d; surface=%q", got, visible)
	}
	if got := strings.Count(visible, shortReply); got != 1 {
		t.Fatalf("terminal surface should contain short reply once, got %d; surface=%q", got, visible)
	}
}

func TestMainScreenRenderer_StageFrameWrapOutput_UsesNextVisibleWindowOnViewportBoundaryCrossing(t *testing.T) {
	controller := NewMainScreenOutputController(MainScreenResetModeVisible)
	term := newMainScreenTestTerminal(32, 4)

	frames := []MainScreenFrameInput{
		{
			Frame: strings.Join([]string{
				"R1: header",
				"R2: body",
				"R3: keep",
				"R4: tail",
			}, "\n"),
			Width:          32,
			Height:         4,
			ViewportHeight: 4,
			ResizeEpoch:    1,
		},
		{
			Frame: strings.Join([]string{
				"R1: header",
				"R2: body",
				"R3: keep",
				"R4: tail",
				"R5: newest",
			}, "\n"),
			Width:          32,
			Height:         4,
			ViewportHeight: 4,
			ResizeEpoch:    1,
		},
	}

	for i, frame := range frames {
		token := controller.StageFrame(frame)
		var out strings.Builder
		writer := controller.WrapOutput(&out)
		if _, err := writer.Write([]byte(token)); err != nil {
			t.Fatalf("frame %d write failed: %v", i, err)
		}
		term.Write(out.String())
	}

	visible := strings.Join(term.VisibleLines(), "\n")
	if strings.Contains(visible, "R1: header") {
		t.Fatalf("old top line should move into scrollback after crossing viewport boundary, surface=%q", visible)
	}
	for _, line := range []string{"R2: body", "R3: keep", "R4: tail", "R5: newest"} {
		if got := strings.Count(visible, line); got != 1 {
			t.Fatalf("line %q should appear once in visible window, got %d; surface=%q", line, got, visible)
		}
	}

	scrollback := strings.Join(term.ScrollbackLines(), "\n")
	if got := strings.Count(scrollback, "R1: header"); got != 1 {
		t.Fatalf("old top line should be committed to native scrollback once, got %d; scrollback=%q", got, scrollback)
	}
	if got := strings.Count(scrollback, "R2: body"); got != 0 {
		t.Fatalf("next visible top line should not be prematurely committed to scrollback, got %d; scrollback=%q", got, scrollback)
	}
}

func TestMainScreenRenderer_StageFrameWrapOutput_NoDuplicateRowsWhenStatusBottomShrinks(t *testing.T) {
	controller := NewMainScreenOutputController(MainScreenResetModeVisible)
	term := newMainScreenTestTerminal(40, 6)

	frames := []MainScreenFrameInput{
		{
			Frame: strings.Join([]string{
				"用户：请继续",
				"助手：处理中",
				"- 方案 A",
				"- 方案 B",
				"状态：处理中",
				"进度：91%",
			}, "\n"),
			Width:          40,
			Height:         6,
			ViewportHeight: 6,
			ResizeEpoch:    1,
		},
		{
			Frame: strings.Join([]string{
				"用户：请继续",
				"助手：处理中",
				"- 方案 A",
				"- 方案 B",
				"- 新增短回复：好的，用 WebSearch 来搜。",
				"状态：空闲",
			}, "\n"),
			Width:          40,
			Height:         6,
			ViewportHeight: 6,
			ResizeEpoch:    1,
		},
	}

	for i, frame := range frames {
		token := controller.StageFrame(frame)
		var out strings.Builder
		writer := controller.WrapOutput(&out)
		if _, err := writer.Write([]byte(token)); err != nil {
			t.Fatalf("frame %d write failed: %v", i, err)
		}
		term.Write(out.String())
	}

	visible := strings.Join(term.VisibleLines(), "\n")
	if strings.Contains(visible, "进度：91%") {
		t.Fatalf("progress row should be removed after status/bottom shrink, surface=%q", visible)
	}
	for _, line := range []string{"- 方案 A", "- 方案 B", "- 新增短回复：好的，用 WebSearch 来搜。", "状态：空闲"} {
		if got := strings.Count(visible, line); got != 1 {
			t.Fatalf("line %q should appear once after shrink repaint, got %d; surface=%q", line, got, visible)
		}
	}
}

type mainScreenTestTerminal struct {
	width  int
	height int
	row    int
	col    int
	cells  [][]rune
	scroll []string
}

func newMainScreenTestTerminal(width, height int) *mainScreenTestTerminal {
	t := &mainScreenTestTerminal{
		width:  width,
		height: height,
		cells:  make([][]rune, height),
	}
	for i := range t.cells {
		t.cells[i] = make([]rune, width)
		for j := range t.cells[i] {
			t.cells[i][j] = ' '
		}
	}
	return t
}

func (t *mainScreenTestTerminal) Write(s string) {
	for i := 0; i < len(s); {
		switch s[i] {
		case '\r':
			t.col = 0
			i++
		case '\n':
			t.lineFeed()
			i++
		case '\x1b':
			next, ok := t.consumeCSI(s, i)
			if !ok {
				i++
				continue
			}
			i = next
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				i++
				continue
			}
			t.put(r)
			i += size
		}
	}
}

func (t *mainScreenTestTerminal) VisibleLines() []string {
	lines := make([]string, len(t.cells))
	for i, row := range t.cells {
		lines[i] = strings.TrimRight(string(row), " ")
	}
	return lines
}

func (t *mainScreenTestTerminal) ScrollbackLines() []string {
	return append([]string(nil), t.scroll...)
}

func (t *mainScreenTestTerminal) put(ch rune) {
	if t.row < 0 || t.row >= t.height || t.col < 0 || t.col >= t.width {
		return
	}
	t.cells[t.row][t.col] = ch
	if t.col < t.width-1 {
		t.col++
	}
}

func (t *mainScreenTestTerminal) lineFeed() {
	t.row++
	if t.row < t.height {
		return
	}
	t.scroll = append(t.scroll, strings.TrimRight(string(t.cells[0]), " "))
	copy(t.cells, t.cells[1:])
	t.cells[t.height-1] = make([]rune, t.width)
	for i := range t.cells[t.height-1] {
		t.cells[t.height-1][i] = ' '
	}
	t.row = t.height - 1
}

func (t *mainScreenTestTerminal) consumeCSI(s string, start int) (int, bool) {
	if start+2 >= len(s) || s[start+1] != '[' {
		return start, false
	}
	end := start + 2
	for end < len(s) && (s[end] < '@' || s[end] > '~') {
		end++
	}
	if end >= len(s) {
		return len(s), true
	}
	params := s[start+2 : end]
	switch s[end] {
	case 'A':
		t.row = max(0, t.row-csiFirstParam(params, 1))
	case 'B':
		t.row = min(t.height-1, t.row+csiFirstParam(params, 1))
	case 'D':
		t.col = max(0, t.col-csiFirstParam(params, 1))
	case 'H':
		t.row, t.col = 0, 0
	case 'J':
		n := csiFirstParam(params, 0)
		if n == 0 {
			t.eraseScreenBelow()
		} else if n == 2 {
			t.clear()
		} else if n == 3 {
			t.scroll = nil
		}
	case 'K':
		t.eraseLineRight()
	}
	return end + 1, true
}

func csiFirstParam(params string, defaultValue int) int {
	if params == "" {
		return defaultValue
	}
	first := params
	if idx := strings.IndexByte(params, ';'); idx >= 0 {
		first = params[:idx]
	}
	n, err := strconv.Atoi(first)
	if err != nil || n < 0 {
		return defaultValue
	}
	return n
}

func (t *mainScreenTestTerminal) eraseLineRight() {
	for col := t.col; col < t.width; col++ {
		t.cells[t.row][col] = ' '
	}
}

func (t *mainScreenTestTerminal) eraseScreenBelow() {
	t.eraseLineRight()
	for row := t.row + 1; row < t.height; row++ {
		for col := 0; col < t.width; col++ {
			t.cells[row][col] = ' '
		}
	}
}

func (t *mainScreenTestTerminal) clear() {
	for row := 0; row < t.height; row++ {
		for col := 0; col < t.width; col++ {
			t.cells[row][col] = ' '
		}
	}
}
