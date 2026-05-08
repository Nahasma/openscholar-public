package permission

import (
	"context"
	"testing"
)

func TestModeContext(t *testing.T) {
	ctx := context.Background()

	// Default mode
	if m := CurrentMode(ctx); m != ModeDefault {
		t.Fatalf("expected ModeDefault, got %s", m)
	}

	// Inject Plan mode
	ctx = WithMode(ctx, ModePlan)
	if m := CurrentMode(ctx); m != ModePlan {
		t.Fatalf("expected ModePlan, got %s", m)
	}
}

func TestIsReadOnlyMode(t *testing.T) {
	tests := []struct {
		mode     Mode
		readOnly bool
	}{
		{ModeDefault, false},
		{ModeAuto, false},
		{ModePlan, true},
		{ModeResearch, false},
	}

	for _, tt := range tests {
		if got := IsReadOnlyMode(tt.mode); got != tt.readOnly {
			t.Errorf("IsReadOnlyMode(%s) = %v, want %v", tt.mode, got, tt.readOnly)
		}
	}
}

func TestIsWriteLikeTool(t *testing.T) {
	tests := []struct {
		tool   string
		action string
		write  bool
	}{
		{"Edit", "", true},
		{"Write", "", true},
		{"View", "", false},
		{"Glob", "", false},
		{"Grep", "", false},
		{"NotebookEdit", "", true},
		{"ScholarSearch", "download", true},
		{"ScholarSearch", "search", false},
		{"Bash", "git status", false},
		{"Bash", "git log --oneline", false},
		{"Bash", "ls -la", false},
		{"Bash", "cat README.md", false},
		{"Bash", "rg pattern", false},
		{"Bash", "go test ./...", false},
		{"Bash", "go list ./...", false},
		{"Bash", "rm -rf /", true},
		{"Bash", "echo hello > file.txt", true},
		{"Bash", "make build", true},
		{"Bash", "npm install", true},
		{"Bash", "echo hi | tee x.txt", true},
		{"Bash", "cat a | tee b", true},
		{"Bash", "echo $(touch x)", true},
		{"Bash", "echo `touch x`", true},
		{"Bash", "cat <(ls)", true},
		{"Bash", "find . -delete", true},
		{"Bash", "find . -exec rm {} \\;", true},
		{"Bash", "git diff --output=out.patch", true},
		{"Bash", "", true},
	}

	for _, tt := range tests {
		got := IsWriteLikeTool(tt.tool, tt.action)
		if got != tt.write {
			t.Errorf("IsWriteLikeTool(%q, %q) = %v, want %v", tt.tool, tt.action, got, tt.write)
		}
	}
}

func TestPlanModeBlocksScholarDownloadRequest(t *testing.T) {
	svc := NewPermissionService()
	result := svc.RequestDecision(CreatePermissionRequest{
		SessionID:   "sess-plan-scholar",
		ToolName:    "ScholarSearch",
		Description: "Download verified paper",
		Action:      "download",
		Mode:        ModePlan,
	})
	if result.Allowed {
		t.Fatal("expected plan mode to block ScholarSearch download")
	}
}

func TestNextMode(t *testing.T) {
	tests := []struct {
		current Mode
		next    Mode
	}{
		{ModeDefault, ModeAuto},
		{ModeAuto, ModePlan},
		{ModePlan, ModeResearch},
		{ModeResearch, ModeDefault},
	}

	for _, tt := range tests {
		got := NextMode(tt.current)
		if got != tt.next {
			t.Errorf("NextMode(%s) = %s, want %s", tt.current, got, tt.next)
		}
	}
}

func TestModeString(t *testing.T) {
	if s := ModeDefault.String(); s != "default" {
		t.Errorf("expected 'default', got %q", s)
	}
	if s := ModePlan.String(); s != "plan" {
		t.Errorf("expected 'plan', got %q", s)
	}
}
