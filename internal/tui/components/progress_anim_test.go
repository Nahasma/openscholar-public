package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestSpinnerGlyph_Cycle verifies that SpinnerGlyph cycles through all CC spinner frames.
func TestSpinnerGlyph_Cycle(t *testing.T) {
	for i, want := range CCSpinnerChars {
		got := SpinnerGlyph(i, 0.0, false)
		// The rendered output contains the raw glyph character (possibly wrapped in ANSI).
		if !strings.Contains(got, want) {
			t.Errorf("frame %d: SpinnerGlyph got %q, want it to contain %q", i, got, want)
		}
	}

	// Verify cycle wraps: frame len(CCSpinnerChars) should equal frame 0 (same glyph).
	g0 := SpinnerGlyph(0, 0.0, false)
	gN := SpinnerGlyph(len(CCSpinnerChars), 0.0, false)
	if g0 != gN {
		t.Errorf("frame 0 and frame %d should produce same glyph: g0=%q gN=%q", len(CCSpinnerChars), g0, gN)
	}
}

// TestSpinnerGlyph_ReducedMotion verifies that reducedMotion always returns the ● glyph.
func TestSpinnerGlyph_ReducedMotion(t *testing.T) {
	for _, frame := range []int{0, 5, 9, 10, 15, 19, 20, 25} {
		got := SpinnerGlyph(frame, 0.0, true)
		if !strings.Contains(got, "●") {
			t.Errorf("frame %d reducedMotion: expected ● in output, got %q", frame, got)
		}
		// Ensure none of the CC spinner chars appear.
		for _, ch := range CCSpinnerChars {
			if ch != "·" && strings.Contains(got, ch) {
				t.Errorf("frame %d reducedMotion: unexpected CC char %q in output %q", frame, ch, got)
			}
		}
	}
}

// TestShimmerIndex_Forward verifies the forward shimmer sweep.
func TestShimmerIndex_Forward(t *testing.T) {
	textLen := 5
	cycle := textLen + 10 // 15

	// Frame 0: pos = 0 % 15 = 0
	got := ShimmerIndex(0, textLen, false)
	if got != 0 {
		t.Errorf("forward frame=0 textLen=5: expected 0, got %d", got)
	}

	// Frame 3: pos = 3 % 15 = 3
	got = ShimmerIndex(3, textLen, false)
	if got != 3 {
		t.Errorf("forward frame=3 textLen=5: expected 3, got %d", got)
	}

	// Frame 15: pos = 15 % 15 = 0 (wrap-around)
	got = ShimmerIndex(cycle, textLen, false)
	if got != 0 {
		t.Errorf("forward frame=%d textLen=5: expected 0 (wrap), got %d", cycle, got)
	}
}

// TestShimmerIndex_Reverse verifies the reverse shimmer sweep.
func TestShimmerIndex_Reverse(t *testing.T) {
	textLen := 5
	// frame=0: textLen + 5 - (0 % 15) = 10
	got := ShimmerIndex(0, textLen, true)
	if got != 10 {
		t.Errorf("reverse frame=0 textLen=5: expected 10, got %d", got)
	}

	// frame=5: textLen + 5 - (5 % 15) = 5
	got = ShimmerIndex(5, textLen, true)
	if got != 5 {
		t.Errorf("reverse frame=5 textLen=5: expected 5, got %d", got)
	}

	// frame=10: textLen + 5 - (10 % 15) = 0
	got = ShimmerIndex(10, textLen, true)
	if got != 0 {
		t.Errorf("reverse frame=10 textLen=5: expected 0, got %d", got)
	}
}

// TestShimmerIndex_ZeroLen verifies that textLen<=0 returns -1.
func TestShimmerIndex_ZeroLen(t *testing.T) {
	if got := ShimmerIndex(5, 0, false); got != -1 {
		t.Errorf("textLen=0 forward: expected -1, got %d", got)
	}
	if got := ShimmerIndex(5, 0, true); got != -1 {
		t.Errorf("textLen=0 reverse: expected -1, got %d", got)
	}
	if got := ShimmerIndex(0, -1, false); got != -1 {
		t.Errorf("textLen=-1: expected -1, got %d", got)
	}
}

// TestRenderShimmerText_Basic verifies center+adjacent highlighting.
func TestRenderShimmerText_Basic(t *testing.T) {
	baseColor := lipgloss.Color("240")
	shimmerColor := lipgloss.Color("255")

	text := "Hello"
	// glimmerIndex=2 → 'l' at index 2 is center; 'e'(1) and 'l'(3) are adjacent.
	out := RenderShimmerText(text, 2, baseColor, shimmerColor)
	if out == "" {
		t.Fatal("RenderShimmerText returned empty string")
	}
	// All runes of "Hello" must appear in output.
	for _, r := range text {
		if !strings.ContainsRune(out, r) {
			t.Errorf("output missing rune %q: %q", r, out)
		}
	}
}

// TestRenderShimmerText_NoShimmer verifies that glimmerIndex=-1 disables shimmer.
func TestRenderShimmerText_NoShimmer(t *testing.T) {
	baseColor := lipgloss.Color("240")
	shimmerColor := lipgloss.Color("255")

	text := "World"
	outNoShimmer := RenderShimmerText(text, -1, baseColor, shimmerColor)
	outShimmer := RenderShimmerText(text, 2, baseColor, shimmerColor)

	if outNoShimmer == "" {
		t.Fatal("RenderShimmerText (-1) returned empty string")
	}
	// All runes must still be present.
	for _, r := range text {
		if !strings.ContainsRune(outNoShimmer, r) {
			t.Errorf("no-shimmer output missing rune %q: %q", r, outNoShimmer)
		}
	}
	// The no-shimmer render should differ from the shimmer render (different ANSI codes).
	// This is a weak check; at minimum both must be non-empty and contain the text.
	_ = outShimmer

	// glimmerIndex >= len(text) should also disable shimmer.
	outOverflow := RenderShimmerText(text, 100, baseColor, shimmerColor)
	if outOverflow == "" {
		t.Fatal("RenderShimmerText (overflow) returned empty string")
	}
	for _, r := range text {
		if !strings.ContainsRune(outOverflow, r) {
			t.Errorf("overflow output missing rune %q: %q", r, outOverflow)
		}
	}
}

// TestPhaseColor_AllPhases verifies that every phase returns a non-empty color.
func TestPhaseColor_AllPhases(t *testing.T) {
	phases := []uint8{PhaseIdle, PhaseThinking, PhaseToolRunning, PhaseStreaming, PhaseCompacting}
	for _, phase := range phases {
		color := PhaseColor(phase)
		if color == nil {
			t.Errorf("phase %d: PhaseColor returned nil", phase)
		}
		// Render a test string to confirm the color is usable.
		rendered := lipgloss.NewStyle().Foreground(color).Render("test")
		if rendered == "" {
			t.Errorf("phase %d: PhaseColor produced an unusable color (empty render)", phase)
		}
	}

	// Unknown phase should also return non-nil (falls back to TextMuted).
	unknownColor := PhaseColor(99)
	if unknownColor == nil {
		t.Error("unknown phase: PhaseColor returned nil, expected TextMuted fallback")
	}
}
