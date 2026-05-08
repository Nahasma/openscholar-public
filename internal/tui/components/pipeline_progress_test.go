package components

import (
	"strings"
	"testing"
)

func TestRenderPipelineProgress_Wide(t *testing.T) {
	data := PipelineProgressData{
		Phases: []PhaseProgressItem{
			{Name: "Literature", Status: PhaseProgressCompleted},
			{Name: "Design", Status: PhaseProgressCompleted},
			{Name: "Experiment", Status: PhaseProgressRunning},
			{Name: "Writing", Status: PhaseProgressPending},
			{Name: "Review", Status: PhaseProgressPending},
		},
		CurrentPhase: 2,
		CurrentLabel: "Experiment",
		WorkerStatus: "running code generation",
	}
	result := RenderPipelineProgress(data, 100)
	// Should have two lines
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines for wide terminal, got %d: %q", len(lines), result)
	}
}

func TestRenderPipelineProgress_Medium(t *testing.T) {
	data := PipelineProgressData{
		Phases: []PhaseProgressItem{
			{Name: "Literature", Status: PhaseProgressCompleted},
			{Name: "Design", Status: PhaseProgressRunning},
			{Name: "Experiment", Status: PhaseProgressPending},
		},
		CurrentPhase: 1,
		CurrentLabel: "Design",
	}
	result := RenderPipelineProgress(data, 70)
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line for medium terminal, got %d: %q", len(lines), result)
	}
}

func TestRenderPipelineProgress_Narrow(t *testing.T) {
	data := PipelineProgressData{
		Phases: []PhaseProgressItem{
			{Name: "Literature", Status: PhaseProgressCompleted},
			{Name: "Design", Status: PhaseProgressRunning},
		},
		CurrentPhase: 1,
		CurrentLabel: "Design",
	}
	result := RenderPipelineProgress(data, 50)
	if !strings.Contains(result, "Phase 2/2") {
		t.Errorf("narrow terminal should show phase count: %q", result)
	}
}

func TestRenderPipelineProgress_Empty(t *testing.T) {
	result := RenderPipelineProgress(PipelineProgressData{}, 100)
	if result != "" {
		t.Errorf("empty phases should return empty string, got: %q", result)
	}
}
