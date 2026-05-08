package template

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestPlanInitDryRunAndApply(t *testing.T) {
	old := TemplatesFS
	t.Cleanup(func() { TemplatesFS = old })
	TemplatesFS = fstest.MapFS{
		"demo/template.yaml": &fstest.MapFile{Data: []byte("name: Demo\ndescription: test\n")},
		"demo/a.txt":         &fstest.MapFile{Data: []byte("A")},
		"demo/nested/b.txt":  &fstest.MapFile{Data: []byte("B")},
	}

	target := t.TempDir()
	plan, err := PlanInit("demo", target)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if len(plan.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(plan.Files))
	}
	for _, f := range plan.Files {
		if _, err := os.Stat(f.TargetPath); !os.IsNotExist(err) {
			t.Fatalf("dry-run wrote file unexpectedly: %s", f.TargetPath)
		}
	}
	if err := ApplyInitPlan(plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	for _, rel := range []string{"a.txt", filepath.Join("nested", "b.txt")} {
		p := filepath.Join(target, rel)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing applied file %s: %v", p, err)
		}
	}
}

func TestPlanInitMarksOverwriteRisk(t *testing.T) {
	old := TemplatesFS
	t.Cleanup(func() { TemplatesFS = old })
	TemplatesFS = fstest.MapFS{
		"demo/template.yaml": &fstest.MapFile{Data: []byte("name: Demo\n")},
		"demo/a.txt":         &fstest.MapFile{Data: []byte("A")},
	}
	target := t.TempDir()
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("old"), fs.FileMode(0o644)); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanInit("demo", target)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if len(plan.Files) != 1 || !plan.Files[0].Exists {
		t.Fatalf("expected overwrite risk, got %#v", plan.Files)
	}
}

func TestApplyInitPlanWithOptionsRejectsExistingFile(t *testing.T) {
	old := TemplatesFS
	t.Cleanup(func() { TemplatesFS = old })
	TemplatesFS = fstest.MapFS{
		"demo/template.yaml": &fstest.MapFile{Data: []byte("name: Demo\n")},
		"demo/a.txt":         &fstest.MapFile{Data: []byte("new")},
	}
	target := t.TempDir()
	path := filepath.Join(target, "a.txt")
	if err := os.WriteFile(path, []byte("old"), fs.FileMode(0o644)); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanInit("demo", target)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if err := ApplyInitPlanWithOptions(plan, ApplyOptions{}); err == nil {
		t.Fatalf("expected safe apply to reject existing file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("safe apply overwrote file: %q", string(data))
	}
	if err := ApplyInitPlanWithOptions(plan, ApplyOptions{Overwrite: true}); err != nil {
		t.Fatalf("overwrite apply failed: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("expected overwritten file, got %q", string(data))
	}
}
