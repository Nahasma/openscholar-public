package components

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

// ─── Permission Dialog ───────────────────────────────────────────────────────

func fixturePermissionBash() permission.PermissionRequest {
	return permission.PermissionRequest{
		ID:          "perm-1",
		SessionID:   "sess-1",
		ToolName:    "Bash",
		Description: "Run shell command",
		Action:      "execute",
		Params:      map[string]any{"command": "rm -rf /tmp/test"},
		Path:        "/Users/test/project",
	}
}

func fixturePermissionEdit() permission.PermissionRequest {
	return permission.PermissionRequest{
		ID:          "perm-2",
		SessionID:   "sess-1",
		ToolName:    "Edit",
		Description: "Edit file",
		Action:      "write",
		Params: map[string]any{
			"file_path":  "main.go",
			"old_string": "func old() {}",
			"new_string": "func new() {\n\treturn nil\n}",
		},
		Path: "/Users/test/project/main.go",
	}
}

func TestRenderPermissionDialog_Golden(t *testing.T) {
	cases := []struct {
		name        string
		perm        permission.PermissionRequest
		selectedIdx int
		width       int
	}{
		{"perm_bash_allow_80", fixturePermissionBash(), 0, 80},
		{"perm_bash_deny_80", fixturePermissionBash(), 2, 80},
		{"perm_edit_allow_120", fixturePermissionEdit(), 0, 120},
		{"perm_edit_always_120", fixturePermissionEdit(), 1, 120},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderPermissionDialog(tc.perm, tc.selectedIdx, tc.width)
			checkGolden(t, "dialog_"+tc.name, got)
		})
	}
}

func TestRenderPermissionPreview_Golden(t *testing.T) {
	cases := []struct {
		name  string
		perm  permission.PermissionRequest
		width int
	}{
		{"preview_bash_80", fixturePermissionBash(), 80},
		{"preview_edit_100", fixturePermissionEdit(), 100},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderPermissionPreview(tc.perm, tc.width)
			checkGolden(t, "dialog_"+tc.name, got)
		})
	}
}

func TestRenderPermissionRequestDialog_Golden(t *testing.T) {
	cases := []struct {
		name            string
		perm            permission.PermissionRequest
		selectedIdx     int
		width           int
		maxPreviewLines int
	}{
		{"perm_request_bash_80", fixturePermissionBash(), 0, 80, 8},
		{"perm_request_edit_100", fixturePermissionEdit(), 2, 100, 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderPermissionRequestDialog(tc.perm, tc.selectedIdx, tc.width, tc.maxPreviewLines)
			checkGolden(t, "dialog_"+tc.name, got)
		})
	}
}

// ─── Clarification Dialog ────────────────────────────────────────────────────

func fixtureClarification() tools.ClarificationEvent {
	return tools.ClarificationEvent{
		ID:            "clar-1",
		Question:      "Which reference format do you prefer?",
		Options:       []string{"APA 7th", "IEEE", "ACM", "Chicago"},
		AllowFreeform: true,
		Context:       "You have cited 12 references so far using mixed formats.",
	}
}

func TestRenderClarificationDialog_Golden(t *testing.T) {
	cases := []struct {
		name            string
		event           tools.ClarificationEvent
		selectedIdx     int
		freeformText    string
		freeformFocused bool
		width           int
	}{
		{"clar_options_80", fixtureClarification(), 0, "", false, 80},
		{"clar_selected2_80", fixtureClarification(), 2, "", false, 80},
		{"clar_freeform_100", fixtureClarification(), -1, "MLA 9th edition", true, 100},
		{"clar_wide_120", fixtureClarification(), 1, "", false, 120},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderClarificationDialog(tc.event, tc.selectedIdx, tc.freeformText, tc.freeformFocused, tc.width)
			checkGolden(t, "dialog_"+tc.name, got)
		})
	}
}

// ─── Checkpoint Dialog ───────────────────────────────────────────────────────

func fixtureCheckpoint() tools.CheckpointEvent {
	return tools.CheckpointEvent{
		ID:           "cp-1",
		PipelineID:   "pipe-1",
		PhaseName:    "Literature Review",
		PhaseOrder:   1,
		Summary:      "Surveyed 35 papers on transformer architectures for NLP tasks.",
		Mode:         "default",
		ReviewScore:  8.5,
		ReviewReport: "Good coverage of foundational works. Consider adding recent 2024 papers on efficient attention.",
	}
}

func TestRenderCheckpointDialog_Golden(t *testing.T) {
	cases := []struct {
		name            string
		event           tools.CheckpointEvent
		selectedIdx     int
		feedbackText    string
		feedbackFocused bool
		width           int
	}{
		{"checkpoint_approve_80", fixtureCheckpoint(), 0, "", false, 80},
		{"checkpoint_reject_80", fixtureCheckpoint(), 1, "", false, 80},
		{"checkpoint_feedback_100", fixtureCheckpoint(), 1, "Please add FlashAttention-2 and Ring Attention papers.", true, 100},
		{"checkpoint_wide_120", fixtureCheckpoint(), 0, "", false, 120},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderCheckpointDialog(tc.event, tc.selectedIdx, tc.feedbackText, tc.feedbackFocused, tc.width)
			checkGolden(t, "dialog_"+tc.name, got)
		})
	}
}
