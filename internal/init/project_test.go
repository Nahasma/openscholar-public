package initwizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureProjectPrompt_CreateAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	created, path, preview, err := EnsureProjectPrompt(dir)
	if err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true")
	}
	if !strings.Contains(path, ".openscholar/prompt.md") {
		t.Fatalf("unexpected path: %s", path)
	}
	if preview == "" {
		t.Fatalf("expected non-empty preview")
	}

	_ = os.WriteFile(path, []byte("custom\ncontent\n"), 0o644)
	created2, _, preview2, err := EnsureProjectPrompt(dir)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if created2 {
		t.Fatalf("expected created=false")
	}
	if !strings.Contains(preview2, "custom") {
		t.Fatalf("expected existing content preview, got: %s", preview2)
	}
}

func TestProjectPromptPath(t *testing.T) {
	dir := t.TempDir()
	got := ProjectPromptPath(dir)
	want := filepath.Join(dir, ".openscholar", "prompt.md")
	if got != want {
		t.Fatalf("path=%s want=%s", got, want)
	}
}
