package agent

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/message"
)

func TestBuildToolOutcomes_ParsesDurableProgressAndArtifactPaths(t *testing.T) {
	assistant := message.Message{
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "DiagramGen", Input: `{"code":"x"}`},
		},
	}
	toolMsg := message.Message{
		Parts: []message.ContentPart{
			message.ToolResult{
				ToolCallID: "tc1",
				Name:       "DiagramGen",
				Content:    "ok",
				Metadata:   `{"durable_progress":true,"svg_path":"a.svg","png_path":"a.png","artifact_paths":["b.d2"]}`,
			},
		},
	}
	outcomes := buildToolOutcomes(assistant, toolMsg)
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if !outcomes[0].DurableProgress {
		t.Fatalf("expected durable progress")
	}
	if len(outcomes[0].ArtifactPaths) != 3 {
		t.Fatalf("expected artifact paths parsed, got %#v", outcomes[0].ArtifactPaths)
	}
}

func TestBuildToolOutcomes_ParsesSearchConvergenceMetadata(t *testing.T) {
	assistant := message.Message{
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tc1", Name: "WebFetch", Input: `{"url":"https://papers.nips.cc/paper_files/paper/2024"}`},
		},
	}
	toolMsg := message.Message{
		Parts: []message.ContentPart{
			message.ToolResult{
				ToolCallID: "tc1",
				Name:       "WebFetch",
				Content:    "ok",
				Metadata:   `{"target_key":"webfetch:https://papers.nips.cc/paper_files/paper/2024","canonical_url":"https://papers.nips.cc/paper_files/paper/2024","content_class":"list_page","prompt_class":"continue_same_target","evidence_keys":["url:https://papers.nips.cc/paper_files/paper/2024"],"outcome_hash":"sha256:abc","low_value_reason":"repeated_target","durable_progress":false}`,
			},
		},
	}
	outcomes := buildToolOutcomes(assistant, toolMsg)
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].TargetKey == "" || outcomes[0].ContentClass != "list_page" || len(outcomes[0].EvidenceKeys) != 1 {
		t.Fatalf("metadata not parsed: %#v", outcomes[0])
	}
}

func TestClassifyRoundProgress_AllLowValueInspection(t *testing.T) {
	outcomes := []ToolOutcome{
		{ToolName: "View", ResultBytes: 256, ContentEmpty: false},
		{ToolName: "Bash", QueryKey: "d2 --help", ResultBytes: 20},
	}
	rp := classifyRoundProgress(&LoopState{SeenEvidence: map[string]int{}, TargetCounts: map[string]int{}, OutcomeCounts: map[string]int{}, FailureCounts: map[string]int{}}, outcomes)
	if rp.LowValueInspection {
		t.Fatalf("substantive View output should not make the whole round low-value")
	}
}

func TestClassifyRoundProgress_BashHelpIsLowValueInspection(t *testing.T) {
	outcomes := []ToolOutcome{
		{ToolName: "Bash", QueryKey: "d2 --help", ResultBytes: 20},
		{ToolName: "Bash", QueryKey: "which d2", ResultBytes: 12},
	}
	rp := classifyRoundProgress(&LoopState{SeenEvidence: map[string]int{}, TargetCounts: map[string]int{}, OutcomeCounts: map[string]int{}, FailureCounts: map[string]int{}}, outcomes)
	if !rp.LowValueInspection {
		t.Fatalf("expected help/which commands to be low-value inspection")
	}
}

func TestClassifyRoundProgress_SearchPageIsLowValue(t *testing.T) {
	outcomes := []ToolOutcome{
		{ToolName: "WebSearch", ProgressKind: "search_page", ResultBytes: 512},
	}
	rp := classifyRoundProgress(&LoopState{SeenEvidence: map[string]int{}, TargetCounts: map[string]int{}, OutcomeCounts: map[string]int{}, FailureCounts: map[string]int{}}, outcomes)
	if rp.DurableProgress {
		t.Fatalf("search_page should not be durable progress")
	}
	if !rp.LowValueInspection {
		t.Fatalf("search_page should count as low-value for no-progress handling")
	}
}

func TestClassifyRoundProgress_NewEvidenceMakesSearchRoundDurable(t *testing.T) {
	state := NewLoopState()
	outcomes := []ToolOutcome{
		{ToolName: "ScholarSearch", ProgressKind: "search_page", EvidenceKeys: []string{"doi:10.1/a", "doi:10.1/b"}, ResultBytes: 512},
	}
	rp := classifyRoundProgress(&state, outcomes)
	if !rp.DurableProgress || rp.NewEvidenceCount != 2 {
		t.Fatalf("expected new evidence to make search durable, got %#v", rp)
	}
	if state.SearchToolCalls != 1 {
		t.Fatalf("expected search call counted, got %d", state.SearchToolCalls)
	}
}

func TestClassifyRoundProgress_CountsTargetOutcomeComposite(t *testing.T) {
	state := NewLoopState()
	outcomes := []ToolOutcome{
		{
			ToolName:    "WebFetch",
			TargetKey:   "webfetch:https://example.com/list",
			OutcomeHash: "sha256:abc",
			ResultBytes: 128,
		},
	}
	classifyRoundProgress(&state, outcomes)
	if state.OutcomeCounts["sha256:abc"] != 1 {
		t.Fatalf("expected raw outcome count, got %#v", state.OutcomeCounts)
	}
	if state.OutcomeCounts["webfetch:https://example.com/list|sha256:abc"] != 1 {
		t.Fatalf("expected target+outcome count, got %#v", state.OutcomeCounts)
	}
}
