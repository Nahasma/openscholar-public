package components

import (
	"regexp"
	"strings"
	"testing"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRenderProgressLine_Idle(t *testing.T) {
	result := RenderProgressLine(PhaseIdle, 0, 0, "", "")
	if result != "" {
		t.Errorf("PhaseIdle should return empty, got %q", result)
	}
}

func TestRenderProgressLine_Streaming(t *testing.T) {
	// Wave 4: streaming phase now shows persistent progress rail
	result := RenderProgressLine(PhaseStreaming, 10, 5, "", "")
	if result == "" {
		t.Error("PhaseStreaming should return non-empty progress rail")
	}
	// Timer only shown when elapsed >= 30s
	if strings.Contains(result, "5s") {
		t.Errorf("PhaseStreaming should NOT contain elapsed time when elapsed < 30s, got %q", result)
	}

	// Timer shown when elapsed >= 30s
	result2 := RenderProgressLine(PhaseStreaming, 10, 30, "", "")
	if !strings.Contains(result2, "30s") {
		t.Errorf("PhaseStreaming should contain elapsed time when elapsed >= 30s, got %q", result2)
	}
}

func TestRenderProgressLine_ThinkingSilent(t *testing.T) {
	result := RenderProgressLine(PhaseThinking, 0, 0, "", "")
	if !strings.Contains(result, FigThinking) {
		t.Errorf("PhaseThinking (silent) should contain FigThinking, got %q", result)
	}
	if !strings.Contains(result, "Thinking") {
		t.Errorf("PhaseThinking (silent) should contain 'Thinking', got %q", result)
	}
}

func TestRenderProgressLine_ThinkingActive(t *testing.T) {
	result := RenderProgressLine(PhaseThinking, 8, 3, "", "")
	// Should contain a spinner character from FigSpinnerFrames
	if result == "" {
		t.Fatal("PhaseThinking (active) should not be empty")
	}
	// Timer only shown when elapsed >= 30s
	if strings.Contains(result, "3s") {
		t.Errorf("PhaseThinking (active) should NOT contain timer when elapsed < 30s, got %q", result)
	}

	// Timer shown when elapsed >= 30s
	result2 := RenderProgressLine(PhaseThinking, 8, 30, "", "")
	if !strings.Contains(result2, "30s") {
		t.Errorf("PhaseThinking (active) should contain elapsed '30s' when elapsed >= 30s, got %q", result2)
	}
}

func TestRenderProgressLine_ToolRunning(t *testing.T) {
	result := RenderProgressLine(PhaseToolRunning, 5, 2, "Read", "")
	plain := strings.ToLower(ansiRe.ReplaceAllString(result, ""))
	if !strings.Contains(plain, "running") {
		t.Errorf("PhaseToolRunning should contain turn-level running wording, got %q", result)
	}
}

func TestRenderProgressLine_Compacting(t *testing.T) {
	result := RenderProgressLine(PhaseCompacting, 0, 5, "", "")
	if result == "" {
		t.Fatal("PhaseCompacting should not be empty")
	}
}

func TestRenderToolDoneLine(t *testing.T) {
	result := RenderToolDoneLine("Read", "view.go", "247 lines", 0.3)
	if !strings.Contains(result, FigCheckmark) {
		t.Errorf("should contain checkmark, got %q", result)
	}
	if !strings.Contains(result, "Read") {
		t.Errorf("should contain tool name, got %q", result)
	}
}

func TestRenderToolFailedLine(t *testing.T) {
	result := RenderToolFailedLine("Read", "/nonexistent.go", "file not found")
	if !strings.Contains(result, FigCross) {
		t.Errorf("should contain cross, got %q", result)
	}
	if !strings.Contains(result, "file not found") {
		t.Errorf("should contain error message, got %q", result)
	}
}

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		seconds int
		want    string
	}{
		{0, "0s"},
		{3, "3s"},
		{59, "59s"},
		{60, "1m"},
		{72, "1m12s"},
	}
	for _, tt := range tests {
		got := formatElapsed(tt.seconds)
		if got != tt.want {
			t.Errorf("formatElapsed(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestExpandState_Constants(t *testing.T) {
	// Verify ExpandState values
	if ExpandCollapsed != 0 {
		t.Errorf("ExpandCollapsed should be 0, got %d", ExpandCollapsed)
	}
	if ExpandInline != 1 {
		t.Errorf("ExpandInline should be 1, got %d", ExpandInline)
	}
	if ExpandOverlay != 2 {
		t.Errorf("ExpandOverlay should be 2, got %d", ExpandOverlay)
	}
}

func TestBlockRenderContext_IsExpandedV2(t *testing.T) {
	// With ExpandNodes
	ctx := BlockRenderContext{
		ExpandNodes: map[string]ExpandState{
			"tool-1": ExpandInline,
			"tool-2": ExpandCollapsed,
		},
	}
	if ctx.IsExpandedV2("tool-1") != ExpandInline {
		t.Error("should return ExpandInline for tool-1")
	}
	if ctx.IsExpandedV2("tool-2") != ExpandCollapsed {
		t.Error("should return ExpandCollapsed for tool-2")
	}
	if ctx.IsExpandedV2("tool-3") != ExpandCollapsed {
		t.Error("should return ExpandCollapsed for missing key")
	}

	// Fallback to legacy Expanded map
	ctxLegacy := BlockRenderContext{
		Expanded: map[string]bool{
			"tool-1": true,
			"tool-2": false,
		},
	}
	if ctxLegacy.IsExpandedV2("tool-1") != ExpandInline {
		t.Error("legacy: should return ExpandInline for true")
	}
	if ctxLegacy.IsExpandedV2("tool-2") != ExpandCollapsed {
		t.Error("legacy: should return ExpandCollapsed for false")
	}
}
