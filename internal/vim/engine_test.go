package vim

import (
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newEnabled() *Engine {
	e := NewEngine()
	e.SetEnabled(true)
	return e
}

// press feeds a sequence of runes into the engine and returns the last Result.
func press(e *Engine, keys ...rune) Result {
	var r Result
	for _, k := range keys {
		r = e.ProcessKey(k)
	}
	return r
}

// ── 1. TestEngine_Disabled ────────────────────────────────────────────────────

func TestEngine_Disabled(t *testing.T) {
	e := NewEngine()
	keys := []rune{'i', 'a', 'h', 'j', 'k', 'l', ':', 'd', 'w', 0x1b}
	for _, k := range keys {
		r := e.ProcessKey(k)
		if r.Handled {
			t.Errorf("disabled: key %q should not be handled, got Handled=true", k)
		}
		if r.NewMode != ModeDisabled {
			t.Errorf("disabled: key %q should keep ModeDisabled, got %v", k, r.NewMode)
		}
	}
}

// ── 2. TestEngine_NormalToInsert ──────────────────────────────────────────────

func TestEngine_NormalToInsert(t *testing.T) {
	e := newEnabled()
	r := e.ProcessKey('i')
	if r.NewMode != ModeInsert {
		t.Errorf("'i' should enter ModeInsert, got %v", r.NewMode)
	}
	if !r.Handled {
		t.Error("'i' should be Handled")
	}
	if e.Mode() != ModeInsert {
		t.Errorf("engine mode should be ModeInsert after 'i', got %v", e.Mode())
	}
}

// ── 3. TestEngine_InsertToNormal ──────────────────────────────────────────────

func TestEngine_InsertToNormal(t *testing.T) {
	e := newEnabled()
	press(e, 'i')
	r := e.ProcessKey(0x1b)
	if r.NewMode != ModeNormal {
		t.Errorf("Esc in Insert should return ModeNormal, got %v", r.NewMode)
	}
	if !r.Handled {
		t.Error("Esc in Insert should be Handled")
	}
}

// ── 4. TestEngine_HJKL ────────────────────────────────────────────────────────

func TestEngine_HJKL(t *testing.T) {
	tests := []struct {
		key    rune
		dx, dy int
	}{
		{'h', -1, 0},
		{'l', 1, 0},
		{'j', 0, 1},
		{'k', 0, -1},
	}
	for _, tt := range tests {
		e := newEnabled()
		r := e.ProcessKey(tt.key)
		if r.CursorMove == nil {
			t.Errorf("'%c': expected CursorMove, got nil", tt.key)
			continue
		}
		if r.CursorMove.DeltaX != tt.dx {
			t.Errorf("'%c': DeltaX want %d got %d", tt.key, tt.dx, r.CursorMove.DeltaX)
		}
		if r.CursorMove.DeltaY != tt.dy {
			t.Errorf("'%c': DeltaY want %d got %d", tt.key, tt.dy, r.CursorMove.DeltaY)
		}
	}
}

// ── 5. TestEngine_WordMotions ─────────────────────────────────────────────────

func TestEngine_WordMotions(t *testing.T) {
	tests := []struct {
		key rune
		dx  int
	}{
		{'w', 1},
		{'e', 1},
		{'b', -1},
	}
	for _, tt := range tests {
		e := newEnabled()
		r := e.ProcessKey(tt.key)
		if r.CursorMove == nil {
			t.Errorf("'%c': expected CursorMove, got nil", tt.key)
			continue
		}
		if r.CursorMove.DeltaX != tt.dx {
			t.Errorf("'%c': DeltaX want %d got %d", tt.key, tt.dx, r.CursorMove.DeltaX)
		}
	}
}

// ── 6. TestEngine_DeleteChar ──────────────────────────────────────────────────

func TestEngine_DeleteChar(t *testing.T) {
	e := newEnabled()
	r := e.ProcessKey('x')
	if r.TextChange == nil {
		t.Fatal("'x': expected TextChange, got nil")
	}
	if r.TextChange.DeleteCount != 1 {
		t.Errorf("'x': DeleteCount want 1 got %d", r.TextChange.DeleteCount)
	}
	if r.NewMode != ModeNormal {
		t.Errorf("'x': NewMode want ModeNormal got %v", r.NewMode)
	}
}

// ── 7. TestEngine_DeleteWord ──────────────────────────────────────────────────

func TestEngine_DeleteWord(t *testing.T) {
	e := newEnabled()
	// d then w
	r1 := e.ProcessKey('d')
	if r1.NewMode != ModeOperatorPending {
		t.Fatalf("'d': want ModeOperatorPending got %v", r1.NewMode)
	}
	r2 := e.ProcessKey('w')
	if r2.TextChange == nil {
		t.Fatal("'dw': expected TextChange, got nil")
	}
	if r2.NewMode != ModeNormal {
		t.Errorf("'dw': NewMode want ModeNormal got %v", r2.NewMode)
	}
}

// ── 8. TestEngine_DeleteLine ──────────────────────────────────────────────────

func TestEngine_DeleteLine(t *testing.T) {
	e := newEnabled()
	r := press(e, 'd', 'd')
	if r.TextChange == nil {
		t.Fatal("'dd': expected TextChange, got nil")
	}
	if !r.TextChange.DeleteLine {
		t.Error("'dd': DeleteLine should be true")
	}
	if r.NewMode != ModeNormal {
		t.Errorf("'dd': NewMode want ModeNormal got %v", r.NewMode)
	}
}

// ── 9. TestEngine_ChangeWord ──────────────────────────────────────────────────

func TestEngine_ChangeWord(t *testing.T) {
	e := newEnabled()
	r := press(e, 'c', 'w')
	if r.TextChange == nil {
		t.Fatal("'cw': expected TextChange, got nil")
	}
	if r.NewMode != ModeInsert {
		t.Errorf("'cw': NewMode want ModeInsert got %v", r.NewMode)
	}
}

// ── 10. TestEngine_CommandLine ────────────────────────────────────────────────

func TestEngine_CommandLine(t *testing.T) {
	e := newEnabled()
	// Enter command line
	r0 := e.ProcessKey(':')
	if r0.NewMode != ModeCommandLine {
		t.Fatalf("':': want ModeCommandLine got %v", r0.NewMode)
	}
	// Type "wq"
	e.ProcessKey('w')
	e.ProcessKey('q')
	// Press Enter
	r := e.ProcessKey('\r')
	if r.Command != "wq" {
		t.Errorf("':wq<Enter>': Command want \"wq\" got %q", r.Command)
	}
	if r.NewMode != ModeNormal {
		t.Errorf("after command: NewMode want ModeNormal got %v", r.NewMode)
	}
}

// ── 11. TestEngine_NumericPrefix ──────────────────────────────────────────────

func TestEngine_NumericPrefix(t *testing.T) {
	e := newEnabled()
	// 3j → down 3
	e.ProcessKey('3')
	r := e.ProcessKey('j')
	if r.CursorMove == nil {
		t.Fatal("'3j': expected CursorMove, got nil")
	}
	if r.CursorMove.DeltaY != 3 {
		t.Errorf("'3j': DeltaY want 3 got %d", r.CursorMove.DeltaY)
	}
}

// ── 12. TestEngine_DotRepeat ──────────────────────────────────────────────────

func TestEngine_DotRepeat(t *testing.T) {
	e := newEnabled()
	// Perform dw
	press(e, 'd', 'w')
	// Dot repeat
	r := e.ProcessKey('.')
	if !r.Handled {
		t.Error("'.': should be Handled after dw")
	}
	if r.TextChange == nil {
		t.Fatal("'.': expected TextChange after dw")
	}
}

// ── 13. TestEngine_SetEnabled ─────────────────────────────────────────────────

func TestEngine_SetEnabled(t *testing.T) {
	e := NewEngine()
	if e.Enabled() {
		t.Error("new engine should be disabled")
	}
	if e.Mode() != ModeDisabled {
		t.Errorf("new engine mode should be ModeDisabled, got %v", e.Mode())
	}

	e.SetEnabled(true)
	if !e.Enabled() {
		t.Error("after SetEnabled(true) should be enabled")
	}
	if e.Mode() != ModeNormal {
		t.Errorf("after SetEnabled(true) mode should be ModeNormal, got %v", e.Mode())
	}

	e.SetEnabled(false)
	if e.Enabled() {
		t.Error("after SetEnabled(false) should be disabled")
	}
	if e.Mode() != ModeDisabled {
		t.Errorf("after SetEnabled(false) mode should be ModeDisabled, got %v", e.Mode())
	}
}

// ── 14. TestEngine_VisualMode ─────────────────────────────────────────────────

func TestEngine_VisualMode(t *testing.T) {
	e := newEnabled()
	r := e.ProcessKey('v')
	if r.NewMode != ModeVisual {
		t.Errorf("'v': want ModeVisual got %v", r.NewMode)
	}
	r2 := e.ProcessKey(0x1b)
	if r2.NewMode != ModeNormal {
		t.Errorf("Esc in Visual: want ModeNormal got %v", r2.NewMode)
	}
}

// ── 15. TestEngine_EscCancelsOperator ─────────────────────────────────────────

func TestEngine_EscCancelsOperator(t *testing.T) {
	e := newEnabled()
	r1 := e.ProcessKey('d')
	if r1.NewMode != ModeOperatorPending {
		t.Fatalf("'d': want ModeOperatorPending got %v", r1.NewMode)
	}
	r2 := e.ProcessKey(0x1b)
	if r2.NewMode != ModeNormal {
		t.Errorf("Esc after 'd': want ModeNormal got %v", r2.NewMode)
	}
	if e.Mode() != ModeNormal {
		t.Errorf("engine mode should be ModeNormal after cancel, got %v", e.Mode())
	}
}

// ── extra: TestEngine_StartOfLine / EndOfLine ─────────────────────────────────

func TestEngine_LineMotions(t *testing.T) {
	e := newEnabled()
	r0 := e.ProcessKey('$')
	if r0.CursorMove == nil || !r0.CursorMove.ToEnd {
		t.Error("'$': want CursorMove.ToEnd=true")
	}

	r1 := e.ProcessKey('0')
	if r1.CursorMove == nil || !r1.CursorMove.ToStart {
		t.Error("'0' (no prefix): want CursorMove.ToStart=true")
	}
}

// ── extra: TestEngine_VisualLine ──────────────────────────────────────────────

func TestEngine_VisualLine(t *testing.T) {
	e := newEnabled()
	r := e.ProcessKey('V')
	if r.NewMode != ModeVisualLine {
		t.Errorf("'V': want ModeVisualLine got %v", r.NewMode)
	}
	r2 := e.ProcessKey(0x1b)
	if r2.NewMode != ModeNormal {
		t.Errorf("Esc in VisualLine: want ModeNormal got %v", r2.NewMode)
	}
}

// ── extra: TestParseTextObject ────────────────────────────────────────────────

func TestParseTextObject(t *testing.T) {
	tests := []struct {
		first, second rune
		wantOk        bool
		wantInner     bool
		wantKind      rune
	}{
		{'i', 'w', true, true, 'w'},
		{'a', 'w', true, false, 'w'},
		{'i', '"', false, false, 0},
		{'x', 'w', false, false, 0},
	}
	for _, tt := range tests {
		obj, ok := parseTextObject(tt.first, tt.second)
		if ok != tt.wantOk {
			t.Errorf("parseTextObject(%q,%q): ok want %v got %v", tt.first, tt.second, tt.wantOk, ok)
			continue
		}
		if ok {
			if obj.Inner != tt.wantInner {
				t.Errorf("parseTextObject(%q,%q): Inner want %v got %v", tt.first, tt.second, tt.wantInner, obj.Inner)
			}
			if obj.Kind != tt.wantKind {
				t.Errorf("parseTextObject(%q,%q): Kind want %q got %q", tt.first, tt.second, tt.wantKind, obj.Kind)
			}
		}
	}
}

// ── extra: TestEngine_Reset ───────────────────────────────────────────────────

func TestEngine_Reset(t *testing.T) {
	e := newEnabled()
	// Put into operator pending
	e.ProcessKey('d')
	if e.Mode() != ModeOperatorPending {
		t.Fatal("should be in OperatorPending")
	}
	e.Reset()
	if e.Mode() != ModeNormal {
		t.Errorf("after Reset: want ModeNormal got %v", e.Mode())
	}
}

// ── extra: TestEngine_ModeString ─────────────────────────────────────────────

func TestEngine_ModeString(t *testing.T) {
	cases := []struct {
		mode VimMode
		want string
	}{
		{ModeNormal, "NORMAL"},
		{ModeInsert, "INSERT"},
		{ModeVisual, "VISUAL"},
		{ModeVisualLine, "VISUAL LINE"},
		{ModeDisabled, "DISABLED"},
		{ModeCommandLine, "COMMAND"},
		{ModeOperatorPending, "OPERATOR PENDING"},
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.want {
			t.Errorf("mode %q String(): want %q got %q", c.mode, c.want, got)
		}
	}
}
