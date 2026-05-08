package workflow

import (
	"testing"
)

// --- Transition: legal paths ---

func TestTransition_DraftToFilling(t *testing.T) {
	if err := Transition(StateDraft, StateFilling); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTransition_DraftToArchived(t *testing.T) {
	if err := Transition(StateDraft, StateArchived); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTransition_FillingToReview(t *testing.T) {
	if err := Transition(StateFilling, StateReview); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTransition_FillingToValidateFailed(t *testing.T) {
	if err := Transition(StateFilling, StateValidateFailed); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTransition_ReviewToApproved(t *testing.T) {
	if err := Transition(StateReview, StateApproved); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTransition_ReviewToRevision(t *testing.T) {
	if err := Transition(StateReview, StateRevision); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTransition_ApprovedToExported(t *testing.T) {
	if err := Transition(StateApproved, StateExported); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTransition_ExportedToArchived(t *testing.T) {
	if err := Transition(StateExported, StateArchived); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// --- Transition: illegal paths ---

func TestTransition_InvalidDraftToApproved(t *testing.T) {
	err := Transition(StateDraft, StateApproved)
	if err == nil {
		t.Fatal("expected error for draft→approved, got nil")
	}
}

func TestTransition_InvalidExportedToFilling(t *testing.T) {
	err := Transition(StateExported, StateFilling)
	if err == nil {
		t.Fatal("expected error for exported→filling, got nil")
	}
}

// --- Transition: unknown state ---

func TestTransition_UnknownState(t *testing.T) {
	err := Transition(DocState("nonexistent"), StateDraft)
	if err == nil {
		t.Fatal("expected error for unknown state, got nil")
	}
}

// --- Transition: exhaustive valid-path coverage ---

func TestTransition_AllValidPaths(t *testing.T) {
	for from, targets := range validTransitions {
		for _, to := range targets {
			if err := Transition(from, to); err != nil {
				t.Errorf("expected valid transition %q → %q, got error: %v", from, to, err)
			}
		}
	}
}

// --- IsTerminal ---

func TestIsTerminal_Archived(t *testing.T) {
	if !IsTerminal(StateArchived) {
		t.Fatal("StateArchived should be terminal")
	}
}

func TestIsTerminal_Draft(t *testing.T) {
	if IsTerminal(StateDraft) {
		t.Fatal("StateDraft should not be terminal")
	}
}

// --- GetTemplate ---

func TestGetTemplate_Patent(t *testing.T) {
	phases := GetTemplate("patent")
	if phases == nil {
		t.Fatal("expected non-nil result for patent")
	}
	if len(phases) != 5 {
		t.Fatalf("expected 5 phases for patent, got %d", len(phases))
	}
}

func TestGetTemplate_Paper(t *testing.T) {
	phases := GetTemplate("paper")
	if phases == nil {
		t.Fatal("expected non-nil result for paper")
	}
	if len(phases) != 4 {
		t.Fatalf("expected 4 phases for paper, got %d", len(phases))
	}
}

func TestGetTemplate_Unknown(t *testing.T) {
	phases := GetTemplate("unknown_doc_type")
	if phases != nil {
		t.Fatalf("expected nil for unknown type, got %v", phases)
	}
}

func TestGetTemplate_IsCopy(t *testing.T) {
	phases := GetTemplate("paper")
	if phases == nil {
		t.Fatal("expected non-nil result for paper")
	}
	originalName := PipelineTemplates["paper"][0].Name
	phases[0].Name = "MUTATED"
	phases[0].Tasks[0] = "MUTATED_TASK"

	if PipelineTemplates["paper"][0].Name != originalName {
		t.Error("modifying returned PhaseSpec.Name should not affect original template")
	}
	if PipelineTemplates["paper"][0].Tasks[0] == "MUTATED_TASK" {
		t.Error("modifying returned Tasks slice should not affect original template")
	}
}

// --- SupportedDocTypes ---

func TestSupportedDocTypes(t *testing.T) {
	types := SupportedDocTypes()
	if len(types) != 3 {
		t.Fatalf("expected 3 supported doc types, got %d: %v", len(types), types)
	}
	want := map[string]bool{"patent": true, "paper": true, "proposal": true}
	for _, dt := range types {
		if !want[dt] {
			t.Errorf("unexpected doc type: %q", dt)
		}
	}
}

// --- AllStates ---

func TestAllStates(t *testing.T) {
	states := AllStates()
	if len(states) != 8 {
		t.Fatalf("expected 8 states, got %d: %v", len(states), states)
	}
	want := map[DocState]bool{
		StateDraft: true, StateFilling: true, StateReview: true,
		StateApproved: true, StateExported: true, StateArchived: true,
		StateValidateFailed: true, StateRevision: true,
	}
	for _, s := range states {
		if !want[s] {
			t.Errorf("unexpected state: %q", s)
		}
	}
}
